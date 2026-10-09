package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// sseTestOpts configures the mock SSE server responses for tools/list and tools/call.
type sseTestOpts struct {
	tools      []MCPTool
	toolResult string
	toolError  bool
}

// newSSETestTransport creates an SSE test server with a full initialize handshake
// and returns a connected transport. The returned cleanup function must be called
// when the test is done.
func newSSETestTransport(t *testing.T, opts sseTestOpts) (*SSETransport, func()) {
	t.Helper()

	type sseSink struct {
		w       http.ResponseWriter
		flusher http.Flusher
	}
	sseReady := make(chan sseSink, 1)
	msgPosted := make(chan jsonrpcRequest, 16)
	handlerDone := make(chan struct{})

	mux := http.NewServeMux()
	mux.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
		f, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "no flush", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "event: endpoint\ndata: http://%s/messages\n\n", r.Host)
		f.Flush()
		sseReady <- sseSink{w: w, flusher: f}
		<-handlerDone
	})
	mux.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		var req jsonrpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		if req.Method != "notifications/initialized" {
			msgPosted <- req
		}
	})

	srv := httptest.NewServer(mux)

	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		sink := <-sseReady
		for req := range msgPosted {
			var result json.RawMessage
			switch req.Method {
			case "initialize":
				result, _ = json.Marshal(map[string]any{
					"protocolVersion": "2024-11-05",
					"capabilities":    map[string]any{},
				})
			case "tools/list":
				result, _ = json.Marshal(map[string]any{
					"tools": opts.tools,
				})
			case "tools/call":
				result, _ = json.Marshal(map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": opts.toolResult},
					},
					"isError": opts.toolError,
				})
			default:
				result, _ = json.Marshal(map[string]any{})
			}
			payload, _ := json.Marshal(jsonrpcResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  result,
			})
			fmt.Fprintf(sink.w, "event: message\ndata: %s\n\n", payload)
			sink.flusher.Flush()
		}
	}()

	transport := NewSSETransport(srv.URL+"/sse", nil)
	// Use Background so the SSE connection stays open after Connect returns.
	// The internal connCtx is cancelled by transport.Close() in cleanup.
	if err := transport.Connect(context.Background()); err != nil {
		close(msgPosted)
		<-writerDone
		close(handlerDone)
		srv.Close()
		t.Fatalf("SSE Connect: %v", err)
	}

	cleanup := func() {
		transport.Close()
		close(msgPosted)
		<-writerDone
		close(handlerDone)
		srv.Close()
	}

	return transport, cleanup
}

func TestSSETransport_ListTools_ReturnsTools(t *testing.T) {
	transport, cleanup := newSSETestTransport(t, sseTestOpts{
		tools: []MCPTool{
			{Name: "search", Description: "Search the web"},
			{Name: "fetch", Description: "Fetch a URL"},
		},
	})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tools, err := transport.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	if tools[0].Name != "search" {
		t.Errorf("first tool = %q, want search", tools[0].Name)
	}
	if tools[1].Name != "fetch" {
		t.Errorf("second tool = %q, want fetch", tools[1].Name)
	}
}

func TestSSETransport_CallTool_ReturnsOutput(t *testing.T) {
	transport, cleanup := newSSETestTransport(t, sseTestOpts{
		toolResult: "Hello from SSE tool",
	})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := transport.CallTool(ctx, "greet", json.RawMessage(`{"name":"World"}`))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result != "Hello from SSE tool" {
		t.Errorf("result = %q, want 'Hello from SSE tool'", result)
	}
}

func TestSSETransport_CallTool_ReturnsErrorOnIsError(t *testing.T) {
	transport, cleanup := newSSETestTransport(t, sseTestOpts{
		toolResult: "something went wrong",
		toolError:  true,
	})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := transport.CallTool(ctx, "failing-tool", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for tool with isError=true")
	}
	if !strings.Contains(err.Error(), "something went wrong") {
		t.Errorf("error = %q, want to contain 'something went wrong'", err.Error())
	}
}
