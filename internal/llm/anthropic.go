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
	"strings"
	"time"
)

const anthropicVersion = "2023-06-01"

// AnthropicClient implements Client for the Anthropic Messages API.
type AnthropicClient struct {
	BaseURL string
	APIKey  string
	Client  *http.Client
}

// NewAnthropicClient creates a client for the Anthropic Messages API.
func NewAnthropicClient(baseURL, apiKey string) *AnthropicClient {
	return &AnthropicClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		Client:  &http.Client{Timeout: 0},
	}
}

// Stream sends a streaming request to the Anthropic Messages API.
func (c *AnthropicClient) Stream(ctx context.Context, req Request, opts ...StreamOption) (<-chan Event, error) {
	cfg := &streamConfig{}
	for _, o := range opts {
		o(cfg)
	}

	body := c.buildRequest(req)

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	url := c.BaseURL + "/v1/messages"

	slog.Info("Anthropic request", "model", req.Model, "messages", len(req.Messages), "tools", len(req.Tools), "url", url)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("x-api-key", c.APIKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)
	for k, v := range cfg.headers {
		httpReq.Header.Set(k, v)
	}

	start := time.Now()
	resp, err := c.Client.Do(httpReq)
	if err != nil {
		slog.Error("Anthropic request failed", "model", req.Model, "elapsed", time.Since(start), "error", err)
		return nil, fmt.Errorf("sending request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		slog.Error("Anthropic HTTP error", "model", req.Model, "status", resp.StatusCode, "body", string(respBody), "elapsed", time.Since(start))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	slog.Info("Anthropic stream started", "model", req.Model, "elapsed", time.Since(start))
	ch := make(chan Event, 64)
	go c.readSSE(ctx, resp.Body, ch)
	return ch, nil
}

func (c *AnthropicClient) buildRequest(req Request) anthropicRequest {
	ar := anthropicRequest{
		Model:  req.Model,
		Stream: true,
	}

	if req.MaxTokens != nil {
		ar.MaxTokens = *req.MaxTokens
	} else {
		ar.MaxTokens = 4096
	}

	if req.Temperature != nil {
		ar.Temperature = req.Temperature
	}
	if req.TopP != nil {
		ar.TopP = req.TopP
	}

	if req.ThinkingBudget != nil && *req.ThinkingBudget > 0 {
		ar.Thinking = &anthropicThinking{
			Type:         "enabled",
			BudgetTokens: *req.ThinkingBudget,
		}
		// Anthropic requires temperature=1 when thinking is enabled.
		ar.Temperature = nil
	}

	// Extract system message and convert messages.
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			switch v := msg.Content.(type) {
			case string:
				ar.System = v
			}
			continue
		}
		content := msg.Content
		if len(msg.ContentParts) > 0 {
			content = resolveContentPartsAnthropic(msg.ContentParts)
		}
		ar.Messages = append(ar.Messages, anthropicMessage{
			Role:    msg.Role,
			Content: content,
		})
	}

	// Convert tools: use input_schema instead of parameters.
	for _, tool := range req.Tools {
		ar.Tools = append(ar.Tools, anthropicTool{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			InputSchema: tool.Function.Parameters,
		})
	}

	return ar
}

// anthropicBlockState tracks a content block's type and tool call accumulation
// during Anthropic SSE streaming.
type anthropicBlockState struct {
	blockType string
	toolID    string
	toolName  string
	args      string
}

func (c *AnthropicClient) readSSE(ctx context.Context, body io.ReadCloser, ch chan<- Event) {
	defer close(ch)
	defer body.Close()

	lines := make(chan string, 1)
	scanDone := make(chan error, 1)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		scanDone <- scanner.Err()
	}()

	blocks := make(map[int]*anthropicBlockState)
	timer := time.NewTimer(chunkTimeout)
	defer timer.Stop()

	var eventType string

	for {
		var line string
		select {
		case <-ctx.Done():
			slog.Warn("Anthropic SSE context cancelled")
			ch <- Event{Type: EventError, Error: ctx.Err()}
			return
		case <-timer.C:
			slog.Error("Anthropic SSE chunk timeout", "timeout", chunkTimeout)
			ch <- Event{Type: EventError, Error: fmt.Errorf("chunk read timeout after %v", chunkTimeout)}
			return
		case l, ok := <-lines:
			if !ok {
				if err := <-scanDone; err != nil {
					slog.Error("Anthropic SSE read error", "error", err)
					ch <- Event{Type: EventError, Error: fmt.Errorf("reading SSE stream: %w", err)}
				}
				return
			}
			timer.Reset(chunkTimeout)
			line = l
		}

		if strings.HasPrefix(line, "event: ") {
			eventType = line[7:]
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := line[6:]
		if data == "[DONE]" {
			return
		}

		var raw json.RawMessage
		if err := json.Unmarshal([]byte(data), &raw); err != nil {
			continue
		}

		c.handleAnthropicSSEEvent(eventType, raw, blocks, ch)
	}
}

