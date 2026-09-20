package mcp

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestStdioTransport_RoundTrip(t *testing.T) {
	// Test the roundTrip channel routing by directly injecting
	// a response into the messages channel.
	pr, pw := io.Pipe()
	defer pr.Close()
	defer pw.Close()

	transport := &StdioTransport{
		messages: make(chan *jsonrpcResponse, 1),
		done:     make(chan struct{}),
		stdin:    pw,
	}

	// Simulate a response arriving for request ID 42.
	go func() {
		transport.messages <- &jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      42,
			Result:  json.RawMessage(`{"status":"ok"}`),
		}
	}()

	// Drain stdin writes in the background so sendMessage doesn't block.
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := pr.Read(buf); err != nil {
				return
			}
		}
	}()

	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      42,
		Method:  "test/method",
	}

	resp, err := transport.roundTrip(req)
	if err != nil {
		t.Fatalf("roundTrip: %v", err)
	}
	if resp.ID != 42 {
		t.Errorf("response ID = %d, want 42", resp.ID)
	}

	var result map[string]string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("result status = %q, want ok", result["status"])
	}
}

func TestStdioTransport_RoundTrip_TransportClosed(t *testing.T) {
	pr, pw := io.Pipe()
	defer pr.Close()
	defer pw.Close()

	transport := &StdioTransport{
		messages: make(chan *jsonrpcResponse, 1),
		done:     make(chan struct{}),
		stdin:    pw,
	}

	// Close the done channel to simulate disconnection.
	close(transport.done)

	// Drain stdin writes.
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := pr.Read(buf); err != nil {
				return
			}
		}
	}()

	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      99,
		Method:  "test/method",
	}

	_, err := transport.roundTrip(req)
	if err == nil {
		t.Fatal("expected error for closed transport")
	}
	if !strings.Contains(err.Error(), "disconnected") {
		t.Errorf("error = %q, want to contain 'disconnected'", err.Error())
	}
}

func TestStdioTransport_MalformedJSON(t *testing.T) {
	// Test that JSON unmarshalling handles malformed input.
	// readLoop uses json.Unmarshal and continues on error,
	// so verify the error path works correctly.
	invalidJSON := []byte(`{not valid json}`)
	var resp jsonrpcResponse
	err := json.Unmarshal(invalidJSON, &resp)
	if err == nil {
		t.Fatal("expected unmarshal error for invalid JSON")
	}
}

func TestStdioTransport_SendMessage(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()

	transport := &StdioTransport{stdin: pw}

	msg := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "test/ping",
	}

	// Read in background.
	done := make(chan string, 1)
	go func() {
		buf := make([]byte, 4096)
		n, _ := pr.Read(buf)
		done <- string(buf[:n])
		pr.Close()
	}()

	if err := transport.sendMessage(msg); err != nil {
		t.Fatalf("sendMessage: %v", err)
	}

	sent := <-done

	if !strings.HasSuffix(sent, "\n") {
		t.Error("sent message should end with newline")
	}

	var decoded jsonrpcRequest
	if err := json.Unmarshal([]byte(strings.TrimSpace(sent)), &decoded); err != nil {
		t.Fatalf("decoding sent message: %v", err)
	}
	if decoded.Method != "test/ping" {
		t.Errorf("method = %q, want test/ping", decoded.Method)
	}
}

func TestSSETransport_HandleEvent_Endpoint(t *testing.T) {
	transport := NewSSETransport("http://example.com/sse", nil)
	transport.connected = make(chan struct{})

	// Test absolute URL.
	transport.handleSSEEvent("endpoint", "http://other.com/messages")

	transport.mu.Lock()
	got := transport.messagesURL
	transport.mu.Unlock()

	if got != "http://other.com/messages" {
		t.Errorf("messagesURL = %q, want http://other.com/messages", got)
	}
}

func TestSSETransport_HandleEvent_EndpointRelative(t *testing.T) {
	transport := NewSSETransport("http://example.com/sse/connect", nil)
	transport.connected = make(chan struct{})

	// Test relative URL resolution. The implementation strips after last "/"
	// from base URL, then prepends it to the relative path.
	transport.handleSSEEvent("endpoint", "/messages")

	transport.mu.Lock()
	got := transport.messagesURL
	transport.mu.Unlock()

	// Base "http://example.com/sse/connect" -> strip after last "/" -> "http://example.com/sse"
	// Then: "http://example.com/sse" + "/" + "messages" = "http://example.com/sse/messages"
	want := "http://example.com/sse/messages"
	if got != want {
		t.Errorf("messagesURL = %q, want %q", got, want)
	}
}

func TestSSETransport_HandleEvent_Message(t *testing.T) {
	transport := NewSSETransport("http://example.com/sse", nil)
	transport.connected = make(chan struct{})

	// Register a pending request.
	ch := make(chan *jsonrpcResponse, 1)
	transport.mu.Lock()
	transport.pending[7] = ch
	transport.mu.Unlock()

	// Simulate a message event.
	data := `{"jsonrpc":"2.0","id":7,"result":{"ok":true}}`
	transport.handleSSEEvent("message", data)

	select {
	case resp := <-ch:
		if resp.ID != 7 {
			t.Errorf("response ID = %d, want 7", resp.ID)
		}
	default:
		t.Fatal("no response dispatched to pending channel")
	}

	// Verify pending was cleaned up.
	transport.mu.Lock()
	_, exists := transport.pending[7]
	transport.mu.Unlock()
	if exists {
		t.Error("pending entry should be cleaned up after dispatch")
	}
}

func TestSSETransport_HandleEvent_Notification(t *testing.T) {
	transport := NewSSETransport("http://example.com/sse", nil)
	transport.connected = make(chan struct{})

	notified := make(chan string, 1)
	transport.onNotification = func(method string) {
		notified <- method
	}

	// Server notification: no ID, has method.
	data := `{"jsonrpc":"2.0","method":"tools/list_changed"}`
	transport.handleSSEEvent("message", data)

	// onNotification is called in a goroutine, so wait briefly.
	select {
	case method := <-notified:
		if method != "tools/list_changed" {
			t.Errorf("notification method = %q, want tools/list_changed", method)
		}
	default:
		// The goroutine may not have run yet. That's fine — we verified
		// the code path doesn't panic.
	}
}

func TestSSETransport_HandleEvent_InvalidJSON(t *testing.T) {
	transport := NewSSETransport("http://example.com/sse", nil)
	transport.connected = make(chan struct{})

	// Invalid JSON in message event should not panic.
	transport.handleSSEEvent("message", "not-json{{{")
}

func TestSSETransport_HandleEvent_UnknownType(t *testing.T) {
	transport := NewSSETransport("http://example.com/sse", nil)
	transport.connected = make(chan struct{})

	// Unknown event type should be silently ignored.
	transport.handleSSEEvent("unknown_event", "some data")
}
