package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/config"
)

func TestResolveTransport(t *testing.T) {
	tests := []struct {
		name   string
		cfg    config.MCPConfig
		expect string
	}{
		{"explicit stdio", config.MCPConfig{Transport: "stdio", Command: "echo"}, "stdio"},
		{"explicit sse", config.MCPConfig{Transport: "sse", URL: "http://localhost"}, "sse"},
		{"explicit streamable", config.MCPConfig{Transport: "streamable-http", URL: "http://localhost"}, "streamable-http"},
		{"infer stdio from command", config.MCPConfig{Command: "node"}, "stdio"},
		{"infer sse from url", config.MCPConfig{URL: "http://localhost:8080/sse"}, "sse"},
		{"default stdio", config.MCPConfig{}, "stdio"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveTransport(tt.cfg)
			if got != tt.expect {
				t.Errorf("expected %q, got %q", tt.expect, got)
			}
		})
	}
}

func TestCreateTransport_StdioMissingCommand(t *testing.T) {
	s := NewService(bus.New())
	defer s.bus.Close()

	_, err := s.createTransport(config.MCPConfig{Transport: "stdio"})
	if err == nil {
		t.Fatal("expected error for stdio without command")
	}
}

func TestCreateTransport_SSEMissingURL(t *testing.T) {
	s := NewService(bus.New())
	defer s.bus.Close()

	_, err := s.createTransport(config.MCPConfig{Transport: "sse"})
	if err == nil {
		t.Fatal("expected error for SSE without URL")
	}
}

func TestCreateTransport_StreamableMissingURL(t *testing.T) {
	s := NewService(bus.New())
	defer s.bus.Close()

	_, err := s.createTransport(config.MCPConfig{Transport: "streamable-http"})
	if err == nil {
		t.Fatal("expected error for streamable-http without URL")
	}
}

func TestCreateTransport_UnknownTransport(t *testing.T) {
	s := NewService(bus.New())
	defer s.bus.Close()

	_, err := s.createTransport(config.MCPConfig{Transport: "grpc"})
	if err == nil {
		t.Fatal("expected error for unknown transport")
	}
}

func TestServiceStatus_Empty(t *testing.T) {
	b := bus.New()
	defer b.Close()
	s := NewService(b)

	status := s.Status(context.Background())
	if len(status) != 0 {
		t.Errorf("expected empty status, got %d entries", len(status))
	}
}

func TestServiceTools_Empty(t *testing.T) {
	b := bus.New()
	defer b.Close()
	s := NewService(b)

	tools := s.Tools(context.Background())
	if len(tools) != 0 {
		t.Errorf("expected no tools, got %d", len(tools))
	}
}

func TestServiceRestart_UnknownServer(t *testing.T) {
	b := bus.New()
	defer b.Close()
	s := NewService(b)

	err := s.Restart(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown server")
	}
}

func TestConvertMCPTool(t *testing.T) {
	transport := &mockTransport{
		callResult: "hello from tool",
	}

	mcpTool := MCPTool{
		Name:        "search",
		Description: "Search the web",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
	}

	def := convertMCPTool("websearch", mcpTool, transport)

	if def.ID != "mcp__websearch__search" {
		t.Errorf("expected mcp__websearch__search, got %s", def.ID)
	}
	if def.Description != "Search the web" {
		t.Errorf("wrong description: %s", def.Description)
	}
	if def.Permission != "mcp" {
		t.Errorf("expected permission 'mcp', got %s", def.Permission)
	}

	result, err := def.Execute(context.Background(), nil, json.RawMessage(`{"query":"test"}`))
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if result.Output != "hello from tool" {
		t.Errorf("expected 'hello from tool', got %q", result.Output)
	}
}

func TestConvertMCPTool_NilSchema(t *testing.T) {
	transport := &mockTransport{callResult: "ok"}

	mcpTool := MCPTool{
		Name:        "ping",
		Description: "Ping",
	}

	def := convertMCPTool("srv", mcpTool, transport)

	props, ok := def.Parameters["properties"]
	if !ok {
		t.Fatal("expected properties in parameters")
	}
	if props == nil {
		t.Fatal("expected non-nil properties")
	}
}

func TestConvertMCPTool_ExecuteError(t *testing.T) {
	transport := &mockTransport{
		callErr: fmt.Errorf("connection refused"),
	}

	mcpTool := MCPTool{Name: "failing", Description: "fails"}
	def := convertMCPTool("srv", mcpTool, transport)

	result, err := def.Execute(context.Background(), nil, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute should not return Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for transport failure")
	}
}

func TestServiceConfigure_AddAndRemoveServers(t *testing.T) {
	b := bus.New()
	defer b.Close()
	s := NewService(b)
	defer s.Close()

	s.Configure(context.Background(), map[string]config.MCPConfig{
		"server-a": {Transport: "stdio", Command: "nonexistent-cmd-1234"},
		"server-b": {Transport: "stdio", Command: "nonexistent-cmd-5678"},
	})

	time.Sleep(100 * time.Millisecond)

	status := s.Status(context.Background())
	if len(status) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(status))
	}
	if _, ok := status["server-a"]; !ok {
		t.Error("expected server-a in status")
	}
	if _, ok := status["server-b"]; !ok {
		t.Error("expected server-b in status")
	}

	s.Configure(context.Background(), map[string]config.MCPConfig{
		"server-b": {Transport: "stdio", Command: "nonexistent-cmd-5678"},
	})

	time.Sleep(100 * time.Millisecond)

	status = s.Status(context.Background())
	if len(status) != 1 {
		t.Fatalf("expected 1 server after removal, got %d", len(status))
	}
	if _, ok := status["server-b"]; !ok {
		t.Error("expected server-b to remain")
	}
}

