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
	_, err := mgr.Load("")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestManagerLoad_NotInRegistry(t *testing.T) {
	mgr := NewManagerWithRegistry(nil)
	_, err := mgr.Load("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown plugin")
	}
	if !errors.Is(err, ErrPluginNotFound) {
		t.Errorf("expected ErrPluginNotFound, got %v", err)
	}
}

func TestManagerUnload(t *testing.T) {
	mgr := NewManagerWithRegistry([]RegistryEntry{
		{Name: "test-plugin", Description: "Test", Package: "@tinycode/test"},
	})
	info, err := mgr.Load("test-plugin")
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
