package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
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

// dispatchHook tests

func TestDispatchHook_SessionStart(t *testing.T) {
	var receivedSessionID string
	hooks := &HookHandlers{
		SessionStart: func(_ context.Context, event SessionStartEvent) (*SessionStartOutput, error) {
			receivedSessionID = event.SessionID
			return &SessionStartOutput{AdditionalContext: []string{"hello"}}, nil
		},
	}

	hookInput, _ := json.Marshal(SessionStartEvent{SessionID: "s1", Directory: "/tmp"})
	hookParams, _ := json.Marshal(HookParams{Name: "session.start", Input: hookInput})
	req := JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "hook/invoke", Params: hookParams}

	var disposed atomic.Bool
	resp := dispatchHook(context.Background(), req, hooks, &disposed)

	if resp.Error != nil {
		t.Fatalf("expected no error, got: %s", resp.Error.Message)
	}
	if receivedSessionID != "s1" {
		t.Errorf("expected sessionId 's1', got %q", receivedSessionID)
	}
}

func TestDispatchHook_SessionEnd(t *testing.T) {
	var receivedSessionID string
	hooks := &HookHandlers{
		SessionEnd: func(_ context.Context, event SessionEndEvent) error {
			receivedSessionID = event.SessionID
			return nil
		},
	}

	hookInput, _ := json.Marshal(SessionEndEvent{SessionID: "s2"})
	hookParams, _ := json.Marshal(HookParams{Name: "session.end", Input: hookInput})
	req := JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "hook/invoke", Params: hookParams}

	var disposed atomic.Bool
	resp := dispatchHook(context.Background(), req, hooks, &disposed)

	if resp.Error != nil {
		t.Fatalf("expected no error, got: %s", resp.Error.Message)
	}
	if receivedSessionID != "s2" {
		t.Errorf("expected sessionId 's2', got %q", receivedSessionID)
	}
}

func TestDispatchHook_PermissionAsk(t *testing.T) {
	hooks := &HookHandlers{
		PermissionAsk: func(_ context.Context, input PermissionInput) (*PermissionOutput, error) {
			return &PermissionOutput{Allowed: true}, nil
		},
	}

	hookInput, _ := json.Marshal(PermissionInput{SessionID: "s1", ToolName: "bash", Permission: "shell"})
	hookParams, _ := json.Marshal(HookParams{Name: "permission.ask", Input: hookInput})
	req := JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "hook/invoke", Params: hookParams}

	var disposed atomic.Bool
	resp := dispatchHook(context.Background(), req, hooks, &disposed)

	if resp.Error != nil {
		t.Fatalf("expected no error, got: %s", resp.Error.Message)
	}
	var hr HookResult
	json.Unmarshal(resp.Result, &hr)
	var perm PermissionOutput
	json.Unmarshal(hr.Output, &perm)
	if !perm.Allowed {
		t.Error("expected permission allowed=true")
	}
}

func TestDispatchHook_ShellEnv(t *testing.T) {
	hooks := &HookHandlers{
		ShellEnv: func(_ context.Context, input ShellEnvInput) (*ShellEnvOutput, error) {
			return &ShellEnvOutput{Env: map[string]string{"MY_VAR": "value"}}, nil
		},
	}

	hookInput, _ := json.Marshal(ShellEnvInput{SessionID: "s1", Directory: "/tmp"})
	hookParams, _ := json.Marshal(HookParams{Name: "shell.env", Input: hookInput})
	req := JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "hook/invoke", Params: hookParams}

	var disposed atomic.Bool
	resp := dispatchHook(context.Background(), req, hooks, &disposed)

	if resp.Error != nil {
		t.Fatalf("expected no error, got: %s", resp.Error.Message)
	}
	var hr HookResult
	json.Unmarshal(resp.Result, &hr)
	var env ShellEnvOutput
	json.Unmarshal(hr.Output, &env)
	if env.Env["MY_VAR"] != "value" {
		t.Errorf("expected MY_VAR=value, got %v", env.Env)
	}
}

