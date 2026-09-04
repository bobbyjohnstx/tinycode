package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRepairToolCallJSON_Valid(t *testing.T) {
	input := `{"key": "value"}`
	result := RepairToolCallJSON(input)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if *result != input {
		t.Errorf("expected %q, got %q", input, *result)
	}
}

func TestRepairToolCallJSON_MarkdownFences(t *testing.T) {
	input := "```json\n{\"key\": \"value\"}\n```"
	result := RepairToolCallJSON(input)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if *result != `{"key": "value"}` {
		t.Errorf("expected clean JSON, got %q", *result)
	}
}

func TestRepairToolCallJSON_TrailingComma(t *testing.T) {
	input := `{"key": "value",}`
	result := RepairToolCallJSON(input)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if *result != `{"key": "value"}` {
		t.Errorf("expected no trailing comma, got %q", *result)
	}
}

func TestRepairToolCallJSON_ArrayTrailingComma(t *testing.T) {
	input := `[1, 2, 3,]`
	result := RepairToolCallJSON(input)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if *result != `[1, 2, 3]` {
		t.Errorf("expected no trailing comma, got %q", *result)
	}
}

func TestRepairToolCallJSON_Unfixable(t *testing.T) {
	input := `{definitely not json at all`
	result := RepairToolCallJSON(input)
	if result != nil {
		t.Errorf("expected nil for unfixable, got %q", *result)
	}
}

func TestRepairToolCallJSON_FencesAndComma(t *testing.T) {
	input := "```json\n{\"a\": 1, \"b\": 2,}\n```"
	result := RepairToolCallJSON(input)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if *result != `{"a": 1, "b": 2}` {
		t.Errorf("got %q", *result)
	}
}

func TestOpenAIClient_StreamText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		chunks := []string{
			`data: {"id":"1","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
			`data: {"id":"1","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
			`data: {"id":"1","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}`,
			`data: {"id":"1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`,
			`data: [DONE]`,
		}

		flusher := w.(http.Flusher)
		for _, chunk := range chunks {
			fmt.Fprintln(w, chunk)
			fmt.Fprintln(w)
			flusher.Flush()
		}
	}))
	defer srv.Close()

	client := NewOpenAIClient(srv.URL, "")

	ch, err := client.Stream(context.Background(), Request{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("stream error: %v", err)
	}

	var text strings.Builder
	var finishReason string
	for event := range ch {
		switch event.Type {
		case EventTextDelta:
			text.WriteString(event.Text)
		case EventFinish:
			if event.FinishReason != "" {
				finishReason = event.FinishReason
			}
		case EventError:
			t.Fatalf("unexpected error: %v", event.Error)
		}
	}

	if text.String() != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", text.String())
	}
	if finishReason != "stop" {
		t.Errorf("expected finish_reason 'stop', got %q", finishReason)
	}
}

