package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTruncate_NoOp(t *testing.T) {
	content := "short content"
	result := Truncate(content, TruncTail)
	if result.Truncated {
		t.Error("expected not truncated")
	}
	if result.Content != content {
		t.Errorf("content changed: %q", result.Content)
	}
}

func TestTruncate_ByLines(t *testing.T) {
	lines := make([]string, MaxLines+100)
	for i := range lines {
		lines[i] = "line"
	}
	content := strings.Join(lines, "\n")

	result := Truncate(content, TruncTail)
	if !result.Truncated {
		t.Error("expected truncated")
	}
	if !strings.Contains(result.Content, "truncated") {
		t.Error("expected truncation hint")
	}
}

func TestTruncate_ByBytes(t *testing.T) {
	content := strings.Repeat("x", MaxBytes+1000)
	result := Truncate(content, TruncTail)
	if !result.Truncated {
		t.Error("expected truncated")
	}
	if result.FullSize != len(content) {
		t.Errorf("expected full size %d, got %d", len(content), result.FullSize)
	}
}

func TestTruncate_HeadDirection(t *testing.T) {
	lines := make([]string, MaxLines+100)
	for i := range lines {
		lines[i] = "line"
	}
	content := strings.Join(lines, "\n")

	result := Truncate(content, TruncHead)
	if !result.Truncated {
		t.Error("expected truncated")
	}
	if !strings.Contains(result.Content, "last") {
		t.Error("expected 'last' in truncation hint for head direction")
	}
}

func TestRegistry_RegisterAndExecute(t *testing.T) {
	r := NewRegistry(&Context{Directory: t.TempDir()})

	r.Register(&Def{
		ID:          "test-tool",
		Description: "A test tool",
		Permission:  "read",
		Parameters:  map[string]any{"type": "object"},
		Execute: func(ctx context.Context, tc *Context, args json.RawMessage) (*ExecuteResult, error) {
			return &ExecuteResult{Output: "test output"}, nil
		},
	})

	output, isErr, err := r.Execute(context.Background(), "test-tool", json.RawMessage(`{}`), "session-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isErr {
		t.Error("expected no error")
	}
	if output != "test output" {
		t.Errorf("expected 'test output', got %q", output)
	}
}

func TestRegistry_UnknownTool(t *testing.T) {
	r := NewRegistry(&Context{Directory: t.TempDir()})

	output, isErr, _ := r.Execute(context.Background(), "nonexistent", json.RawMessage(`{}`), "session-1")
	if !isErr {
		t.Error("expected error for unknown tool")
	}
	if !strings.Contains(output, "Unknown tool") {
		t.Errorf("expected unknown tool message, got %q", output)
	}
}

func TestRegistry_DisabledTool(t *testing.T) {
	r := NewRegistry(&Context{Directory: t.TempDir()})
	r.Register(&Def{
		ID:         "my-tool",
		Permission: "read",
		Parameters: map[string]any{"type": "object"},
		Execute: func(ctx context.Context, tc *Context, args json.RawMessage) (*ExecuteResult, error) {
			return &ExecuteResult{Output: "ok"}, nil
		},
	})
	r.SetDisabled(map[string]bool{"my-tool": true})

	output, isErr, _ := r.Execute(context.Background(), "my-tool", json.RawMessage(`{}`), "session-1")
	if !isErr {
		t.Error("expected error for disabled tool")
	}
	if !strings.Contains(output, "disabled") {
		t.Errorf("expected disabled message, got %q", output)
	}
}

func TestRegistry_ToolDefs_PermissionFiltering(t *testing.T) {
	r := NewRegistry(&Context{Directory: t.TempDir()})
	r.Register(&Def{ID: "read-tool", Permission: "read", Parameters: map[string]any{"type": "object"}})
	r.Register(&Def{ID: "edit-tool", Permission: "edit", Parameters: map[string]any{"type": "object"}})
	r.Register(&Def{ID: "shell-tool", Permission: "shell", Parameters: map[string]any{"type": "object"}})

	defs := r.ToolDefs([]string{"read"})
	if len(defs) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(defs))
	}
	if defs[0].Function.Name != "read-tool" {
		t.Errorf("expected read-tool, got %s", defs[0].Function.Name)
	}

	allDefs := r.ToolDefs([]string{"*"})
	if len(allDefs) != 3 {
		t.Errorf("expected 3 tools with wildcard, got %d", len(allDefs))
	}

	noPerm := r.ToolDefs(nil)
	if len(noPerm) != 3 {
		t.Errorf("expected all tools when no perms specified, got %d", len(noPerm))
	}
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry(&Context{Directory: t.TempDir()})
	r.Register(&Def{ID: "a", Permission: "read"})
	r.Register(&Def{ID: "b", Permission: "edit"})
	r.Register(&Def{ID: "c", Permission: "read"})
	r.SetDisabled(map[string]bool{"b": true})

	names := r.List()
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
	if names[0] != "a" || names[1] != "c" {
		t.Errorf("expected [a, c], got %v", names)
	}
}

