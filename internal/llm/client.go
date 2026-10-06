package llm

import (
	"context"
	"strings"
)

type Request struct {
	Model         string         `json:"model"`
	Messages      []Message      `json:"messages"`
	Tools         []Tool         `json:"tools,omitempty"`
	ToolChoice    string         `json:"tool_choice,omitempty"`
	Stream        bool           `json:"stream"`
	StreamOptions *StreamOptions `json:"stream_options,omitempty"`
	Temperature   *float64       `json:"temperature,omitempty"`
	TopP          *float64       `json:"top_p,omitempty"`
	MaxTokens      *int           `json:"max_tokens,omitempty"`
	ThinkingBudget *int           `json:"thinking_budget,omitempty"`
}

// UsesAnthropicProtocol reports whether requests should use the Anthropic
// Messages API. Prefers an explicit npm/protocol hint (e.g. "@ai-sdk/anthropic")
// or provider ID, then falls back to the api.anthropic.com hostname.
func UsesAnthropicProtocol(npm, providerID, apiURL string) bool {
	if strings.Contains(strings.ToLower(npm), "anthropic") {
		return true
	}
	if strings.EqualFold(providerID, "anthropic") {
		return true
	}
	return strings.Contains(apiURL, "api.anthropic.com")
}

// NewClient selects OpenAI or Anthropic based on protocol hints.
func NewClient(baseURL, apiKey, npm, providerID string) Client {
	if UsesAnthropicProtocol(npm, providerID, baseURL) {
		return NewAnthropicClient(baseURL, apiKey)
	}
	return NewOpenAIClient(baseURL+"/v1", apiKey)
}

type StreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
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
