package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func sseHandler(lines []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		for _, line := range lines {
			fmt.Fprintf(w, "data: %s\n\n", line)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}

func textChunk(content string) string {
	return fmt.Sprintf(`{"choices":[{"delta":{"content":"%s"},"finish_reason":""}]}`, content)
}

func finishChunk(reason string) string {
	return fmt.Sprintf(`{"choices":[{"delta":{},"finish_reason":"%s"}]}`, reason)
}

func usageChunk(prompt, completion, total int) string {
	return fmt.Sprintf(`{"choices":[],"usage":{"prompt_tokens":%d,"completion_tokens":%d,"total_tokens":%d}}`, prompt, completion, total)
}

func collectEvents(t *testing.T, ch <-chan Event, timeout time.Duration) []Event {
	t.Helper()
	var events []Event
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return events
			}
			events = append(events, ev)
		case <-timer.C:
			t.Fatal("timed out waiting for events")
			return events
		}
	}
}

func TestNewOpenAIClient(t *testing.T) {
	client := NewOpenAIClient("http://localhost:8080/", "sk-test")
	if client.BaseURL != "http://localhost:8080" {
		t.Errorf("BaseURL = %q, want trailing slash trimmed", client.BaseURL)
	}
	if client.APIKey != "sk-test" {
		t.Errorf("APIKey = %q, want sk-test", client.APIKey)
	}
	if client.Client == nil {
		t.Error("Client is nil")
	}
	if client.Client.Timeout != 0 {
		t.Errorf("Timeout = %v, want 0", client.Client.Timeout)
	}
}