func TestDispatchHook_ToolExecBefore(t *testing.T) {
	hooks := &HookHandlers{
		ToolExecBefore: func(_ context.Context, input ToolExecBeforeInput) (*ToolExecBeforeOutput, error) {
			return &ToolExecBeforeOutput{AdditionalContext: []string{"before-ctx"}}, nil
		},
	}

	hookInput, _ := json.Marshal(ToolExecBeforeInput{SessionID: "s1", ToolName: "bash", ToolArgs: "{}"})
	hookParams, _ := json.Marshal(HookParams{Name: "tool.execute.before", Input: hookInput})
	req := JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "hook/invoke", Params: hookParams}

	var disposed atomic.Bool
	resp := dispatchHook(context.Background(), req, hooks, &disposed)

	if resp.Error != nil {
		t.Fatalf("expected no error, got: %s", resp.Error.Message)
	}
}

func TestDispatchHook_ToolExecAfter(t *testing.T) {
	hooks := &HookHandlers{
		ToolExecAfter: func(_ context.Context, input ToolExecAfterInput) (*ToolExecAfterOutput, error) {
			return &ToolExecAfterOutput{Output: "modified output"}, nil
		},
	}

	hookInput, _ := json.Marshal(ToolExecAfterInput{SessionID: "s1", ToolName: "bash", Output: "original", IsError: false})
	hookParams, _ := json.Marshal(HookParams{Name: "tool.execute.after", Input: hookInput})
	req := JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "hook/invoke", Params: hookParams}

	var disposed atomic.Bool
	resp := dispatchHook(context.Background(), req, hooks, &disposed)

	if resp.Error != nil {
		t.Fatalf("expected no error, got: %s", resp.Error.Message)
	}
}

func TestDispatchHook_Dispose(t *testing.T) {
	var disposeCalled bool
	hooks := &HookHandlers{
		Dispose: func(_ context.Context) error {
			disposeCalled = true
			return nil
		},
	}

	hookParams, _ := json.Marshal(HookParams{Name: "dispose"})
	req := JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "hook/invoke", Params: hookParams}

	var disposed atomic.Bool
	resp := dispatchHook(context.Background(), req, hooks, &disposed)

	if resp.Error != nil {
		t.Fatalf("expected no error, got: %s", resp.Error.Message)
	}
	if !disposeCalled {
		t.Error("expected Dispose handler to be called")
	}
	if !disposed.Load() {
		t.Error("expected disposed flag to be set to true")
	}
}

func TestDispatchHook_DisposeCalledOnlyOnce(t *testing.T) {
	var disposeCount int
	hooks := &HookHandlers{
		Dispose: func(_ context.Context) error {
			disposeCount++
			return nil
		},
	}

	hookParams, _ := json.Marshal(HookParams{Name: "dispose"})
	req := JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "hook/invoke", Params: hookParams}

	var disposed atomic.Bool
	// First call should run Dispose.
	dispatchHook(context.Background(), req, hooks, &disposed)
	// Second call should be a no-op.
	dispatchHook(context.Background(), req, hooks, &disposed)

	if disposeCount != 1 {
		t.Fatalf("expected Dispose called exactly once, got %d", disposeCount)
	}
}

