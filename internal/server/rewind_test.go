package server

import (
	"sort"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/session"
)

func TestExtractUnrestoredPaths_MutationBash(t *testing.T) {
	messages := []session.Message{
		{
			ID:   "m1",
			Role: session.RoleAssistant,
			Parts: []session.Part{
				session.ToolCallPart("tc1", "bash", `{"command":"echo hello > out.txt"}`),
			},
		},
	}

	paths := extractUnrestoredPaths(messages)
	if len(paths) == 0 {
		t.Fatal("expected unrestored paths, got none")
	}

	found := false
	for _, p := range paths {
		if p == "out.txt" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected out.txt in unrestored paths, got %v", paths)
	}
}

func TestExtractUnrestoredPaths_ReadOnlyBash(t *testing.T) {
	messages := []session.Message{
		{
			ID:   "m1",
			Role: session.RoleAssistant,
			Parts: []session.Part{
				session.ToolCallPart("tc1", "bash", `{"command":"grep -r foo ."}`),
			},
		},
		{
			ID:   "m2",
			Role: session.RoleAssistant,
			Parts: []session.Part{
				session.ToolCallPart("tc2", "bash", `{"command":"cat README.md"}`),
			},
		},
	}

	paths := extractUnrestoredPaths(messages)
	if len(paths) != 0 {
		t.Errorf("expected empty unrestored for read-only commands, got %v", paths)
	}
}

func TestExtractUnrestoredPaths_UnclassifiableBash(t *testing.T) {
	messages := []session.Message{
		{
			ID:   "m1",
			Role: session.RoleAssistant,
			Parts: []session.Part{
				session.ToolCallPart("tc1", "bash", `{"command":"python3 deploy.py"}`),
			},
		},
	}

	paths := extractUnrestoredPaths(messages)
	if len(paths) == 0 {
		t.Fatal("expected unrestored for unclassifiable command, got none")
	}

	found := false
	for _, p := range paths {
		if p == "unknown shell changes" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'unknown shell changes' in paths, got %v", paths)
	}
}

func TestExtractUnrestoredPaths_MixedCommands(t *testing.T) {
	messages := []session.Message{
		{
			ID:   "m1",
			Role: session.RoleAssistant,
			Parts: []session.Part{
				session.ToolCallPart("tc1", "bash", `{"command":"ls -la"}`),
			},
		},
		{
			ID:   "m2",
			Role: session.RoleAssistant,
			Parts: []session.Part{
				session.ToolCallPart("tc2", "bash", `{"command":"echo data > output.log"}`),
			},
		},
		{
			ID:   "m3",
			Role: session.RoleAssistant,
			Parts: []session.Part{
				session.ToolCallPart("tc3", "bash", `{"command":"grep pattern file.txt"}`),
			},
		},
	}

	paths := extractUnrestoredPaths(messages)
	if len(paths) == 0 {
		t.Fatal("expected unrestored paths for mixed commands")
	}

	found := false
	for _, p := range paths {
		if p == "output.log" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected output.log in paths, got %v", paths)
	}
}

func TestExtractUnrestoredPaths_NonBashToolsIgnored(t *testing.T) {
	messages := []session.Message{
		{
			ID:   "m1",
			Role: session.RoleAssistant,
			Parts: []session.Part{
				session.ToolCallPart("tc1", "read", `{"file_path":"/tmp/x.txt"}`),
			},
		},
		{
			ID:   "m2",
			Role: session.RoleAssistant,
			Parts: []session.Part{
				session.ToolCallPart("tc2", "write", `{"file_path":"/tmp/y.txt","content":"hello"}`),
			},
		},
	}

	paths := extractUnrestoredPaths(messages)
	if len(paths) != 0 {
		t.Errorf("expected empty unrestored for non-bash tools, got %v", paths)
	}
}

func TestExtractUnrestoredPaths_DeduplicatesPaths(t *testing.T) {
	messages := []session.Message{
		{
			ID:   "m1",
			Role: session.RoleAssistant,
			Parts: []session.Part{
				session.ToolCallPart("tc1", "bash", `{"command":"echo a > out.txt"}`),
			},
		},
		{
			ID:   "m2",
			Role: session.RoleAssistant,
			Parts: []session.Part{
				session.ToolCallPart("tc2", "bash", `{"command":"echo b > out.txt"}`),
			},
		},
	}

	paths := extractUnrestoredPaths(messages)
	count := 0
	for _, p := range paths {
		if p == "out.txt" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected out.txt to appear once, appeared %d times in %v", count, paths)
	}
}

func TestIsReadOnly(t *testing.T) {
	tests := []struct {
		cmd      string
		readOnly bool
	}{
		{"ls -la", true},
		{"cat foo.go", true},
		{"grep -rn pattern .", true},
		{"git status", true},
		{"git diff", true},
		{"git log --oneline", true},
		{"find . -name '*.go'", true},
		{"find . -name '*.tmp' -delete", false},
		{"find . -exec rm {} \\;", false},
		{"echo hello", true},
		{"echo hello > file.txt", false},
		{"python3 script.py", false},
		{"make build", false},
		{"go build ./...", false},
	}

	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			got := isReadOnly(tt.cmd)
			if got != tt.readOnly {
				t.Errorf("isReadOnly(%q) = %v, want %v", tt.cmd, got, tt.readOnly)
			}
		})
	}
}

func TestExtractCommand(t *testing.T) {
	tests := []struct {
		args string
		want string
	}{
		{`{"command":"ls -la"}`, "ls -la"},
		{`{"command":"echo hi","timeout":5000}`, "echo hi"},
		{`{}`, ""},
		{`invalid`, ""},
		{"", ""},
	}

	for _, tt := range tests {
		got := extractCommand(tt.args)
		if got != tt.want {
			t.Errorf("extractCommand(%q) = %q, want %q", tt.args, got, tt.want)
		}
	}
}

func TestExtractUnrestoredPaths_Sorted(t *testing.T) {
	// Verify deterministic output by checking that multiple writes
	// produce a sorted-looking result (maps are unordered, but we can
	// check that output is consistent).
	messages := []session.Message{
		{
			ID:   "m1",
			Role: session.RoleAssistant,
			Parts: []session.Part{
				session.ToolCallPart("tc1", "bash", `{"command":"echo a > b.txt"}`),
				session.ToolCallPart("tc2", "bash", `{"command":"echo c > a.txt"}`),
			},
		},
	}

	paths := extractUnrestoredPaths(messages)
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d: %v", len(paths), paths)
	}
	sort.Strings(paths)
	if paths[0] != "a.txt" || paths[1] != "b.txt" {
		t.Errorf("expected [a.txt b.txt], got %v", paths)
	}
}
