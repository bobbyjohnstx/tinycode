package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/llm"
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

	if strings.Contains(result, "Read a file") || strings.Contains(result, "**read**") {
		t.Errorf("tool schemas are sent separately; prompt should not list them:\n%s", result)
	}
}

func TestBuildSystemPrompt_LSPHintWithoutToolList(t *testing.T) {
	tools := []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{Name: "read", Description: "Read a file"}},
		{Type: "function", Function: llm.ToolFunction{Name: "lsp_diagnostics", Description: "Show diagnostics"}},
	}

	result := BuildSystemPrompt(SystemPromptInput{
		AgentPrompt: "Agent.",
		ToolDefs:    tools,
	})

	if !strings.Contains(result, "lsp_diagnostics") {
		t.Error("expected the LSP usage hint")
	}
	if strings.Contains(result, "Read a file") || strings.Contains(result, "# Available Tools") {
		t.Errorf("expected no tool inventory, got:\n%s", result)
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
	if result != "" {
		t.Errorf("expected no tool section without LSP tools, got %q", result)
	}
}

// --- Issue #951: scoped instruction blocks ---

func TestBuildSystemPrompt_LabeledBlocks(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("root rules"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "CLAUDE.md"), []byte("sub rules"), 0644); err != nil {
		t.Fatal(err)
	}

	result := BuildSystemPrompt(SystemPromptInput{Directory: sub})

	// Blocks should have relative path headers
	if !strings.Contains(result, "[../CLAUDE.md]") {
		t.Errorf("expected [../CLAUDE.md] header, got:\n%s", result)
	}
	if !strings.Contains(result, "[CLAUDE.md]") {
		t.Errorf("expected [CLAUDE.md] header, got:\n%s", result)
	}
	// Content should follow headers
	if !strings.Contains(result, "[../CLAUDE.md]\nroot rules") {
		t.Errorf("expected root content after header, got:\n%s", result)
	}
	if !strings.Contains(result, "[CLAUDE.md]\nsub rules") {
		t.Errorf("expected sub content after header, got:\n%s", result)
	}
	// Outermost first
	rootIdx := strings.Index(result, "[../CLAUDE.md]")
	subIdx := strings.Index(result, "[CLAUDE.md]\nsub")
	if rootIdx >= subIdx {
		t.Error("expected outermost block before innermost")
	}
}

func TestDiscoverInstructionFiles_OverrideReplacesAgents(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("claude"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("agents original"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.override.md"), []byte("agents override"), 0644); err != nil {
		t.Fatal(err)
	}

	files := discoverInstructionFiles(dir)

	foundOverride := false
	foundAgents := false
	for _, f := range files {
		if strings.HasSuffix(f, "AGENTS.override.md") {
			foundOverride = true
		}
		if strings.HasSuffix(f, "AGENTS.md") && !strings.HasSuffix(f, "override.md") {
			foundAgents = true
		}
	}
	if !foundOverride {
		t.Error("expected AGENTS.override.md in discovered files")
	}
	if foundAgents {
		t.Error("AGENTS.md should be replaced by AGENTS.override.md")
	}
}

func TestBuildSystemPrompt_OverrideContent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("original agents"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.override.md"), []byte("override agents"), 0644); err != nil {
		t.Fatal(err)
	}

	result := BuildSystemPrompt(SystemPromptInput{Directory: dir})

	if strings.Contains(result, "original agents") {
		t.Error("AGENTS.md content should not appear when override exists")
	}
	if !strings.Contains(result, "override agents") {
		t.Error("expected AGENTS.override.md content in prompt")
	}
}

func TestBuildSystemPrompt_OverrideDoesNotAffectParent(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	// Parent has AGENTS.md (no override)
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("parent agents"), 0644); err != nil {
		t.Fatal(err)
	}
	// Child has override
	if err := os.WriteFile(filepath.Join(sub, "AGENTS.md"), []byte("child agents"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "AGENTS.override.md"), []byte("child override"), 0644); err != nil {
		t.Fatal(err)
	}

	result := BuildSystemPrompt(SystemPromptInput{Directory: sub})

	if !strings.Contains(result, "parent agents") {
		t.Error("parent AGENTS.md should still appear")
	}
	if strings.Contains(result, "child agents") {
		t.Error("child AGENTS.md should be replaced by override")
	}
	if !strings.Contains(result, "child override") {
		t.Error("expected child AGENTS.override.md content")
	}
}

