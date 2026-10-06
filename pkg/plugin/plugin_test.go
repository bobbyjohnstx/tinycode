package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestBuildManifest(t *testing.T) {
	p := Plugin{
		ID: "test-plugin",
		Tools: []ToolDef{
			{
				Name:        "greet",
				Description: "Greet someone",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name": map[string]any{"type": "string"},
					},
				},
			},
			{
				Name:        "farewell",
				Description: "Say goodbye",
			},
		},
		Hooks: HookHandlers{
			SessionStart: func(_ context.Context, _ SessionStartEvent) (*SessionStartOutput, error) { return nil, nil },
		},
	}

	m := buildManifest(p)

	if m.ID != "test-plugin" {
		t.Errorf("id: expected 'test-plugin', got %q", m.ID)
	}
	if len(m.Tools) != 2 {
		t.Fatalf("tools: expected 2, got %d", len(m.Tools))
	}
	if m.Tools[0].Name != "greet" {
		t.Errorf("tool[0].name: expected 'greet', got %q", m.Tools[0].Name)
	}
	if m.Tools[1].Name != "farewell" {
		t.Errorf("tool[1].name: expected 'farewell', got %q", m.Tools[1].Name)
	}
	if len(m.Hooks) != 1 || m.Hooks[0] != "session.start" {
		t.Errorf("hooks: expected [session.start], got %v", m.Hooks)
	}
}

func TestRegisteredHooks_AllSet(t *testing.T) {
	h := &HookHandlers{
		SessionStart:   func(_ context.Context, _ SessionStartEvent) (*SessionStartOutput, error) { return nil, nil },
		SessionEnd:     func(_ context.Context, _ SessionEndEvent) error { return nil },
		PermissionAsk:  func(_ context.Context, _ PermissionInput) (*PermissionOutput, error) { return nil, nil },
		ShellEnv:       func(_ context.Context, _ ShellEnvInput) (*ShellEnvOutput, error) { return nil, nil },
		ToolExecBefore: func(_ context.Context, _ ToolExecBeforeInput) (*ToolExecBeforeOutput, error) { return nil, nil },
		ToolExecAfter:  func(_ context.Context, _ ToolExecAfterInput) (*ToolExecAfterOutput, error) { return nil, nil },
		Dispose:        func(_ context.Context) error { return nil },
	}

	hooks := registeredHooks(h)
	expected := []string{
		"session.start", "session.end", "permission.ask", "shell.env",
		"tool.execute.before", "tool.execute.after", "dispose",
	}
	if len(hooks) != len(expected) {
		t.Fatalf("expected %d hooks, got %d: %v", len(expected), len(hooks), hooks)
	}
	for i, name := range expected {
		if hooks[i] != name {
			t.Errorf("hook[%d]: expected %q, got %q", i, name, hooks[i])
		}
	}
}

func TestRegisteredHooks_NoneSet(t *testing.T) {
	h := &HookHandlers{}
	hooks := registeredHooks(h)
	if len(hooks) != 0 {
		t.Errorf("expected no hooks, got %v", hooks)
	}
}

func TestRun_InitializeAndToolCall(t *testing.T) {
	p := Plugin{
		ID: "echo",
		Tools: []ToolDef{
			{
				Name:        "echo",
				Description: "Echo the input",
				Execute: func(_ context.Context, args json.RawMessage, _ ToolContext) (string, error) {
					var input struct {
						Message string `json:"message"`
					}
					json.Unmarshal(args, &input)
					return "echo: " + input.Message, nil
				},
			},
		},
	}

	initParams, _ := json.Marshal(InitializeParams{Version: "0.1.0", Directory: "/tmp"})
	toolParams, _ := json.Marshal(ToolCallParams{
		Name: "echo",
		Args: json.RawMessage(`{"message":"hello"}`),
		Context: ToolContext{SessionID: "s1", Directory: "/tmp"},
	})

	var input bytes.Buffer
	writeRequest(&input, JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: initParams})
	writeRequest(&input, JSONRPCRequest{JSONRPC: "2.0", ID: 2, Method: "tool/call", Params: toolParams})

	var output bytes.Buffer
	err := run(context.Background(), p, &input, &output)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}

	responses := parseResponses(t, output.String())
	if len(responses) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(responses))
	}

	// Check initialize response.
	if responses[0].Error != nil {
		t.Fatalf("initialize error: %s", responses[0].Error.Message)
	}
	var initResult InitializeResult
	if err := json.Unmarshal(responses[0].Result, &initResult); err != nil {
		t.Fatalf("unmarshal init result: %v", err)
	}
	if initResult.ID != "echo" {
		t.Errorf("expected plugin id 'echo', got %q", initResult.ID)
	}

	// Check tool call response.
	if responses[1].Error != nil {
		t.Fatalf("tool call error: %s", responses[1].Error.Message)
	}
	var toolResult ToolCallResult
	if err := json.Unmarshal(responses[1].Result, &toolResult); err != nil {
		t.Fatalf("unmarshal tool result: %v", err)
	}
	if toolResult.Content != "echo: hello" {
		t.Errorf("expected 'echo: hello', got %q", toolResult.Content)
	}
	if toolResult.IsError {
		t.Error("expected IsError=false")
	}
}

