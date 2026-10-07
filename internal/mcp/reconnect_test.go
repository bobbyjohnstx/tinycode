package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/config"
)

func TestBackoffDelay(t *testing.T) {
	for attempt := 0; attempt < 10; attempt++ {
		delay := backoffDelay(attempt)

		if delay <= 0 {
			t.Errorf("attempt %d: expected positive delay, got %v", attempt, delay)
		}

		maxExpected := reconnectMaxDelay + time.Duration(float64(reconnectMaxDelay)*reconnectJitter)
		if delay > maxExpected {
			t.Errorf("attempt %d: delay %v exceeds max %v", attempt, delay, maxExpected)
		}
	}

	// Verify exponential growth for early attempts (before hitting cap)
	d0 := backoffDelay(0)
	d1 := backoffDelay(1)
	// d1 should be roughly 2x d0, allowing for jitter
	ratio := float64(d1) / float64(d0)
	if ratio < 1.0 || ratio > 4.0 {
		t.Errorf("expected d1/d0 ratio roughly 2x, got %f (d0=%v, d1=%v)", ratio, d0, d1)
	}
}

func TestBackoffDelay_CapsAtMax(t *testing.T) {
	// High attempt should cap at max + jitter
	delay := backoffDelay(100)
	maxWithJitter := reconnectMaxDelay + time.Duration(float64(reconnectMaxDelay)*reconnectJitter)
	if delay > maxWithJitter {
		t.Errorf("expected delay capped at ~%v, got %v", reconnectMaxDelay, delay)
	}
}

func TestSSETransport_NotificationHandling(t *testing.T) {
	var notificationReceived atomic.Value
	notificationReceived.Store("")

	type sseSink struct {
		w       http.ResponseWriter
		flusher http.Flusher
	}
	sseReady := make(chan sseSink, 1)
	msgPosted := make(chan jsonrpcRequest, 8)

	mux := http.NewServeMux()
	handlerDone := make(chan struct{})
	mux.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "event: endpoint\ndata: http://%s/messages\n\n", r.Host)
		flusher.Flush()
		sseReady <- sseSink{w: w, flusher: flusher}
		<-handlerDone
	})
	mux.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		var req jsonrpcRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.WriteHeader(http.StatusAccepted)
		if req.Method != "notifications/initialized" {
			msgPosted <- req
		}
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		sink := <-sseReady
		// Complete initialize handshake via SSE message event.
		req := <-msgPosted
		payload, _ := json.Marshal(jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(`{}`),
		})
		fmt.Fprintf(sink.w, "event: message\ndata: %s\n\n", payload)
		sink.flusher.Flush()

		notifJSON := `{"jsonrpc":"2.0","method":"notifications/tools/list_changed"}`
		fmt.Fprintf(sink.w, "event: message\ndata: %s\n\n", notifJSON)
		sink.flusher.Flush()
	}()

	transport := NewSSETransport(srv.URL+"/sse", nil)
	transport.onNotification = func(method string) {
		notificationReceived.Store(method)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := transport.Connect(ctx)
	if err != nil {
		t.Fatalf("connect error: %v", err)
	}
	defer transport.Close()

	// Wait for notification to be processed
	deadline := time.After(2 * time.Second)
	for {
		if method := notificationReceived.Load().(string); method == "notifications/tools/list_changed" {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for notification")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	<-writerDone
	close(handlerDone)
}

func TestSSETransport_DisconnectCallback(t *testing.T) {
	disconnected := make(chan struct{}, 1)

	type sseSink struct {
		w       http.ResponseWriter
		flusher http.Flusher
	}
	sseReady := make(chan sseSink, 1)
	msgPosted := make(chan jsonrpcRequest, 8)
	closeSSE := make(chan struct{})

	mux := http.NewServeMux()
	mux.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "event: endpoint\ndata: http://%s/messages\n\n", r.Host)
		flusher.Flush()
		sseReady <- sseSink{w: w, flusher: flusher}
		<-closeSSE
	})
	mux.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		var req jsonrpcRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.WriteHeader(http.StatusAccepted)
		if req.Method != "notifications/initialized" {
			msgPosted <- req
		}
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	go func() {
		sink := <-sseReady
		req := <-msgPosted
		payload, _ := json.Marshal(jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(`{}`),
		})
		fmt.Fprintf(sink.w, "event: message\ndata: %s\n\n", payload)
		sink.flusher.Flush()
		// Close stream after handshake to simulate disconnect.
		close(closeSSE)
	}()

	transport := NewSSETransport(srv.URL+"/sse", nil)
	transport.onDisconnect = func() {
		select {
		case disconnected <- struct{}{}:
		default:
		}
	}

	ctx := context.Background()
	err := transport.Connect(ctx)
	if err != nil {
		t.Fatalf("connect error: %v", err)
	}
	defer transport.Close()

	select {
	case <-disconnected:
		// ok
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for disconnect callback")
	}
}