func TestBuildSystemPrompt_CapDropsOutermost(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}

	// Outer file: large enough that outer + inner > cap
	outerContent := strings.Repeat("x", maxInstructionBytes)
	innerContent := "inner rules"
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte(outerContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "CLAUDE.md"), []byte(innerContent), 0644); err != nil {
		t.Fatal(err)
	}

	result := BuildSystemPrompt(SystemPromptInput{Directory: sub})

	if strings.Contains(result, outerContent) {
		t.Error("outer file should be dropped when over cap")
	}
	if !strings.Contains(result, innerContent) {
		t.Error("inner file should be kept")
	}
	if !strings.Contains(result, "1 instruction files omitted due to size limit") {
		t.Error("expected omission note")
	}
}

func TestBuildSystemPrompt_CapCountsAppendSystemPrompt(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}

	// Outer file is small, but outer + inner + append > cap
	outerContent := strings.Repeat("o", 20000)
	innerContent := strings.Repeat("i", 10000)
	appendText := strings.Repeat("a", 10000)
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte(outerContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "CLAUDE.md"), []byte(innerContent), 0644); err != nil {
		t.Fatal(err)
	}

	result := BuildSystemPrompt(SystemPromptInput{
		Directory:          sub,
		AppendSystemPrompt: appendText,
	})

	// outer (20k) + inner (10k) + append (10k) = 40k > 32k
	// Drop outer: inner (10k) + append (10k) = 20k < 32k
	if strings.Contains(result, outerContent) {
		t.Error("outer file should be dropped when total (with append) exceeds cap")
	}
	if !strings.Contains(result, innerContent) {
		t.Error("inner file should be kept")
	}
	if !strings.Contains(result, appendText) {
		t.Error("AppendSystemPrompt should still be present")
	}
}

func TestBuildSystemPrompt_UnreadableFileSkipped(t *testing.T) {
	dir := t.TempDir()
	unreadable := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(unreadable, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(unreadable, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(unreadable, 0644) })

	// Should not panic or include unreadable content
	result := BuildSystemPrompt(SystemPromptInput{Directory: dir})

	if strings.Contains(result, "secret") {
		t.Error("unreadable file content should not appear")
	}
}

func TestDiscoverClaudeMD_ReturnsOverrideFiles(t *testing.T) {
	dir := t.TempDir()
	tinycodeDir := filepath.Join(dir, ".tinycode")
	if err := os.MkdirAll(tinycodeDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("claude"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.override.md"), []byte("override"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tinycodeDir, "CLAUDE.md"), []byte("tinycode"), 0644); err != nil {
		t.Fatal(err)
	}

	files := discoverClaudeMD(dir)

	foundClaude := false
	foundOverride := false
	foundTinycode := false
	for _, f := range files {
		switch {
		case strings.HasSuffix(f, filepath.Join(dir, "CLAUDE.md")):
			foundClaude = true
		case strings.HasSuffix(f, "AGENTS.override.md"):
			foundOverride = true
		case strings.HasSuffix(f, filepath.Join(".tinycode", "CLAUDE.md")):
			foundTinycode = true
		}
	}
	if !foundClaude {
		t.Errorf("expected CLAUDE.md in results: %v", files)
	}
	if !foundOverride {
		t.Errorf("expected AGENTS.override.md in results: %v", files)
	}
	if !foundTinycode {
		t.Errorf("expected .tinycode/CLAUDE.md in results: %v", files)
	}
}

func TestBuildSystemPrompt_LargeFileNotTruncated(t *testing.T) {
	dir := t.TempDir()
	// Single file larger than cap — should be kept whole, not split
	largeContent := strings.Repeat("L", maxInstructionBytes+5000)
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(largeContent), 0644); err != nil {
		t.Fatal(err)
	}

	result := BuildSystemPrompt(SystemPromptInput{Directory: dir})

	if !strings.Contains(result, largeContent) {
		t.Error("single file larger than cap should be kept whole, not truncated")
	}
	if strings.Contains(result, "omitted due to size limit") {
		t.Error("should not omit the only file even if over cap")
	}
}

