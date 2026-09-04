package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
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
	return &OpenAIClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		Client: &http.Client{
			Timeout: 0, // no overall timeout; we manage per-phase timeouts
		},
	}
}

func (c *OpenAIClient) Stream(ctx context.Context, req Request, opts ...StreamOption) (<-chan Event, error) {
	cfg := &streamConfig{}
	for _, o := range opts {
		o(cfg)
	}

	req.Stream = true
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	url := c.BaseURL + "/chat/completions"
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

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan Event, 64)
	go c.readSSE(ctx, resp.Body, ch)
	return ch, nil
}

func (c *OpenAIClient) readSSE(ctx context.Context, body io.ReadCloser, ch chan<- Event) {
	defer close(ch)
	defer body.Close()

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	toolCalls := make(map[int]*toolCallAccum)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			ch <- Event{Type: EventError, Error: ctx.Err()}
			return
		default:
		}

		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := line[6:]
		if data == "[DONE]" {
			return
		}

		var chunk chatCompletionChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

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
			continue
		}

		choice := chunk.Choices[0]
		delta := choice.Delta

		if delta.Content != "" {
			ch <- Event{Type: EventTextDelta, Text: delta.Content}
		}

		if delta.ReasoningContent != "" {
			ch <- Event{Type: EventTextDelta, Text: delta.ReasoningContent}
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
			for idx, accum := range toolCalls {
				args := accum.args
				if !json.Valid([]byte(args)) {
					if repaired := RepairToolCallJSON(args); repaired != nil {
						args = *repaired
					}
				}
				ch <- Event{
					Type:         EventToolCallEnd,
					ToolCallID:   accum.id,
					ToolName:     accum.name,
					ToolCallArgs: args,
				}
				delete(toolCalls, idx)
			}

			ch <- Event{
				Type:         EventFinish,
				FinishReason: choice.FinishReason,
			}
		}
	}

	if err := scanner.Err(); err != nil {
		ch <- Event{Type: EventError, Error: fmt.Errorf("reading SSE stream: %w", err)}
	}
}

type toolCallAccum struct {
	id   string
	name string
	args string
}

type chatCompletionChunk struct {
	ID      string                   `json:"id"`
	Choices []chatCompletionChoice   `json:"choices"`
	Usage   *chatCompletionUsage     `json:"usage,omitempty"`
}

type chatCompletionChoice struct {
	Index        int                    `json:"index"`
	Delta        chatCompletionDelta    `json:"delta"`
	FinishReason string                 `json:"finish_reason,omitempty"`
}

type chatCompletionDelta struct {
	Role             string                    `json:"role,omitempty"`
	Content          string                    `json:"content,omitempty"`
	ReasoningContent string                    `json:"reasoning_content,omitempty"`
	ToolCalls        []chatCompletionToolCall  `json:"tool_calls,omitempty"`
}

type chatCompletionToolCall struct {
	Index    int                     `json:"index"`
	ID       string                  `json:"id,omitempty"`
	Type     string                  `json:"type,omitempty"`
	Function chatCompletionFunction  `json:"function"`
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