func TestDispatchHook_NilHandler(t *testing.T) {
	hooks := &HookHandlers{} // all nil handlers

	tests := []struct {
		name     string
		hookName string
		input    any
	}{
		{"nil session.start handler", "session.start", SessionStartEvent{SessionID: "s1"}},
		{"nil session.end handler", "session.end", SessionEndEvent{SessionID: "s1"}},
		{"nil permission.ask handler", "permission.ask", PermissionInput{SessionID: "s1"}},
		{"nil shell.env handler", "shell.env", ShellEnvInput{SessionID: "s1"}},
		{"nil tool.execute.before handler", "tool.execute.before", ToolExecBeforeInput{SessionID: "s1"}},
		{"nil tool.execute.after handler", "tool.execute.after", ToolExecAfterInput{SessionID: "s1"}},
		{"nil dispose handler", "dispose", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hookInput json.RawMessage
			if tt.input != nil {
				hookInput, _ = json.Marshal(tt.input)
			}
			hookParams, _ := json.Marshal(HookParams{Name: tt.hookName, Input: hookInput})
			req := JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "hook/invoke", Params: hookParams}

			var disposed atomic.Bool
			resp := dispatchHook(context.Background(), req, hooks, &disposed)

			if resp.Error != nil {
				t.Fatalf("expected no error for nil handler, got: %s", resp.Error.Message)
			}
		})
	}
}

func TestDispatchHook_ErrorPropagation(t *testing.T) {
	hooks := &HookHandlers{
		SessionStart: func(_ context.Context, _ SessionStartEvent) (*SessionStartOutput, error) {
			return nil, errors.New("session start failed")
		},
		SessionEnd: func(_ context.Context, _ SessionEndEvent) error {
			return errors.New("session end failed")
		},
		Dispose: func(_ context.Context) error {
			return errors.New("dispose failed")
		},
	}

	tests := []struct {
		name     string
		hookName string
		input    any
		wantErr  string
	}{
		{"session.start error", "session.start", SessionStartEvent{SessionID: "s1"}, "session start failed"},
		{"session.end error", "session.end", SessionEndEvent{SessionID: "s1"}, "session end failed"},
		{"dispose error", "dispose", nil, "dispose failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hookInput json.RawMessage
			if tt.input != nil {
				hookInput, _ = json.Marshal(tt.input)
			}
			hookParams, _ := json.Marshal(HookParams{Name: tt.hookName, Input: hookInput})
			req := JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "hook/invoke", Params: hookParams}

			var disposed atomic.Bool
			resp := dispatchHook(context.Background(), req, hooks, &disposed)

			if resp.Error == nil {
				t.Fatal("expected error in response")
			}
			if resp.Error.Code != -32000 {
				t.Errorf("expected error code -32000, got %d", resp.Error.Code)
			}
			if !strings.Contains(resp.Error.Message, tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, resp.Error.Message)
			}
		})
	}
}

func TestDispatchHook_UnknownHookName(t *testing.T) {
	hooks := &HookHandlers{}

	hookParams, _ := json.Marshal(HookParams{Name: "unknown.hook"})
	req := JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "hook/invoke", Params: hookParams}

	var disposed atomic.Bool
	resp := dispatchHook(context.Background(), req, hooks, &disposed)

	// Unknown hook names return a success response with empty result (no-op),
	// not an error, since the switch has no default that produces an error.
	if resp.Error != nil {
		t.Fatalf("expected no error for unknown hook, got: %s", resp.Error.Message)
	}
}

func TestDispatchHook_InvalidParams(t *testing.T) {
	hooks := &HookHandlers{}

	req := JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "hook/invoke", Params: json.RawMessage(`invalid json`)}

	var disposed atomic.Bool
	resp := dispatchHook(context.Background(), req, hooks, &disposed)

	if resp.Error == nil {
		t.Fatal("expected error for invalid hook params")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("expected error code -32602, got %d", resp.Error.Code)
	}
}

func TestDispatchHook_InvalidHookInput(t *testing.T) {
	hooks := &HookHandlers{
		SessionStart: func(_ context.Context, event SessionStartEvent) (*SessionStartOutput, error) {
			return nil, nil
		},
	}

	hookParams, _ := json.Marshal(HookParams{Name: "session.start", Input: json.RawMessage(`not valid json`)})
	req := JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "hook/invoke", Params: hookParams}

	var disposed atomic.Bool
	resp := dispatchHook(context.Background(), req, hooks, &disposed)

	if resp.Error == nil {
		t.Fatal("expected error for invalid hook input")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("expected error code -32602, got %d", resp.Error.Code)
	}
}
