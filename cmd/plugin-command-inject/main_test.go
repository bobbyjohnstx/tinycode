package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

func TestPluginID(t *testing.T) {
	p := newPlugin("")
	if p.ID != "command-inject" {
		t.Errorf("expected plugin ID 'command-inject', got %q", p.ID)
	}
}

func TestPluginEmptyDirNoTools(t *testing.T) {
	p := newPlugin("")
	if len(p.Tools) != 0 {
		t.Errorf("expected 0 tools with empty dir, got %d", len(p.Tools))
	}
}

func TestPluginNonexistentDirNoTools(t *testing.T) {
	p := newPlugin("/nonexistent/path/that/does/not/exist")
	if len(p.Tools) != 0 {
		t.Errorf("expected 0 tools with nonexistent dir, got %d", len(p.Tools))
	}
}

func TestDeriveToolName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello.sh", "hello"},
		{"my-script.py", "my_script"},
		{"test_tool", "test_tool"},
		{"CamelCase.rb", "camelcase"},
		{"dots.in.name.sh", "dots_in_name"},
		{"special!chars@here", "special_chars_here"},
	}
	for _, tt := range tests {
		got := deriveToolName(tt.input)
		if got != tt.want {
			t.Errorf("deriveToolName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExtractDescriptionFromComment(t *testing.T) {
	dir := t.TempDir()

	// Hash comment
	hashScript := filepath.Join(dir, "hash.sh")
	os.WriteFile(hashScript, []byte("#!/bin/bash\n# description: List all pods\necho hello\n"), 0o755)

	desc := extractDescription(hashScript, "hash.sh")
	if desc != "List all pods" {
		t.Errorf("expected 'List all pods', got %q", desc)
	}

	// Slash comment
	slashScript := filepath.Join(dir, "slash.sh")
	os.WriteFile(slashScript, []byte("#!/bin/bash\n// description: Deploy the app\necho deploy\n"), 0o755)

	desc = extractDescription(slashScript, "slash.sh")
	if desc != "Deploy the app" {
		t.Errorf("expected 'Deploy the app', got %q", desc)
	}
}

func TestExtractDescriptionDefault(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "nodesc.sh")
	os.WriteFile(script, []byte("#!/bin/bash\necho hello\n"), 0o755)

	desc := extractDescription(script, "nodesc.sh")
	if desc != "Run nodesc.sh" {
		t.Errorf("expected 'Run nodesc.sh', got %q", desc)
	}
}

func TestExtractDescriptionMissingFile(t *testing.T) {
	desc := extractDescription("/nonexistent/file", "missing.sh")
	if desc != "Run missing.sh" {
		t.Errorf("expected 'Run missing.sh', got %q", desc)
	}
}

func TestExtractDescriptionCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "upper.sh")
	os.WriteFile(script, []byte("# Description: Upper case\n"), 0o755)

	desc := extractDescription(script, "upper.sh")
	if desc != "Upper case" {
		t.Errorf("expected 'Upper case', got %q", desc)
	}
}

func TestDiscoverScripts(t *testing.T) {
	dir := t.TempDir()

	// Create executable file
	execFile := filepath.Join(dir, "run.sh")
	os.WriteFile(execFile, []byte("#!/bin/bash\necho ok\n"), 0o755)

	// Create non-executable file
	noExecFile := filepath.Join(dir, "data.txt")
	os.WriteFile(noExecFile, []byte("just data\n"), 0o644)

	// Create subdirectory (should be skipped)
	os.Mkdir(filepath.Join(dir, "subdir"), 0o755)

	scripts, err := discoverScripts(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scripts) != 1 {
		t.Fatalf("expected 1 script, got %d", len(scripts))
	}
	if scripts[0].Filename != "run.sh" {
		t.Errorf("expected filename 'run.sh', got %q", scripts[0].Filename)
	}
}

func TestDiscoverScriptsEmptyDir(t *testing.T) {
	dir := t.TempDir()
	scripts, err := discoverScripts(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scripts) != 0 {
		t.Errorf("expected 0 scripts, got %d", len(scripts))
	}
}

