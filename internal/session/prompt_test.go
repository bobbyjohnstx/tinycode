package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode-go/internal/llm"
)

func TestBuildSystemPrompt_AgentPromptOnly(t *testing.T) {
	result := BuildSystemPrompt(SystemPromptInput{
		AgentPrompt: "You are a helpful coding assistant.",
	})

	if result != "You are a helpful coding assistant." {
		t.Errorf("expected agent prompt only, got %q", result)
	}
}

func TestBuildSystemPrompt_WithInstructions(t *testing.T) {
	result := BuildSystemPrompt(SystemPromptInput{
		AgentPrompt:  "You are a helpful coding assistant.",
		Instructions: "Always use Go.",
	})

	if !strings.Contains(result, "You are a helpful coding assistant.") {
		t.Error("expected agent prompt in result")
	}
	if !strings.Contains(result, "Always use Go.") {
		t.Error("expected instructions in result")
	}
	if !strings.Contains(result, "---") {
		t.Error("expected section separator between agent prompt and instructions")
	}

	// Agent prompt should come before instructions
	agentIdx := strings.Index(result, "You are a helpful coding assistant.")
	instrIdx := strings.Index(result, "Always use Go.")
	if agentIdx >= instrIdx {
		t.Error("expected agent prompt before instructions")
	}
}

func TestBuildSystemPrompt_EmptyAgentPrompt(t *testing.T) {
	result := BuildSystemPrompt(SystemPromptInput{
		Instructions: "Always use Go.",
	})

	if result != "Always use Go." {
		t.Errorf("expected instructions only, got %q", result)
	}
}

func TestBuildSystemPrompt_EmptyAll(t *testing.T) {
	result := BuildSystemPrompt(SystemPromptInput{})

	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestBuildSystemPrompt_WithDirectory(t *testing.T) {
	dir := t.TempDir()
	claudeContent := "# Project Rules\nUse tabs for indentation."
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(claudeContent), 0644); err != nil {
		t.Fatalf("writing CLAUDE.md: %v", err)
	}

	result := BuildSystemPrompt(SystemPromptInput{
		AgentPrompt: "You are a coding assistant.",
		Directory:   dir,
	})

	if !strings.Contains(result, "You are a coding assistant.") {
		t.Error("expected agent prompt in result")
	}
	if !strings.Contains(result, "# Project Rules") {
		t.Error("expected CLAUDE.md content in result")
	}
	if !strings.Contains(result, "Use tabs for indentation.") {
		t.Error("expected CLAUDE.md content in result")
	}
}

func TestBuildSystemPrompt_WithTinycodeCLAUDEmd(t *testing.T) {
	dir := t.TempDir()
	tinycodeDir := filepath.Join(dir, ".tinycode")
	if err := os.MkdirAll(tinycodeDir, 0755); err != nil {
		t.Fatalf("creating .tinycode dir: %v", err)
	}
	content := "# Tinycode project instructions"
	if err := os.WriteFile(filepath.Join(tinycodeDir, "CLAUDE.md"), []byte(content), 0644); err != nil {
		t.Fatalf("writing .tinycode/CLAUDE.md: %v", err)
	}

	result := BuildSystemPrompt(SystemPromptInput{
		AgentPrompt: "Agent prompt.",
		Directory:   dir,
	})

	if !strings.Contains(result, "# Tinycode project instructions") {
		t.Error("expected .tinycode/CLAUDE.md content in result")
	}
}

func TestBuildSystemPrompt_DirectoryWithNoClaudeMD(t *testing.T) {
	dir := t.TempDir()

	result := BuildSystemPrompt(SystemPromptInput{
		AgentPrompt: "Agent prompt.",
		Directory:   dir,
	})

	// Should only have agent prompt + environment section
	if !strings.Contains(result, "Agent prompt.") {
		t.Error("expected agent prompt in result")
	}
}

func TestBuildSystemPrompt_AllSections(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("Project context."), 0644); err != nil {
		t.Fatalf("writing CLAUDE.md: %v", err)
	}

	result := BuildSystemPrompt(SystemPromptInput{
		AgentPrompt:  "Agent persona.",
		Instructions: "Extra instructions.",
		Directory:    dir,
	})

	// Verify ordering: agent prompt, then CLAUDE.md, then instructions
	agentIdx := strings.Index(result, "Agent persona.")
	claudeIdx := strings.Index(result, "Project context.")
	instrIdx := strings.Index(result, "Extra instructions.")

	if agentIdx < 0 || claudeIdx < 0 || instrIdx < 0 {
		t.Fatalf("missing section in result: %q", result)
	}
	if agentIdx >= claudeIdx {
		t.Error("expected agent prompt before CLAUDE.md content")
	}
	if claudeIdx >= instrIdx {
		t.Error("expected CLAUDE.md content before instructions")
	}
}

