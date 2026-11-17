package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMatchAndAttach_MatchesGlob(t *testing.T) {
	sr := NewSafeScopedRules([]ScopedRule{
		{Path: ".tinycode/rules/testing.md", Content: "Use table-driven tests.", Globs: []string{"*_test.go"}},
	})
	projectDir := "/project"

	result := sr.MatchAndAttach("/project/foo_test.go", projectDir)
	if !strings.Contains(result, "Use table-driven tests.") {
		t.Errorf("expected rule content on match, got %q", result)
	}
	if !strings.Contains(result, "[Rule from .tinycode/rules/testing.md]") {
		t.Errorf("expected rule header, got %q", result)
	}
}

func TestMatchAndAttach_NoMatch(t *testing.T) {
	sr := NewSafeScopedRules([]ScopedRule{
		{Path: ".tinycode/rules/testing.md", Content: "Use table-driven tests.", Globs: []string{"*_test.go"}},
	})

	result := sr.MatchAndAttach("/project/foo.go", "/project")
	if result != "" {
		t.Errorf("expected empty result for non-matching path, got %q", result)
	}
}

func TestMatchAndAttach_SecondCallNotReattached(t *testing.T) {
	sr := NewSafeScopedRules([]ScopedRule{
		{Path: "testing.md", Content: "Test rule.", Globs: []string{"*_test.go"}},
	})

	result1 := sr.MatchAndAttach("/project/foo_test.go", "/project")
	if result1 == "" {
		t.Fatal("expected match on first call")
	}

	result2 := sr.MatchAndAttach("/project/bar_test.go", "/project")
	if result2 != "" {
		t.Errorf("rule should not re-attach on second call, got %q", result2)
	}
}

func TestMatchAndAttach_OutsideProject(t *testing.T) {
	sr := NewSafeScopedRules([]ScopedRule{
		{Path: "testing.md", Content: "Test rule.", Globs: []string{"*_test.go"}},
	})

	result := sr.MatchAndAttach("/other/foo_test.go", "/project")
	if result != "" {
		t.Errorf("expected no match for file outside project, got %q", result)
	}
}

func TestMatchAndAttach_NilSafe(t *testing.T) {
	var sr *SafeScopedRules
	result := sr.MatchAndAttach("/project/foo.go", "/project")
	if result != "" {
		t.Errorf("expected empty result for nil, got %q", result)
	}
}

func TestMatchAndAttach_MultipleRules(t *testing.T) {
	sr := NewSafeScopedRules([]ScopedRule{
		{Path: "a-rule.md", Content: "Rule A.", Globs: []string{"*.go"}},
		{Path: "b-rule.md", Content: "Rule B.", Globs: []string{"*.go"}},
	})

	result := sr.MatchAndAttach("/project/main.go", "/project")
	if !strings.Contains(result, "Rule A.") {
		t.Error("expected Rule A in result")
	}
	if !strings.Contains(result, "Rule B.") {
		t.Error("expected Rule B in result")
	}
}

func TestMatchesAnyGlob_BaseName(t *testing.T) {
	if !matchesAnyGlob("internal/foo_test.go", []string{"*_test.go"}) {
		t.Error("expected *_test.go to match via base name")
	}
}

func TestMatchesAnyGlob_FullPath(t *testing.T) {
	if matchesAnyGlob("internal/foo.go", []string{"*_test.go"}) {
		t.Error("expected no match for foo.go against *_test.go")
	}
}

func TestExecuteRead_ScopedRuleAttached(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "foo_test.go")
	if err := os.WriteFile(testFile, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	sr := NewSafeScopedRules([]ScopedRule{
		{Path: ".tinycode/rules/testing.md", Content: "Use table-driven tests.", Globs: []string{"*_test.go"}},
	})

	tc := &Context{
		Directory:              dir,
		ReadFiles:              NewSafeReadFiles(),
		DiscoveredInstructions: NewSafeReadFiles(),
		ScopedRules:            sr,
	}
	raw, _ := json.Marshal(readArgs{FilePath: testFile})

	result, err := executeRead(context.Background(), tc, raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Use table-driven tests.") {
		t.Errorf("expected scoped rule in output, got:\n%s", result.Output)
	}
	if !strings.Contains(result.Output, "package main") {
		t.Errorf("expected file content in output, got:\n%s", result.Output)
	}
}

func TestExecuteRead_ScopedRuleNotAttachedForNonMatch(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "foo.go")
	if err := os.WriteFile(goFile, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	sr := NewSafeScopedRules([]ScopedRule{
		{Path: ".tinycode/rules/testing.md", Content: "Test rule.", Globs: []string{"*_test.go"}},
	})

	tc := &Context{
		Directory:              dir,
		ReadFiles:              NewSafeReadFiles(),
		DiscoveredInstructions: NewSafeReadFiles(),
		ScopedRules:            sr,
	}
	raw, _ := json.Marshal(readArgs{FilePath: goFile})

	result, err := executeRead(context.Background(), tc, raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(result.Output, "Test rule.") {
		t.Errorf("test rule should NOT appear for non-test file")
	}
}

func TestExecuteRead_ScopedRuleNotReattachedOnSecondRead(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "foo_test.go")
	if err := os.WriteFile(testFile, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	sr := NewSafeScopedRules([]ScopedRule{
		{Path: "testing.md", Content: "Test rule.", Globs: []string{"*_test.go"}},
	})

	tc := &Context{
		Directory:              dir,
		ReadFiles:              NewSafeReadFiles(),
		DiscoveredInstructions: NewSafeReadFiles(),
		ScopedRules:            sr,
	}
	raw, _ := json.Marshal(readArgs{FilePath: testFile})

	// First read: rule attached.
	result1, err := executeRead(context.Background(), tc, raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result1.Output, "Test rule.") {
		t.Fatal("expected rule on first read")
	}

	// Second read: rule NOT re-attached.
	result2, err := executeRead(context.Background(), tc, raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(result2.Output, "Test rule.") {
		t.Error("rule should not re-attach on second read")
	}
}

func TestExecuteRead_NilScopedRules(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "foo.go")
	if err := os.WriteFile(goFile, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	tc := &Context{
		Directory:              dir,
		ReadFiles:              NewSafeReadFiles(),
		DiscoveredInstructions: NewSafeReadFiles(),
		ScopedRules:            nil, // no scoped rules
	}
	raw, _ := json.Marshal(readArgs{FilePath: goFile})

	result, err := executeRead(context.Background(), tc, raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "package main") {
		t.Errorf("expected file content, got:\n%s", result.Output)
	}
}
