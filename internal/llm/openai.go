package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/safego"
)

const (
	headerTimeout = 5 * time.Minute
	chunkTimeout  = 5 * time.Minute
)

type OpenAIClient struct {
	BaseURL string
	APIKey  string
	Client  *http.Client
}

func NewOpenAIClient(baseURL, apiKey string) *OpenAIClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = headerTimeout
	return &OpenAIClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		Client: &http.Client{
			Timeout:   0, // no overall timeout; we manage per-phase timeouts
			Transport: transport,
		},
	}
}

func (c *OpenAIClient) Stream(ctx context.Context, req Request, opts ...StreamOption) (<-chan Event, error) {
	cfg := &streamConfig{}
	for _, o := range opts {
		o(cfg)
	}

	// Marshal a copy so thinking_budget (Anthropic-only) never hits the OpenAI wire.
	wire := req
	wire.Stream = true
	wire.StreamOptions = &StreamOptions{IncludeUsage: true}
	wire.ThinkingBudget = nil
	resolveContentPartsOpenAI(wire.Messages)
	body, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	url := c.BaseURL + "/chat/completions"

	slog.Info("LLM request", "model", req.Model, "messages", len(req.Messages), "tools", len(req.Tools), "url", url, "bodyLen", len(body))

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if c.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	for k, v := range cfg.headers {
		httpReq.Header.Set(k, v)
	}

	start := time.Now()
	resp, err := c.Client.Do(httpReq)
	if err != nil {
		slog.Error("LLM request failed", "model", req.Model, "elapsed", time.Since(start), "error", err)
		return nil, fmt.Errorf("sending request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		logBody := string(respBody)
		if len(logBody) > 500 {
			logBody = logBody[:500] + "...(truncated)"
		}
		slog.Error("LLM HTTP error", "model", req.Model, "status", resp.StatusCode, "body", logBody, "elapsed", time.Since(start))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	slog.Info("LLM stream started", "model", req.Model, "elapsed", time.Since(start))
	ch := make(chan Event, 64)
	safego.Go(func() { c.readSSE(ctx, resp.Body, ch) })
	return ch, nil
}

func (c *OpenAIClient) readSSE(ctx context.Context, body io.ReadCloser, ch chan<- Event) {
	defer close(ch)
	defer body.Close()

	lines := make(chan string, 1)
	scanDone := make(chan error, 1)
	safego.Go(func() {
		defer close(lines)
		scanner := bufio.NewScanner(body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		scanDone <- scanner.Err()
	})

	toolCalls := make(map[int]*toolCallAccum)
	timer := time.NewTimer(chunkTimeout)
	defer timer.Stop()

	for {
		var line string
		select {
		case <-ctx.Done():
			slog.Warn("SSE stream context cancelled")
			ch <- Event{Type: EventError, Error: ctx.Err()}
			return
		case <-timer.C:
			slog.Error("SSE chunk timeout", "timeout", chunkTimeout)
			ch <- Event{Type: EventError, Error: fmt.Errorf("chunk read timeout after %v with no data from model", chunkTimeout)}
			return
		case l, ok := <-lines:
			if !ok {
				if err := <-scanDone; err != nil {
					slog.Error("SSE stream read error", "error", err)
					ch <- Event{Type: EventError, Error: fmt.Errorf("reading SSE stream: %w", err)}
				} else {
					slog.Info("SSE stream ended normally")
				}
				// Finalize tools when stream ends without finish_reason (e.g. [DONE] only).
				if len(toolCalls) > 0 {
					c.finalizeOpenAIToolCalls(toolCalls, ch)
				}
				return
			}
			timer.Reset(chunkTimeout)
			line = l
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := line[6:]
		if data == "[DONE]" {
			if len(toolCalls) > 0 {
				c.finalizeOpenAIToolCalls(toolCalls, ch)
			}
			return
		}

		var chunk chatCompletionChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			snippet := data
			if len(snippet) > 200 {
				snippet = snippet[:200] + "..."
			}
			slog.Warn("SSE JSON parse error", "error", err, "snippet", snippet)
			continue
		}

		c.processOpenAIChunk(chunk, toolCalls, ch)
	}
}

// processOpenAIChunk handles a single parsed SSE chunk: emits text/reasoning
// deltas, accumulates tool call arguments, and finalizes tool calls on finish.
func (c *OpenAIClient) processOpenAIChunk(chunk chatCompletionChunk, toolCalls map[int]*toolCallAccum, ch chan<- Event) {
	if len(chunk.Choices) == 0 {
		if chunk.Usage != nil {
			ch <- Event{
				Type: EventFinish,
				Usage: &Usage{
					PromptTokens:     chunk.Usage.PromptTokens,
					CompletionTokens: chunk.Usage.CompletionTokens,
					TotalTokens:      chunk.Usage.TotalTokens,
				},
			}
		}
		return
	}

	choice := chunk.Choices[0]
	delta := choice.Delta

	if delta.Content != "" {
		ch <- Event{Type: EventTextDelta, Text: delta.Content}
	}
	if delta.ReasoningContent != "" {
		ch <- Event{Type: EventReasoningDelta, Text: delta.ReasoningContent}
	}

	for _, tc := range delta.ToolCalls {
		accum, exists := toolCalls[tc.Index]
		if !exists {
			accum = &toolCallAccum{id: tc.ID, name: tc.Function.Name}
			toolCalls[tc.Index] = accum
			ch <- Event{
				Type:       EventToolCallBegin,
				ToolCallID: tc.ID,
				ToolName:   tc.Function.Name,
			}
		} else {
			if tc.ID != "" {
				accum.id = tc.ID
			}
			if tc.Function.Name != "" {
				accum.name = tc.Function.Name
			}
		}
		if tc.Function.Arguments != "" {
			accum.args += tc.Function.Arguments
			ch <- Event{
				Type:         EventToolCallDelta,
				ToolCallID:   accum.id,
				ToolCallArgs: tc.Function.Arguments,
			}
		}
	}

	if choice.FinishReason != "" {
		c.finalizeOpenAIToolCalls(toolCalls, ch)
		ch <- Event{
			Type:         EventFinish,
			FinishReason: choice.FinishReason,
		}
	}
}

// finalizeOpenAIToolCalls emits EventToolCallEnd for each accumulated tool call,
// repairing or redirecting invalid JSON arguments. Indices are finalized in
// ascending order for stable multi-tool output.
func (c *OpenAIClient) finalizeOpenAIToolCalls(toolCalls map[int]*toolCallAccum, ch chan<- Event) {
	if len(toolCalls) == 0 {
		return
	}
	idxs := make([]int, 0, len(toolCalls))
	for idx := range toolCalls {
		idxs = append(idxs, idx)
	}
	sort.Ints(idxs)

	for _, idx := range idxs {
		accum := toolCalls[idx]
		args := accum.args
		if !json.Valid([]byte(args)) {
			repaired := RepairToolCallJSON(args)
			if repaired == nil {
				invalidArgs, _ := json.Marshal(map[string]string{
					"error":         "invalid JSON in tool call arguments",
					"original_name": accum.name,
					"original_args": args,
				})
				ch <- Event{
					Type:         EventToolCallEnd,
					ToolCallID:   accum.id,
					ToolName:     "invalid",
					ToolCallArgs: string(invalidArgs),
				}
				delete(toolCalls, idx)
				continue
			}
			args = *repaired
		}
		ch <- Event{
			Type:         EventToolCallEnd,
			ToolCallID:   accum.id,
			ToolName:     accum.name,
			ToolCallArgs: args,
		}
		delete(toolCalls, idx)
	}
}

type toolCallAccum struct {
	id   string
	name string
	args string
}

type chatCompletionChunk struct {
	ID      string                 `json:"id"`
	Choices []chatCompletionChoice `json:"choices"`
	Usage   *chatCompletionUsage   `json:"usage,omitempty"`
}

type chatCompletionChoice struct {
	Index        int                 `json:"index"`
	Delta        chatCompletionDelta `json:"delta"`
	FinishReason string              `json:"finish_reason,omitempty"`
}

type chatCompletionDelta struct {
	Role             string                   `json:"role,omitempty"`
	Content          string                   `json:"content,omitempty"`
	ReasoningContent string                   `json:"reasoning_content,omitempty"`
	ToolCalls        []chatCompletionToolCall `json:"tool_calls,omitempty"`
}

type chatCompletionToolCall struct {
	Index    int                    `json:"index"`
	ID       string                 `json:"id,omitempty"`
	Type     string                 `json:"type,omitempty"`
	Function chatCompletionFunction `json:"function"`
}

type chatCompletionFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type chatCompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// resolveContentPartsOpenAI converts ContentParts to the OpenAI multipart
// content format and assigns the result to Message.Content.
func resolveContentPartsOpenAI(messages []Message) {
	for i := range messages {
		if len(messages[i].ContentParts) == 0 {
			continue
		}
		parts := make([]map[string]any, 0, len(messages[i].ContentParts))
		for _, cp := range messages[i].ContentParts {
			switch cp.Type {
			case "text":
				parts = append(parts, map[string]any{
					"type": "text",
					"text": cp.Text,
				})
			case "image":
				parts = append(parts, map[string]any{
					"type": "image_url",
					"image_url": map[string]string{
						"url": "data:" + cp.MediaType + ";base64," + cp.ImageData,
					},
				})
			}
		}
		messages[i].Content = parts
	}
}