// handleAnthropicSSEEvent processes a single parsed Anthropic SSE event.
func (c *AnthropicClient) handleAnthropicSSEEvent(eventType string, raw json.RawMessage, blocks map[int]*anthropicBlockState, ch chan<- Event) {
	switch eventType {
	case "content_block_start":
		var evt anthropicContentBlockStart
		if err := json.Unmarshal(raw, &evt); err != nil {
			return
		}
		bs := &anthropicBlockState{blockType: evt.ContentBlock.Type}
		if evt.ContentBlock.Type == "tool_use" {
			bs.toolID = evt.ContentBlock.ID
			bs.toolName = evt.ContentBlock.Name
			ch <- Event{
				Type:       EventToolCallBegin,
				ToolCallID: evt.ContentBlock.ID,
				ToolName:   evt.ContentBlock.Name,
			}
		}
		blocks[evt.Index] = bs

	case "content_block_delta":
		var evt anthropicContentBlockDelta
		if err := json.Unmarshal(raw, &evt); err != nil {
			return
		}
		switch evt.Delta.Type {
		case "text_delta":
			ch <- Event{Type: EventTextDelta, Text: evt.Delta.Text}
		case "thinking_delta":
			ch <- Event{Type: EventReasoningDelta, Text: evt.Delta.Thinking}
		case "input_json_delta":
			if bs, ok := blocks[evt.Index]; ok {
				bs.args += evt.Delta.PartialJSON
				ch <- Event{
					Type:         EventToolCallDelta,
					ToolCallID:   bs.toolID,
					ToolCallArgs: evt.Delta.PartialJSON,
				}
			}
		}

	case "content_block_stop":
		var evt anthropicContentBlockStop
		if err := json.Unmarshal(raw, &evt); err != nil {
			return
		}
		bs, ok := blocks[evt.Index]
		if !ok || bs.blockType != "tool_use" {
			break
		}
		args := bs.args
		if !json.Valid([]byte(args)) {
			if repaired := RepairToolCallJSON(args); repaired != nil {
				args = *repaired
			}
		}
		ch <- Event{
			Type:         EventToolCallEnd,
			ToolCallID:   bs.toolID,
			ToolName:     bs.toolName,
			ToolCallArgs: args,
		}
		delete(blocks, evt.Index)

	case "message_delta":
		var evt anthropicMessageDelta
		if err := json.Unmarshal(raw, &evt); err != nil {
			return
		}
		ev := Event{
			Type:         EventFinish,
			FinishReason: evt.Delta.StopReason,
		}
		if evt.Usage != nil {
			ev.Usage = &Usage{
				CompletionTokens: evt.Usage.OutputTokens,
			}
		}
		ch <- ev

	case "message_start":
		var evt anthropicMessageStart
		if err := json.Unmarshal(raw, &evt); err != nil {
			return
		}
		if evt.Message.Usage != nil {
			ch <- Event{
				Type: EventFinish,
				Usage: &Usage{
					PromptTokens: evt.Message.Usage.InputTokens,
				},
			}
		}
	}
}

// --- Anthropic API types ---

type anthropicRequest struct {
	Model       string              `json:"model"`
	Messages    []anthropicMessage  `json:"messages"`
	System      string              `json:"system,omitempty"`
	MaxTokens   int                 `json:"max_tokens"`
	Stream      bool                `json:"stream"`
	Temperature *float64            `json:"temperature,omitempty"`
	TopP        *float64            `json:"top_p,omitempty"`
	Tools       []anthropicTool     `json:"tools,omitempty"`
	Thinking    *anthropicThinking  `json:"thinking,omitempty"`
}

type anthropicThinking struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type anthropicTool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema,omitempty"`
}

type anthropicContentBlockStart struct {
	Index        int `json:"index"`
	ContentBlock struct {
		Type string `json:"type"`
		ID   string `json:"id,omitempty"`
		Name string `json:"name,omitempty"`
	} `json:"content_block"`
}

type anthropicContentBlockDelta struct {
	Index int `json:"index"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text,omitempty"`
		Thinking    string `json:"thinking,omitempty"`
		PartialJSON string `json:"partial_json,omitempty"`
	} `json:"delta"`
}

type anthropicContentBlockStop struct {
	Index int `json:"index"`
}

type anthropicMessageDelta struct {
	Delta struct {
		StopReason string `json:"stop_reason,omitempty"`
	} `json:"delta"`
	Usage *struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage,omitempty"`
}

type anthropicMessageStart struct {
	Message struct {
		Usage *struct {
			InputTokens int `json:"input_tokens"`
		} `json:"usage,omitempty"`
	} `json:"message"`
}

// resolveContentPartsAnthropic converts ContentParts to the Anthropic
// multipart content format.
func resolveContentPartsAnthropic(parts []ContentPart) []map[string]any {
	result := make([]map[string]any, 0, len(parts))
	for _, cp := range parts {
		switch cp.Type {
		case "text":
			result = append(result, map[string]any{
				"type": "text",
				"text": cp.Text,
			})
		case "image":
			result = append(result, map[string]any{
				"type": "image",
				"source": map[string]string{
					"type":       "base64",
					"media_type": cp.MediaType,
					"data":       cp.ImageData,
				},
			})
		default:
			slog.Debug("unsupported content part type, passing as text", "type", cp.Type)
			result = append(result, map[string]any{
				"type": "text",
				"text": fmt.Sprintf("[%s: %s]", cp.Type, cp.Text),
			})
		}
	}
	return result
}
