package plugin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestHandoffPlugin_ID(t *testing.T) {
	p := NewHandoffPlugin()
	if p.ID() != "handoff" {
		t.Errorf("ID() = %q, want %q", p.ID(), "handoff")
	}
}

func TestHandoffPlugin_ToolsReturnsSingleTool(t *testing.T) {
	p := NewHandoffPlugin()
	tools := p.Tools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "handoff_save" {
		t.Errorf("tool name = %q, want %q", tools[0].Name, "handoff_save")
	}
}

func TestLoadMostRecentHandoff_EmptyDirectoryReturnsNil(t *testing.T) {
	dir := t.TempDir()
	got, err := loadMostRecentHandoff(dir, "current-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for empty directory, got %+v", got)
	}
}

func TestLoadMostRecentHandoff_ValidJSONReturnsParsedState(t *testing.T) {
	dir := t.TempDir()
	state := handoffState{
		SessionID: "sess-abc",
		Timestamp: 1000,
		Goal:      "fix the bug",
		Decisions: []string{"use retry logic"},
	}
	data, _ := json.Marshal(state)
	if err := os.WriteFile(filepath.Join(dir, "sess-abc.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := loadMostRecentHandoff(dir, "different-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil state")
	}
	if got.SessionID != "sess-abc" {
		t.Errorf("SessionID = %q, want %q", got.SessionID, "sess-abc")
	}
	if got.Goal != "fix the bug" {
		t.Errorf("Goal = %q, want %q", got.Goal, "fix the bug")
	}
	if len(got.Decisions) != 1 || got.Decisions[0] != "use retry logic" {
		t.Errorf("Decisions = %v, want [use retry logic]", got.Decisions)
	}
}

func TestLoadMostRecentHandoff_SkipsCurrentSessionID(t *testing.T) {
	dir := t.TempDir()
	state := handoffState{
		SessionID: "current-session",
		Timestamp: 9999,
		Goal:      "should be skipped",
	}
	data, _ := json.Marshal(state)
	if err := os.WriteFile(filepath.Join(dir, "current-session.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := loadMostRecentHandoff(dir, "current-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil when only file matches current session, got %+v", got)
	}
}

func TestLoadMostRecentHandoff_InvalidJSONIsSkipped(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("not-json{{{"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := loadMostRecentHandoff(dir, "some-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil when only file has invalid JSON, got %+v", got)
	}
}

func TestLoadMostRecentHandoff_ReturnsMostRecentByTimestamp(t *testing.T) {
	dir := t.TempDir()

	older := handoffState{SessionID: "old", Timestamp: 100, Goal: "old goal"}
	newer := handoffState{SessionID: "new", Timestamp: 200, Goal: "new goal"}

	oldData, _ := json.Marshal(older)
	newData, _ := json.Marshal(newer)

	// Write older file first, newer second -- names don't matter, timestamp does.
	if err := os.WriteFile(filepath.Join(dir, "aaa-old.json"), oldData, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "zzz-new.json"), newData, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := loadMostRecentHandoff(dir, "different")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil state")
	}
	if got.Goal != "new goal" {
		t.Errorf("Goal = %q, want %q (most recent by timestamp)", got.Goal, "new goal")
	}
}

func TestLoadMostRecentHandoff_SkipsNonJSONFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := loadMostRecentHandoff(dir, "some-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil when no .json files exist, got %+v", got)
	}
}

func TestLoadMostRecentHandoff_SkipsDirectories(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "subdir.json"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := loadMostRecentHandoff(dir, "some-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil when .json entry is a directory, got %+v", got)
	}
}

func TestLoadMostRecentHandoff_NonexistentDirectoryReturnsError(t *testing.T) {
	_, err := loadMostRecentHandoff("/nonexistent/path/abc123", "sess")
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
}

