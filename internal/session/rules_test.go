package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverRules_ProjectRuleWithoutPaths(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, ".tinycode", "rules")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "style.md"), []byte("Always use tabs."), 0644); err != nil {
		t.Fatal(err)
	}

	rules := DiscoverRules(dir, "")

	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].Content != "Always use tabs." {
		t.Errorf("content = %q, want %q", rules[0].Content, "Always use tabs.")
	}
	if len(rules[0].Globs) != 0 {
		t.Errorf("expected no globs, got %v", rules[0].Globs)
	}
}

func TestDiscoverRules_ProjectRuleWithPaths(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, ".tinycode", "rules")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\npaths:\n  - \"*_test.go\"\n---\nTest files must use table-driven tests."
	if err := os.WriteFile(filepath.Join(rulesDir, "testing.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	rules := DiscoverRules(dir, "")

	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].Content != "Test files must use table-driven tests." {
		t.Errorf("content = %q, want %q", rules[0].Content, "Test files must use table-driven tests.")
	}
	if len(rules[0].Globs) != 1 || rules[0].Globs[0] != "*_test.go" {
		t.Errorf("globs = %v, want [*_test.go]", rules[0].Globs)
	}
}

func TestDiscoverRules_UserLevelRules(t *testing.T) {
	projectDir := t.TempDir()
	configDir := t.TempDir()
	userRulesDir := filepath.Join(configDir, "rules")
	if err := os.MkdirAll(userRulesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userRulesDir, "global.md"), []byte("Global rule."), 0644); err != nil {
		t.Fatal(err)
	}

	rules := DiscoverRules(projectDir, configDir)

	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].Content != "Global rule." {
		t.Errorf("content = %q, want %q", rules[0].Content, "Global rule.")
	}
}

func TestDiscoverRules_UserAndProjectBothLoad(t *testing.T) {
	projectDir := t.TempDir()
	configDir := t.TempDir()

	// User-level rule
	userRulesDir := filepath.Join(configDir, "rules")
	if err := os.MkdirAll(userRulesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userRulesDir, "user.md"), []byte("User rule."), 0644); err != nil {
		t.Fatal(err)
	}

	// Project-level rule
	projRulesDir := filepath.Join(projectDir, ".tinycode", "rules")
	if err := os.MkdirAll(projRulesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projRulesDir, "project.md"), []byte("Project rule."), 0644); err != nil {
		t.Fatal(err)
	}

	rules := DiscoverRules(projectDir, configDir)

	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	// User rules come first (outermost)
	if rules[0].Content != "User rule." {
		t.Errorf("first rule content = %q, want %q", rules[0].Content, "User rule.")
	}
	if rules[1].Content != "Project rule." {
		t.Errorf("second rule content = %q, want %q", rules[1].Content, "Project rule.")
	}
}

func TestDiscoverRules_AlphabeticalOrder(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, ".tinycode", "rules")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"c-rule.md", "a-rule.md", "b-rule.md"} {
		if err := os.WriteFile(filepath.Join(rulesDir, name), []byte("Rule: "+name), 0644); err != nil {
			t.Fatal(err)
		}
	}

	rules := DiscoverRules(dir, "")

	if len(rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(rules))
	}
	if rules[0].Content != "Rule: a-rule.md" {
		t.Errorf("first = %q, want alphabetical first", rules[0].Content)
	}
	if rules[1].Content != "Rule: b-rule.md" {
		t.Errorf("second = %q", rules[1].Content)
	}
	if rules[2].Content != "Rule: c-rule.md" {
		t.Errorf("third = %q", rules[2].Content)
	}
}

func TestDiscoverRules_EmptyBodySkipped(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, ".tinycode", "rules")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		t.Fatal(err)
	}
	// File with frontmatter but empty body
	if err := os.WriteFile(filepath.Join(rulesDir, "empty.md"), []byte("---\npaths:\n  - \"*.go\"\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}

	rules := DiscoverRules(dir, "")

	if len(rules) != 0 {
		t.Errorf("expected 0 rules (empty body), got %d", len(rules))
	}
}

func TestDiscoverRules_NonMDSkipped(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, ".tinycode", "rules")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "notes.txt"), []byte("Not a rule."), 0644); err != nil {
		t.Fatal(err)
	}

	rules := DiscoverRules(dir, "")

	if len(rules) != 0 {
		t.Errorf("expected 0 rules (non-.md), got %d", len(rules))
	}
}

func TestDiscoverRules_NoRulesDir(t *testing.T) {
	dir := t.TempDir()

	rules := DiscoverRules(dir, "")

	if len(rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(rules))
	}
}

func TestBuildSystemPrompt_UnscopedRuleInPrompt(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, ".tinycode", "rules")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "style.md"), []byte("Use tabs."), 0644); err != nil {
		t.Fatal(err)
	}

	result := BuildSystemPrompt(SystemPromptInput{
		AgentPrompt: "Agent.",
		Directory:   dir,
	})

	if !strings.Contains(result, "Use tabs.") {
		t.Errorf("expected unscoped rule in prompt, got:\n%s", result)
	}
}

func TestBuildSystemPrompt_ScopedRuleNotInPrompt(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, ".tinycode", "rules")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\npaths:\n  - \"*_test.go\"\n---\nTest rule."
	if err := os.WriteFile(filepath.Join(rulesDir, "testing.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result := BuildSystemPrompt(SystemPromptInput{
		AgentPrompt: "Agent.",
		Directory:   dir,
	})

	if strings.Contains(result, "Test rule.") {
		t.Errorf("scoped rule should NOT appear in initial prompt, got:\n%s", result)
	}
}

func TestDiscoverRules_MultiplePathGlobs(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, ".tinycode", "rules")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\npaths:\n  - \"*_test.go\"\n  - \"*.spec.ts\"\n---\nTest files rule."
	if err := os.WriteFile(filepath.Join(rulesDir, "testing.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	rules := DiscoverRules(dir, "")

	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if len(rules[0].Globs) != 2 {
		t.Errorf("expected 2 globs, got %v", rules[0].Globs)
	}
}
