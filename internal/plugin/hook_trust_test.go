package plugin

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/config"
)

func TestHookHash_Deterministic(t *testing.T) {
	h1 := HookHash("echo hello", "session.start", "/tmp/proj")
	h2 := HookHash("echo hello", "session.start", "/tmp/proj")
	if h1 != h2 {
		t.Errorf("same inputs produced different hashes: %s vs %s", h1, h2)
	}
	if len(h1) != 64 {
		t.Errorf("expected 64-char hex SHA-256, got %d chars", len(h1))
	}
}

func TestHookHash_DifferentInputs(t *testing.T) {
	base := HookHash("echo hello", "session.start", "/tmp/proj")
	diffCmd := HookHash("echo world", "session.start", "/tmp/proj")
	diffEvent := HookHash("echo hello", "session.end", "/tmp/proj")
	diffDir := HookHash("echo hello", "session.start", "/tmp/other")

	if base == diffCmd {
		t.Error("different commands should produce different hashes")
	}
	if base == diffEvent {
		t.Error("different events should produce different hashes")
	}
	if base == diffDir {
		t.Error("different directories should produce different hashes")
	}
}

func TestHookTrustStore_UntrustedByDefault(t *testing.T) {
	store := NewHookTrustStore(t.TempDir())

	if store.IsTrusted("echo hi", "session.start", "/proj") {
		t.Error("new hook should not be trusted")
	}
	if store.Status("echo hi", "session.start", "/proj") != "untrusted" {
		t.Errorf("expected untrusted status, got %s", store.Status("echo hi", "session.start", "/proj"))
	}
}

func TestHookTrustStore_TrustAndCheck(t *testing.T) {
	store := NewHookTrustStore(t.TempDir())

	if err := store.Trust("echo hi", "session.start", "/proj"); err != nil {
		t.Fatalf("Trust failed: %v", err)
	}
	if !store.IsTrusted("echo hi", "session.start", "/proj") {
		t.Error("hook should be trusted after Trust()")
	}
	if store.Status("echo hi", "session.start", "/proj") != "trusted" {
		t.Errorf("expected trusted status, got %s", store.Status("echo hi", "session.start", "/proj"))
	}
}

func TestHookTrustStore_ChangedCommandRequiresRetrust(t *testing.T) {
	store := NewHookTrustStore(t.TempDir())

	if err := store.Trust("echo v1", "session.start", "/proj"); err != nil {
		t.Fatalf("Trust failed: %v", err)
	}
	if !store.IsTrusted("echo v1", "session.start", "/proj") {
		t.Error("original command should be trusted")
	}
	if store.IsTrusted("echo v2", "session.start", "/proj") {
		t.Error("changed command should NOT be trusted")
	}
}

func TestHookTrustStore_Disable(t *testing.T) {
	store := NewHookTrustStore(t.TempDir())

	if err := store.Disable("echo hi", "session.start", "/proj"); err != nil {
		t.Fatalf("Disable failed: %v", err)
	}
	if !store.IsDisabled("echo hi", "session.start", "/proj") {
		t.Error("hook should be disabled after Disable()")
	}
	if store.Status("echo hi", "session.start", "/proj") != "disabled" {
		t.Errorf("expected disabled status, got %s", store.Status("echo hi", "session.start", "/proj"))
	}
}

func TestHookTrustStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	store1 := NewHookTrustStore(dir)

	if err := store1.Trust("echo hi", "session.start", "/proj"); err != nil {
		t.Fatalf("Trust failed: %v", err)
	}

	// Load a new store from the same directory — should see persisted trust.
	store2 := NewHookTrustStore(dir)
	if !store2.IsTrusted("echo hi", "session.start", "/proj") {
		t.Error("trust should persist across store instances")
	}
}

func TestHookTrustStore_PersistenceFile(t *testing.T) {
	dir := t.TempDir()
	store := NewHookTrustStore(dir)

	if err := store.Trust("echo hi", "session.start", "/proj"); err != nil {
		t.Fatalf("Trust failed: %v", err)
	}

	path := filepath.Join(dir, "hook-trust.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("trust file should exist: %v", err)
	}
}

func TestHookTrustStore_WarnOnce(t *testing.T) {
	store := NewHookTrustStore(t.TempDir())
	logger := slog.Default()

	// Should not panic — first call logs, second is silent.
	store.WarnOnce("echo hi", "session.start", "/proj", logger)
	store.WarnOnce("echo hi", "session.start", "/proj", logger)
	store.WarnOnce("echo hi", "session.start", "/proj", nil) // nil logger
}

func TestShellHookRunner_WithTrust_UntrustedStillRuns(t *testing.T) {
	hooks := map[string][]config.HookConfig{
		"session.start": {{Command: "echo untrusted-but-runs"}},
	}
	store := NewHookTrustStore(t.TempDir())
	r := NewShellHookRunner(hooks, slog.Default())
	r.WithTrust(store, "/proj")

	// Untrusted hooks run until the approval prompt UI is implemented.
	_, err := r.RunBefore("session.start", nil)
	if err != nil {
		t.Fatalf("untrusted hook should still run: %v", err)
	}
}

func TestShellHookRunner_WithTrust_TrustedRuns(t *testing.T) {
	hooks := map[string][]config.HookConfig{
		"session.start": {{Command: "echo trusted-output"}},
	}
	store := NewHookTrustStore(t.TempDir())
	if err := store.Trust("echo trusted-output", "session.start", "/proj"); err != nil {
		t.Fatalf("Trust failed: %v", err)
	}
	r := NewShellHookRunner(hooks, slog.Default())
	r.WithTrust(store, "/proj")

	_, err := r.RunBefore("session.start", nil)
	if err != nil {
		t.Fatalf("trusted hook should run without error: %v", err)
	}
}

func TestShellHookRunner_WithTrust_DisabledSkips(t *testing.T) {
	hooks := map[string][]config.HookConfig{
		"session.start": {{Command: "echo disabled"}},
	}
	store := NewHookTrustStore(t.TempDir())
	if err := store.Disable("echo disabled", "session.start", "/proj"); err != nil {
		t.Fatalf("Disable failed: %v", err)
	}
	r := NewShellHookRunner(hooks, slog.Default())
	r.WithTrust(store, "/proj")

	ctx, err := r.RunBefore("session.start", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ctx) > 0 {
		t.Error("disabled hook should not produce output")
	}
}

func TestShellHookRunner_WithoutTrust_AlwaysRuns(t *testing.T) {
	// No trust store means user-config hooks — always trusted.
	hooks := map[string][]config.HookConfig{
		"session.start": {{Command: "echo always-runs"}},
	}
	r := NewShellHookRunner(hooks, slog.Default())

	_, err := r.RunBefore("session.start", nil)
	if err != nil {
		t.Fatalf("hook without trust store should run: %v", err)
	}
}

func TestShellHookRunner_WithTrust_NilRunner(t *testing.T) {
	var r *ShellHookRunner
	got := r.WithTrust(nil, "/proj")
	if got != nil {
		t.Error("WithTrust on nil runner should return nil")
	}
}