func TestOpenAIStream_TextDelta(t *testing.T) {
	server := httptest.NewServer(sseHandler([]string{
		textChunk("Hello"),
		textChunk(" world"),
		finishChunk("stop"),
	}))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "test-key")
	ch, err := client.Stream(context.Background(), Request{
		Model:    "gpt-4",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	events := collectEvents(t, ch, 5*time.Second)

	var text string
	var gotFinish bool
	for _, ev := range events {
		if ev.Type == EventTextDelta {
			text += ev.Text
		}
		if ev.Type == EventFinish && ev.FinishReason == "stop" {
			gotFinish = true
		}
	}
	if text != "Hello world" {
		t.Errorf("text = %q, want %q", text, "Hello world")
	}
	if !gotFinish {
		t.Error("missing finish event")
	}
}

func TestOpenAIStream_ToolCall(t *testing.T) {
	lines := []string{
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"tc_01","type":"function","function":{"name":"read","arguments":""}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"file"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"_path\":\"test.go\"}"}}]}}]}`,
		finishChunk("tool_calls"),
	}

	server := httptest.NewServer(sseHandler(lines))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "test-key")
	ch, err := client.Stream(context.Background(), Request{
		Model:    "gpt-4",
		Messages: []Message{{Role: "user", Content: "Read file"}},
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	events := collectEvents(t, ch, 5*time.Second)

	var gotBegin, gotDelta, gotEnd bool
	var toolName, toolID, toolArgs string
	for _, ev := range events {
		switch ev.Type {
		case EventToolCallBegin:
			gotBegin = true
			toolName = ev.ToolName
			toolID = ev.ToolCallID
		case EventToolCallDelta:
			gotDelta = true
		case EventToolCallEnd:
			gotEnd = true
			toolArgs = ev.ToolCallArgs
		}
	}

	if !gotBegin {
		t.Error("missing tool call begin")
	}
	if !gotDelta {
		t.Error("missing tool call delta")
	}
	if !gotEnd {
		t.Error("missing tool call end")
	}
	if toolName != "read" {
		t.Errorf("tool name = %q, want read", toolName)
	}
	if toolID != "tc_01" {
		t.Errorf("tool id = %q, want tc_01", toolID)
	}
	if toolArgs != `{"file_path":"test.go"}` {
		t.Errorf("tool args = %q, want {\"file_path\":\"test.go\"}", toolArgs)
	}
}

func TestOpenAIStream_UsageStats(t *testing.T) {
	server := httptest.NewServer(sseHandler([]string{
		textChunk("ok"),
		finishChunk("stop"),
		usageChunk(10, 5, 15),
	}))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "test-key")
	ch, err := client.Stream(context.Background(), Request{
		Model:    "gpt-4",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	events := collectEvents(t, ch, 5*time.Second)

	var gotUsage bool
	for _, ev := range events {
		if ev.Type == EventFinish && ev.Usage != nil {
			gotUsage = true
			if ev.Usage.PromptTokens != 10 {
				t.Errorf("prompt tokens = %d, want 10", ev.Usage.PromptTokens)
			}
			if ev.Usage.CompletionTokens != 5 {
				t.Errorf("completion tokens = %d, want 5", ev.Usage.CompletionTokens)
			}
			if ev.Usage.TotalTokens != 15 {
				t.Errorf("total tokens = %d, want 15", ev.Usage.TotalTokens)
			}
		}
	}
	if !gotUsage {
		t.Error("missing usage event")
	}
}

func TestOpenAIStream_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		fmt.Fprint(w, `{"error":"rate limited"}`)
	}))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "test-key")
	_, err := client.Stream(context.Background(), Request{
		Model:    "gpt-4",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error for 429 response")
	}
}

func TestOpenAIStream_AuthorizationHeader(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "sk-secret")
	ch, err := client.Stream(context.Background(), Request{
		Model:    "gpt-4",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	collectEvents(t, ch, 5*time.Second)

	if gotAuth != "Bearer sk-secret" {
		t.Errorf("Authorization = %q, want 'Bearer sk-secret'", gotAuth)
	}
}

func TestOpenAIStream_NoAuthWhenKeyEmpty(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "")
	ch, err := client.Stream(context.Background(), Request{
		Model:    "gpt-4",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	collectEvents(t, ch, 5*time.Second)

	if gotAuth != "" {
		t.Errorf("Authorization = %q, want empty for no API key", gotAuth)
	}
}

func TestOpenAIStream_WithHeaders(t *testing.T) {
	var gotCustom string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCustom = r.Header.Get("X-Custom-Header")
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "key")
	ch, err := client.Stream(context.Background(), Request{
		Model:    "gpt-4",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	}, WithHeaders(map[string]string{"X-Custom-Header": "custom-value"}))
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	collectEvents(t, ch, 5*time.Second)

	if gotCustom != "custom-value" {
		t.Errorf("X-Custom-Header = %q, want custom-value", gotCustom)
	}
}

func TestOpenAIStream_RequestBody(t *testing.T) {
	var gotBody Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("Accept = %q, want text/event-stream", r.Header.Get("Accept"))
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "key")
	ch, err := client.Stream(context.Background(), Request{
		Model:    "gpt-4",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	collectEvents(t, ch, 5*time.Second)

	if gotBody.Model != "gpt-4" {
		t.Errorf("request model = %q, want gpt-4", gotBody.Model)
	}
	if !gotBody.Stream {
		t.Error("request stream should be true")
	}
	if len(gotBody.Messages) != 1 {
		t.Fatalf("messages count = %d, want 1", len(gotBody.Messages))
	}
}

func TestOpenAIStream_ReasoningContent(t *testing.T) {
	lines := []string{
		`{"choices":[{"delta":{"reasoning_content":"Let me think..."}}]}`,
		textChunk("Answer"),
		finishChunk("stop"),
	}

	server := httptest.NewServer(sseHandler(lines))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "key")
	ch, err := client.Stream(context.Background(), Request{
		Model:    "deepseek-r1",
		Messages: []Message{{Role: "user", Content: "Think"}},
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	events := collectEvents(t, ch, 5*time.Second)

	var gotReasoning bool
	for _, ev := range events {
		if ev.Type == EventReasoningDelta {
			gotReasoning = true
			if ev.Text != "Let me think..." {
				t.Errorf("reasoning text = %q, want 'Let me think...'", ev.Text)
			}
		}
	}
	if !gotReasoning {
		t.Error("missing reasoning delta event")
	}
}

func TestOpenAIStream_InvalidToolCallJSON_Repaired(t *testing.T) {
	lines := []string{
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"tc_02","type":"function","function":{"name":"write","arguments":""}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":\"a.go\",}"}}]}}]}`,
		finishChunk("tool_calls"),
	}

	server := httptest.NewServer(sseHandler(lines))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "key")
	ch, err := client.Stream(context.Background(), Request{
		Model:    "gpt-4",
		Messages: []Message{{Role: "user", Content: "Write"}},
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	events := collectEvents(t, ch, 5*time.Second)

	var endEvent *Event
	for i := range events {
		if events[i].Type == EventToolCallEnd {
			endEvent = &events[i]
			break
		}
	}
	if endEvent == nil {
		t.Fatal("missing tool call end event")
	}
	if endEvent.ToolName != "write" {
		t.Errorf("tool name = %q, want write", endEvent.ToolName)
	}
	if endEvent.ToolCallArgs != `{"path":"a.go"}` {
		t.Errorf("repaired args = %q, want {\"path\":\"a.go\"}", endEvent.ToolCallArgs)
	}
}

func TestOpenAIStream_InvalidToolCallJSON_Unrepairable(t *testing.T) {
	lines := []string{
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"tc_03","type":"function","function":{"name":"write","arguments":""}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{not valid json at all"}}]}}]}`,
		finishChunk("tool_calls"),
	}

	server := httptest.NewServer(sseHandler(lines))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "key")
	ch, err := client.Stream(context.Background(), Request{
		Model:    "gpt-4",
		Messages: []Message{{Role: "user", Content: "Write"}},
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	events := collectEvents(t, ch, 5*time.Second)

	var endEvent *Event
	for i := range events {
		if events[i].Type == EventToolCallEnd {
			endEvent = &events[i]
			break
		}
	}
	if endEvent == nil {
		t.Fatal("missing tool call end event")
	}
	if endEvent.ToolName != "invalid" {
		t.Errorf("tool name = %q, want 'invalid' for unrepairable JSON", endEvent.ToolName)
	}
	var parsed map[string]string
	if err := json.Unmarshal([]byte(endEvent.ToolCallArgs), &parsed); err != nil {
		t.Fatalf("invalid tool args not valid JSON: %v", err)
	}
	if parsed["error"] != "invalid JSON in tool call arguments" {
		t.Errorf("error field = %q", parsed["error"])
	}
	if parsed["original_name"] != "write" {
		t.Errorf("original_name = %q, want write", parsed["original_name"])
	}
}

func TestOpenAIStream_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Keep connection open — context cancel should terminate
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	client := NewOpenAIClient(server.URL, "key")
	ch, err := client.Stream(ctx, Request{
		Model:    "gpt-4",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	cancel()

	events := collectEvents(t, ch, 5*time.Second)
	var gotError bool
	for _, ev := range events {
		if ev.Type == EventError {
			gotError = true
		}
	}
	if !gotError {
		t.Error("expected error event on context cancellation")
	}
}

func TestOpenAIStream_FinalizeToolsWithoutFinishReason(t *testing.T) {
	// Proxy ends with [DONE] only — no finish_reason.
	lines := []string{
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"tc_done","type":"function","function":{"name":"read","arguments":""}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":\"x.go\"}"}}]}}]}`,
	}
	server := httptest.NewServer(sseHandler(lines))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "key")
	ch, err := client.Stream(context.Background(), Request{
		Model:    "gpt-4",
		Messages: []Message{{Role: "user", Content: "Read"}},
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	events := collectEvents(t, ch, 5*time.Second)

	var endEvent *Event
	for i := range events {
		if events[i].Type == EventToolCallEnd {
			endEvent = &events[i]
			break
		}
	}
	if endEvent == nil {
		t.Fatal("expected tool call end when stream ends without finish_reason")
	}
	if endEvent.ToolName != "read" {
		t.Errorf("tool name = %q, want read", endEvent.ToolName)
	}
	if endEvent.ToolCallArgs != `{"path":"x.go"}` {
		t.Errorf("args = %q", endEvent.ToolCallArgs)
	}
}

func TestOpenAIStream_MultiToolOrderAndIDUpdate(t *testing.T) {
	lines := []string{
		// Index 1 first, then 0 — finalize must emit in ascending index order.
		`{"choices":[{"delta":{"tool_calls":[{"index":1,"id":"","type":"function","function":{"name":"","arguments":""}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"","type":"function","function":{"name":"","arguments":""}}]}}]}`,
		// Later deltas supply id/name.
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"tc_a","function":{"name":"read","arguments":"{\"a\":1}"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":1,"id":"tc_b","function":{"name":"write","arguments":"{\"b\":2}"}}]}}]}`,
		finishChunk("tool_calls"),
	}
	server := httptest.NewServer(sseHandler(lines))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "key")
	ch, err := client.Stream(context.Background(), Request{
		Model:    "gpt-4",
		Messages: []Message{{Role: "user", Content: "Do both"}},
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	events := collectEvents(t, ch, 5*time.Second)

	var ends []Event
	for _, ev := range events {
		if ev.Type == EventToolCallEnd {
			ends = append(ends, ev)
		}
	}
	if len(ends) != 2 {
		t.Fatalf("tool ends = %d, want 2", len(ends))
	}
	if ends[0].ToolCallID != "tc_a" || ends[0].ToolName != "read" {
		t.Errorf("first end = id=%q name=%q, want tc_a/read", ends[0].ToolCallID, ends[0].ToolName)
	}
	if ends[1].ToolCallID != "tc_b" || ends[1].ToolName != "write" {
		t.Errorf("second end = id=%q name=%q, want tc_b/write", ends[1].ToolCallID, ends[1].ToolName)
	}
}

func TestOpenAIStream_OmitsThinkingBudget(t *testing.T) {
	var raw map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&raw)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	budget := 8192
	client := NewOpenAIClient(server.URL, "key")
	ch, err := client.Stream(context.Background(), Request{
		Model:          "gpt-4",
		Messages:       []Message{{Role: "user", Content: "Hi"}},
		ThinkingBudget: &budget,
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	collectEvents(t, ch, 5*time.Second)

	if _, ok := raw["thinking_budget"]; ok {
		t.Errorf("OpenAI wire body must omit thinking_budget, got %#v", raw["thinking_budget"])
	}
}

func TestOpenAIClient_HeaderTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(200)
	}))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "key")
	client.Client.Transport = &http.Transport{ResponseHeaderTimeout: 50 * time.Millisecond}

	_, err := client.Stream(context.Background(), Request{
		Model:    "gpt-4",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected header timeout error")
	}
}