func TestRun_ToolCallError(t *testing.T) {
	p := Plugin{
		ID: "failing",
		Tools: []ToolDef{
			{
				Name: "fail",
				Execute: func(_ context.Context, _ json.RawMessage, _ ToolContext) (string, error) {
					return "", errors.New("tool execution failed")
				},
			},
		},
	}

	toolParams, _ := json.Marshal(ToolCallParams{Name: "fail", Args: json.RawMessage(`{}`), Context: ToolContext{}})

	var input bytes.Buffer
	initParams, _ := json.Marshal(InitializeParams{Version: "0.1.0"})
	writeRequest(&input, JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: initParams})
	writeRequest(&input, JSONRPCRequest{JSONRPC: "2.0", ID: 2, Method: "tool/call", Params: toolParams})

	var output bytes.Buffer
	if err := run(context.Background(), p, &input, &output); err != nil {
		t.Fatalf("run error: %v", err)
	}

	responses := parseResponses(t, output.String())
	if len(responses) < 2 {
		t.Fatalf("expected 2 responses, got %d", len(responses))
	}

	var toolResult ToolCallResult
	json.Unmarshal(responses[1].Result, &toolResult)
	if !toolResult.IsError {
		t.Error("expected IsError=true for tool that returns error")
	}
}

func TestRun_UnknownTool(t *testing.T) {
	p := Plugin{ID: "empty"}

	toolParams, _ := json.Marshal(ToolCallParams{Name: "nonexistent", Args: json.RawMessage(`{}`), Context: ToolContext{}})

	var input bytes.Buffer
	initParams, _ := json.Marshal(InitializeParams{Version: "0.1.0"})
	writeRequest(&input, JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: initParams})
	writeRequest(&input, JSONRPCRequest{JSONRPC: "2.0", ID: 2, Method: "tool/call", Params: toolParams})

	var output bytes.Buffer
	if err := run(context.Background(), p, &input, &output); err != nil {
		t.Fatalf("run error: %v", err)
	}

	responses := parseResponses(t, output.String())
	if responses[1].Error == nil {
		t.Fatal("expected error for unknown tool")
	}
	if responses[1].Error.Code != -32602 {
		t.Errorf("expected code -32602, got %d", responses[1].Error.Code)
	}
}

func TestRun_UnknownMethod(t *testing.T) {
	p := Plugin{ID: "empty"}

	var input bytes.Buffer
	initParams, _ := json.Marshal(InitializeParams{Version: "0.1.0"})
	writeRequest(&input, JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: initParams})
	writeRequest(&input, JSONRPCRequest{JSONRPC: "2.0", ID: 2, Method: "unknown/method"})

	var output bytes.Buffer
	if err := run(context.Background(), p, &input, &output); err != nil {
		t.Fatalf("run error: %v", err)
	}

	responses := parseResponses(t, output.String())
	if responses[1].Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if responses[1].Error.Code != -32601 {
		t.Errorf("expected code -32601, got %d", responses[1].Error.Code)
	}
}

func TestRun_HookDispatch(t *testing.T) {
	var receivedSessionID string
	p := Plugin{
		ID: "hooks",
		Hooks: HookHandlers{
			SessionStart: func(_ context.Context, event SessionStartEvent) (*SessionStartOutput, error) {
				receivedSessionID = event.SessionID
				return nil, nil
			},
		},
	}

	hookInput, _ := json.Marshal(SessionStartEvent{SessionID: "ses-abc", Directory: "/tmp"})
	hookParams, _ := json.Marshal(HookParams{Name: "session.start", Input: hookInput})

	var input bytes.Buffer
	initParams, _ := json.Marshal(InitializeParams{Version: "0.1.0"})
	writeRequest(&input, JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: initParams})
	writeRequest(&input, JSONRPCRequest{JSONRPC: "2.0", ID: 2, Method: "hook/invoke", Params: hookParams})

	var output bytes.Buffer
	if err := run(context.Background(), p, &input, &output); err != nil {
		t.Fatalf("run error: %v", err)
	}

	if receivedSessionID != "ses-abc" {
		t.Errorf("expected session id 'ses-abc', got %q", receivedSessionID)
	}

	responses := parseResponses(t, output.String())
	if responses[1].Error != nil {
		t.Fatalf("hook error: %s", responses[1].Error.Message)
	}
}

func TestRun_NotificationsIgnored(t *testing.T) {
	p := Plugin{ID: "quiet"}

	var input bytes.Buffer
	initParams, _ := json.Marshal(InitializeParams{Version: "0.1.0"})
	writeRequest(&input, JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: initParams})
	// Notification (no ID).
	writeRequest(&input, JSONRPCRequest{JSONRPC: "2.0", Method: "notifications/initialized"})

	var output bytes.Buffer
	if err := run(context.Background(), p, &input, &output); err != nil {
		t.Fatalf("run error: %v", err)
	}

	responses := parseResponses(t, output.String())
	// Only the initialize response.
	if len(responses) != 1 {
		t.Errorf("expected 1 response (initialize only), got %d", len(responses))
	}
}

