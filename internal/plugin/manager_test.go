package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestNewManager_Empty(t *testing.T) {
	m := NewManager(slog.Default())
	if m.plugins == nil {
		t.Fatal("expected non-nil plugins map")
	}
	if len(m.plugins) != 0 {
		t.Errorf("expected empty plugins map, got %d entries", len(m.plugins))
	}
}

func TestPlugins_EmptyInitially(t *testing.T) {
	m := NewManager(slog.Default())
	infos := m.Plugins()
	if len(infos) != 0 {
		t.Errorf("expected empty plugin list, got %d entries", len(infos))
	}
}

func TestUnloadPlugin_UnknownID(t *testing.T) {
	m := NewManager(slog.Default())
	err := m.UnloadPlugin("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown plugin id")
	}
}

func TestShutdown_EmptyManager(t *testing.T) {
	m := NewManager(slog.Default())
	// Should not panic or error on empty manager.
	m.Shutdown()
}

// TestHelperProcess is the mock plugin subprocess. It is not a real test.
// It reads JSON-RPC requests from stdin and writes responses to stdout.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_TEST_HELPER_PROCESS") != "1" {
		return
	}

	behavior := os.Getenv("GO_TEST_HELPER_BEHAVIOR")

	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	for {
		var req jsonrpcRequest
		if err := decoder.Decode(&req); err != nil {
			os.Exit(0)
		}

		switch behavior {
		case "crash_on_init":
			os.Exit(1)
		case "timeout_on_init":
			time.Sleep(30 * time.Second)
			os.Exit(1)
		}

		switch req.Method {
		case "initialize":
			helperHandleInitialize(encoder, req, behavior)
		case "hook/invoke":
			helperHandleHookInvoke(encoder, req, behavior)
		default:
			encoder.Encode(jsonrpcResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &jsonrpcError{Code: -32601, Message: "method not found"},
			})
		}
	}
}

func helperHandleInitialize(encoder *json.Encoder, req jsonrpcRequest, behavior string) {
	hooks := []string{"session.start", "session.end", "permission.ask", "shell.env", "tool.execute.before", "tool.execute.after"}
	if behavior == "no_hooks" {
		hooks = nil
	}
	if behavior == "session_hooks_only" {
		hooks = []string{"session.start", "session.end"}
	}
	tools := []toolManifest{}
	if behavior == "with_tools" {
		tools = []toolManifest{
			{Name: "greet", Description: "Greet someone", InputSchema: map[string]any{"type": "object"}},
		}
	}
	result := initializeResult{
		ID:    "test-plugin",
		Tools: tools,
		Hooks: hooks,
	}
	raw, _ := json.Marshal(result)
	encoder.Encode(jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  raw,
	})
}

func helperHandleHookInvoke(encoder *json.Encoder, req jsonrpcRequest, behavior string) {
	var params hookInvokeParams
	paramsBytes, _ := json.Marshal(req.Params)
	json.Unmarshal(paramsBytes, &params)

	var resultOutput json.RawMessage
	switch params.Name {
	case "session.start", "session.end", "tool.execute.before", "tool.execute.after":
		resultOutput = nil
	case "dispose":
		hr := hookResult{Output: nil}
		raw, _ := json.Marshal(hr)
		encoder.Encode(jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  raw,
		})
		os.Exit(0)
	case "permission.ask":
		if behavior == "deny_permission" {
			resultOutput, _ = json.Marshal(permissionResult{Allowed: false, Reason: "blocked by test"})
		} else {
			resultOutput, _ = json.Marshal(permissionResult{Allowed: true})
		}
	case "shell.env":
		var envResult shellEnvResult
		if behavior == "env_contrib_1" {
			envResult = shellEnvResult{Env: map[string]string{"PLUGIN_A": "a_value"}}
		} else if behavior == "env_contrib_2" {
			envResult = shellEnvResult{Env: map[string]string{"PLUGIN_B": "b_value"}}
		} else {
			envResult = shellEnvResult{Env: map[string]string{"TEST_VAR": "from_plugin"}}
		}
		resultOutput, _ = json.Marshal(envResult)
	default:
		encoder.Encode(jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &jsonrpcError{Code: -32601, Message: "unknown hook: " + params.Name},
		})
		return
	}

	hr := hookResult{Output: resultOutput}
	raw, _ := json.Marshal(hr)
	encoder.Encode(jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  raw,
	})
}

// helperCommandFactory returns a CommandFactory that spawns TestHelperProcess
// with the given behavior.
func helperCommandFactory(behavior string) CommandFactory {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestHelperProcess")
		cmd.Env = append(os.Environ(),
			"GO_TEST_HELPER_PROCESS=1",
			fmt.Sprintf("GO_TEST_HELPER_BEHAVIOR=%s", behavior),
		)
		return cmd
	}
}

