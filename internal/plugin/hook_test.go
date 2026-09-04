package plugin

import (
	"log/slog"
	"testing"
)

func newTestManager() *Manager {
	return NewManager(slog.Default())
}

func TestDispatchSessionStart_NilManager(t *testing.T) {
	err := DispatchSessionStart(nil, SessionStartEvent{SessionID: "ses_1"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestDispatchSessionStart_EmptyManager(t *testing.T) {
	mgr := newTestManager()
	err := DispatchSessionStart(mgr, SessionStartEvent{SessionID: "ses_1"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestDispatchSessionEnd_EmptyManager(t *testing.T) {
	mgr := newTestManager()
	err := DispatchSessionEnd(mgr, SessionEndEvent{SessionID: "ses_1"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestDispatchPermissionAsk_EmptyManager(t *testing.T) {
	mgr := newTestManager()
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
	mgr := newTestManager()
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