func TestHandoffTool_ValidInputSavesState(t *testing.T) {
	p := NewHandoffPlugin()
	tool := p.Tools()[0]

	input := `{"goal":"deploy v2","decisions":["use blue-green"],"openTasks":["write tests"],"filesModified":["main.go"]}`
	result, err := tool.Execute(context.Background(), json.RawMessage(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Session context saved. Will be persisted when session ends." {
		t.Errorf("unexpected result: %q", result)
	}

	hp := p.(*handoffPlugin)
	hp.mu.Lock()
	defer hp.mu.Unlock()
	if hp.currentState.Goal != "deploy v2" {
		t.Errorf("Goal = %q, want %q", hp.currentState.Goal, "deploy v2")
	}
	if len(hp.currentState.Decisions) != 1 || hp.currentState.Decisions[0] != "use blue-green" {
		t.Errorf("Decisions = %v, want [use blue-green]", hp.currentState.Decisions)
	}
	if len(hp.currentState.OpenTasks) != 1 || hp.currentState.OpenTasks[0] != "write tests" {
		t.Errorf("OpenTasks = %v, want [write tests]", hp.currentState.OpenTasks)
	}
	if len(hp.currentState.FilesModified) != 1 || hp.currentState.FilesModified[0] != "main.go" {
		t.Errorf("FilesModified = %v, want [main.go]", hp.currentState.FilesModified)
	}
}

func TestHandoffTool_InvalidJSONReturnsError(t *testing.T) {
	p := NewHandoffPlugin()
	tool := p.Tools()[0]

	_, err := tool.Execute(context.Background(), json.RawMessage(`{invalid`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestHandoffTool_AccumulatesMultipleCalls(t *testing.T) {
	p := NewHandoffPlugin()
	tool := p.Tools()[0]

	_, err := tool.Execute(context.Background(), json.RawMessage(`{"decisions":["d1"],"openTasks":["t1"]}`))
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	_, err = tool.Execute(context.Background(), json.RawMessage(`{"decisions":["d2"],"openTasks":["t2"]}`))
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	hp := p.(*handoffPlugin)
	hp.mu.Lock()
	defer hp.mu.Unlock()
	if len(hp.currentState.Decisions) != 2 {
		t.Errorf("expected 2 decisions, got %d: %v", len(hp.currentState.Decisions), hp.currentState.Decisions)
	}
	if len(hp.currentState.OpenTasks) != 2 {
		t.Errorf("expected 2 open tasks, got %d: %v", len(hp.currentState.OpenTasks), hp.currentState.OpenTasks)
	}
}

func TestHandoffTool_PartialUpdatePreservesExistingFields(t *testing.T) {
	p := NewHandoffPlugin()
	tool := p.Tools()[0]

	// Set goal first.
	_, err := tool.Execute(context.Background(), json.RawMessage(`{"goal":"initial goal"}`))
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	// Add decisions without changing goal.
	_, err = tool.Execute(context.Background(), json.RawMessage(`{"decisions":["d1"]}`))
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	hp := p.(*handoffPlugin)
	hp.mu.Lock()
	defer hp.mu.Unlock()
	if hp.currentState.Goal != "initial goal" {
		t.Errorf("Goal = %q, want %q (should be preserved)", hp.currentState.Goal, "initial goal")
	}
	if len(hp.currentState.Decisions) != 1 || hp.currentState.Decisions[0] != "d1" {
		t.Errorf("Decisions = %v, want [d1]", hp.currentState.Decisions)
	}
}

func TestSessionStartHook_InitializesStateForNewSession(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HANDOFF_DIR", dir)

	p := NewHandoffPlugin()
	hooks := p.Hooks()

	if err := hooks.SessionStart(context.Background(), "new-sess"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hp := p.(*handoffPlugin)
	hp.mu.Lock()
	defer hp.mu.Unlock()
	if hp.currentState.SessionID != "new-sess" {
		t.Errorf("SessionID = %q, want %q", hp.currentState.SessionID, "new-sess")
	}
	if hp.currentState.Timestamp == 0 {
		t.Error("expected non-zero timestamp")
	}
	if hp.loadedState != nil {
		t.Error("expected nil loadedState with empty handoff directory")
	}
}

func TestSessionStartHook_LoadsPreviousHandoff(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HANDOFF_DIR", dir)

	// Seed a previous session's handoff file.
	prev := handoffState{
		SessionID: "old-sess",
		Timestamp: 500,
		Goal:      "previous goal",
		Decisions: []string{"prior decision"},
	}
	data, _ := json.Marshal(prev)
	if err := os.WriteFile(filepath.Join(dir, "old-sess.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	p := NewHandoffPlugin()
	hooks := p.Hooks()

	if err := hooks.SessionStart(context.Background(), "new-sess"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hp := p.(*handoffPlugin)
	hp.mu.Lock()
	defer hp.mu.Unlock()
	if hp.loadedState == nil {
		t.Fatal("expected loadedState to be loaded from previous session")
	}
	if hp.loadedState.Goal != "previous goal" {
		t.Errorf("loadedState.Goal = %q, want %q", hp.loadedState.Goal, "previous goal")
	}
}

func TestSessionEndHook_PersistsStateToDisk(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HANDOFF_DIR", dir)

	p := NewHandoffPlugin()
	hooks := p.Hooks()
	tool := p.Tools()[0]

	// Start session and save some state.
	if err := hooks.SessionStart(context.Background(), "test-sess"); err != nil {
		t.Fatal(err)
	}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"goal":"ship it","decisions":["d1"]}`)); err != nil {
		t.Fatal(err)
	}

	// End session -- should write file.
	if err := hooks.SessionEnd(context.Background(), "test-sess"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the file exists and has correct content.
	filePath := filepath.Join(dir, "test-sess.json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("expected handoff file at %s: %v", filePath, err)
	}

	var saved handoffState
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("invalid JSON in handoff file: %v", err)
	}
	if saved.SessionID != "test-sess" {
		t.Errorf("saved SessionID = %q, want %q", saved.SessionID, "test-sess")
	}
	if saved.Goal != "ship it" {
		t.Errorf("saved Goal = %q, want %q", saved.Goal, "ship it")
	}
	if len(saved.Decisions) != 1 || saved.Decisions[0] != "d1" {
		t.Errorf("saved Decisions = %v, want [d1]", saved.Decisions)
	}
	if saved.Timestamp == 0 {
		t.Error("expected non-zero timestamp in saved file")
	}
}

func TestSessionEndHook_CreatesDirectoryIfMissing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "handoff")
	t.Setenv("HANDOFF_DIR", dir)

	p := NewHandoffPlugin()
	hooks := p.Hooks()

	if err := hooks.SessionStart(context.Background(), "sess-1"); err != nil {
		t.Fatal(err)
	}
	if err := hooks.SessionEnd(context.Background(), "sess-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the nested directory was created.
	if _, err := os.Stat(filepath.Join(dir, "sess-1.json")); err != nil {
		t.Errorf("expected handoff file to exist: %v", err)
	}
}

func TestDisposeHook_ResetsState(t *testing.T) {
	p := NewHandoffPlugin()
	hooks := p.Hooks()
	tool := p.Tools()[0]

	// Save some state.
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"goal":"something","decisions":["d1"]}`)); err != nil {
		t.Fatal(err)
	}

	// Dispose should reset.
	if err := hooks.Dispose(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hp := p.(*handoffPlugin)
	hp.mu.Lock()
	defer hp.mu.Unlock()
	if hp.currentState.Goal != "" {
		t.Errorf("Goal = %q, want empty after dispose", hp.currentState.Goal)
	}
	if hp.loadedState != nil {
		t.Error("expected nil loadedState after dispose")
	}
	if len(hp.currentState.Decisions) != 0 {
		t.Errorf("Decisions should be empty after dispose, got %v", hp.currentState.Decisions)
	}
}

func TestHandoffDir_RespectsEnvVar(t *testing.T) {
	t.Setenv("HANDOFF_DIR", "/custom/handoff/path")
	if got := handoffDir(); got != "/custom/handoff/path" {
		t.Errorf("handoffDir() = %q, want %q", got, "/custom/handoff/path")
	}
}

func TestHandoffDir_DefaultsToHomeDir(t *testing.T) {
	t.Setenv("HANDOFF_DIR", "")
	got := handoffDir()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}
	expected := filepath.Join(home, ".tinycode", "handoff")
	if got != expected {
		t.Errorf("handoffDir() = %q, want %q", got, expected)
	}
}
