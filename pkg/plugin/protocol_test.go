package plugin

import (
	"encoding/json"
	"testing"
)

func TestJSONRPCRequest_Roundtrip(t *testing.T) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      42,
		Method:  "tool/call",
		Params:  json.RawMessage(`{"name":"read"}`),
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got JSONRPCRequest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.JSONRPC != req.JSONRPC {
		t.Errorf("jsonrpc: expected %q, got %q", req.JSONRPC, got.JSONRPC)
	}
	if got.ID != req.ID {
		t.Errorf("id: expected %d, got %d", req.ID, got.ID)
	}
	if got.Method != req.Method {
		t.Errorf("method: expected %q, got %q", req.Method, got.Method)
	}
	if string(got.Params) != string(req.Params) {
		t.Errorf("params: expected %s, got %s", req.Params, got.Params)
	}
}

func TestJSONRPCResponse_Roundtrip(t *testing.T) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      7,
		Result:  json.RawMessage(`{"content":"hello"}`),
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got JSONRPCResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.ID != resp.ID {
		t.Errorf("id: expected %d, got %d", resp.ID, got.ID)
	}
	if string(got.Result) != string(resp.Result) {
		t.Errorf("result: expected %s, got %s", resp.Result, got.Result)
	}
	if got.Error != nil {
		t.Errorf("error: expected nil, got %v", got.Error)
	}
}

func TestJSONRPCResponse_ErrorRoundtrip(t *testing.T) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      3,
		Error: &JSONRPCError{
			Code:    -32601,
			Message: "method not found",
			Data:    "extra detail",
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got JSONRPCResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Error == nil {
		t.Fatal("expected error, got nil")
	}
	if got.Error.Code != -32601 {
		t.Errorf("error code: expected -32601, got %d", got.Error.Code)
	}
	if got.Error.Message != "method not found" {
		t.Errorf("error message: expected 'method not found', got %q", got.Error.Message)
	}
}

func TestJSONRPCRequest_Notification(t *testing.T) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// ID should be omitted for notifications.
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	if _, ok := raw["id"]; ok {
		t.Error("notification should not have id field")
	}
}

func TestInitializeParams_Roundtrip(t *testing.T) {
	params := InitializeParams{
		Version:   "0.1.0",
		Directory: "/home/user/project",
		Options:   map[string]any{"verbose": true},
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got InitializeParams
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Version != params.Version {
		t.Errorf("version: expected %q, got %q", params.Version, got.Version)
	}
	if got.Directory != params.Directory {
		t.Errorf("directory: expected %q, got %q", params.Directory, got.Directory)
	}
}

func TestInitializeResult_Roundtrip(t *testing.T) {
	result := InitializeResult{
		ID: "notify",
		Tools: []ToolManifest{
			{
				Name:        "send-alert",
				Description: "Send an alert",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"message": map[string]any{"type": "string"},
					},
				},
			},
		},
		Hooks: []string{"session.start", "session.end"},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got InitializeResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.ID != "notify" {
		t.Errorf("id: expected 'notify', got %q", got.ID)
	}
	if len(got.Tools) != 1 {
		t.Fatalf("tools: expected 1, got %d", len(got.Tools))
	}
	if got.Tools[0].Name != "send-alert" {
		t.Errorf("tool name: expected 'send-alert', got %q", got.Tools[0].Name)
	}
	if len(got.Hooks) != 2 {
		t.Fatalf("hooks: expected 2, got %d", len(got.Hooks))
	}
}

func TestToolCallParams_Roundtrip(t *testing.T) {
	params := ToolCallParams{
		Name: "read-file",
		Args: json.RawMessage(`{"path":"/tmp/test.txt"}`),
		Context: ToolContext{
			SessionID: "ses-123",
			Directory: "/home/user",
		},
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got ToolCallParams
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Name != params.Name {
		t.Errorf("name: expected %q, got %q", params.Name, got.Name)
	}
	if got.Context.SessionID != "ses-123" {
		t.Errorf("sessionId: expected 'ses-123', got %q", got.Context.SessionID)
	}
}

func TestToolCallResult_Roundtrip(t *testing.T) {
	result := ToolCallResult{
		Content: "file contents here",
		IsError: false,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got ToolCallResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Content != result.Content {
		t.Errorf("content: expected %q, got %q", result.Content, got.Content)
	}
	if got.IsError {
		t.Error("expected IsError=false")
	}
}

func TestToolCallResult_ErrorOmitted(t *testing.T) {
	result := ToolCallResult{Content: "ok"}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	if _, ok := raw["isError"]; ok {
		t.Error("isError should be omitted when false")
	}
}

func TestHookParams_Roundtrip(t *testing.T) {
	params := HookParams{
		Name:  "session.start",
		Input: json.RawMessage(`{"sessionId":"s1","directory":"/tmp"}`),
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got HookParams
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Name != "session.start" {
		t.Errorf("name: expected 'session.start', got %q", got.Name)
	}
	if string(got.Input) != string(params.Input) {
		t.Errorf("input mismatch")
	}
}

func TestHookResult_Roundtrip(t *testing.T) {
	result := HookResult{
		Output: json.RawMessage(`{"allowed":true}`),
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got HookResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if string(got.Output) != `{"allowed":true}` {
		t.Errorf("output: expected '{\"allowed\":true}', got %s", got.Output)
	}
}