func TestReadTool_Basic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("line 1\nline 2\nline 3\n"), 0644)

	r := NewRegistry(&Context{Directory: dir})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"file_path": "` + path + `"}`)
	output, isErr, err := r.Execute(context.Background(), "read", args, "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isErr {
		t.Errorf("expected no error, got: %s", output)
	}
	if !strings.Contains(output, "line 1") {
		t.Error("expected file content in output")
	}
	if !strings.Contains(output, "1\t") {
		t.Error("expected line numbers in output")
	}
}

func TestReadTool_NotFound(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(&Context{Directory: dir})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"file_path": "/nonexistent/file.txt"}`)
	output, isErr, _ := r.Execute(context.Background(), "read", args, "s1")
	if !isErr {
		t.Error("expected error for missing file")
	}
	if !strings.Contains(output, "not found") {
		t.Errorf("expected 'not found' message, got %q", output)
	}
}

func TestReadTool_WithOffsetLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	var lines []string
	for i := 0; i < 100; i++ {
		lines = append(lines, strings.Repeat("x", 10))
	}
	os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)

	r := NewRegistry(&Context{Directory: dir})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"file_path": "` + path + `", "offset": 10, "limit": 5}`)
	output, isErr, _ := r.Execute(context.Background(), "read", args, "s1")
	if isErr {
		t.Errorf("unexpected error: %s", output)
	}
	if !strings.Contains(output, "11\t") {
		t.Error("expected line 11 in output")
	}
}

func TestWriteTool_CreateFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.txt")

	r := NewRegistry(&Context{Directory: dir})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"file_path": "` + path + `", "content": "hello world"}`)
	output, isErr, _ := r.Execute(context.Background(), "write", args, "s1")
	if isErr {
		t.Errorf("unexpected error: %s", output)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("expected 'hello world', got %q", string(data))
	}
}

func TestEditTool_ReplaceOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	os.WriteFile(path, []byte("foo bar baz"), 0644)

	r := NewRegistry(&Context{Directory: dir})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"file_path": "` + path + `", "old_string": "bar", "new_string": "qux"}`)
	output, isErr, _ := r.Execute(context.Background(), "edit", args, "s1")
	if isErr {
		t.Errorf("unexpected error: %s", output)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "foo qux baz" {
		t.Errorf("expected 'foo qux baz', got %q", string(data))
	}
}

func TestEditTool_MultipleOccurrences(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	os.WriteFile(path, []byte("aa bb aa"), 0644)

	r := NewRegistry(&Context{Directory: dir})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"file_path": "` + path + `", "old_string": "aa", "new_string": "cc"}`)
	output, isErr, _ := r.Execute(context.Background(), "edit", args, "s1")
	if !isErr {
		t.Error("expected error for multiple matches without replace_all")
	}
	if !strings.Contains(output, "2 times") {
		t.Errorf("expected count message, got %q", output)
	}
}

func TestEditTool_ReplaceAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	os.WriteFile(path, []byte("aa bb aa"), 0644)

	r := NewRegistry(&Context{Directory: dir})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"file_path": "` + path + `", "old_string": "aa", "new_string": "cc", "replace_all": true}`)
	output, isErr, _ := r.Execute(context.Background(), "edit", args, "s1")
	if isErr {
		t.Errorf("unexpected error: %s", output)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "cc bb cc" {
		t.Errorf("expected 'cc bb cc', got %q", string(data))
	}
}

func TestEditTool_NotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	os.WriteFile(path, []byte("hello world"), 0644)

	r := NewRegistry(&Context{Directory: dir})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"file_path": "` + path + `", "old_string": "xyz", "new_string": "abc"}`)
	output, isErr, _ := r.Execute(context.Background(), "edit", args, "s1")
	if !isErr {
		t.Error("expected error for missing string")
	}
	if !strings.Contains(output, "not found") {
		t.Errorf("expected 'not found' message, got %q", output)
	}
}