func TestOpenAIClient_StreamToolCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		chunks := []string{
			`data: {"id":"1","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"read","arguments":""}}]},"finish_reason":null}]}`,
			`data: {"id":"1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"file\":"}}]},"finish_reason":null}]}`,
			`data: {"id":"1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":" \"test.go\"}"}}]},"finish_reason":null}]}`,
			`data: {"id":"1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		}

		flusher := w.(http.Flusher)
		for _, chunk := range chunks {
			fmt.Fprintln(w, chunk)
			fmt.Fprintln(w)
			flusher.Flush()
		}
	}))
	defer srv.Close()

	client := NewOpenAIClient(srv.URL, "")

	ch, err := client.Stream(context.Background(), Request{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "read test.go"}},
		Tools: []Tool{{
			Type: "function",
			Function: ToolFunction{
				Name:        "read",
				Description: "Read a file",
				Parameters:  map[string]any{"type": "object"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("stream error: %v", err)
	}

	var toolBeginCount, toolEndCount int
	var toolName, toolArgs string
	for event := range ch {
		switch event.Type {
		case EventToolCallBegin:
			toolBeginCount++
			toolName = event.ToolName
		case EventToolCallEnd:
			toolEndCount++
			toolArgs = event.ToolCallArgs
		case EventError:
			t.Fatalf("unexpected error: %v", event.Error)
		}
	}

	if toolBeginCount != 1 {
		t.Errorf("expected 1 tool-call-begin, got %d", toolBeginCount)
	}
	if toolEndCount != 1 {
		t.Errorf("expected 1 tool-call-end, got %d", toolEndCount)
	}
	if toolName != "read" {
		t.Errorf("expected tool name 'read', got %q", toolName)
	}
	if toolArgs != `{"file": "test.go"}` {
		t.Errorf("expected tool args, got %q", toolArgs)
	}
}

func TestOpenAIClient_StreamToolCallWithBrokenJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// Simulate a tool call with trailing comma (common LLM mistake)
		chunks := []string{
			`data: {"id":"1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"edit","arguments":""}}]},"finish_reason":null}]}`,
			`data: {"id":"1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"file\": \"x.go\","}}]},"finish_reason":null}]}`,
			`data: {"id":"1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"}"}}]},"finish_reason":null}]}`,
			`data: {"id":"1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		}

		flusher := w.(http.Flusher)
		for _, chunk := range chunks {
			fmt.Fprintln(w, chunk)
			fmt.Fprintln(w)
			flusher.Flush()
		}
	}))
	defer srv.Close()

	client := NewOpenAIClient(srv.URL, "")

	ch, err := client.Stream(context.Background(), Request{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "edit"}},
	})
	if err != nil {
		t.Fatalf("stream error: %v", err)
	}

	var toolArgs string
	for event := range ch {
		if event.Type == EventToolCallEnd {
			toolArgs = event.ToolCallArgs
		}
	}

	// The repair should fix the trailing comma
	if toolArgs != `{"file": "x.go"}` {
		t.Errorf("expected repaired JSON, got %q", toolArgs)
	}
}

func TestOpenAIClient_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "invalid api key"}`))
	}))
	defer srv.Close()

	client := NewOpenAIClient(srv.URL, "bad-key")

	_, err := client.Stream(context.Background(), Request{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 in error, got %v", err)
	}
}

func TestOpenAIClient_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		// Hold the connection open until the client disconnects
		<-r.Context().Done()
	}))
	defer srv.Close()

	client := NewOpenAIClient(srv.URL, "")
	ctx, cancel := context.WithCancel(context.Background())

	ch, err := client.Stream(ctx, Request{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("stream error: %v", err)
	}

	cancel()

	var gotError bool
	for event := range ch {
		if event.Type == EventError {
			gotError = true
		}
	}
	if !gotError {
		t.Error("expected error event after context cancellation")
	}
}

func TestOpenAIClient_WithHeaders(t *testing.T) {
	var receivedHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "data: [DONE]")
		w.(http.Flusher).Flush()
	}))
	defer srv.Close()

	client := NewOpenAIClient(srv.URL, "test-key")

	ch, err := client.Stream(context.Background(), Request{
		Model:    "test",
		Messages: []Message{{Role: "user", Content: "hi"}},
	}, WithHeaders(map[string]string{
		"X-Custom": "custom-value",
	}))
	if err != nil {
		t.Fatalf("stream error: %v", err)
	}
	for range ch {
	}

	if receivedHeaders.Get("Authorization") != "Bearer test-key" {
		t.Error("expected Authorization header")
	}
	if receivedHeaders.Get("X-Custom") != "custom-value" {
		t.Error("expected custom header")
	}
}

func TestOpenAIClient_UsageInSeparateChunk(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		chunks := []string{
			`data: {"id":"1","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}]}`,
			`data: {"id":"1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			`data: {"id":"1","choices":[],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`,
			`data: [DONE]`,
		}

		flusher := w.(http.Flusher)
		for _, chunk := range chunks {
			fmt.Fprintln(w, chunk)
			fmt.Fprintln(w)
			flusher.Flush()
		}
	}))
	defer srv.Close()

	client := NewOpenAIClient(srv.URL, "")

	ch, err := client.Stream(context.Background(), Request{
		Model:    "test",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("stream error: %v", err)
	}

	var usage *Usage
	for event := range ch {
		if event.Usage != nil {
			usage = event.Usage
		}
	}

	if usage == nil {
		t.Fatal("expected usage data")
	}
	if usage.PromptTokens != 5 || usage.CompletionTokens != 1 {
		t.Errorf("unexpected usage: %+v", usage)
	}
}
