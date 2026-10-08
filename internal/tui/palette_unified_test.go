package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildFilePaletteItems_SkipsHiddenDirs(t *testing.T) {
	dir := t.TempDir()
	// Create a hidden directory with a file.
	os.MkdirAll(filepath.Join(dir, ".hidden"), 0o755)
	os.WriteFile(filepath.Join(dir, ".hidden", "secret.txt"), []byte("x"), 0o644)
	// Create a visible file.
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("x"), 0o644)

	items := buildFilePaletteItems(dir)

	for _, item := range items {
		if item.Label == ".hidden/secret.txt" || item.Label == ".hidden" {
			t.Errorf("hidden dir contents should be skipped, got %q", item.Label)
		}
	}

	found := false
	for _, item := range items {
		if item.Label == "readme.md" {
			found = true
			if item.Category != "file" {
				t.Errorf("expected category 'file', got %q", item.Category)
			}
			if item.Value != "@readme.md" {
				t.Errorf("expected value '@readme.md', got %q", item.Value)
			}
		}
	}
	if !found {
		t.Error("expected readme.md in results")
	}
}

func TestBuildFilePaletteItems_SkipsNodeModules(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "node_modules", "pkg"), 0o755)
	os.WriteFile(filepath.Join(dir, "node_modules", "pkg", "index.js"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("x"), 0o644)

	items := buildFilePaletteItems(dir)

	for _, item := range items {
		if item.Label == "node_modules/pkg/index.js" {
			t.Error("node_modules contents should be skipped")
		}
	}

	found := false
	for _, item := range items {
		if item.Label == "main.go" {
			found = true
		}
	}
	if !found {
		t.Error("expected main.go in results")
	}
}

func TestBuildFilePaletteItems_RespectsDepthLimit(t *testing.T) {
	dir := t.TempDir()
	// Create a file at depth 4 (a/b/c/deep.txt = 4 segments, beyond limit of 3).
	deep := filepath.Join(dir, "a", "b", "c")
	os.MkdirAll(deep, 0o755)
	os.WriteFile(filepath.Join(deep, "deep.txt"), []byte("x"), 0o644)
	// Create a file at depth 3 (a/b/ok.txt = 3 segments, within limit).
	shallow := filepath.Join(dir, "a", "b")
	os.WriteFile(filepath.Join(shallow, "ok.txt"), []byte("x"), 0o644)

	items := buildFilePaletteItems(dir)

	for _, item := range items {
		if item.Label == filepath.Join("a", "b", "c", "deep.txt") {
			t.Error("file beyond depth 3 should be skipped")
		}
	}

	found := false
	for _, item := range items {
		if item.Label == filepath.Join("a", "b", "ok.txt") {
			found = true
		}
	}
	if !found {
		t.Error("expected a/b/ok.txt in results (depth 3 should be included)")
	}
}

func TestBuildFilePaletteItems_RespectsCountLimit(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 120; i++ {
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("file%03d.txt", i)), []byte("x"), 0o644)
	}

	items := buildFilePaletteItems(dir)

	if len(items) > 100 {
		t.Errorf("expected at most 100 items, got %d", len(items))
	}
}

func TestBuildSessionPaletteItems_SkipsActive(t *testing.T) {
	sessions := []SessionInfo{
		{ID: "s1", Title: "Active Session", UpdatedAt: time.Now().Unix()},
		{ID: "s2", Title: "Other Session", UpdatedAt: time.Now().Add(-2 * time.Hour).Unix()},
		{ID: "s3", Title: "Old Session", UpdatedAt: time.Now().Add(-48 * time.Hour).Unix()},
	}

	items := buildSessionPaletteItems(sessions, "s1")

	if len(items) != 2 {
		t.Fatalf("expected 2 items (active skipped), got %d", len(items))
	}

	for _, item := range items {
		if item.Value == "session:s1" {
			t.Error("active session should be skipped")
		}
		if item.Category != "session" {
			t.Errorf("expected category 'session', got %q", item.Category)
		}
	}
}