func TestDiscoverClaudeMD_WalkUp(t *testing.T) {
	// Create a nested directory structure:
	// root/CLAUDE.md
	// root/sub/CLAUDE.md
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatalf("creating sub dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("root rules"), 0644); err != nil {
		t.Fatalf("writing root CLAUDE.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sub, "CLAUDE.md"), []byte("sub rules"), 0644); err != nil {
		t.Fatalf("writing sub CLAUDE.md: %v", err)
	}

	files := discoverClaudeMD(sub)

	if len(files) < 2 {
		t.Fatalf("expected at least 2 CLAUDE.md files, got %d: %v", len(files), files)
	}

	// Outermost should come first (root before sub)
	foundRoot := -1
	foundSub := -1
	for i, f := range files {
		if f == filepath.Join(root, "CLAUDE.md") {
			foundRoot = i
		}
		if f == filepath.Join(sub, "CLAUDE.md") {
			foundSub = i
		}
	}

	if foundRoot < 0 {
		t.Error("expected root CLAUDE.md in results")
	}
	if foundSub < 0 {
		t.Error("expected sub CLAUDE.md in results")
	}
	if foundRoot >= foundSub {
		t.Error("expected root CLAUDE.md before sub CLAUDE.md (outermost first)")
	}
}

// --- Issue #121 tests ---

func TestDiscoverInstructionFiles_IncludesAgentsMD(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("claude"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("agents"), 0644); err != nil {
		t.Fatal(err)
	}

	files := discoverInstructionFiles(dir)

	foundClaude := false
	foundAgents := false
	for _, f := range files {
		if strings.HasSuffix(f, "CLAUDE.md") {
			foundClaude = true
		}
		if strings.HasSuffix(f, "AGENTS.md") {
			foundAgents = true
		}
	}
	if !foundClaude {
		t.Error("expected CLAUDE.md in instruction files")
	}
	if !foundAgents {
		t.Error("expected AGENTS.md in instruction files")
	}
}

func TestBuildSystemPrompt_WithToolDefs(t *testing.T) {
	tools := []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{Name: "read", Description: "Read a file"}},
		{Type: "function", Function: llm.ToolFunction{Name: "write", Description: "Write a file"}},
	}

	result := BuildSystemPrompt(SystemPromptInput{
		AgentPrompt: "Agent.",
		ToolDefs:    tools,
	})

	if !strings.Contains(result, "# Available Tools") {
		t.Error("expected tools section header")
	}
	if !strings.Contains(result, "**read**: Read a file") {
		t.Error("expected read tool in output")
	}
	if !strings.Contains(result, "**write**: Write a file") {
		t.Error("expected write tool in output")
	}
}

func TestBuildSystemPrompt_WithEnvironment(t *testing.T) {
	dir := t.TempDir()
	result := BuildSystemPrompt(SystemPromptInput{
		AgentPrompt: "Agent.",
		Directory:   dir,
		Platform:    "linux",
		GitBranch:   "main",
	})

	if !strings.Contains(result, "# Environment") {
		t.Error("expected environment section")
	}
	if !strings.Contains(result, "Platform: linux") {
		t.Error("expected platform in environment")
	}
	if !strings.Contains(result, "Git branch: main") {
		t.Error("expected git branch in environment")
	}
	if !strings.Contains(result, "Working directory:") {
		t.Error("expected working directory in environment")
	}
}

func TestBuildEnvironmentSection_Empty(t *testing.T) {
	result := buildEnvironmentSection(SystemPromptInput{})
	if result != "" {
		t.Errorf("expected empty environment section, got %q", result)
	}
}

func TestBuildToolSection(t *testing.T) {
	tools := []llm.Tool{
		{Function: llm.ToolFunction{Name: "grep"}},
	}
	result := buildToolSection(tools)
	if !strings.Contains(result, "**grep**") {
		t.Error("expected tool name in section")
	}
	// No description means no colon
	if strings.Contains(result, ":") {
		t.Error("expected no colon for tool without description")
	}
}
