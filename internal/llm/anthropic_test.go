package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func TestAnthropicBuildRequest_ThinkingBudget(t *testing.T) {
	client := NewAnthropicClient("http://localhost", "test-key")
	budget := 16384
	req := Request{
		Model:          "claude-sonnet-4-20250514",
		Messages:       []Message{{Role: "user", Content: "hello"}},
		ThinkingBudget: &budget,
		Temperature:    func() *float64 { f := 0.5; return &f }(),
	}

	ar := client.buildRequest(req)

	if ar.Thinking == nil {
		t.Fatal("expected Thinking to be set")
	}
	if ar.Thinking.Type != "enabled" {
		t.Errorf("expected Thinking.Type 'enabled', got %q", ar.Thinking.Type)
	}
	if ar.Thinking.BudgetTokens != 16384 {
		t.Errorf("expected BudgetTokens 16384, got %d", ar.Thinking.BudgetTokens)
	}
	// Temperature must be nil when thinking is enabled.
	if ar.Temperature != nil {
		t.Errorf("expected Temperature nil when thinking enabled, got %v", *ar.Temperature)
	}
}

func TestAnthropicBuildRequest_NoThinkingBudget(t *testing.T) {
	client := NewAnthropicClient("http://localhost", "test-key")
	req := Request{
		Model:    "claude-sonnet-4-20250514",
		Messages: []Message{{Role: "user", Content: "hello"}},
	}

	ar := client.buildRequest(req)

	if ar.Thinking != nil {
		t.Errorf("expected Thinking nil when no budget, got %+v", ar.Thinking)
	}
}

func TestAnthropicBuildRequest_ZeroThinkingBudget(t *testing.T) {
	client := NewAnthropicClient("http://localhost", "test-key")
	budget := 0
	req := Request{
		Model:          "claude-sonnet-4-20250514",
		Messages:       []Message{{Role: "user", Content: "hello"}},
		ThinkingBudget: &budget,
	}

	ar := client.buildRequest(req)

	if ar.Thinking != nil {
		t.Errorf("expected Thinking nil when budget is 0, got %+v", ar.Thinking)
	}
}

func intPtr(n int) *int { return &n }

func TestAnthropicBuildRequest_ToolUseAndResultHistory(t *testing.T) {
	client := NewAnthropicClient("https://api.anthropic.com", "key")
	req := Request{
		Model: "claude-3-opus-20240229",
		Messages: []Message{
			{Role: "user", Content: "Read the file"},
			{
				Role:    "assistant",
				Content: "I'll read it.",
				ToolCalls: []ToolCall{
					{
						ID:   "tc_01",
						Type: "function",
						Function: FunctionCall{
							Name:      "read",
							Arguments: `{"file_path":"test.go"}`,
						},
					},
				},
			},
			{Role: "tool", Content: "package main", ToolCallID: "tc_01"},
			{Role: "user", Content: "Summarize it"},
		},
		MaxTokens: intPtr(100),
	}

	ar := client.buildRequest(req)

	if len(ar.Messages) != 4 {
		t.Fatalf("messages count = %d, want 4 (user, assistant tool_use, user tool_result, user)", len(ar.Messages))
	}

	assistant := ar.Messages[1]
	if assistant.Role != "assistant" {
		t.Fatalf("message[1] role = %q, want assistant", assistant.Role)
	}
	blocks, ok := assistant.Content.([]map[string]any)
	if !ok {
		t.Fatalf("assistant content type = %T, want []map[string]any", assistant.Content)
	}
	if len(blocks) != 2 {
		t.Fatalf("assistant blocks = %d, want 2 (text + tool_use)", len(blocks))
	}
	if blocks[0]["type"] != "text" || blocks[0]["text"] != "I'll read it." {
		t.Errorf("unexpected text block: %#v", blocks[0])
	}
	if blocks[1]["type"] != "tool_use" || blocks[1]["id"] != "tc_01" || blocks[1]["name"] != "read" {
		t.Errorf("unexpected tool_use block: %#v", blocks[1])
	}
	input, ok := blocks[1]["input"].(map[string]any)
	if !ok || input["file_path"] != "test.go" {
		t.Errorf("tool_use input = %#v, want file_path=test.go", blocks[1]["input"])
	}

	toolResultMsg := ar.Messages[2]
	if toolResultMsg.Role != "user" {
		t.Fatalf("message[2] role = %q, want user", toolResultMsg.Role)
	}
	results, ok := toolResultMsg.Content.([]map[string]any)
	if !ok || len(results) != 1 {
		t.Fatalf("tool result content = %#v", toolResultMsg.Content)
	}
	if results[0]["type"] != "tool_result" || results[0]["tool_use_id"] != "tc_01" || results[0]["content"] != "package main" {
		t.Errorf("unexpected tool_result: %#v", results[0])
	}
}

