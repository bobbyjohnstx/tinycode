package llm

import (
	"encoding/json"
	"testing"
)

func TestEventType_Constants(t *testing.T) {
	tests := []struct {
		got  EventType
		want string
	}{
		{EventTextDelta, "text-delta"},
		{EventReasoningDelta, "reasoning-delta"},
		{EventToolCallBegin, "tool-call-begin"},
		{EventToolCallDelta, "tool-call-delta"},
		{EventToolCallEnd, "tool-call-end"},
		{EventFinish, "finish"},
		{EventError, "error"},
	}
	for _, tt := range tests {
		if string(tt.got) != tt.want {
			t.Errorf("EventType %q != %q", tt.got, tt.want)
		}
	}
}

func TestEvent_JSONMarshal_TextDelta(t *testing.T) {
	ev := Event{Type: EventTextDelta, Text: "hello"}
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded["type"] != "text-delta" {
		t.Errorf("type = %v, want text-delta", decoded["type"])
	}
	if decoded["text"] != "hello" {
		t.Errorf("text = %v, want hello", decoded["text"])
	}
}

func TestEvent_JSONMarshal_ToolCall(t *testing.T) {
	ev := Event{
		Type:       EventToolCallBegin,
		ToolCallID: "tc_01",
		ToolName:   "read",
	}
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded["type"] != "tool-call-begin" {
		t.Errorf("type = %v, want tool-call-begin", decoded["type"])
	}
	if decoded["toolCallID"] != "tc_01" {
		t.Errorf("toolCallID = %v, want tc_01", decoded["toolCallID"])
	}
	if decoded["toolName"] != "read" {
		t.Errorf("toolName = %v, want read", decoded["toolName"])
	}
}

func TestUsage_JSONRoundTrip(t *testing.T) {
	u := Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150}
	data, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var decoded Usage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded != u {
		t.Errorf("roundtrip mismatch: got %+v, want %+v", decoded, u)
	}
}

func TestMessage_JSONMarshal_WithToolCalls(t *testing.T) {
	msg := Message{
		Role:    "assistant",
		Content: "I'll read that file.",
		ToolCalls: []ToolCall{
			{
				ID:   "tc_01",
				Type: "function",
				Function: FunctionCall{
					Name:      "read",
					Arguments: `{"file_path": "test.go"}`,
				},
			},
		},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded.Role != "assistant" {
		t.Errorf("role = %q, want assistant", decoded.Role)
	}
	if len(decoded.ToolCalls) != 1 {
		t.Fatalf("tool_calls count = %d, want 1", len(decoded.ToolCalls))
	}
	if decoded.ToolCalls[0].Function.Name != "read" {
		t.Errorf("tool call name = %q, want read", decoded.ToolCalls[0].Function.Name)
	}
}

func TestMessage_JSONMarshal_ToolResult(t *testing.T) {
	msg := Message{
		Role:       "tool",
		Content:    "file contents here",
		ToolCallID: "tc_01",
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded.ToolCallID != "tc_01" {
		t.Errorf("tool_call_id = %q, want tc_01", decoded.ToolCallID)
	}
}

func TestTool_JSONRoundTrip(t *testing.T) {
	tool := Tool{
		Type: "function",
		Function: ToolFunction{
			Name:        "read",
			Description: "Read a file",
			Parameters:  map[string]any{"type": "object"},
		},
	}
	data, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var decoded Tool
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded.Type != "function" {
		t.Errorf("type = %q, want function", decoded.Type)
	}
	if decoded.Function.Name != "read" {
		t.Errorf("name = %q, want read", decoded.Function.Name)
	}
	if decoded.Function.Description != "Read a file" {
		t.Errorf("description = %q, want 'Read a file'", decoded.Function.Description)
	}
}

func TestToolFunction_OmitEmpty(t *testing.T) {
	tf := ToolFunction{Name: "test"}
	data, err := json.Marshal(tf)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if _, ok := decoded["description"]; ok {
		t.Error("description should be omitted when empty")
	}
	if _, ok := decoded["parameters"]; ok {
		t.Error("parameters should be omitted when nil")
	}
}

func TestRequest_JSONMarshal(t *testing.T) {
	temp := 0.7
	req := Request{
		Model:       "gpt-4",
		Messages:    []Message{{Role: "user", Content: "hi"}},
		Stream:      true,
		Temperature: &temp,
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded["model"] != "gpt-4" {
		t.Errorf("model = %v, want gpt-4", decoded["model"])
	}
	if decoded["stream"] != true {
		t.Errorf("stream = %v, want true", decoded["stream"])
	}
	if decoded["temperature"] != 0.7 {
		t.Errorf("temperature = %v, want 0.7", decoded["temperature"])
	}
}

func TestWithHeaders(t *testing.T) {
	headers := map[string]string{"X-Custom": "value"}
	opt := WithHeaders(headers)
	cfg := &streamConfig{}
	opt(cfg)
	if cfg.headers["X-Custom"] != "value" {
		t.Errorf("header X-Custom = %q, want value", cfg.headers["X-Custom"])
	}
}
