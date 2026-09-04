package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

	if result != "Agent prompt." {
		t.Errorf("expected agent prompt only when no CLAUDE.md found, got %q", result)
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
