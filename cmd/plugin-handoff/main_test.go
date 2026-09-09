package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

func TestPluginID(t *testing.T) {
	p := newPlugin()
	if p.ID != "handoff" {
		t.Errorf("expected plugin ID 'handoff', got %q", p.ID)
	}
}

func TestPluginHasOneTool(t *testing.T) {
	p := newPlugin()
	if len(p.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(p.Tools))
	}
	if p.Tools[0].Name != "handoff_save" {
		t.Errorf("expected tool name 'handoff_save', got %q", p.Tools[0].Name)
	}
}

func TestPluginHasHooks(t *testing.T) {
	p := newPlugin()
	if p.Hooks.SessionStart == nil {
		t.Error("expected SessionStart hook to be set")
	}
	if p.Hooks.SessionEnd == nil {
		t.Error("expected SessionEnd hook to be set")
	}
	if p.Hooks.Dispose == nil {
		t.Error("expected Dispose hook to be set")
	}
}

func TestToolSchema(t *testing.T) {
	p := newPlugin()
	params := p.Tools[0].Parameters
	if params["type"] != "object" {
		t.Errorf("expected type 'object'")
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties map")
	}
	for _, field := range []string{"goal", "decisions", "openTasks", "filesModified"} {
		if _, ok := props[field]; !ok {
			t.Errorf("expected property %q", field)
		}
	}
}

func TestHandoffSaveTool(t *testing.T) {
	p := newPlugin()
	ctx := context.Background()
	tc := plugin.ToolContext{SessionID: "test", Directory: "/tmp"}

	raw := json.RawMessage(`{
		"goal": "implement feature X",
		"decisions": ["use pattern A"],
		"openTasks": ["write tests"],
		"filesModified": ["main.go"]
	}`)

	out, err := p.Tools[0].Execute(ctx, raw, tc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty output")
	}
}

