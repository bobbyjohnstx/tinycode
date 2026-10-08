package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/permission"
	"github.com/bobbyjohnstx/tinycode/internal/tool"
	pkgplugin "github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

func busNewForTest(t *testing.T) *bus.Bus {
	t.Helper()
	b := bus.New()
	t.Cleanup(func() { b.Close() })
	return b
}

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
		var req pkgplugin.JSONRPCRequest
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

		// Simulate plugins that log to stdout before JSON-RPC responses.
		if behavior == "stdout_pollution" {
			fmt.Fprintln(os.Stdout, "DEBUG: not a json-rpc response")
		}

		switch req.Method {
		case "initialize":
			helperHandleInitialize(encoder, req, behavior)
		case "hook/invoke":
			helperHandleHookInvoke(encoder, req, behavior)
		case "tool/call":
			helperHandleToolCall(encoder, req, behavior)
		default:
			encoder.Encode(pkgplugin.JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &pkgplugin.JSONRPCError{Code: -32601, Message: "method not found"},
			})
		}
	}
}

func helperHandleInitialize(encoder *json.Encoder, req pkgplugin.JSONRPCRequest, behavior string) {
	hooks := []string{"session.start", "session.end", "permission.ask", "shell.env", "tool.execute.before", "tool.execute.after"}
	if behavior == "no_hooks" {
		hooks = nil
	}
	if behavior == "session_hooks_only" {
		hooks = []string{"session.start", "session.end"}
	}
	tools := []pkgplugin.ToolManifest{}
	if behavior == "with_tools" || behavior == "tool_error" || behavior == "stdout_pollution" || behavior == "slow_then_ok" {
		tools = []pkgplugin.ToolManifest{
			{Name: "greet", Description: "Greet someone", InputSchema: map[string]any{"type": "object"}},
		}
	}
	result := pkgplugin.InitializeResult{
		ID:    "test-plugin",
		Tools: tools,
		Hooks: hooks,
	}
	raw, _ := json.Marshal(result)
	encoder.Encode(pkgplugin.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  raw,
	})
}

func helperHandleToolCall(encoder *json.Encoder, req pkgplugin.JSONRPCRequest, behavior string) {
	var params pkgplugin.ToolCallParams
	_ = json.Unmarshal(req.Params, &params)

	if behavior == "slow_then_ok" {
		// First call is slow (exceeds short test timeout); later calls are fast.
		flagPath := os.Getenv("GO_TEST_HELPER_SLOW_FLAG")
		if flagPath != "" {
			if _, err := os.Stat(flagPath); os.IsNotExist(err) {
				_ = os.WriteFile(flagPath, []byte("1"), 0o600)
				time.Sleep(500 * time.Millisecond)
			}
		} else {
			time.Sleep(500 * time.Millisecond)
		}
	}

	content := "ok"
	isError := false
	switch {
	case behavior == "tool_error":
		content = "tool failed"
		isError = true
	case params.Name == "greet":
		var args map[string]any
		_ = json.Unmarshal(params.Args, &args)
		if name, ok := args["name"].(string); ok {
			content = "Hello, " + name
		} else {
			content = "Hello"
		}
	}

	raw, _ := json.Marshal(pkgplugin.ToolCallResult{Content: content, IsError: isError})
	_ = encoder.Encode(pkgplugin.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  raw,
	})
}

