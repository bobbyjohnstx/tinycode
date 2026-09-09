package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnthropicStream_TextDelta(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Error("missing x-api-key header")
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Error("missing anthropic-version header")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)

		events := []string{
			"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":10}}}\n\n",
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\" world\"}}\n\n",
			"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":5}}\n\n",
		}
		for _, e := range events {
			fmt.Fprint(w, e)
		}
	}))
	defer server.Close()

	client := NewAnthropicClient(server.URL, "test-key")
	req := Request{
		Model: "claude-3-opus-20240229",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
		MaxTokens: intPtr(100),
	}

	ch, err := client.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	var textParts []string
	var gotFinish bool
	for ev := range ch {
		switch ev.Type {
		case EventTextDelta:
			textParts = append(textParts, ev.Text)
		case EventFinish:
			if ev.FinishReason == "end_turn" {
				gotFinish = true
			}
		}
	}

	fullText := ""
	for _, p := range textParts {
		fullText += p
	}
	if fullText != "Hello world" {
		t.Errorf("got text %q, want %q", fullText, "Hello world")
	}
	if !gotFinish {
		t.Error("did not receive finish event")
	}
}

func TestAnthropicStream_ToolUse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)

		events := []string{
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"tc_01\",\"name\":\"read\"}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"file\"}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"_path\\\":\\\"test.go\\\"}\"}}\n\n",
			"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"output_tokens\":20}}\n\n",
		}
		for _, e := range events {
			fmt.Fprint(w, e)
		}
	}))
	defer server.Close()

	client := NewAnthropicClient(server.URL, "test-key")
	ch, err := client.Stream(context.Background(), Request{
		Model:     "claude-3-opus-20240229",
		Messages:  []Message{{Role: "user", Content: "Read file"}},
		MaxTokens: intPtr(100),
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	var gotBegin, gotDelta, gotEnd bool
	var toolName, toolID string
	for ev := range ch {
		switch ev.Type {
		case EventToolCallBegin:
			gotBegin = true
			toolName = ev.ToolName
			toolID = ev.ToolCallID
		case EventToolCallDelta:
			gotDelta = true
		case EventToolCallEnd:
			gotEnd = true
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
		t.Errorf("tool name = %q, want %q", toolName, "read")
	}
	if toolID != "tc_01" {
		t.Errorf("tool id = %q, want %q", toolID, "tc_01")
	}
}

func TestAnthropicStream_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		fmt.Fprint(w, `{"error":{"type":"authentication_error","message":"invalid api key"}}`)
	}))
	defer server.Close()

	client := NewAnthropicClient(server.URL, "bad-key")
	_, err := client.Stream(context.Background(), Request{
		Model:     "claude-3-opus-20240229",
		Messages:  []Message{{Role: "user", Content: "Hello"}},
		MaxTokens: intPtr(100),
	})
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
}

func TestAnthropicBuildRequest_SystemMessage(t *testing.T) {
	client := NewAnthropicClient("https://api.anthropic.com", "key")
	req := Request{
		Model: "claude-3-opus-20240229",
		Messages: []Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "Hello"},
		},
		MaxTokens: intPtr(100),
	}

	ar := client.buildRequest(req)

	if ar.System != "You are helpful." {
		t.Errorf("system = %q, want %q", ar.System, "You are helpful.")
	}
	if len(ar.Messages) != 1 {
		t.Fatalf("messages count = %d, want 1", len(ar.Messages))
	}
	if ar.Messages[0].Role != "user" {
		t.Errorf("message role = %q, want %q", ar.Messages[0].Role, "user")
	}
}

func TestAnthropicBuildRequest_Tools(t *testing.T) {
	client := NewAnthropicClient("https://api.anthropic.com", "key")
	req := Request{
		Model:    "claude-3-opus-20240229",
		Messages: []Message{{Role: "user", Content: "test"}},
		Tools: []Tool{
			{
				Type: "function",
				Function: ToolFunction{
					Name:        "read",
					Description: "Read a file",
					Parameters:  map[string]any{"type": "object"},
				},
			},
		},
		MaxTokens: intPtr(100),
	}

	ar := client.buildRequest(req)

	if len(ar.Tools) != 1 {
		t.Fatalf("tools count = %d, want 1", len(ar.Tools))
	}
	if ar.Tools[0].Name != "read" {
		t.Errorf("tool name = %q, want %q", ar.Tools[0].Name, "read")
	}
	if ar.Tools[0].InputSchema == nil {
		t.Error("tool input_schema is nil")
	}
}

func TestAnthropicStream_ThinkingDelta(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)

		events := []string{
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"thinking\",\"thinking\":\"\"}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"Let me think...\"}}\n\n",
			"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\n",
		}
		for _, e := range events {
			fmt.Fprint(w, e)
		}
	}))
	defer server.Close()

	client := NewAnthropicClient(server.URL, "test-key")
	ch, err := client.Stream(context.Background(), Request{
		Model:     "claude-3-opus-20240229",
		Messages:  []Message{{Role: "user", Content: "Think"}},
		MaxTokens: intPtr(100),
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	var gotReasoning bool
	for ev := range ch {
		if ev.Type == EventReasoningDelta {
			gotReasoning = true
			if ev.Text != "Let me think..." {
				t.Errorf("reasoning text = %q, want %q", ev.Text, "Let me think...")
			}
		}
	}
	if !gotReasoning {
		t.Error("did not receive reasoning delta event")
	}
}

func intPtr(n int) *int { return &n }