func TestShellTool_Basic(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(&Context{Directory: dir})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"command": "echo hello"}`)
	output, isErr, _ := r.Execute(context.Background(), "shell", args, "s1")
	if isErr {
		t.Errorf("unexpected error: %s", output)
	}
	if !strings.Contains(output, "hello") {
		t.Errorf("expected 'hello' in output, got %q", output)
	}
}

func TestShellTool_DestructiveBlocked(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(&Context{Directory: dir})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"command": "rm -rf /"}`)
	output, isErr, _ := r.Execute(context.Background(), "shell", args, "s1")
	if !isErr {
		t.Error("expected destructive command to be blocked")
	}
	if !strings.Contains(output, "destructive") {
		t.Errorf("expected destructive warning, got %q", output)
	}
}

func TestShellTool_Timeout(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(&Context{Directory: dir})
	RegisterBuiltins(r)

	output, isErr, _ := r.Execute(context.Background(), "shell", json.RawMessage(`{"command": "sleep 30", "timeout": 100}`), "s1")
	if !isErr {
		t.Error("expected timeout error")
	}
	if !strings.Contains(output, "timed out") {
		t.Errorf("expected timeout message, got %q", output)
	}
}

func TestGrepTool_BasicMatch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.go"), []byte("func main() {\n\tfmt.Println(\"hello\")\n}\n"), 0644)

	r := NewRegistry(&Context{Directory: dir})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"pattern": "Println", "path": "` + dir + `"}`)
	output, isErr, _ := r.Execute(context.Background(), "grep", args, "s1")
	if isErr {
		t.Errorf("unexpected error: %s", output)
	}
	if !strings.Contains(output, "Println") {
		t.Error("expected match in output")
	}
	if !strings.Contains(output, "test.go:2") {
		t.Error("expected file:line format")
	}
}

func TestGrepTool_NoMatch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.go"), []byte("nothing here"), 0644)

	r := NewRegistry(&Context{Directory: dir})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"pattern": "xyz123", "path": "` + dir + `"}`)
	output, isErr, _ := r.Execute(context.Background(), "grep", args, "s1")
	if isErr {
		t.Errorf("unexpected error: %s", output)
	}
	if !strings.Contains(output, "No matches") {
		t.Error("expected no matches message")
	}
}

func TestGlobTool_BasicMatch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte(""), 0644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte(""), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte(""), 0644)

	r := NewRegistry(&Context{Directory: dir})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"pattern": "*.go", "path": "` + dir + `"}`)
	output, isErr, _ := r.Execute(context.Background(), "glob", args, "s1")
	if isErr {
		t.Errorf("unexpected error: %s", output)
	}
	if !strings.Contains(output, "a.go") || !strings.Contains(output, "b.go") {
		t.Errorf("expected .go files, got %q", output)
	}
	if strings.Contains(output, "c.txt") {
		t.Error("should not include .txt files")
	}
}

func TestGlobTool_DoubleGlob(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	os.MkdirAll(sub, 0755)
	os.WriteFile(filepath.Join(dir, "a.go"), []byte(""), 0644)
	os.WriteFile(filepath.Join(sub, "b.go"), []byte(""), 0644)

	r := NewRegistry(&Context{Directory: dir})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"pattern": "**/*.go", "path": "` + dir + `"}`)
	output, isErr, _ := r.Execute(context.Background(), "glob", args, "s1")
	if isErr {
		t.Errorf("unexpected error: %s", output)
	}
	if !strings.Contains(output, "a.go") || !strings.Contains(output, "b.go") {
		t.Errorf("expected both files, got %q", output)
	}
}

func TestIsDestructive(t *testing.T) {
	tests := []struct {
		cmd  string
		want bool
	}{
		{"echo hello", false},
		{"ls -la", false},
		{"rm -rf /tmp/foo", true},
		{"git push --force", true},
		{"git reset --hard HEAD", true},
		{"git clean -fd", true},
		{"git branch -D feature", true},
		{"DROP TABLE users", true},
		{"cat file.txt", false},
	}

	for _, tt := range tests {
		got := isDestructive(tt.cmd)
		if got != tt.want {
			t.Errorf("isDestructive(%q) = %v, want %v", tt.cmd, got, tt.want)
		}
	}
}

func TestBuiltinRegistration(t *testing.T) {
	r := NewRegistry(&Context{Directory: t.TempDir()})
	RegisterBuiltins(r)

	expected := []string{"read", "write", "edit", "shell", "grep", "glob", "question", "webfetch", "invalid", "task", "todowrite"}
	names := r.List()

	if len(names) != len(expected) {
		t.Fatalf("expected %d tools, got %d: %v", len(expected), len(names), names)
	}

	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	for _, e := range expected {
		if !nameSet[e] {
			t.Errorf("missing builtin tool: %s", e)
		}
	}
}
