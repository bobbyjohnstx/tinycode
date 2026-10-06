package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/safego"
)

const (
	sseConnectTimeout = 30 * time.Second
	sseCallTimeout    = 60 * time.Second
)

type SSETransport struct {
	url     string
	headers map[string]string

	mu           sync.Mutex
	client       *http.Client
	messagesURL  string
	nextID       atomic.Int64
	pending      map[int64]chan *jsonrpcResponse
	cancel       context.CancelFunc
	connCtx      context.Context
	connected    chan struct{}

	onDisconnect   func()
	onNotification func(method string)
}

func NewSSETransport(url string, headers map[string]string) *SSETransport {
	return &SSETransport{
		url:     url,
		headers: headers,
		client:  &http.Client{Timeout: 0},
		pending: make(map[int64]chan *jsonrpcResponse),
	}
}

func (t *SSETransport) Connect(ctx context.Context) error {
	t.connected = make(chan struct{})
	connCtx, cancel := context.WithCancel(ctx)
	t.cancel = cancel
	t.connCtx = connCtx

	req, err := http.NewRequestWithContext(connCtx, "GET", t.url, nil)
	if err != nil {
		cancel()
		return fmt.Errorf("creating SSE request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		cancel()
		return fmt.Errorf("SSE connect: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		cancel()
		resp.Body.Close()
		return fmt.Errorf("SSE connect: status %d", resp.StatusCode)
	}

	safego.Go(func() { t.readSSEStream(resp.Body) })

	select {
	case <-t.connected:
	case <-time.After(sseConnectTimeout):
		cancel()
		resp.Body.Close()
		return fmt.Errorf("SSE connect: timeout waiting for endpoint event")
	case <-ctx.Done():
		cancel()
		resp.Body.Close()
		return ctx.Err()
	}

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

	initResp, err := t.sendRequest(connCtx, initReq)
	if err != nil {
		cancel()
		return fmt.Errorf("SSE initialize: %w", err)
	}
	if initResp.Error != nil {
		cancel()
		return fmt.Errorf("SSE initialize error: %s", initResp.Error.Message)
	}

	notif := jsonrpcRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	if err := t.postJSON(connCtx, notif); err != nil {
		cancel()
		return fmt.Errorf("sending initialized notification: %w", err)
	}

	return nil
}

// postJSON POSTs a JSON-RPC message without waiting for an SSE response.
// Used for notifications that have no response ID.
func (t *SSETransport) postJSON(ctx context.Context, msg any) error {
	t.mu.Lock()
	messagesURL := t.messagesURL
	t.mu.Unlock()

	if messagesURL == "" {
		return fmt.Errorf("SSE not connected: no messages endpoint")
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", messagesURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range t.headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("request failed: status %d", resp.StatusCode)
	}
	return nil
}

func (t *SSETransport) readSSEStream(body io.ReadCloser) {
	defer body.Close()

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	var eventType string
	var dataBuf bytes.Buffer

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			if dataBuf.Len() > 0 {
				t.handleSSEEvent(eventType, dataBuf.String())
				eventType = ""
				dataBuf.Reset()
			}
			continue
		}

		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			if dataBuf.Len() > 0 {
				dataBuf.WriteByte('\n')
			}
			dataBuf.WriteString(strings.TrimPrefix(line, "data: "))
		}
	}

	// Stream closed — clean up pending requests so callers don't hang
	t.mu.Lock()
	for id, ch := range t.pending {
		ch <- &jsonrpcResponse{
			Error: &jsonrpcError{Code: -1, Message: "SSE connection closed"},
		}
		delete(t.pending, id)
	}
	t.mu.Unlock()

	// Fire disconnect if not intentional
	if t.connCtx != nil && t.connCtx.Err() == nil && t.onDisconnect != nil {
		t.onDisconnect()
	}
}

func (t *SSETransport) handleSSEEvent(eventType, data string) {
	switch eventType {
	case "endpoint":
		messagesURL := strings.TrimSpace(data)
		if !strings.HasPrefix(messagesURL, "http") {
			base := t.url
			if idx := strings.LastIndex(base, "/"); idx > 0 {
				base = base[:idx]
			}
			messagesURL = base + "/" + strings.TrimPrefix(messagesURL, "/")
		}
		t.mu.Lock()
		t.messagesURL = messagesURL
		t.mu.Unlock()

		select {
		case <-t.connected:
		default:
			close(t.connected)
		}

	case "message":
		var resp jsonrpcResponse
		if err := json.Unmarshal([]byte(data), &resp); err != nil {
			return
		}

		// Handle server-initiated notifications (no ID, has method)
		if resp.ID == 0 && resp.Method != "" {
			if t.onNotification != nil {
				safego.Go(func() { t.onNotification(resp.Method) })
			}
			return
		}

		t.mu.Lock()
		ch, ok := t.pending[resp.ID]
		if ok {
			delete(t.pending, resp.ID)
		}
		t.mu.Unlock()
		if ok {
			ch <- &resp
		}
	}
}

func (t *SSETransport) sendRequest(ctx context.Context, req jsonrpcRequest) (*jsonrpcResponse, error) {
	t.mu.Lock()
	messagesURL := t.messagesURL
	t.mu.Unlock()

	if messagesURL == "" {
		return nil, fmt.Errorf("SSE not connected: no messages endpoint")
	}

	ch := make(chan *jsonrpcResponse, 1)
	t.mu.Lock()
	t.pending[req.ID] = ch
	t.mu.Unlock()

	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", messagesURL, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range t.headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := t.client.Do(httpReq)
	if err != nil {
		t.mu.Lock()
		delete(t.pending, req.ID)
		t.mu.Unlock()
		return nil, fmt.Errorf("sending request: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		t.mu.Lock()
		delete(t.pending, req.ID)
		t.mu.Unlock()
		return nil, fmt.Errorf("request failed: status %d", resp.StatusCode)
	}

	select {
	case result := <-ch:
		return result, nil
	case <-time.After(sseCallTimeout):
		t.mu.Lock()
		delete(t.pending, req.ID)
		t.mu.Unlock()
		return nil, fmt.Errorf("timeout waiting for response")
	case <-ctx.Done():
		t.mu.Lock()
		delete(t.pending, req.ID)
		t.mu.Unlock()
		return nil, ctx.Err()
	}
}

func (t *SSETransport) ListTools(ctx context.Context) ([]MCPTool, error) {
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

func (t *SSETransport) CallTool(ctx context.Context, name string, args json.RawMessage) (string, error) {
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

func (t *SSETransport) Close() error {
	if t.cancel != nil {
		t.cancel()
	}
	return nil
}