func TestAnthropicBuildRequest_MergesConsecutiveToolResults(t *testing.T) {
	client := NewAnthropicClient("https://api.anthropic.com", "key")
	ar := client.buildRequest(Request{
		Model: "claude-3-opus-20240229",
		Messages: []Message{
			{Role: "user", Content: "do both"},
			{
				Role: "assistant",
				ToolCalls: []ToolCall{
					{ID: "a", Type: "function", Function: FunctionCall{Name: "read", Arguments: `{}`}},
					{ID: "b", Type: "function", Function: FunctionCall{Name: "read", Arguments: `{}`}},
				},
			},
			{Role: "tool", Content: "ra", ToolCallID: "a"},
			{Role: "tool", Content: "rb", ToolCallID: "b"},
		},
	})

	if len(ar.Messages) != 3 {
		t.Fatalf("messages = %d, want 3 (user, assistant, merged tool results)", len(ar.Messages))
	}
	results, ok := ar.Messages[2].Content.([]map[string]any)
	if !ok || len(results) != 2 {
		t.Fatalf("merged tool results = %#v", ar.Messages[2].Content)
	}
}

func TestAnthropicStream_InvalidToolCallJSON_Unrepairable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		events := []string{
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"tc_bad\",\"name\":\"write\"}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{not valid json at all\"}}\n\n",
			"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"}}\n\n",
		}
		for _, e := range events {
			fmt.Fprint(w, e)
		}
	}))
	defer server.Close()

	client := NewAnthropicClient(server.URL, "test-key")
	ch, err := client.Stream(context.Background(), Request{
		Model:     "claude-3-opus-20240229",
		Messages:  []Message{{Role: "user", Content: "Write"}},
		MaxTokens: intPtr(100),
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	var endEvent *Event
	for ev := range ch {
		if ev.Type == EventToolCallEnd {
			endEvent = &ev
		}
	}
	if endEvent == nil {
		t.Fatal("missing tool call end")
	}
	if endEvent.ToolName != "invalid" {
		t.Errorf("tool name = %q, want invalid", endEvent.ToolName)
	}
	var parsed map[string]string
	if err := json.Unmarshal([]byte(endEvent.ToolCallArgs), &parsed); err != nil {
		t.Fatalf("invalid args not JSON: %v", err)
	}
	if parsed["original_name"] != "write" {
		t.Errorf("original_name = %q, want write", parsed["original_name"])
	}
}

func TestAnthropicStream_ToolHistoryRoundTrip(t *testing.T) {
	var secondBody []byte
	var requestNum int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNum++
		body, _ := io.ReadAll(r.Body)
		if requestNum == 2 {
			secondBody = body
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		if requestNum == 1 {
			events := []string{
				"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"tc_rt\",\"name\":\"read\"}}\n\n",
				"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"path\\\":\\\"a.go\\\"}\"}}\n\n",
				"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
				"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"}}\n\n",
			}
			for _, e := range events {
				fmt.Fprint(w, e)
			}
			return
		}
		fmt.Fprint(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
		fmt.Fprint(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"done\"}}\n\n")
		fmt.Fprint(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
		fmt.Fprint(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\n")
	}))
	defer server.Close()

	client := NewAnthropicClient(server.URL, "test-key")

	ch1, err := client.Stream(context.Background(), Request{
		Model:     "claude-3-opus-20240229",
		Messages:  []Message{{Role: "user", Content: "Read a.go"}},
		MaxTokens: intPtr(100),
	})
	if err != nil {
		t.Fatalf("first Stream: %v", err)
	}
	var toolID, toolName, toolArgs string
	for ev := range ch1 {
		if ev.Type == EventToolCallEnd {
			toolID, toolName, toolArgs = ev.ToolCallID, ev.ToolName, ev.ToolCallArgs
		}
	}
	if toolID == "" {
		t.Fatal("first stream missing tool call")
	}

	ch2, err := client.Stream(context.Background(), Request{
		Model: "claude-3-opus-20240229",
		Messages: []Message{
			{Role: "user", Content: "Read a.go"},
			{
				Role: "assistant",
				ToolCalls: []ToolCall{{
					ID: toolID, Type: "function",
					Function: FunctionCall{Name: toolName, Arguments: toolArgs},
				}},
			},
			{Role: "tool", Content: "file contents", ToolCallID: toolID},
		},
		MaxTokens: intPtr(100),
	})
	if err != nil {
		t.Fatalf("second Stream: %v", err)
	}
	for range ch2 {
	}

	if len(secondBody) == 0 {
		t.Fatal("second request body not captured")
	}
	bodyStr := string(secondBody)
	if !strings.Contains(bodyStr, `"type":"tool_use"`) && !strings.Contains(bodyStr, `"type": "tool_use"`) {
		// json.Marshal omits spaces
		if !strings.Contains(bodyStr, `"tool_use"`) {
			t.Errorf("second body missing tool_use: %s", bodyStr)
		}
	}
	if !strings.Contains(bodyStr, `"tool_result"`) {
		t.Errorf("second body missing tool_result: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, toolID) {
		t.Errorf("second body missing tool id %q: %s", toolID, bodyStr)
	}
}

func TestAnthropicClient_HeaderTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(200)
	}))
	defer server.Close()

	client := NewAnthropicClient(server.URL, "key")
	client.Client.Transport = &http.Transport{ResponseHeaderTimeout: 50 * time.Millisecond}

	_, err := client.Stream(context.Background(), Request{
		Model:     "claude-3-opus-20240229",
		Messages:  []Message{{Role: "user", Content: "Hi"}},
		MaxTokens: intPtr(10),
	})
	if err == nil {
		t.Fatal("expected header timeout error")
	}
}

