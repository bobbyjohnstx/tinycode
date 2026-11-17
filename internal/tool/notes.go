package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxNotesLines = 200

// NotesTool returns a tool that reads, writes, and lists files in a
// per-repository notes directory. Files persist across sessions.
func NotesTool(notesDir string) *Def {
	return &Def{
		ID:          "notes",
		Description: "Read, write, or list files in the persistent notes directory. Notes survive across sessions (unlike notepad which is session-scoped). Use this to record context, decisions, or memory that should carry forward.",
		Permission:  "write",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action":   map[string]any{"type": "string", "enum": []string{"read", "write", "list"}, "description": "Operation to perform"},
				"path":     map[string]any{"type": "string", "description": "File path relative to the notes directory (required for read/write). Example: MEMORY.md"},
				"content":  map[string]any{"type": "string", "description": "File content (required for write)"},
			},
			"required": []string{"action"},
		},
		Execute: makeNotesExecute(notesDir),
	}
}

func makeNotesExecute(notesDir string) func(context.Context, *Context, json.RawMessage) (*ExecuteResult, error) {
	return func(_ context.Context, _ *Context, rawArgs json.RawMessage) (*ExecuteResult, error) {
		var args struct {
			Action  string `json:"action"`
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return &ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
		}

		switch args.Action {
		case "write":
			return notesWrite(notesDir, args.Path, args.Content)
		case "read":
			return notesRead(notesDir, args.Path)
		case "list":
			return notesList(notesDir)
		default:
			return &ExecuteResult{
				Output:  fmt.Sprintf("invalid action %q: must be read, write, or list", args.Action),
				IsError: true,
			}, nil
		}
	}
}

func notesWrite(notesDir, path, content string) (*ExecuteResult, error) {
	if path == "" {
		return &ExecuteResult{Output: "path is required for write", IsError: true}, nil
	}
	if content == "" {
		return &ExecuteResult{Output: "content is required for write", IsError: true}, nil
	}

	resolved, err := resolveNotesPath(notesDir, path)
	if err != nil {
		return &ExecuteResult{Output: err.Error(), IsError: true}, nil
	}

	// Cap enforcement for MEMORY.md
	if filepath.Base(resolved) == "MEMORY.md" {
		lineCount := strings.Count(content, "\n") + 1
		if lineCount > maxNotesLines {
			return &ExecuteResult{
				Output:  fmt.Sprintf("MEMORY.md would have %d lines, exceeding the %d-line cap. Shorten the index before writing.", lineCount, maxNotesLines),
				IsError: true,
			}, nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(resolved), 0755); err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("failed to create directory: %v", err), IsError: true}, nil
	}

	if err := os.WriteFile(resolved, []byte(content), 0644); err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("failed to write: %v", err), IsError: true}, nil
	}

	return &ExecuteResult{Output: fmt.Sprintf("Wrote %s (%d bytes)", path, len(content))}, nil
}

func notesRead(notesDir, path string) (*ExecuteResult, error) {
	if path == "" {
		return &ExecuteResult{Output: "path is required for read", IsError: true}, nil
	}

	resolved, err := resolveNotesPath(notesDir, path)
	if err != nil {
		return &ExecuteResult{Output: err.Error(), IsError: true}, nil
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return &ExecuteResult{Output: fmt.Sprintf("file %q not found in notes directory", path)}, nil
		}
		return &ExecuteResult{Output: fmt.Sprintf("failed to read: %v", err), IsError: true}, nil
	}

	return &ExecuteResult{Output: string(data)}, nil
}

func notesList(notesDir string) (*ExecuteResult, error) {
	entries, err := os.ReadDir(notesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return &ExecuteResult{Output: "Notes directory is empty."}, nil
		}
		return &ExecuteResult{Output: fmt.Sprintf("failed to list: %v", err), IsError: true}, nil
	}

	if len(entries) == 0 {
		return &ExecuteResult{Output: "Notes directory is empty."}, nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%d files:\n", len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		fmt.Fprintf(&sb, "  %s  %d bytes\n", e.Name(), info.Size())
	}
	return &ExecuteResult{Output: sb.String()}, nil
}

// resolveNotesPath validates and resolves a relative path inside notesDir.
// Returns an error if the path escapes the notes directory.
func resolveNotesPath(notesDir, path string) (string, error) {
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("path must be relative to the notes directory, got absolute path %q", path)
	}

	resolved := filepath.Clean(filepath.Join(notesDir, path))

	// Ensure the resolved path is inside notesDir.
	if !strings.HasPrefix(resolved, filepath.Clean(notesDir)+string(filepath.Separator)) && resolved != filepath.Clean(notesDir) {
		return "", fmt.Errorf("path %q escapes the notes directory", path)
	}

	return resolved, nil
}