func TestOpenAIStream_MalformedSSEContinues(t *testing.T) {
	server := httptest.NewServer(sseHandler([]string{
		`{not valid json`,
		textChunk("ok"),
		finishChunk("stop"),
	}))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "key")
	ch, err := client.Stream(context.Background(), Request{
		Model:    "gpt-4",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	events := collectEvents(t, ch, 5*time.Second)
	var text string
	for _, ev := range events {
		if ev.Type == EventTextDelta {
			text += ev.Text
		}
	}
	if text != "ok" {
		t.Errorf("text = %q, want ok (stream should continue after bad SSE line)", text)
	}
}

func TestOpenAIStream_DefaultsMaxTokens(t *testing.T) {
	var raw map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&raw)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "key")
	ch, err := client.Stream(context.Background(), Request{
		Model:    "gpt-4",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	collectEvents(t, ch, 5*time.Second)
	got, _ := raw["max_tokens"].(float64)
	if int(got) != DefaultStreamTokens {
		t.Errorf("max_tokens = %v, want %d", raw["max_tokens"], DefaultStreamTokens)
	}
}

func TestOpenAIStream_StopsWhenReasoningLoops(t *testing.T) {
	sentence := "But wait, index.html still has the same unclosed footer. "
	lines := make([]string, 8)
	for i := range lines {
		lines[i] = `{"choices":[{"delta":{"reasoning_content":"` + sentence + `"}}]}`
	}
	server := httptest.NewServer(sseHandler(lines))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "key")
	ch, err := client.Stream(context.Background(), Request{
		Model:    "ornith",
		Messages: []Message{{Role: "user", Content: "Think"}},
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	events := collectEvents(t, ch, 5*time.Second)
	var reasoning int
	var budget bool
	for _, ev := range events {
		if ev.Type == EventReasoningDelta {
			reasoning++
		}
		if ev.Type == EventError && errors.Is(ev.Error, ErrStreamBudget) {
			budget = true
		}
	}
	if !budget {
		t.Fatal("expected the reasoning loop to stop the stream")
	}
	if reasoning >= len(lines) {
		t.Fatalf("reasoning chunks = %d, loop was not cut before the end", reasoning)
	}
}

func TestOpenAIStream_StopsWhenReasoningExceedsBudget(t *testing.T) {
	chunk := `{"choices":[{"delta":{"reasoning_content":"` + strings.Repeat("x", 400) + `"}}]}`
	// 400 bytes is about 100 tokens. Send more than the cap so the stream stops early.
	lines := make([]string, DefaultStreamTokens/100+20)
	for i := range lines {
		lines[i] = chunk
	}
	server := httptest.NewServer(sseHandler(lines))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "key")
	ch, err := client.Stream(context.Background(), Request{
		Model:    "ornith",
		Messages: []Message{{Role: "user", Content: "Think"}},
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	events := collectEvents(t, ch, 5*time.Second)
	var reasoning int
	var budget bool
	for _, ev := range events {
		if ev.Type == EventReasoningDelta {
			reasoning++
		}
		if ev.Type == EventError && errors.Is(ev.Error, ErrStreamBudget) {
			budget = true
		}
	}
	if !budget {
		t.Fatal("expected stream budget error")
	}
	if reasoning == 0 || reasoning >= len(lines) {
		t.Fatalf("reasoning chunks = %d, want a stop before all %d", reasoning, len(lines))
	}
}