func TestUsesAnthropicProtocol(t *testing.T) {
	tests := []struct {
		npm, providerID, url string
		want                 bool
	}{
		{"@ai-sdk/anthropic", "", "https://proxy.example.com", true},
		{"", "anthropic", "https://proxy.example.com", true},
		{"", "", "https://api.anthropic.com", true},
		{"@ai-sdk/openai-compatible", "ollama", "http://localhost:11434", false},
	}
	for _, tt := range tests {
		got := UsesAnthropicProtocol(tt.npm, tt.providerID, tt.url)
		if got != tt.want {
			t.Errorf("UsesAnthropicProtocol(%q,%q,%q)=%v, want %v", tt.npm, tt.providerID, tt.url, got, tt.want)
		}
	}
}

func TestNewClient_SelectsAnthropicByNPM(t *testing.T) {
	c := NewClient("https://proxy.example.com", "key", "@ai-sdk/anthropic", "custom")
	if _, ok := c.(*AnthropicClient); !ok {
		t.Fatalf("got %T, want *AnthropicClient", c)
	}
}

func TestNewClient_SelectsOpenAIByDefault(t *testing.T) {
	c := NewClient("http://localhost:11434", "", "", "ollama")
	if _, ok := c.(*OpenAIClient); !ok {
		t.Fatalf("got %T, want *OpenAIClient", c)
	}
}