func TestBuildToolsCreatesCorrectTools(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "hello.sh")
	os.WriteFile(script, []byte("#!/bin/bash\n# description: Say hello\necho hello\n"), 0o755)

	scripts := []scriptInfo{{Path: script, Filename: "hello.sh"}}
	tools := buildTools(scripts)

	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "hello" {
		t.Errorf("expected tool name 'hello', got %q", tools[0].Name)
	}
	if tools[0].Description != "Say hello" {
		t.Errorf("expected description 'Say hello', got %q", tools[0].Description)
	}
	if tools[0].Execute == nil {
		t.Error("expected non-nil Execute function")
	}
}

func TestToolExecution(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "greet.sh")
	os.WriteFile(script, []byte("#!/bin/bash\n# description: Greet someone\necho \"Hello $1\"\n"), 0o755)

	scripts := []scriptInfo{{Path: script, Filename: "greet.sh"}}
	tools := buildTools(scripts)

	ctx := context.Background()
	tc := plugin.ToolContext{SessionID: "test", Directory: dir}

	// Test with no args
	raw := json.RawMessage(`{}`)
	out, err := tools[0].Execute(ctx, raw, tc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "Hello" {
		t.Errorf("expected 'Hello', got %q", out)
	}

	// Test with args
	raw = json.RawMessage(`{"args":"World"}`)
	out, err = tools[0].Execute(ctx, raw, tc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "Hello World" {
		t.Errorf("expected 'Hello World', got %q", out)
	}
}

func TestToolExecutionInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "test.sh")
	os.WriteFile(script, []byte("#!/bin/bash\necho ok\n"), 0o755)

	scripts := []scriptInfo{{Path: script, Filename: "test.sh"}}
	tools := buildTools(scripts)

	ctx := context.Background()
	tc := plugin.ToolContext{SessionID: "test", Directory: dir}
	raw := json.RawMessage(`{invalid}`)
	_, err := tools[0].Execute(ctx, raw, tc)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestToolExecutionFailingScript(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fail.sh")
	os.WriteFile(script, []byte("#!/bin/bash\necho 'oh no' >&2\nexit 42\n"), 0o755)

	scripts := []scriptInfo{{Path: script, Filename: "fail.sh"}}
	tools := buildTools(scripts)

	ctx := context.Background()
	tc := plugin.ToolContext{SessionID: "test", Directory: dir}
	raw := json.RawMessage(`{}`)
	out, err := tools[0].Execute(ctx, raw, tc)
	if err != nil {
		t.Fatalf("expected graceful error handling, got error: %v", err)
	}
	if out == "" {
		t.Error("expected error output, got empty string")
	}
}

func TestToolExecution_ShellMetacharsNotInterpreted(t *testing.T) {
	dir := t.TempDir()
	// Script that echoes its arguments literally, one per line
	script := filepath.Join(dir, "echo_args.sh")
	os.WriteFile(script, []byte("#!/bin/bash\nfor arg in \"$@\"; do echo \"$arg\"; done\n"), 0o755)

	scripts := []scriptInfo{{Path: script, Filename: "echo_args.sh"}}
	tools := buildTools(scripts)

	ctx := context.Background()
	tc := plugin.ToolContext{SessionID: "test", Directory: dir}

	// Pass args with shell metacharacters that would be dangerous if shell-interpreted
	raw := json.RawMessage(`{"args":"hello; echo injected"}`)
	out, err := tools[0].Execute(ctx, raw, tc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// With direct exec (no shell), the semicolon is a literal arg, not a command separator.
	// The script receives ["hello;", "echo", "injected"] as separate args.
	if strings.Contains(out, "injected\n") {
		// If "injected" appeared on its own line, the shell interpreted the semicolon
		t.Errorf("shell metacharacters were interpreted: output = %q", out)
	}
	// The output should contain "hello;" as a literal argument
	if !strings.Contains(out, "hello;") {
		t.Errorf("expected literal 'hello;' in output, got %q", out)
	}
}

func TestPluginWithScripts(t *testing.T) {
	dir := t.TempDir()

	script1 := filepath.Join(dir, "alpha.sh")
	os.WriteFile(script1, []byte("#!/bin/bash\n# description: Alpha tool\necho alpha\n"), 0o755)

	script2 := filepath.Join(dir, "beta.sh")
	os.WriteFile(script2, []byte("#!/bin/bash\n# description: Beta tool\necho beta\n"), 0o755)

	p := newPlugin(dir)
	if len(p.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(p.Tools))
	}
}