// --- Issue #958: plan-mode reminder survives compaction ---

func TestBuildSystemPrompt_PlanAgentIncludesReminder(t *testing.T) {
	result := BuildSystemPrompt(SystemPromptInput{
		AgentPrompt: "You are Plan.",
		AgentName:   "plan",
	})

	expected := "You are in plan mode. Do not modify files outside plans/ and drafts/. If the user asks to implement, tell them to switch agents."
	if !strings.Contains(result, expected) {
		t.Errorf("expected plan-mode reminder in prompt, got:\n%s", result)
	}
}

func TestBuildSystemPrompt_NonPlanAgentNoReminder(t *testing.T) {
	result := BuildSystemPrompt(SystemPromptInput{
		AgentPrompt: "You are Build.",
		AgentName:   "build",
	})

	if strings.Contains(result, "plan mode") {
		t.Errorf("non-plan agent should not contain plan-mode reminder, got:\n%s", result)
	}
}

func TestFormatInstructionInfo(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("project rules"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("agent config"), 0644); err != nil {
		t.Fatal(err)
	}

	info := FormatInstructionInfo(dir)

	if !strings.Contains(info, "INSTRUCTION BLOCKS") {
		t.Error("expected header")
	}
	if !strings.Contains(info, "2 files") {
		t.Errorf("expected 2 files in header, got:\n%s", info)
	}
	if !strings.Contains(info, "CLAUDE.md") {
		t.Error("expected CLAUDE.md path in listing")
	}
	if !strings.Contains(info, "AGENTS.md") {
		t.Error("expected AGENTS.md path in listing")
	}
	if !strings.Contains(info, "bytes") {
		t.Error("expected byte sizes in listing")
	}
}

// --- Issue #967: @path imports in instruction files ---

func TestExpandImports_BasicInclude(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "CLAUDE.md")
	other := filepath.Join(dir, "other.md")
	if err := os.WriteFile(main, []byte("rules\n@other.md"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("included content"), 0644); err != nil {
		t.Fatal(err)
	}

	result := BuildSystemPrompt(SystemPromptInput{Directory: dir})

	if !strings.Contains(result, "rules") {
		t.Error("expected original content")
	}
	if !strings.Contains(result, "included content") {
		t.Error("expected included file content")
	}
	if strings.Contains(result, "@other.md") {
		t.Error("@other.md directive should be replaced by included content")
	}
}

func TestExpandImports_FencedCodeBlock(t *testing.T) {
	dir := t.TempDir()
	content := "rules\n```\n@secret.md\n```\n@other.md"
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "secret.md"), []byte("SHOULD NOT APPEAR"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "other.md"), []byte("included"), 0644); err != nil {
		t.Fatal(err)
	}

	result := BuildSystemPrompt(SystemPromptInput{Directory: dir})

	if !strings.Contains(result, "@secret.md") {
		t.Error("@path inside fenced code block should be left as text")
	}
	if strings.Contains(result, "SHOULD NOT APPEAR") {
		t.Error("file inside fenced code block should not be included")
	}
	if !strings.Contains(result, "included") {
		t.Error("@other.md outside code block should be expanded")
	}
}

func TestExpandImports_InlineBackticks(t *testing.T) {
	dir := t.TempDir()
	content := "Use `@config.md` for settings\n@other.md"
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.md"), []byte("SHOULD NOT APPEAR"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "other.md"), []byte("included"), 0644); err != nil {
		t.Fatal(err)
	}

	result := BuildSystemPrompt(SystemPromptInput{Directory: dir})

	if !strings.Contains(result, "`@config.md`") {
		t.Error("@path inside backticks should be left as text")
	}
	if strings.Contains(result, "SHOULD NOT APPEAR") {
		t.Error("file referenced inside backticks should not be included")
	}
	if !strings.Contains(result, "included") {
		t.Error("@other.md outside backticks should be expanded")
	}
}