func TestBuildSessionPaletteItems_FormatsTitle(t *testing.T) {
	sessions := []SessionInfo{
		{ID: "s1", Title: "", UpdatedAt: time.Now().Unix()},
		{ID: "s2", Title: "Named", UpdatedAt: time.Now().Unix()},
	}

	items := buildSessionPaletteItems(sessions, "none")

	if items[0].Label != "s1" {
		t.Errorf("untitled session should use ID as label, got %q", items[0].Label)
	}
	if items[1].Label != "Named" {
		t.Errorf("titled session should use title as label, got %q", items[1].Label)
	}
}

func TestBuildSessionPaletteItems_TimeAgoDescription(t *testing.T) {
	sessions := []SessionInfo{
		{ID: "s1", Title: "Recent", UpdatedAt: time.Now().Add(-30 * time.Second).Unix()},
		{ID: "s2", Title: "Minutes", UpdatedAt: time.Now().Add(-5 * time.Minute).Unix()},
		{ID: "s3", Title: "Hours", UpdatedAt: time.Now().Add(-3 * time.Hour).Unix()},
		{ID: "s4", Title: "Days", UpdatedAt: time.Now().Add(-72 * time.Hour).Unix()},
	}

	items := buildSessionPaletteItems(sessions, "none")

	expected := []string{"just now", "5m ago", "3h ago", "3d ago"}
	for i, item := range items {
		if item.Description != expected[i] {
			t.Errorf("item %d: expected description %q, got %q", i, expected[i], item.Description)
		}
	}
}

func TestCategoryTag(t *testing.T) {
	tests := []struct {
		category string
		want     string
	}{
		{"command", "[cmd] "},
		{"file", "[fil] "},
		{"session", "[ses] "},
		{"", ""},
		{"unknown", ""},
	}
	for _, tt := range tests {
		got := categoryTag(tt.category)
		if got != tt.want {
			t.Errorf("categoryTag(%q) = %q, want %q", tt.category, got, tt.want)
		}
	}
}

func TestUnifiedPaletteSelection_File(t *testing.T) {
	app := NewApp("")
	app.prompt.SetValue("existing text")

	result, _, handled := app.handleDialogMsg(PaletteSelectedMsg{
		Item: PaletteItem{Label: "main.go", Value: "@main.go", Category: "file"},
	})
	if !handled {
		t.Fatal("expected palette selection handled")
	}
	want := "existing text @main.go"
	if result.prompt.Value() != want {
		t.Errorf("prompt = %q, want %q", result.prompt.Value(), want)
	}
}

func TestUnifiedPaletteSelection_FileEmptyPrompt(t *testing.T) {
	app := NewApp("")

	result, _, handled := app.handleDialogMsg(PaletteSelectedMsg{
		Item: PaletteItem{Label: "main.go", Value: "@main.go", Category: "file"},
	})
	if !handled {
		t.Fatal("expected palette selection handled")
	}
	want := "@main.go"
	if result.prompt.Value() != want {
		t.Errorf("prompt = %q, want %q", result.prompt.Value(), want)
	}
}

func TestUnifiedPaletteSelection_Session(t *testing.T) {
	app := NewApp("")

	_, cmd, handled := app.handleDialogMsg(PaletteSelectedMsg{
		Item: PaletteItem{Label: "Debug session", Value: "session:abc123", Category: "session"},
	})
	if !handled {
		t.Fatal("expected palette selection handled")
	}
	if cmd == nil {
		t.Fatal("expected a command for session switch")
	}
	msg := cmd()
	sw, ok := msg.(SessionSwitchedMsg)
	if !ok {
		t.Fatalf("expected SessionSwitchedMsg, got %T", msg)
	}
	if sw.SessionID != "abc123" {
		t.Errorf("SessionID = %q, want %q", sw.SessionID, "abc123")
	}
}

func TestUnifiedPaletteSelection_CommandFallsThrough(t *testing.T) {
	app := NewApp("")

	_, _, handled := app.handleDialogMsg(PaletteSelectedMsg{
		Item: PaletteItem{Label: "exit", Value: "exit", Category: "command"},
	})
	if !handled {
		t.Fatal("expected palette selection handled")
	}
}