// newTestManager creates a Manager with a test registry and helper process factory.
func newTestManager(behavior string) *Manager {
	mgr := NewManagerWithRegistry([]RegistryEntry{
		{Name: "test-plugin", Description: "Test plugin", Binary: "test-plugin"},
	})
	mgr.SetCommandFactory(helperCommandFactory(behavior))
	mgr.SetResolveFunc(func(name string) (string, error) {
		return "test-plugin-binary", nil
	})
	mgr.SetDirectory("/tmp/test")
	return mgr
}

func TestLoadPlugin_InitializeHandshake(t *testing.T) {
	mgr := newTestManager("")

	info, err := mgr.Load("test-plugin")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer mgr.Shutdown()

	if info.Name != "test-plugin" {
		t.Errorf("expected name test-plugin, got %s", info.Name)
	}
	if info.ID == "" {
		t.Error("expected non-empty ID")
	}

	list := mgr.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(list))
	}
	if list[0].Name != "test-plugin" {
		t.Errorf("expected test-plugin in list, got %s", list[0].Name)
	}
}

func TestLoadPlugin_NoHooks(t *testing.T) {
	mgr := newTestManager("no_hooks")

	info, err := mgr.Load("test-plugin")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer mgr.Shutdown()

	procs := mgr.pluginsWithHook("session.start")
	if len(procs) != 0 {
		t.Errorf("expected 0 procs with session.start hook, got %d", len(procs))
	}

	_ = info
}

func TestShutdown_SendsDispose(t *testing.T) {
	mgr := newTestManager("")

	_, err := mgr.Load("test-plugin")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Shutdown should send dispose and exit cleanly.
	mgr.Shutdown()

	list := mgr.List()
	if len(list) != 0 {
		t.Errorf("expected 0 plugins after shutdown, got %d", len(list))
	}
}

func TestUnload_StopsProcess(t *testing.T) {
	mgr := newTestManager("")

	info, err := mgr.Load("test-plugin")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if err := mgr.Unload(info.ID); err != nil {
		t.Fatalf("Unload: %v", err)
	}

	list := mgr.List()
	if len(list) != 0 {
		t.Errorf("expected 0 plugins after unload, got %d", len(list))
	}
}

func TestBroadcastHook_UnknownMethod(t *testing.T) {
	mgr := newTestManager("")

	info, err := mgr.Load("test-plugin")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer mgr.Shutdown()

	// Send a method the helper doesn't handle well (returns error).
	_, err = mgr.broadcastHook(info.ID, "unknown.method", nil)
	if err == nil {
		t.Fatal("expected error for unknown method")
	}
}

func TestBroadcastHook_DeadProcess(t *testing.T) {
	mgr := newTestManager("")

	info, err := mgr.Load("test-plugin")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Kill the process
	mgr.mu.RLock()
	proc := mgr.plugins[info.ID]
	mgr.mu.RUnlock()
	proc.dead.Store(true)

	_, err = mgr.broadcastHook(info.ID, "session.start", nil)
	if err == nil {
		t.Fatal("expected error for dead process")
	}
	mgr.Shutdown()
}

func TestPluginsWithHook(t *testing.T) {
	mgr := newTestManager("")

	_, err := mgr.Load("test-plugin")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer mgr.Shutdown()

	procs := mgr.pluginsWithHook("session.start")
	if len(procs) != 1 {
		t.Errorf("expected 1 proc with session.start, got %d", len(procs))
	}

	procs = mgr.pluginsWithHook("nonexistent.hook")
	if len(procs) != 0 {
		t.Errorf("expected 0 procs with nonexistent hook, got %d", len(procs))
	}
}

func TestSetDirectory(t *testing.T) {
	mgr := NewManagerWithRegistry(nil)
	mgr.SetDirectory("/test/dir")
	if mgr.directory != "/test/dir" {
		t.Errorf("expected /test/dir, got %s", mgr.directory)
	}
}

func TestShutdown_DisposeViaHookInvoke(t *testing.T) {
	// Verifies dispose goes through hook/invoke, not a raw "dispose" RPC method.
	// The test helper process handles dispose in the hook/invoke case and exits;
	// if it were sent as a raw method, the helper would return "method not found".
	mgr := newTestManager("")

	_, err := mgr.Load("test-plugin")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	mgr.Shutdown()

	list := mgr.List()
	if len(list) != 0 {
		t.Errorf("expected 0 plugins after shutdown, got %d", len(list))
	}
}

func TestLoadPlugin_StoresTools(t *testing.T) {
	mgr := newTestManager("with_tools")

	_, err := mgr.Load("test-plugin")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer mgr.Shutdown()

	tools := mgr.Tools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "greet" {
		t.Errorf("expected tool name 'greet', got %q", tools[0].Name)
	}
	if tools[0].Description != "Greet someone" {
		t.Errorf("expected description 'Greet someone', got %q", tools[0].Description)
	}
}

func TestTools_Empty(t *testing.T) {
	mgr := newTestManager("")

	_, err := mgr.Load("test-plugin")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer mgr.Shutdown()

	tools := mgr.Tools()
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
}
