package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func execNotes(t *testing.T, notesDir string, args any) *ExecuteResult {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	def := NotesTool(notesDir)
	result, err := def.Execute(context.Background(), &Context{SessionID: "test"}, raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return result
}

func TestNotes_WriteAndRead(t *testing.T) {
	dir := t.TempDir()

	r := execNotes(t, dir, map[string]string{"action": "write", "path": "MEMORY.md", "content": "line 1\nline 2"})
	if r.IsError {
		t.Fatalf("write failed: %s", r.Output)
	}
	if !strings.Contains(r.Output, "MEMORY.md") {
		t.Errorf("expected path in output, got: %s", r.Output)
	}

	r = execNotes(t, dir, map[string]string{"action": "read", "path": "MEMORY.md"})
	if r.IsError {
		t.Fatalf("read failed: %s", r.Output)
	}
	if r.Output != "line 1\nline 2" {
		t.Errorf("expected stored content, got: %s", r.Output)
	}
}

func TestNotes_List(t *testing.T) {
	dir := t.TempDir()

	execNotes(t, dir, map[string]string{"action": "write", "path": "MEMORY.md", "content": "notes"})
	execNotes(t, dir, map[string]string{"action": "write", "path": "scratch.md", "content": "scratch data"})

	r := execNotes(t, dir, map[string]string{"action": "list"})
	if r.IsError {
		t.Fatalf("list failed: %s", r.Output)
	}
	if !strings.Contains(r.Output, "MEMORY.md") {
		t.Errorf("expected MEMORY.md in list, got: %s", r.Output)
	}
	if !strings.Contains(r.Output, "scratch.md") {
		t.Errorf("expected scratch.md in list, got: %s", r.Output)
	}
}

func TestNotes_ListEmpty(t *testing.T) {
	dir := t.TempDir()

	r := execNotes(t, dir, map[string]string{"action": "list"})
	if r.IsError {
		t.Fatalf("list failed: %s", r.Output)
	}
	if !strings.Contains(r.Output, "empty") {
		t.Errorf("expected empty message, got: %s", r.Output)
	}
}

func TestNotes_ListNonexistentDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nonexistent")

	r := execNotes(t, dir, map[string]string{"action": "list"})
	if r.IsError {
		t.Fatalf("list of nonexistent dir should not error: %s", r.Output)
	}
	if !strings.Contains(r.Output, "empty") {
		t.Errorf("expected empty message, got: %s", r.Output)
	}
}

func TestNotes_ReadNotFound(t *testing.T) {
	dir := t.TempDir()

	r := execNotes(t, dir, map[string]string{"action": "read", "path": "missing.md"})
	if r.IsError {
		t.Errorf("read missing should not be an error result")
	}
	if !strings.Contains(r.Output, "not found") {
		t.Errorf("expected not found message, got: %s", r.Output)
	}
}

func TestNotes_PathEscapeBlocked(t *testing.T) {
	dir := t.TempDir()

	r := execNotes(t, dir, map[string]string{"action": "write", "path": "../escape.md", "content": "bad"})
	if !r.IsError {
		t.Error("expected error for path escape")
	}
	if !strings.Contains(r.Output, "escapes") {
		t.Errorf("expected escape message, got: %s", r.Output)
	}
}

func TestNotes_AbsolutePathBlocked(t *testing.T) {
	dir := t.TempDir()

	r := execNotes(t, dir, map[string]string{"action": "write", "path": "/etc/passwd", "content": "bad"})
	if !r.IsError {
		t.Error("expected error for absolute path")
	}
	if !strings.Contains(r.Output, "absolute") {
		t.Errorf("expected absolute path message, got: %s", r.Output)
	}
}

func TestNotes_MemoryMDLineCap(t *testing.T) {
	dir := t.TempDir()

	// 201 lines should be rejected
	lines := make([]string, maxNotesLines+1)
	for i := range lines {
		lines[i] = "line"
	}
	content := strings.Join(lines, "\n")

	r := execNotes(t, dir, map[string]string{"action": "write", "path": "MEMORY.md", "content": content})
	if !r.IsError {
		t.Error("expected error when MEMORY.md exceeds line cap")
	}
	if !strings.Contains(r.Output, "cap") {
		t.Errorf("expected cap message, got: %s", r.Output)
	}

	// File should not exist
	if _, err := os.Stat(filepath.Join(dir, "MEMORY.md")); err == nil {
		t.Error("MEMORY.md should not have been written")
	}
}

func TestNotes_MemoryMDExactLineCap(t *testing.T) {
	dir := t.TempDir()

	// Exactly 200 lines should succeed
	lines := make([]string, maxNotesLines)
	for i := range lines {
		lines[i] = "line"
	}
	content := strings.Join(lines, "\n")

	r := execNotes(t, dir, map[string]string{"action": "write", "path": "MEMORY.md", "content": content})
	if r.IsError {
		t.Errorf("write of exactly %d lines should succeed: %s", maxNotesLines, r.Output)
	}
}

func TestNotes_NonMemoryFileNoLineCap(t *testing.T) {
	dir := t.TempDir()

	// A non-MEMORY.md file with many lines should succeed
	lines := make([]string, maxNotesLines+100)
	for i := range lines {
		lines[i] = "line"
	}
	content := strings.Join(lines, "\n")

	r := execNotes(t, dir, map[string]string{"action": "write", "path": "other.md", "content": content})
	if r.IsError {
		t.Errorf("non-MEMORY.md file should not be line-capped: %s", r.Output)
	}
}

func TestNotes_MissingRequiredArgs(t *testing.T) {
	dir := t.TempDir()

	r := execNotes(t, dir, map[string]string{"action": "write", "content": "data"})
	if !r.IsError {
		t.Error("expected error for missing path")
	}

	r = execNotes(t, dir, map[string]string{"action": "write", "path": "foo.md"})
	if !r.IsError {
		t.Error("expected error for missing content")
	}

	r = execNotes(t, dir, map[string]string{"action": "read"})
	if !r.IsError {
		t.Error("expected error for missing path on read")
	}
}

func TestNotes_InvalidAction(t *testing.T) {
	dir := t.TempDir()

	r := execNotes(t, dir, map[string]string{"action": "delete"})
	if !r.IsError {
		t.Error("expected error for invalid action")
	}
	if !strings.Contains(r.Output, "invalid action") {
		t.Errorf("expected invalid action message, got: %s", r.Output)
	}
}

func TestNotes_Permission(t *testing.T) {
	def := NotesTool(t.TempDir())
	if def.Permission != "write" {
		t.Errorf("expected write permission, got: %s", def.Permission)
	}
}