func helperHandleHookInvoke(encoder *json.Encoder, req pkgplugin.JSONRPCRequest, behavior string) {
	var params pkgplugin.HookParams
	json.Unmarshal(req.Params, &params)

	var resultOutput json.RawMessage
	switch params.Name {
	case "tool.execute.before":
		if behavior == "redact_args" {
			resultOutput, _ = json.Marshal(map[string]any{
				"toolArgs": "token=[REDACTED]",
			})
			break
		}
		if behavior == "abort_before" {
			_ = encoder.Encode(pkgplugin.JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &pkgplugin.JSONRPCError{Code: -32000, Message: "blocked by plugin"},
			})
			return
		}
		if behavior == "with_context" {
			resultOutput, _ = json.Marshal(map[string]any{
				"additionalContext": []string{"ctx from plugin"},
			})
		} else {
			resultOutput = nil
		}
	case "session.start", "session.end", "tool.execute.after":
		if behavior == "with_context" {
			resultOutput, _ = json.Marshal(map[string]any{
				"additionalContext": []string{"ctx from plugin"},
			})
		} else {
			resultOutput = nil
		}
	case "dispose":
		hr := pkgplugin.HookResult{Output: nil}
		raw, _ := json.Marshal(hr)
		encoder.Encode(pkgplugin.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  raw,
		})
		os.Exit(0)
	case "permission.ask":
		switch behavior {
		case "deny_permission":
			resultOutput, _ = json.Marshal(permissionResult{Allowed: false, Reason: "blocked by test"})
		case "bad_permission":
			resultOutput = json.RawMessage(`"nope"`)
		default:
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
		encoder.Encode(pkgplugin.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &pkgplugin.JSONRPCError{Code: -32601, Message: "unknown hook: " + params.Name},
		})
		return
	}

	hr := pkgplugin.HookResult{Output: resultOutput}
	raw, _ := json.Marshal(hr)
	encoder.Encode(pkgplugin.JSONRPCResponse{
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

	info, err := mgr.Load("test-plugin", nil)
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

func TestLoadPlugin_NotInRegistry_ResolveSucceeds(t *testing.T) {
	mgr := NewManagerWithRegistry(nil) // empty curated registry
	mgr.SetCommandFactory(helperCommandFactory(""))
	mgr.SetResolveFunc(func(name string) (string, error) {
		return "greet-plugin-binary", nil
	})
	mgr.SetDirectory("/tmp/test")

	info, err := mgr.Load("greet", nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer mgr.Shutdown()

	if info.Name != "greet" {
		t.Errorf("expected name greet, got %s", info.Name)
	}
	if len(mgr.List()) != 1 {
		t.Fatalf("expected 1 loaded plugin, got %d", len(mgr.List()))
	}
}

func TestLoadPlugin_NotInRegistry_ResolveFails(t *testing.T) {
	mgr := NewManagerWithRegistry(nil)
	mgr.SetResolveFunc(func(name string) (string, error) {
		return "", fmt.Errorf("not found")
	})

	_, err := mgr.Load("greet", nil)
	if err == nil {
		t.Fatal("expected error when resolve fails and name not in registry")
	}
	if !errors.Is(err, ErrPluginNotFound) {
		t.Errorf("expected ErrPluginNotFound, got %v", err)
	}
}

func TestLoadPlugin_BuiltinSoftSkip(t *testing.T) {
	mgr := NewManagerWithRegistry(nil)
	mgr.SetResolveFunc(func(name string) (string, error) {
		return "", fmt.Errorf("not found")
	})

	info, err := mgr.Load("notify", nil)
	if !errors.Is(err, ErrPluginSkipped) {
		t.Fatalf("expected ErrPluginSkipped, got %v", err)
	}
	if info != nil {
		t.Errorf("expected nil PluginInfo on soft-skip, got %+v", info)
	}
	if len(mgr.List()) != 0 {
		t.Errorf("expected 0 loaded plugins, got %d", len(mgr.List()))
	}
}

func TestLoadPlugin_AlreadyLoaded(t *testing.T) {
	mgr := newTestManager("")
	defer mgr.Shutdown()

	if _, err := mgr.Load("test-plugin", nil); err != nil {
		t.Fatalf("first Load: %v", err)
	}
	_, err := mgr.Load("test-plugin", nil)
	if !errors.Is(err, ErrAlreadyLoaded) {
		t.Fatalf("expected ErrAlreadyLoaded, got %v", err)
	}
}

func TestLoad_PluginToolPermissionAsk(t *testing.T) {
	mgr := newTestManager("with_tools")
	b := busNewForTest(t)
	permSvc := permission.NewService(b)

	asked := make(chan permission.Request, 1)
	sub := b.Subscribe("permission.asked")
	go func() {
		for evt := range sub.C {
			if req, ok := evt.Properties.(permission.Request); ok {
				asked <- req
				_ = permSvc.RespondToAsk(permission.ReplyInput{
					RequestID: req.ID,
					Reply:     permission.ReplyOnce,
				})
			}
		}
	}()

	reg := tool.NewRegistry(&tool.Context{
		Directory: t.TempDir(),
		Perms:     permSvc,
		Bus:       b,
	})
	mgr.SetToolRegistry(reg)

	if _, err := mgr.Load("test-plugin", nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer mgr.Shutdown()

	const toolID = "plugin__test-plugin__greet"
	def := reg.Get(toolID)
	if def == nil {
		t.Fatal("expected tool registered")
	}
	if def.Permission != "plugin" {
		t.Fatalf("expected Permission=plugin, got %q", def.Permission)
	}

	out, isErr, err := reg.Execute(context.Background(), toolID, json.RawMessage(`{"name":"world"}`), "sess1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if isErr {
		t.Fatalf("Execute IsError=true, output=%q", out)
	}

	select {
	case req := <-asked:
		if req.Permission != "plugin" {
			t.Errorf("expected permission plugin, got %s", req.Permission)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for permission.asked")
	}
}

func TestCallTool_TimeoutDoesNotKillProcess(t *testing.T) {
	oldTimeout := toolCallTimeout
	toolCallTimeout = 100 * time.Millisecond
	defer func() { toolCallTimeout = oldTimeout }()

	flagPath := t.TempDir() + "/slow_flag"
	mgr := NewManagerWithRegistry([]RegistryEntry{
		{Name: "test-plugin", Description: "Test plugin", Binary: "test-plugin"},
	})
	mgr.SetCommandFactory(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestHelperProcess")
		cmd.Env = append(os.Environ(),
			"GO_TEST_HELPER_PROCESS=1",
			"GO_TEST_HELPER_BEHAVIOR=slow_then_ok",
			"GO_TEST_HELPER_SLOW_FLAG="+flagPath,
		)
		return cmd
	})
	mgr.SetResolveFunc(func(name string) (string, error) {
		return "test-plugin-binary", nil
	})
	mgr.SetDirectory("/tmp/test")

	info, err := mgr.Load("test-plugin", nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer mgr.Shutdown()

	_, _, err = mgr.CallTool(info.ID, "greet", json.RawMessage(`{}`), "sess1")
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("expected timeout error, got %v", err)
	}

	// Wait for the late response to drain so the next RPC can proceed.
	time.Sleep(600 * time.Millisecond)

	content, _, err := mgr.CallTool(info.ID, "greet", json.RawMessage(`{"name":"world"}`), "sess1")
	if err != nil {
		t.Fatalf("second CallTool after timeout: %v", err)
	}
	if content != "Hello, world" {
		t.Errorf("expected Hello, world, got %q", content)
	}
	if len(mgr.List()) != 1 {
		t.Fatalf("expected plugin still loaded, got %d", len(mgr.List()))
	}
}

func TestLoadPlugin_NoHooks(t *testing.T) {
	mgr := newTestManager("no_hooks")

	info, err := mgr.Load("test-plugin", nil)
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

	_, err := mgr.Load("test-plugin", nil)
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

	info, err := mgr.Load("test-plugin", nil)
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

	info, err := mgr.Load("test-plugin", nil)
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

	info, err := mgr.Load("test-plugin", nil)
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

	_, err := mgr.Load("test-plugin", nil)
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

	_, err := mgr.Load("test-plugin", nil)
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

	_, err := mgr.Load("test-plugin", nil)
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

	_, err := mgr.Load("test-plugin", nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer mgr.Shutdown()

	tools := mgr.Tools()
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
}

func TestLoad_RegistersToolsInRegistry(t *testing.T) {
	mgr := newTestManager("with_tools")
	reg := tool.NewRegistry(&tool.Context{Directory: t.TempDir()})
	mgr.SetToolRegistry(reg)

	info, err := mgr.Load("test-plugin", nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer mgr.Shutdown()

	const toolID = "plugin__test-plugin__greet"
	defs := reg.ToolDefs(nil)
	found := false
	for _, d := range defs {
		if d.Function.Name == toolID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("ToolDefs missing %s; got %#v", toolID, defs)
	}

	out, isErr, err := reg.Execute(context.Background(), toolID, json.RawMessage(`{"name":"world"}`), "sess1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if isErr {
		t.Fatalf("Execute IsError=true, output=%q", out)
	}
	if out != "Hello, world" {
		t.Errorf("expected Hello, world, got %q", out)
	}

	if err := mgr.Unload(info.ID); err != nil {
		t.Fatalf("Unload: %v", err)
	}
	if reg.Get(toolID) != nil {
		t.Fatal("expected tool unregistered after Unload")
	}
}

func TestCallTool_ToolLevelError(t *testing.T) {
	mgr := newTestManager("tool_error")

	info, err := mgr.Load("test-plugin", nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer mgr.Shutdown()

	content, isError, err := mgr.CallTool(info.ID, "greet", json.RawMessage(`{}`), "sess1")
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !isError {
		t.Fatal("expected isError=true")
	}
	if content != "tool failed" {
		t.Errorf("expected tool failed, got %q", content)
	}
}

func TestLoad_StdoutPollution_StillSucceeds(t *testing.T) {
	mgr := newTestManager("stdout_pollution")
	reg := tool.NewRegistry(&tool.Context{Directory: t.TempDir()})
	mgr.SetToolRegistry(reg)

	info, err := mgr.Load("test-plugin", nil)
	if err != nil {
		t.Fatalf("Load with stdout pollution: %v", err)
	}
	defer mgr.Shutdown()

	content, isErr, err := mgr.CallTool(info.ID, "greet", json.RawMessage(`{"name":"world"}`), "sess1")
	if err != nil {
		t.Fatalf("CallTool with stdout pollution: %v", err)
	}
	if isErr {
		t.Fatalf("CallTool IsError=true, output=%q", content)
	}
	if content != "Hello, world" {
		t.Errorf("expected Hello, world, got %q", content)
	}
}

func TestHealthMonitor_RemovesDeadPlugin(t *testing.T) {
	mgr := newTestManager("with_tools")
	reg := tool.NewRegistry(&tool.Context{Directory: t.TempDir()})
	mgr.SetToolRegistry(reg)

	info, err := mgr.Load("test-plugin", nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	mgr.mu.RLock()
	proc := mgr.plugins[info.ID]
	mgr.mu.RUnlock()
	if proc == nil {
		t.Fatal("expected plugin in map after Load")
	}

	// Kill the subprocess; health monitor should remove it from the map
	// and unregister tools without going through Unload.
	if err := proc.cmd.Process.Kill(); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	select {
	case <-proc.done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for process exit")
	}

	// Allow health monitor to finish removeProcess.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(mgr.List()) == 0 && reg.Get("plugin__test-plugin__greet") == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(mgr.List()) != 0 {
		t.Errorf("expected plugin removed from map, still have %d", len(mgr.List()))
	}
	if reg.Get("plugin__test-plugin__greet") != nil {
		t.Error("expected tool unregistered after process death")
	}
}