func TestRun_DisposeExactlyOnce(t *testing.T) {
	var disposeCount int
	p := Plugin{
		ID: "dispose-once",
		Hooks: HookHandlers{
			Dispose: func(_ context.Context) error {
				disposeCount++
				return nil
			},
		},
	}

	disposeParams, _ := json.Marshal(HookParams{Name: "dispose"})
	var input bytes.Buffer
	initParams, _ := json.Marshal(InitializeParams{Version: "0.1.0"})
	writeRequest(&input, JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: initParams})
	writeRequest(&input, JSONRPCRequest{JSONRPC: "2.0", ID: 2, Method: "hook/invoke", Params: disposeParams})
	// stdin EOF after dispose hook — must not call Dispose again.

	var output bytes.Buffer
	if err := run(context.Background(), p, &input, &output); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if disposeCount != 1 {
		t.Fatalf("expected Dispose called exactly once, got %d", disposeCount)
	}
}

func TestRun_AdditionalContextRoundTrip(t *testing.T) {
	p := Plugin{
		ID: "ctx",
		Hooks: HookHandlers{
			SessionStart: func(_ context.Context, _ SessionStartEvent) (*SessionStartOutput, error) {
				return &SessionStartOutput{AdditionalContext: []string{"from-start"}}, nil
			},
			ToolExecBefore: func(_ context.Context, _ ToolExecBeforeInput) (*ToolExecBeforeOutput, error) {
				return &ToolExecBeforeOutput{AdditionalContext: []string{"from-before"}}, nil
			},
			ToolExecAfter: func(_ context.Context, _ ToolExecAfterInput) (*ToolExecAfterOutput, error) {
				return &ToolExecAfterOutput{
					Output:            "modified",
					AdditionalContext: []string{"from-after"},
				}, nil
			},
		},
	}

	startInput, _ := json.Marshal(SessionStartEvent{SessionID: "s1"})
	startParams, _ := json.Marshal(HookParams{Name: "session.start", Input: startInput})
	beforeInput, _ := json.Marshal(ToolExecBeforeInput{SessionID: "s1", ToolName: "t"})
	beforeParams, _ := json.Marshal(HookParams{Name: "tool.execute.before", Input: beforeInput})
	afterInput, _ := json.Marshal(ToolExecAfterInput{SessionID: "s1", ToolName: "t", Output: "orig"})
	afterParams, _ := json.Marshal(HookParams{Name: "tool.execute.after", Input: afterInput})

	var input bytes.Buffer
	initParams, _ := json.Marshal(InitializeParams{Version: "0.1.0"})
	writeRequest(&input, JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: initParams})
	writeRequest(&input, JSONRPCRequest{JSONRPC: "2.0", ID: 2, Method: "hook/invoke", Params: startParams})
	writeRequest(&input, JSONRPCRequest{JSONRPC: "2.0", ID: 3, Method: "hook/invoke", Params: beforeParams})
	writeRequest(&input, JSONRPCRequest{JSONRPC: "2.0", ID: 4, Method: "hook/invoke", Params: afterParams})

	var output bytes.Buffer
	if err := run(context.Background(), p, &input, &output); err != nil {
		t.Fatalf("run error: %v", err)
	}
	responses := parseResponses(t, output.String())
	if len(responses) != 4 {
		t.Fatalf("expected 4 responses, got %d", len(responses))
	}

	assertHookAdditionalContext(t, responses[1], "from-start")
	assertHookAdditionalContext(t, responses[2], "from-before")
	assertHookAdditionalContext(t, responses[3], "from-after")
}

func assertHookAdditionalContext(t *testing.T, resp JSONRPCResponse, want string) {
	t.Helper()
	if resp.Error != nil {
		t.Fatalf("hook error: %s", resp.Error.Message)
	}
	var hr HookResult
	if err := json.Unmarshal(resp.Result, &hr); err != nil {
		t.Fatalf("unmarshal HookResult: %v", err)
	}
	var parsed struct {
		AdditionalContext []string `json:"additionalContext"`
	}
	if err := json.Unmarshal(hr.Output, &parsed); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if len(parsed.AdditionalContext) != 1 || parsed.AdditionalContext[0] != want {
		t.Fatalf("expected additionalContext %q, got %v", want, parsed.AdditionalContext)
	}
}

// Helpers

func writeRequest(buf *bytes.Buffer, req JSONRPCRequest) {
	data, _ := json.Marshal(req)
	buf.Write(data)
	buf.WriteByte('\n')
}

func parseResponses(t *testing.T, output string) []JSONRPCResponse {
	t.Helper()
	var responses []JSONRPCResponse
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		var resp JSONRPCResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("parse response %q: %v", line, err)
		}
		responses = append(responses, resp)
	}
	return responses
}