func TestExpandImports_DepthLimit(t *testing.T) {
	dir := t.TempDir()
	// Create a chain: a→b→c→d→e (5 @path directives, depth 0→1→2→3→try 4)
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("@a.md"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("level-a\n@b.md"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.md"), []byte("level-b\n@c.md"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "c.md"), []byte("level-c\n@d.md"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "d.md"), []byte("level-d\n@e.md"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "e.md"), []byte("level-e"), 0644); err != nil {
		t.Fatal(err)
	}

	result := BuildSystemPrompt(SystemPromptInput{Directory: dir})

	// Levels a through d should be included (depths 1–3 for the include site)
	for _, level := range []string{"level-a", "level-b", "level-c"} {
		if !strings.Contains(result, level) {
			t.Errorf("expected %q in result", level)
		}
	}
	// d.md is expanded at depth 3; its @e.md directive is at depth 4 → skipped
	if !strings.Contains(result, "level-d") {
		t.Error("expected level-d in result")
	}
	if strings.Contains(result, "level-e") {
		t.Error("level-e should be skipped (depth limit)")
	}
	if !strings.Contains(result, "[import skipped (depth limit): e.md]") {
		t.Errorf("expected depth-limit skip message, got:\n%s", result)
	}
}

func TestExpandImports_Cycle(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("start\n@a.md"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("file-a\n@b.md"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.md"), []byte("file-b\n@a.md"), 0644); err != nil {
		t.Fatal(err)
	}

	result := BuildSystemPrompt(SystemPromptInput{Directory: dir})

	if !strings.Contains(result, "file-a") {
		t.Error("expected a.md content")
	}
	if !strings.Contains(result, "file-b") {
		t.Error("expected b.md content")
	}
	if !strings.Contains(result, "[import cycle: a.md]") {
		t.Errorf("expected cycle message, got:\n%s", result)
	}
}

func TestExpandImports_RelativeToContainingFile(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	// CLAUDE.md at root references sub/rules.md
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("@sub/rules.md"), 0644); err != nil {
		t.Fatal(err)
	}
	// sub/rules.md references sibling.md (relative to sub/, not to dir)
	if err := os.WriteFile(filepath.Join(sub, "rules.md"), []byte("sub-rules\n@sibling.md"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "sibling.md"), []byte("sibling content"), 0644); err != nil {
		t.Fatal(err)
	}

	result := BuildSystemPrompt(SystemPromptInput{Directory: dir})

	if !strings.Contains(result, "sub-rules") {
		t.Error("expected sub/rules.md content")
	}
	if !strings.Contains(result, "sibling content") {
		t.Error("expected sibling.md resolved relative to containing file")
	}
}

// --- Issue #967: AGENTS.local.md / CLAUDE.local.md ---

func TestDiscoverInstructionFiles_LocalFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"CLAUDE.md", "CLAUDE.local.md", "AGENTS.md", "AGENTS.local.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name+" content"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	files := discoverInstructionFiles(dir)

	// Find indices of each file in the discovery list
	idxClaude := -1
	idxClaudeLocal := -1
	idxAgents := -1
	idxAgentsLocal := -1
	for i, f := range files {
		base := filepath.Base(f)
		switch base {
		case "CLAUDE.md":
			idxClaude = i
		case "CLAUDE.local.md":
			idxClaudeLocal = i
		case "AGENTS.md":
			idxAgents = i
		case "AGENTS.local.md":
			idxAgentsLocal = i
		}
	}

	if idxClaude < 0 {
		t.Fatal("expected CLAUDE.md in results")
	}
	if idxClaudeLocal < 0 {
		t.Fatal("expected CLAUDE.local.md in results")
	}
	if idxAgents < 0 {
		t.Fatal("expected AGENTS.md in results")
	}
	if idxAgentsLocal < 0 {
		t.Fatal("expected AGENTS.local.md in results")
	}

	// Local files appear AFTER shared files
	if idxClaudeLocal <= idxClaude {
		t.Errorf("CLAUDE.local.md (idx %d) should appear after CLAUDE.md (idx %d)", idxClaudeLocal, idxClaude)
	}
	if idxAgentsLocal <= idxAgents {
		t.Errorf("AGENTS.local.md (idx %d) should appear after AGENTS.md (idx %d)", idxAgentsLocal, idxAgents)
	}
}