func TestServicePublishStatus(t *testing.T) {
	b := bus.New()
	defer b.Close()
	s := NewService(b)
	defer s.Close()

	sub := b.Subscribe("mcp.status")
	defer sub.Unsubscribe()

	s.Configure(context.Background(), map[string]config.MCPConfig{
		"test-srv": {Transport: "stdio", Command: "nonexistent-cmd-9999"},
	})

	gotConnecting := false
	timeout := time.After(2 * time.Second)
	for !gotConnecting {
		select {
		case evt := <-sub.C:
			props := evt.Properties.(map[string]any)
			server := props["server"].(ServerStatus)
			if server.Name == "test-srv" && server.Status == StatusConnecting {
				gotConnecting = true
			}
		case <-timeout:
			t.Fatal("timeout waiting for mcp.status event")
		}
	}
}

// Streamable HTTP transport tests with real HTTP server

func TestStreamableHTTPTransport_Connect(t *testing.T) {
	handler := &mockMCPHandler{}
	srv := httptest.NewServer(handler)
	defer srv.Close()

	transport := NewStreamableHTTPTransport(srv.URL, nil)
	err := transport.Connect(context.Background())
	if err != nil {
		t.Fatalf("connect error: %v", err)
	}

	if !handler.initialized {
		t.Error("expected initialize to be called")
	}
}

func TestStreamableHTTPTransport_ListTools(t *testing.T) {
	handler := &mockMCPHandler{
		tools: []MCPTool{
			{Name: "search", Description: "Search things"},
			{Name: "read", Description: "Read things"},
		},
	}
	srv := httptest.NewServer(handler)
	defer srv.Close()

	transport := NewStreamableHTTPTransport(srv.URL, nil)
	if err := transport.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}

	tools, err := transport.ListTools(context.Background())
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	if tools[0].Name != "search" {
		t.Errorf("expected 'search', got %q", tools[0].Name)
	}
}

func TestStreamableHTTPTransport_CallTool(t *testing.T) {
	handler := &mockMCPHandler{
		toolResult: "Hello, World!",
	}
	srv := httptest.NewServer(handler)
	defer srv.Close()

	transport := NewStreamableHTTPTransport(srv.URL, nil)
	if err := transport.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}

	result, err := transport.CallTool(context.Background(), "greet", json.RawMessage(`{"name":"World"}`))
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}

	if result != "Hello, World!" {
		t.Errorf("expected 'Hello, World!', got %q", result)
	}
}

func TestStreamableHTTPTransport_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	transport := NewStreamableHTTPTransport(srv.URL, nil)
	err := transport.Connect(context.Background())
	if err == nil {
		t.Fatal("expected error for unauthorized")
	}

	var unauthErr *UnauthorizedError
	if !errors.As(err, &unauthErr) {
		t.Errorf("expected UnauthorizedError, got %T: %v", err, err)
	}
}

func TestStreamableHTTPTransport_CustomHeaders(t *testing.T) {
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		resp := jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      1,
			Result:  json.RawMessage(`{}`),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	transport := NewStreamableHTTPTransport(srv.URL, map[string]string{
		"Authorization": "Bearer test-token",
	})
	_ = transport.Connect(context.Background())

	if receivedAuth != "Bearer test-token" {
		t.Errorf("expected Bearer test-token, got %q", receivedAuth)
	}
}

func TestStreamableHTTPTransport_SessionID(t *testing.T) {
	var lastSessionID string
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		lastSessionID = r.Header.Get("Mcp-Session-Id")

		if callCount == 1 {
			w.Header().Set("Mcp-Session-Id", "ses-abc-123")
		}

		var req jsonrpcRequest
		json.NewDecoder(r.Body).Decode(&req)
		resp := jsonrpcResponse{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{}`)}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	transport := NewStreamableHTTPTransport(srv.URL, nil)
	_ = transport.Connect(context.Background())

	transport.ListTools(context.Background())

	if lastSessionID != "ses-abc-123" {
		t.Errorf("expected session ID to be forwarded, got %q", lastSessionID)
	}
}

// Mock transport for unit tests

type mockTransport struct {
	tools      []MCPTool
	callResult string
	callErr    error
}

func (m *mockTransport) Connect(_ context.Context) error {
	return nil
}

func (m *mockTransport) ListTools(_ context.Context) ([]MCPTool, error) {
	return m.tools, nil
}

func (m *mockTransport) CallTool(_ context.Context, _ string, _ json.RawMessage) (string, error) {
	if m.callErr != nil {
		return "", m.callErr
	}
	return m.callResult, nil
}

func (m *mockTransport) Close() error {
	return nil
}

// Mock MCP HTTP handler for streamable HTTP transport tests

type mockMCPHandler struct {
	initialized bool
	tools       []MCPTool
	toolResult  string
}

func (h *mockMCPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req jsonrpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	var result any
	switch req.Method {
	case "initialize":
		h.initialized = true
		result = map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"serverInfo":      map[string]any{"name": "mock-server"},
		}
	case "notifications/initialized":
		w.WriteHeader(http.StatusAccepted)
		return
	case "tools/list":
		result = map[string]any{"tools": h.tools}
	case "tools/call":
		result = map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": h.toolResult},
			},
		}
	default:
		result = map[string]any{}
	}

	resultJSON, _ := json.Marshal(result)
	resp := jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  resultJSON,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
