package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const streamableCallTimeout = 60 * time.Second

type StreamableHTTPTransport struct {
	url     string
	headers map[string]string

	mu        sync.Mutex
	client    *http.Client
	sessionID string
	nextID    atomic.Int64
}

func NewStreamableHTTPTransport(url string, headers map[string]string) *StreamableHTTPTransport {
	return &StreamableHTTPTransport{
		url:     url,
		headers: headers,
		client:  &http.Client{Timeout: streamableCallTimeout},
	}
}

func (t *StreamableHTTPTransport) Connect(ctx context.Context) error {
	initReq := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      t.nextID.Add(1),
		Method:  "initialize",
		Params: map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "tinycode",
				"version": "0.1.0",
			},
		},
	}

	resp, err := t.sendRequest(ctx, initReq)
	if err != nil {
		return fmt.Errorf("streamable-http initialize: %w", err)
	}
	if resp.Error != nil {
		return fmt.Errorf("streamable-http initialize: %s", resp.Error.Message)
	}

	notif := jsonrpcRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	_, _ = t.sendRequest(ctx, notif)

	return nil
}

func (t *StreamableHTTPTransport) sendRequest(ctx context.Context, req jsonrpcRequest) (*jsonrpcResponse, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", t.url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range t.headers {
		httpReq.Header.Set(k, v)
	}

	t.mu.Lock()
	if t.sessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", t.sessionID)
	}
	t.mu.Unlock()

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		t.mu.Lock()
		t.sessionID = sid
		t.mu.Unlock()
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, &UnauthorizedError{StatusCode: resp.StatusCode}
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("request failed: status %d", resp.StatusCode)
	}

	if resp.StatusCode == http.StatusAccepted {
		return &jsonrpcResponse{JSONRPC: "2.0"}, nil
	}

	var jsonResp jsonrpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&jsonResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &jsonResp, nil
}

func (t *StreamableHTTPTransport) ListTools(ctx context.Context) ([]MCPTool, error) {
	var allTools []MCPTool
	var cursor string

	for {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}

		req := jsonrpcRequest{
			JSONRPC: "2.0",
			ID:      t.nextID.Add(1),
			Method:  "tools/list",
			Params:  params,
		}

		resp, err := t.sendRequest(ctx, req)
		if err != nil {
			return nil, err
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("listing tools: %s", resp.Error.Message)
		}

		var result struct {
			Tools      []MCPTool `json:"tools"`
			NextCursor string    `json:"nextCursor,omitempty"`
		}
		if err := json.Unmarshal(resp.Result, &result); err != nil {
			return nil, fmt.Errorf("parsing tools: %w", err)
		}

		allTools = append(allTools, result.Tools...)

		if result.NextCursor == "" {
			break
		}
		cursor = result.NextCursor
	}

	return allTools, nil
}

func (t *StreamableHTTPTransport) CallTool(ctx context.Context, name string, args json.RawMessage) (string, error) {
	var arguments map[string]any
	if len(args) > 0 {
		if err := json.Unmarshal(args, &arguments); err != nil {
			return "", fmt.Errorf("parsing tool arguments: %w", err)
		}
	}

	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      t.nextID.Add(1),
		Method:  "tools/call",
		Params: map[string]any{
			"name":      name,
			"arguments": arguments,
		},
	}

	resp, err := t.sendRequest(ctx, req)
	if err != nil {
		return "", err
	}
	if resp.Error != nil {
		return "", fmt.Errorf("tool %s error: %s", name, resp.Error.Message)
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError,omitempty"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("parsing tool result: %w", err)
	}

	var output string
	for _, c := range result.Content {
		if c.Type == "text" {
			if output != "" {
				output += "\n"
			}
			output += c.Text
		}
	}

	if result.IsError {
		return "", fmt.Errorf("tool error: %s", output)
	}

	return output, nil
}

func (t *StreamableHTTPTransport) Close() error {
	return nil
}

type UnauthorizedError struct {
	StatusCode int
}

func (e *UnauthorizedError) Error() string {
	return fmt.Sprintf("unauthorized: HTTP %d", e.StatusCode)
}