func TestDiscoverInstructionFiles_LocalWithoutShared(t *testing.T) {
	dir := t.TempDir()
	// Only local file, no shared file
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.local.md"), []byte("local only"), 0644); err != nil {
		t.Fatal(err)
	}

	files := discoverInstructionFiles(dir)

	found := false
	for _, f := range files {
		if filepath.Base(f) == "AGENTS.local.md" {
			found = true
		}
	}
	if !found {
		t.Error("AGENTS.local.md should be discovered even without AGENTS.md")
	}
}

func TestBuildSystemPrompt_LocalAfterShared(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("shared agents"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.local.md"), []byte("local agents"), 0644); err != nil {
		t.Fatal(err)
	}

	result := BuildSystemPrompt(SystemPromptInput{Directory: dir})

	if !strings.Contains(result, "shared agents") {
		t.Error("shared AGENTS.md content should appear")
	}
	if !strings.Contains(result, "local agents") {
		t.Error("AGENTS.local.md content should appear")
	}
	// Local content should come after shared content
	sharedIdx := strings.Index(result, "shared agents")
	localIdx := strings.Index(result, "local agents")
	if localIdx <= sharedIdx {
		t.Error("local content should appear after shared content")
	}
}

// --- Issue #968: model-written notes ---

func TestBuildSystemPrompt_NotesPresent(t *testing.T) {
	notesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(notesDir, "MEMORY.md"), []byte("- remember this\n- and this"), 0644); err != nil {
		t.Fatal(err)
	}

	result := BuildSystemPrompt(SystemPromptInput{
		AgentPrompt:  "Agent.",
		NotesDir:     notesDir,
		NotesEnabled: true,
	})

	if !strings.Contains(result, "Notes from earlier sessions") {
		t.Error("expected notes heading in prompt")
	}
	if !strings.Contains(result, "remember this") {
		t.Error("expected notes content in prompt")
	}
}

func TestBuildSystemPrompt_NotesDisabled(t *testing.T) {
	notesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(notesDir, "MEMORY.md"), []byte("- secret note"), 0644); err != nil {
		t.Fatal(err)
	}

	result := BuildSystemPrompt(SystemPromptInput{
		AgentPrompt:  "Agent.",
		NotesDir:     notesDir,
		NotesEnabled: false,
	})

	if strings.Contains(result, "secret note") {
		t.Error("notes should not appear when disabled")
	}
	if strings.Contains(result, "Notes from earlier sessions") {
		t.Error("notes heading should not appear when disabled")
	}
}

func TestBuildSystemPrompt_NotesMissing(t *testing.T) {
	notesDir := t.TempDir()
	// No MEMORY.md written

	result := BuildSystemPrompt(SystemPromptInput{
		AgentPrompt:  "Agent.",
		NotesDir:     notesDir,
		NotesEnabled: true,
	})

	if strings.Contains(result, "Notes from earlier sessions") {
		t.Error("notes heading should not appear when MEMORY.md doesn't exist")
	}
}

func TestBuildSystemPrompt_NotesLineCap(t *testing.T) {
	notesDir := t.TempDir()

	// Write 250 lines; only first 200 should appear
	var lines []string
	for i := 1; i <= 250; i++ {
		lines = append(lines, fmt.Sprintf("line-%03d", i))
	}
	if err := os.WriteFile(filepath.Join(notesDir, "MEMORY.md"), []byte(strings.Join(lines, "\n")), 0644); err != nil {
		t.Fatal(err)
	}

	result := BuildSystemPrompt(SystemPromptInput{
		AgentPrompt:  "Agent.",
		NotesDir:     notesDir,
		NotesEnabled: true,
	})

	if !strings.Contains(result, "line-001") {
		t.Error("expected first line in prompt")
	}
	if !strings.Contains(result, "line-200") {
		t.Error("expected line 200 in prompt")
	}
	if strings.Contains(result, "line-201") {
		t.Error("line 201 should be absent (line cap)")
	}
}

func TestBuildSystemPrompt_SubagentNoNotes(t *testing.T) {
	notesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(notesDir, "MEMORY.md"), []byte("- parent note"), 0644); err != nil {
		t.Fatal(err)
	}

	// Subagent prompt: NotesEnabled is false
	result := BuildSystemPrompt(SystemPromptInput{
		AgentPrompt:  "Agent.",
		NotesDir:     notesDir,
		NotesEnabled: false,
	})

	if strings.Contains(result, "parent note") {
		t.Error("subagent should not receive notes")
	}
}