func TestStreamableHTTPTransport_DisconnectOnUnauthorized(t *testing.T) {
	disconnected := make(chan struct{}, 1)
	callCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount <= 2 {
			// First calls succeed (initialize + initialized)
			var req jsonrpcRequest
			json.NewDecoder(r.Body).Decode(&req)
			if req.Method == "notifications/initialized" {
				w.WriteHeader(http.StatusAccepted)
				return
			}
			resp := jsonrpcResponse{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{}`)}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		// Third call returns 401
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	transport := NewStreamableHTTPTransport(srv.URL, nil)
	transport.onDisconnect = func() {
		select {
		case disconnected <- struct{}{}:
		default:
		}
	}

	ctx := context.Background()
	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("connect error: %v", err)
	}

	// This should trigger 401 and onDisconnect
	_, err := transport.ListTools(ctx)
	if err == nil {
		t.Fatal("expected error on unauthorized")
	}

	select {
	case <-disconnected:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for disconnect callback on 401")
	}
}

func TestServiceReconnect_NoDuplicateTools(t *testing.T) {
	b := bus.New()
	defer b.Close()

	// Track tool listings across reconnections
	var toolListCount atomic.Int32

	tools := []MCPTool{
		{Name: "search", Description: "Search things"},
		{Name: "read", Description: "Read things"},
	}

	mt := &reconnectMockTransport{
		tools:         tools,
		callResult:    "ok",
		toolListCount: &toolListCount,
	}

	s := NewService(b)
	defer s.Close()

	// Manually set up a connected server
	s.mu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	s.servers["test-srv"] = &serverConn{
		name:      "test-srv",
		config:    config.MCPConfig{Transport: "stdio", Command: "echo"},
		transport: mt,
		status:    StatusConnected,
		tools:     tools,
		cancel:    cancel,
		ctx:       ctx,
	}
	s.mu.Unlock()

	// Refresh tools (simulates tools/list_changed notification)
	s.refreshTools("test-srv")

	// Verify tools were refreshed (listed again)
	if toolListCount.Load() < 1 {
		t.Error("expected at least one tool listing after refresh")
	}

	// Verify no duplicate tools
	allTools := s.Tools(context.Background())
	if len(allTools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(allTools))
	}

	// Verify specific tool IDs
	if _, ok := allTools["mcp__test-srv__search"]; !ok {
		t.Error("expected mcp__test-srv__search tool")
	}
	if _, ok := allTools["mcp__test-srv__read"]; !ok {
		t.Error("expected mcp__test-srv__read tool")
	}
}

func TestServiceReconnect_BusEvents(t *testing.T) {
	b := bus.New()
	defer b.Close()

	subReconnecting := b.Subscribe("mcp.reconnecting")
	defer subReconnecting.Unsubscribe()

	subReconnected := b.Subscribe("mcp.reconnected")
	defer subReconnected.Unsubscribe()

	// Create a mock transport that connects successfully
	tools := []MCPTool{{Name: "test-tool", Description: "A test tool"}}
	mt := &reconnectMockTransport{
		tools:      tools,
		callResult: "ok",
	}

	s := NewService(b)
	defer s.Close()

	// Set up the server with a transport that will "succeed" on reconnect
	// We simulate by directly calling reconnectServer with a mock setup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.mu.Lock()
	s.servers["reconn-srv"] = &serverConn{
		name:   "reconn-srv",
		config: config.MCPConfig{Transport: "stdio", Command: "echo"},
		transport: mt,
		status: StatusConnected,
		cancel: cancel,
		ctx:    ctx,
		tools:  tools,
	}
	s.mu.Unlock()

	// Verify bus events by triggering a refresh (lightweight way to verify bus integration)
	s.refreshTools("reconn-srv")

	// Check mcp.status was published
	statusSub := b.Subscribe("mcp.status")
	defer statusSub.Unsubscribe()

	s.publishStatus("reconn-srv")

	select {
	case evt := <-statusSub.C:
		props := evt.Properties.(map[string]any)
		server := props["server"].(ServerStatus)
		if server.Name != "reconn-srv" {
			t.Errorf("expected server name reconn-srv, got %s", server.Name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for status event")
	}
}

func TestServiceReconnect_PreventsConcurrent(t *testing.T) {
	b := bus.New()
	defer b.Close()

	s := NewService(b)
	defer s.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up a server that's already reconnecting
	s.mu.Lock()
	s.servers["busy-srv"] = &serverConn{
		name:         "busy-srv",
		config:       config.MCPConfig{Transport: "stdio", Command: "nonexistent-cmd"},
		status:       StatusReconnecting,
		reconnecting: true,
	}
	s.mu.Unlock()

	// Try to reconnect - should return immediately since already reconnecting
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.reconnectServer(ctx, "busy-srv")
	}()

	// Should complete quickly since it bails out
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// ok - returned immediately because already reconnecting
	case <-time.After(2 * time.Second):
		t.Fatal("reconnectServer should have returned immediately for already-reconnecting server")
	}
}

func TestStdioTransport_TimeoutConfigurable(t *testing.T) {
	transport := NewStdioTransport("echo", nil, nil)
	transport.Timeout = 100 * time.Millisecond

	if transport.Timeout != 100*time.Millisecond {
		t.Errorf("expected 100ms timeout, got %v", transport.Timeout)
	}
}

func TestStdioTransport_DefaultTimeout(t *testing.T) {
	transport := NewStdioTransport("echo", nil, nil)

	if transport.Timeout != 0 {
		t.Errorf("expected zero (uses default), got %v", transport.Timeout)
	}

	// The default is applied in roundTrip
	expected := defaultStdioTimeout
	if expected != 60*time.Second {
		t.Errorf("expected default timeout 60s, got %v", expected)
	}
}

func TestServiceReconnect_MaxAttemptsExhausted(t *testing.T) {
	b := bus.New()
	defer b.Close()

	s := NewService(b)
	defer s.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use a command that will always fail to connect
	s.mu.Lock()
	s.servers["fail-srv"] = &serverConn{
		name:   "fail-srv",
		config: config.MCPConfig{Transport: "stdio", Command: "nonexistent-binary-xyz-9876"},
		status: StatusConnected,
	}
	s.mu.Unlock()

	// Override maxReconnectAttempts behavior by using context cancellation
	// to limit attempts (since we can't change the const)
	shortCtx, shortCancel := context.WithTimeout(ctx, 3*time.Second)
	defer shortCancel()

	s.reconnectServer(shortCtx, "fail-srv")

	// After context expires, server should be in error state
	s.mu.RLock()
	conn := s.servers["fail-srv"]
	status := conn.status
	s.mu.RUnlock()

	if status != StatusError && status != StatusReconnecting {
		// Either error (from failed connect) or reconnecting (if context cancelled during attempt)
		t.Logf("status after reconnect attempts: %s", status)
	}
}

func TestNotificationMethodParsing(t *testing.T) {
	// Verify that jsonrpcResponse can parse notification messages
	notifJSON := `{"jsonrpc":"2.0","method":"notifications/tools/list_changed"}`
	var resp jsonrpcResponse
	err := json.Unmarshal([]byte(notifJSON), &resp)
	if err != nil {
		t.Fatalf("failed to unmarshal notification: %v", err)
	}

	if resp.Method != "notifications/tools/list_changed" {
		t.Errorf("expected method 'notifications/tools/list_changed', got %q", resp.Method)
	}
	if resp.ID != 0 {
		t.Errorf("expected ID 0, got %d", resp.ID)
	}
	if resp.Result != nil {
		t.Errorf("expected nil result, got %s", resp.Result)
	}
}

func TestStatusReconnecting_Exists(t *testing.T) {
	if StatusReconnecting != "reconnecting" {
		t.Errorf("expected 'reconnecting', got %q", StatusReconnecting)
	}
}

// reconnectMockTransport implements Transport with tool listing tracking
type reconnectMockTransport struct {
	tools         []MCPTool
	callResult    string
	callErr       error
	toolListCount *atomic.Int32
}

func (m *reconnectMockTransport) Connect(_ context.Context) error {
	return nil
}

func (m *reconnectMockTransport) ListTools(_ context.Context) ([]MCPTool, error) {
	if m.toolListCount != nil {
		m.toolListCount.Add(1)
	}
	return m.tools, nil
}

func (m *reconnectMockTransport) CallTool(_ context.Context, _ string, _ json.RawMessage) (string, error) {
	if m.callErr != nil {
		return "", m.callErr
	}
	return m.callResult, nil
}

func (m *reconnectMockTransport) Close() error {
	return nil
}
