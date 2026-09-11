package plugin

import (
	"errors"
	"testing"
)

func TestDispatchSessionStart_NilManager(t *testing.T) {
	err := DispatchSessionStart(nil, SessionStartEvent{SessionID: "ses_1"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestDispatchSessionStart_EmptyManager(t *testing.T) {
	mgr := NewManagerWithRegistry(nil)
	err := DispatchSessionStart(mgr, SessionStartEvent{SessionID: "ses_1"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestDispatchSessionEnd_EmptyManager(t *testing.T) {
	mgr := NewManagerWithRegistry(nil)
	err := DispatchSessionEnd(mgr, SessionEndEvent{SessionID: "ses_1"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestDispatchPermissionAsk_EmptyManager(t *testing.T) {
	mgr := NewManagerWithRegistry(nil)
	out, err := DispatchPermissionAsk(mgr, PermissionInput{
		SessionID: "ses_1",
		ToolName:  "bash",
		ToolArgs:  `{"command":"ls"}`,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if out != nil {
		t.Fatalf("expected nil output for empty manager, got %+v", out)
	}
}

func TestDispatchShellEnv_EmptyManager(t *testing.T) {
	mgr := NewManagerWithRegistry(nil)
	out, err := DispatchShellEnv(mgr, ShellEnvInput{
		SessionID: "ses_1",
		Env:       map[string]string{"PATH": "/usr/bin"},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if out != nil {
		t.Fatalf("expected nil output for empty manager, got %+v", out)
	}
}

func TestManagerLoad_EmptyName(t *testing.T) {
	mgr := NewManagerWithRegistry(nil)
	_, err := mgr.Load("", nil)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestManagerLoad_NotInRegistry(t *testing.T) {
	mgr := NewManagerWithRegistry(nil)
	_, err := mgr.Load("nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for unknown plugin")
	}
	if !errors.Is(err, ErrPluginNotFound) {
		t.Errorf("expected ErrPluginNotFound, got %v", err)
	}
}

func TestManagerUnload(t *testing.T) {
	mgr := newTestManager("")
	info, err := mgr.Load("test-plugin", nil)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if err := mgr.Unload(info.ID); err != nil {
		t.Fatalf("unload: %v", err)
	}

	list := mgr.List()
	if len(list) != 0 {
		t.Errorf("expected 0 plugins after unload, got %d", len(list))
	}
}

func TestManagerUnload_NotFound(t *testing.T) {
	mgr := NewManagerWithRegistry(nil)
	err := mgr.Unload("plg_nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown plugin ID")
	}
}

func TestManagerList_Empty(t *testing.T) {
	mgr := NewManagerWithRegistry(nil)
	list := mgr.List()
	if len(list) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(list))
	}
}

func TestManagerRegistry_Empty(t *testing.T) {
	mgr := NewManagerWithRegistry(nil)
	reg := mgr.Registry()
	if len(reg) != 0 {
		t.Errorf("expected 0 entries, got %d", len(reg))
	}
}

func TestManagerRegistry_WithEntries(t *testing.T) {
	entries := []RegistryEntry{
		{Name: "foo", Description: "Foo plugin", Package: "@tinycode/foo"},
		{Name: "bar", Description: "Bar plugin", Package: "@tinycode/bar"},
	}
	mgr := NewManagerWithRegistry(entries)
	reg := mgr.Registry()
	if len(reg) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(reg))
	}
	if reg[0].Name != "foo" {
		t.Errorf("expected foo, got %s", reg[0].Name)
	}
}

func TestDispatchSessionStart_WithPlugin(t *testing.T) {
	mgr := newTestManager("")

	_, err := mgr.Load("test-plugin", nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer mgr.Shutdown()

	err = DispatchSessionStart(mgr, SessionStartEvent{SessionID: "ses_1"})
	if err != nil {
		t.Fatalf("DispatchSessionStart: %v", err)
	}
}

func TestDispatchSessionEnd_WithPlugin(t *testing.T) {
	mgr := newTestManager("")

	_, err := mgr.Load("test-plugin", nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer mgr.Shutdown()

	err = DispatchSessionEnd(mgr, SessionEndEvent{SessionID: "ses_1"})
	if err != nil {
		t.Fatalf("DispatchSessionEnd: %v", err)
	}
}

func TestDispatchSessionStart_NoMatchingHook(t *testing.T) {
	mgr := newTestManager("no_hooks")

	_, err := mgr.Load("test-plugin", nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer mgr.Shutdown()

	err = DispatchSessionStart(mgr, SessionStartEvent{SessionID: "ses_1"})
	if err != nil {
		t.Fatalf("DispatchSessionStart: %v", err)
	}
}

func TestDispatchPermissionAsk_Allowed(t *testing.T) {
	mgr := newTestManager("")

	_, err := mgr.Load("test-plugin", nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer mgr.Shutdown()

	out, err := DispatchPermissionAsk(mgr, PermissionInput{
		SessionID: "ses_1",
		ToolName:  "bash",
		ToolArgs:  `{"command":"ls"}`,
	})
	if err != nil {
		t.Fatalf("DispatchPermissionAsk: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil output")
	}
	if !out.Allowed {
		t.Errorf("expected allowed=true, got false (reason: %s)", out.Reason)
	}
}

func TestDispatchPermissionAsk_Denied(t *testing.T) {
	mgr := newTestManager("deny_permission")

	_, err := mgr.Load("test-plugin", nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer mgr.Shutdown()

	out, err := DispatchPermissionAsk(mgr, PermissionInput{
		SessionID: "ses_1",
		ToolName:  "bash",
		ToolArgs:  `{"command":"rm -rf /"}`,
	})
	if err != nil {
		t.Fatalf("DispatchPermissionAsk: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil output")
	}
	if out.Allowed {
		t.Error("expected allowed=false")
	}
	if out.Reason != "blocked by test" {
		t.Errorf("expected reason 'blocked by test', got %q", out.Reason)
	}
}

func TestDispatchPermissionAsk_NoHook(t *testing.T) {
	mgr := newTestManager("session_hooks_only")

	_, err := mgr.Load("test-plugin", nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer mgr.Shutdown()

	out, err := DispatchPermissionAsk(mgr, PermissionInput{
		SessionID: "ses_1",
		ToolName:  "bash",
	})
	if err != nil {
		t.Fatalf("DispatchPermissionAsk: %v", err)
	}
	if out != nil {
		t.Fatalf("expected nil output when no plugin has permission.ask hook, got %+v", out)
	}
}

func TestDispatchShellEnv_MergesEnv(t *testing.T) {
	mgr := newTestManager("")

	_, err := mgr.Load("test-plugin", nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer mgr.Shutdown()

	out, err := DispatchShellEnv(mgr, ShellEnvInput{
		SessionID: "ses_1",
		Env:       map[string]string{"EXISTING": "keep"},
	})
	if err != nil {
		t.Fatalf("DispatchShellEnv: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil output")
	}
	if out.Env["EXISTING"] != "keep" {
		t.Errorf("expected EXISTING=keep, got %s", out.Env["EXISTING"])
	}
	if out.Env["TEST_VAR"] != "from_plugin" {
		t.Errorf("expected TEST_VAR=from_plugin, got %s", out.Env["TEST_VAR"])
	}
}

func TestDispatchToolExecBefore_WithPlugin(t *testing.T) {
	mgr := newTestManager("")

	_, err := mgr.Load("test-plugin", nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer mgr.Shutdown()

	err = DispatchToolExecBefore(mgr, ToolExecBeforeEvent{
		SessionID: "ses_1",
		ToolName:  "bash",
		ToolArgs:  `{"command":"ls"}`,
	})
	if err != nil {
		t.Fatalf("DispatchToolExecBefore: %v", err)
	}
}

func TestDispatchToolExecAfter_WithPlugin(t *testing.T) {
	mgr := newTestManager("")

	_, err := mgr.Load("test-plugin", nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer mgr.Shutdown()

	_, err = DispatchToolExecAfter(mgr, ToolExecAfterEvent{
		SessionID: "ses_1",
		ToolName:  "bash",
		Output:    "file1\nfile2",
		IsError:   false,
	})
	if err != nil {
		t.Fatalf("DispatchToolExecAfter: %v", err)
	}
}

func TestDispatchToolExecBefore_NilManager(t *testing.T) {
	err := DispatchToolExecBefore(nil, ToolExecBeforeEvent{SessionID: "ses_1", ToolName: "bash"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestDispatchToolExecAfter_NilManager(t *testing.T) {
	_, err := DispatchToolExecAfter(nil, ToolExecAfterEvent{SessionID: "ses_1", ToolName: "bash"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestDispatchToolExecBefore_NoHook(t *testing.T) {
	mgr := newTestManager("session_hooks_only")

	_, err := mgr.Load("test-plugin", nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer mgr.Shutdown()

	err = DispatchToolExecBefore(mgr, ToolExecBeforeEvent{
		SessionID: "ses_1",
		ToolName:  "bash",
		ToolArgs:  `{"command":"ls"}`,
	})
	if err != nil {
		t.Fatalf("DispatchToolExecBefore: %v", err)
	}
}