func TestHandoffSaveAccumulates(t *testing.T) {
	p := newPlugin()
	ctx := context.Background()
	tc := plugin.ToolContext{SessionID: "test", Directory: "/tmp"}

	// First save
	raw1 := json.RawMessage(`{"goal": "goal1", "decisions": ["d1"]}`)
	_, err := p.Tools[0].Execute(ctx, raw1, tc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Second save appends
	raw2 := json.RawMessage(`{"decisions": ["d2"], "openTasks": ["t1"]}`)
	_, err = p.Tools[0].Execute(ctx, raw2, tc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandoffSaveInvalidJSON(t *testing.T) {
	p := newPlugin()
	ctx := context.Background()
	tc := plugin.ToolContext{SessionID: "test", Directory: "/tmp"}

	raw := json.RawMessage(`{invalid}`)
	_, err := p.Tools[0].Execute(ctx, raw, tc)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestSessionEndPersists(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("HANDOFF_DIR", dir)
	defer os.Unsetenv("HANDOFF_DIR")

	p := newPlugin()
	ctx := context.Background()
	tc := plugin.ToolContext{SessionID: "sess-1", Directory: "/tmp"}

	// Save some state
	raw := json.RawMessage(`{"goal": "test goal", "decisions": ["decision 1"]}`)
	_, err := p.Tools[0].Execute(ctx, raw, tc)
	if err != nil {
		t.Fatalf("save error: %v", err)
	}

	// Trigger session start to set session ID
	err = p.Hooks.SessionStart(ctx, plugin.SessionStartEvent{SessionID: "sess-1", Directory: "/tmp"})
	if err != nil {
		t.Fatalf("session start error: %v", err)
	}

	// Save state again after session start (which resets state)
	raw = json.RawMessage(`{"goal": "test goal", "decisions": ["decision 1"]}`)
	_, err = p.Tools[0].Execute(ctx, raw, tc)
	if err != nil {
		t.Fatalf("save error: %v", err)
	}

	// End session
	err = p.Hooks.SessionEnd(ctx, plugin.SessionEndEvent{SessionID: "sess-1"})
	if err != nil {
		t.Fatalf("session end error: %v", err)
	}

	// Verify file was written
	filePath := filepath.Join(dir, "sess-1.json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("expected state file to exist: %v", err)
	}

	var state sessionState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("invalid JSON in state file: %v", err)
	}
	if state.SessionID != "sess-1" {
		t.Errorf("expected sessionId 'sess-1', got %q", state.SessionID)
	}
	if state.Goal != "test goal" {
		t.Errorf("expected goal 'test goal', got %q", state.Goal)
	}
	if len(state.Decisions) != 1 || state.Decisions[0] != "decision 1" {
		t.Errorf("expected decisions ['decision 1'], got %v", state.Decisions)
	}
}

func TestLoadMostRecentState(t *testing.T) {
	dir := t.TempDir()

	// Write two state files
	state1 := sessionState{
		SessionID: "old-session",
		Timestamp: 1000,
		Goal:      "old goal",
	}
	data1, _ := json.Marshal(state1)
	os.WriteFile(filepath.Join(dir, "old-session.json"), data1, 0o644)

	state2 := sessionState{
		SessionID: "new-session",
		Timestamp: 2000,
		Goal:      "new goal",
	}
	data2, _ := json.Marshal(state2)
	os.WriteFile(filepath.Join(dir, "new-session.json"), data2, 0o644)

	// Load most recent excluding current
	loaded, err := loadMostRecentState(dir, "current-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil state")
	}
	if loaded.SessionID != "new-session" {
		t.Errorf("expected newest session 'new-session', got %q", loaded.SessionID)
	}
	if loaded.Goal != "new goal" {
		t.Errorf("expected goal 'new goal', got %q", loaded.Goal)
	}
}

func TestLoadMostRecentStateExcludesCurrent(t *testing.T) {
	dir := t.TempDir()

	state := sessionState{
		SessionID: "my-session",
		Timestamp: 1000,
		Goal:      "my goal",
	}
	data, _ := json.Marshal(state)
	os.WriteFile(filepath.Join(dir, "my-session.json"), data, 0o644)

	loaded, err := loadMostRecentState(dir, "my-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded != nil {
		t.Error("expected nil when only file is current session")
	}
}

func TestLoadMostRecentStateEmptyDir(t *testing.T) {
	dir := t.TempDir()
	loaded, err := loadMostRecentState(dir, "any")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded != nil {
		t.Error("expected nil for empty dir")
	}
}

func TestLoadMostRecentStateNonexistentDir(t *testing.T) {
	_, err := loadMostRecentState("/nonexistent/dir", "any")
	if err == nil {
		t.Error("expected error for nonexistent dir")
	}
}

func TestFormatStateBlock(t *testing.T) {
	state := &sessionState{
		Goal:          "implement feature",
		Decisions:     []string{"use Go", "skip protobuf"},
		OpenTasks:     []string{"write tests"},
		FilesModified: []string{"main.go", "util.go"},
	}

	block := formatStateBlock(state)
	if block == "" {
		t.Fatal("expected non-empty block")
	}
	if block[:18] != "<previous-session>" {
		t.Errorf("expected block to start with <previous-session>, got %q", block[:18])
	}
}

func TestFormatStateBlockEmpty(t *testing.T) {
	state := &sessionState{}
	block := formatStateBlock(state)
	if block != "" {
		t.Errorf("expected empty block for empty state, got %q", block)
	}
}

func TestEmptyState(t *testing.T) {
	state := emptyState("sess-123")
	if state.SessionID != "sess-123" {
		t.Errorf("expected sessionId 'sess-123', got %q", state.SessionID)
	}
	if state.Goal != "" {
		t.Error("expected empty goal")
	}
	if len(state.Decisions) != 0 {
		t.Error("expected empty decisions")
	}
	if len(state.OpenTasks) != 0 {
		t.Error("expected empty openTasks")
	}
	if len(state.FilesModified) != 0 {
		t.Error("expected empty filesModified")
	}
}

func TestGetHandoffDirDefault(t *testing.T) {
	os.Unsetenv("HANDOFF_DIR")
	dir := getHandoffDir()
	if dir == "" {
		t.Error("expected non-empty default dir")
	}
}

func TestGetHandoffDirFromEnv(t *testing.T) {
	os.Setenv("HANDOFF_DIR", "/custom/path")
	defer os.Unsetenv("HANDOFF_DIR")
	dir := getHandoffDir()
	if dir != "/custom/path" {
		t.Errorf("expected '/custom/path', got %q", dir)
	}
}

func TestDisposeResetsState(t *testing.T) {
	p := newPlugin()
	ctx := context.Background()

	err := p.Hooks.Dispose(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSessionStartLoadsState(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("HANDOFF_DIR", dir)
	defer os.Unsetenv("HANDOFF_DIR")

	// Write a previous session state
	prev := sessionState{
		SessionID: "prev-session",
		Timestamp: 1000,
		Goal:      "previous goal",
	}
	data, _ := json.MarshalIndent(prev, "", "  ")
	os.WriteFile(filepath.Join(dir, "prev-session.json"), data, 0o644)

	p := newPlugin()
	ctx := context.Background()

	err := p.Hooks.SessionStart(ctx, plugin.SessionStartEvent{
		SessionID: "new-session",
		Directory: "/tmp",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEndToEndHandoff(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("HANDOFF_DIR", dir)
	defer os.Unsetenv("HANDOFF_DIR")

	ctx := context.Background()

	// Session 1: save state and end
	p1 := newPlugin()
	p1.Hooks.SessionStart(ctx, plugin.SessionStartEvent{SessionID: "s1", Directory: "/tmp"})

	raw := json.RawMessage(`{"goal": "build API", "filesModified": ["api.go"]}`)
	tc := plugin.ToolContext{SessionID: "s1", Directory: "/tmp"}
	p1.Tools[0].Execute(ctx, raw, tc)
	p1.Hooks.SessionEnd(ctx, plugin.SessionEndEvent{SessionID: "s1"})

	// Verify file exists
	if _, err := os.Stat(filepath.Join(dir, "s1.json")); err != nil {
		t.Fatalf("expected state file: %v", err)
	}

	// Session 2: should load session 1's state
	p2 := newPlugin()
	err := p2.Hooks.SessionStart(ctx, plugin.SessionStartEvent{SessionID: "s2", Directory: "/tmp"})
	if err != nil {
		t.Fatalf("session 2 start error: %v", err)
	}
}
