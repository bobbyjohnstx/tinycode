package llm

import (
	"context"
)

type Request struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Tools       []Tool    `json:"tools,omitempty"`
	ToolChoice  string    `json:"tool_choice,omitempty"`
	Stream      bool      `json:"stream"`
	Temperature *float64  `json:"temperature,omitempty"`
	TopP        *float64  `json:"top_p,omitempty"`
	MaxTokens   *int      `json:"max_tokens,omitempty"`
}

// StreamOption configures a streaming request.
type StreamOption func(*streamConfig)

type streamConfig struct {
	headers map[string]string
}

// WithHeaders adds custom headers to the request.
func WithHeaders(h map[string]string) StreamOption {
	return func(c *streamConfig) {
		c.headers = h
	}
}

// Client streams LLM responses from an OpenAI-compatible endpoint.
type Client interface {
	Stream(ctx context.Context, req Request, opts ...StreamOption) (<-chan Event, error)
}
