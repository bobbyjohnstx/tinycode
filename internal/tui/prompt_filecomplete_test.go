package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestExtractAtToken_AtStart(t *testing.T) {
	query, start, end, found := extractAtToken("@main.go", 8)
	if !found {
		t.Fatal("expected to find @token at start")
	}
	if query != "main.go" {
		t.Errorf("expected query 'main.go', got %q", query)
	}
	if start != 0 || end != 8 {
		t.Errorf("expected bounds [0,8], got [%d,%d]", start, end)
	}
}

func TestExtractAtToken_MidText(t *testing.T) {
	query, start, end, found := extractAtToken("review @file.go please", 12)
	if !found {
		t.Fatal("expected to find @token mid-text")
	}
	if query != "file.go" {
		t.Errorf("expected query 'file.go', got %q", query)
	}
	if start != 7 || end != 15 {
		t.Errorf("expected bounds [7,15], got [%d,%d]", start, end)
	}
}

func TestExtractAtToken_NoWhitespaceBefore(t *testing.T) {
	_, _, _, found := extractAtToken("email@domain.com", 16)
	if found {
		t.Fatal("expected no @token when not preceded by whitespace")
	}
}

func TestExtractAtToken_PathWithSlash(t *testing.T) {
	query, start, end, found := extractAtToken("@path/to/file.go", 16)
	if !found {
		t.Fatal("expected to find @token with path separators")
	}
	if query != "path/to/file.go" {
		t.Errorf("expected query 'path/to/file.go', got %q", query)
	}
	if start != 0 || end != 16 {
		t.Errorf("expected bounds [0,16], got [%d,%d]", start, end)
	}
}

func TestExtractAtToken_EmptyString(t *testing.T) {
	_, _, _, found := extractAtToken("", 0)
	if found {
		t.Fatal("expected no @token in empty string")
	}
}

func TestExtractAtToken_CursorBeforeAt(t *testing.T) {
	_, _, _, found := extractAtToken("hello @file.go", 3)
	if found {
		t.Fatal("expected no @token when cursor is before @")
	}
}

func TestExtractAtToken_CursorAtAt(t *testing.T) {
	// Cursor right after @, before any path chars
	query, _, _, found := extractAtToken("@file.go", 1)
	if !found {
		t.Fatal("expected to find @token with cursor at position 1")
	}
	if query != "file.go" {
		t.Errorf("expected query 'file.go', got %q", query)
	}
}

func TestExtractAtToken_JustAt(t *testing.T) {
	// Cursor at position 6 (right after @), should find bare @ with empty query
	query, start, end, found := extractAtToken("test @ here", 6)
	if !found {
		t.Fatal("expected to find bare @")
	}
	if query != "" {
		t.Errorf("expected empty query for bare @, got %q", query)
	}
	if start != 5 || end != 6 {
		t.Errorf("expected bounds [5,6], got [%d,%d]", start, end)
	}
}

func TestFindAllAtTokens_Multiple(t *testing.T) {
	refs := findAllAtTokens("review @file1.go and @file2.go")
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
	if refs[0].path != "file1.go" {
		t.Errorf("expected first ref 'file1.go', got %q", refs[0].path)
	}
	if refs[1].path != "file2.go" {
		t.Errorf("expected second ref 'file2.go', got %q", refs[1].path)
	}
}

func TestFindAllAtTokens_SkipsEmail(t *testing.T) {
	refs := findAllAtTokens("email user@domain.com and @real.go")
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref (skipping email), got %d", len(refs))
	}
	if refs[0].path != "real.go" {
		t.Errorf("expected 'real.go', got %q", refs[0].path)
	}
}

func TestFindAllAtTokens_Empty(t *testing.T) {
	refs := findAllAtTokens("no at symbols here")
	if len(refs) != 0 {
		t.Fatalf("expected 0 refs, got %d", len(refs))
	}
}

func TestFindAllAtTokens_BareAt(t *testing.T) {
	refs := findAllAtTokens("@ alone")
	if len(refs) != 0 {
		t.Fatalf("expected 0 refs for bare @ with no path, got %d", len(refs))
	}
}

func TestFileCompleter_UpdateAtCursor_ShowsAndHides(t *testing.T) {
	fc := NewFileCompleter("/tmp")

	cmd := fc.UpdateAtCursor("@", 1)
	// Should return a command to list files (query is "")
	if cmd == nil {
		t.Fatal("expected a command to list files")
	}

	// No @ token, should hide
	cmd = fc.UpdateAtCursor("hello", 5)
	if fc.IsVisible() {
		t.Fatal("expected file completer to be hidden without @token")
	}
}

func TestFileCompleter_SetResults_StaleQueryIgnored(t *testing.T) {
	fc := NewFileCompleter("/tmp")
	fc.UpdateAtCursor("@src", 4)

	// Set results for a different query — should be ignored
	fc.SetResults([]FileItem{{Path: "old.go", IsDir: false}}, "old")
	if fc.IsVisible() {
		t.Fatal("expected stale results to be ignored")
	}

	// Set results for current query
	fc.SetResults([]FileItem{{Path: "src/main.go", IsDir: false}}, "src")
	if !fc.IsVisible() {
		t.Fatal("expected file completer to be visible after matching results")
	}
}

