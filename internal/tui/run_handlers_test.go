package tui

import (
	"testing"
)

func TestExtractModifiedFiles_WriteAndEdit(t *testing.T) {
	messages := []MessageView{
		{
			Parts: []PartView{
				{Type: "tool", ToolName: "write", ToolArgs: `{"file_path": "/tmp/a.go", "content": "package main"}`},
				{Type: "tool", ToolName: "edit", ToolArgs: `{"file_path": "/tmp/b.go", "old_string": "x", "new_string": "y"}`},
			},
		},
	}
	files := extractModifiedFiles(messages)
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if files[0] != "/tmp/a.go" {
		t.Errorf("expected /tmp/a.go, got %s", files[0])
	}
	if files[1] != "/tmp/b.go" {
		t.Errorf("expected /tmp/b.go, got %s", files[1])
	}
}

func TestExtractModifiedFiles_Deduplication(t *testing.T) {
	messages := []MessageView{
		{
			Parts: []PartView{
				{Type: "tool", ToolName: "write", ToolArgs: `{"file_path": "/tmp/a.go"}`},
				{Type: "tool", ToolName: "edit", ToolArgs: `{"file_path": "/tmp/a.go", "old_string": "x", "new_string": "y"}`},
			},
		},
	}
	files := extractModifiedFiles(messages)
	if len(files) != 1 {
		t.Fatalf("expected 1 file (deduplicated), got %d", len(files))
	}
}

func TestExtractModifiedFiles_IgnoresReadAndBash(t *testing.T) {
	messages := []MessageView{
		{
			Parts: []PartView{
				{Type: "tool", ToolName: "read", ToolArgs: `{"file_path": "/tmp/a.go"}`},
				{Type: "tool", ToolName: "bash", ToolArgs: `{"command": "ls"}`},
				{Type: "tool", ToolName: "grep", ToolArgs: `{"pattern": "foo"}`},
			},
		},
	}
	files := extractModifiedFiles(messages)
	if len(files) != 0 {
		t.Fatalf("expected 0 files, got %d", len(files))
	}
}

func TestExtractModifiedFiles_EmptyMessages(t *testing.T) {
	files := extractModifiedFiles(nil)
	if len(files) != 0 {
		t.Fatalf("expected 0 files, got %d", len(files))
	}
}

func TestExtractModifiedFiles_InvalidJSON(t *testing.T) {
	messages := []MessageView{
		{
			Parts: []PartView{
				{Type: "tool", ToolName: "write", ToolArgs: `not json`},
			},
		},
	}
	files := extractModifiedFiles(messages)
	if len(files) != 0 {
		t.Fatalf("expected 0 files for invalid JSON, got %d", len(files))
	}
}

func TestExtractModifiedFiles_MissingFilePath(t *testing.T) {
	messages := []MessageView{
		{
			Parts: []PartView{
				{Type: "tool", ToolName: "write", ToolArgs: `{"content": "hello"}`},
			},
		},
	}
	files := extractModifiedFiles(messages)
	if len(files) != 0 {
		t.Fatalf("expected 0 files when file_path missing, got %d", len(files))
	}
}

func TestExtractModifiedFiles_CaseInsensitiveToolName(t *testing.T) {
	messages := []MessageView{
		{
			Parts: []PartView{
				{Type: "tool", ToolName: "Write", ToolArgs: `{"file_path": "/tmp/upper.go"}`},
				{Type: "tool", ToolName: "EDIT", ToolArgs: `{"file_path": "/tmp/upper2.go"}`},
			},
		},
	}
	files := extractModifiedFiles(messages)
	if len(files) != 2 {
		t.Fatalf("expected 2 files with case-insensitive matching, got %d", len(files))
	}
}