func TestFileCompleter_UpDownNavigation(t *testing.T) {
	fc := NewFileCompleter("/tmp")
	fc.items = []FileItem{
		{Path: "a.go", IsDir: false},
		{Path: "b.go", IsDir: false},
		{Path: "c.go", IsDir: false},
	}
	fc.visible = true

	// Down
	fc, _, _ = fc.Update(tea.KeyMsg{Type: tea.KeyDown})
	if fc.cursor != 1 {
		t.Fatalf("expected cursor at 1, got %d", fc.cursor)
	}

	// Up back to 0
	fc, _, _ = fc.Update(tea.KeyMsg{Type: tea.KeyUp})
	if fc.cursor != 0 {
		t.Fatalf("expected cursor at 0, got %d", fc.cursor)
	}

	// Up wraps to last
	fc, _, _ = fc.Update(tea.KeyMsg{Type: tea.KeyUp})
	if fc.cursor != 2 {
		t.Fatalf("expected cursor to wrap to 2, got %d", fc.cursor)
	}

	// Down wraps to 0
	fc, _, _ = fc.Update(tea.KeyMsg{Type: tea.KeyDown})
	if fc.cursor != 0 {
		t.Fatalf("expected cursor to wrap to 0, got %d", fc.cursor)
	}
}

func TestFileCompleter_EscDismisses(t *testing.T) {
	fc := NewFileCompleter("/tmp")
	fc.visible = true
	fc.items = []FileItem{{Path: "a.go"}}

	fc, _, consumed := fc.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if !consumed {
		t.Fatal("expected escape to be consumed")
	}
	if fc.IsVisible() {
		t.Fatal("expected file completer dismissed after escape")
	}
}

func TestFileCompleter_NotVisible_NoConsume(t *testing.T) {
	fc := NewFileCompleter("/tmp")
	_, _, consumed := fc.Update(tea.KeyMsg{Type: tea.KeyTab})
	if consumed {
		t.Fatal("expected key not consumed when file completer is hidden")
	}
}

func TestFileCompleter_ViewEmpty_WhenHidden(t *testing.T) {
	fc := NewFileCompleter("/tmp")
	if fc.View() != "" {
		t.Fatal("expected empty view when hidden")
	}
}

func TestListFiles_WithTempDir(t *testing.T) {
	dir := t.TempDir()

	// Create test files and directories
	os.MkdirAll(filepath.Join(dir, "subdir"), 0o755)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0o644)
	os.WriteFile(filepath.Join(dir, "util.go"), []byte("package util"), 0o644)
	os.WriteFile(filepath.Join(dir, ".hidden"), []byte("hidden"), 0o644)

	cmd := listFiles(dir, "")
	msg := cmd()
	result, ok := msg.(FileCompletionMsg)
	if !ok {
		t.Fatalf("expected FileCompletionMsg, got %T", msg)
	}

	// Should have subdir, main.go, util.go (not .hidden)
	if len(result.Items) != 3 {
		t.Fatalf("expected 3 items, got %d: %+v", len(result.Items), result.Items)
	}

	// Directories first
	if !result.Items[0].IsDir || result.Items[0].Path != "subdir" {
		t.Errorf("expected first item to be dir 'subdir', got %+v", result.Items[0])
	}
}

func TestListFiles_WithPrefix(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0o644)
	os.WriteFile(filepath.Join(dir, "util.go"), []byte("package util"), 0o644)
	os.WriteFile(filepath.Join(dir, "main_test.go"), []byte("package main"), 0o644)

	cmd := listFiles(dir, "main")
	msg := cmd()
	result, ok := msg.(FileCompletionMsg)
	if !ok {
		t.Fatalf("expected FileCompletionMsg, got %T", msg)
	}

	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items matching 'main', got %d: %+v", len(result.Items), result.Items)
	}
}

func TestListFiles_SubdirListing(t *testing.T) {
	dir := t.TempDir()

	os.MkdirAll(filepath.Join(dir, "pkg"), 0o755)
	os.WriteFile(filepath.Join(dir, "pkg", "a.go"), []byte("package pkg"), 0o644)
	os.WriteFile(filepath.Join(dir, "pkg", "b.go"), []byte("package pkg"), 0o644)

	cmd := listFiles(dir, "pkg/")
	msg := cmd()
	result, ok := msg.(FileCompletionMsg)
	if !ok {
		t.Fatalf("expected FileCompletionMsg, got %T", msg)
	}

	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items in pkg/, got %d: %+v", len(result.Items), result.Items)
	}
	// Paths should be relative with dir prefix
	if result.Items[0].Path != "pkg/a.go" {
		t.Errorf("expected path 'pkg/a.go', got %q", result.Items[0].Path)
	}
}

func TestListFiles_NonExistentDir(t *testing.T) {
	cmd := listFiles("/nonexistent/path", "")
	msg := cmd()
	result, ok := msg.(FileCompletionMsg)
	if !ok {
		t.Fatalf("expected FileCompletionMsg, got %T", msg)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected 0 items for nonexistent dir, got %d", len(result.Items))
	}
}
