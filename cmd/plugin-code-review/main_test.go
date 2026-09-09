package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

func TestPluginID(t *testing.T) {
	p := newPlugin()
	if p.ID != "code-review" {
		t.Errorf("expected plugin ID 'code-review', got %q", p.ID)
	}
}

func TestToolDefinition(t *testing.T) {
	p := newPlugin()
	if len(p.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(p.Tools))
	}

	tool := p.Tools[0]
	if tool.Name != "code_review" {
		t.Errorf("expected tool name 'code_review', got %q", tool.Name)
	}
	if tool.Description == "" {
		t.Error("expected non-empty tool description")
	}
	if tool.Execute == nil {
		t.Error("expected non-nil Execute function")
	}
}

func TestToolSchema(t *testing.T) {
	p := newPlugin()
	params := p.Tools[0].Parameters

	typ, ok := params["type"]
	if !ok || typ != "object" {
		t.Errorf("expected type 'object', got %v", typ)
	}

	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties to be map[string]any")
	}

	for _, field := range []string{"ref", "path", "staged", "context_lines"} {
		prop, ok := props[field].(map[string]any)
		if !ok {
			t.Errorf("expected property %q to be map[string]any", field)
			continue
		}
		if prop["type"] == nil {
			t.Errorf("expected %q to have a type", field)
		}
	}
}

func TestArgsUnmarshal(t *testing.T) {
	raw := json.RawMessage(`{"ref":"main","path":"src/","staged":true,"context_lines":5}`)
	var args codeReviewArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if args.Ref != "main" {
		t.Errorf("expected ref 'main', got %q", args.Ref)
	}
	if args.Path != "src/" {
		t.Errorf("expected path 'src/', got %q", args.Path)
	}
	if !args.Staged {
		t.Error("expected staged to be true")
	}
	if args.ContextLines == nil || *args.ContextLines != 5 {
		t.Errorf("expected context_lines 5, got %v", args.ContextLines)
	}
}

func TestArgsUnmarshalDefaults(t *testing.T) {
	raw := json.RawMessage(`{}`)
	var args codeReviewArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if args.Ref != "" {
		t.Errorf("expected empty ref, got %q", args.Ref)
	}
	if args.Path != "" {
		t.Errorf("expected empty path, got %q", args.Path)
	}
	if args.Staged {
		t.Error("expected staged to be false")
	}
	if args.ContextLines != nil {
		t.Errorf("expected nil context_lines, got %v", args.ContextLines)
	}
}

func TestArgsUnmarshalInvalid(t *testing.T) {
	raw := json.RawMessage(`not json`)
	var args codeReviewArgs
	if err := json.Unmarshal(raw, &args); err == nil {
		t.Error("expected unmarshal error for invalid JSON")
	}
}

func TestBuildDiffArgs(t *testing.T) {
	tests := []struct {
		name string
		args codeReviewArgs
		want []string
	}{
		{
			name: "defaults",
			args: codeReviewArgs{},
			want: []string{"diff", "-U3", "HEAD"},
		},
		{
			name: "custom ref",
			args: codeReviewArgs{Ref: "main"},
			want: []string{"diff", "-U3", "main"},
		},
		{
			name: "staged",
			args: codeReviewArgs{Staged: true},
			want: []string{"diff", "-U3", "--cached", "HEAD"},
		},
		{
			name: "with path",
			args: codeReviewArgs{Path: "src/main.go"},
			want: []string{"diff", "-U3", "HEAD", "--", "src/main.go"},
		},
		{
			name: "custom context lines",
			args: codeReviewArgs{ContextLines: intPtr(10)},
			want: []string{"diff", "-U10", "HEAD"},
		},
		{
			name: "all options",
			args: codeReviewArgs{Ref: "develop", Path: "pkg/", Staged: true, ContextLines: intPtr(0)},
			want: []string{"diff", "-U0", "--cached", "develop", "--", "pkg/"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildDiffArgs(tt.args)
			if len(got) != len(tt.want) {
				t.Fatalf("buildDiffArgs() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("buildDiffArgs()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestFormatOutput(t *testing.T) {
	t.Run("empty diff", func(t *testing.T) {
		result := formatOutput("HEAD", "")
		if result != "No changes found." {
			t.Errorf("expected 'No changes found.', got %q", result)
		}
	})

	t.Run("single file diff", func(t *testing.T) {
		diff := "diff --git a/file.go b/file.go\n--- a/file.go\n+++ b/file.go\n@@ -1 +1 @@\n-old\n+new\n"
		result := formatOutput("HEAD", diff)

		if !strings.Contains(result, "## Code Review: Changes against HEAD") {
			t.Error("expected header with ref")
		}
		if !strings.Contains(result, "1 file(s) changed") {
			t.Error("expected file count")
		}
		if !strings.Contains(result, "```diff") {
			t.Error("expected diff code block")
		}
		if !strings.Contains(result, "+new") {
			t.Error("expected diff content")
		}
	})

	t.Run("multiple files", func(t *testing.T) {
		diff := "diff --git a/a.go b/a.go\n+line\ndiff --git a/b.go b/b.go\n+line\n"
		result := formatOutput("main", diff)

		if !strings.Contains(result, "2 file(s) changed") {
			t.Error("expected 2 files changed")
		}
		if !strings.Contains(result, "Changes against main") {
			t.Error("expected ref 'main' in header")
		}
	})
}

// Integration test using a real temporary git repo.
func TestExecuteCodeReview_Integration(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()

	// Initialize a git repo.
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	run("init")
	run("checkout", "-b", "main")

	// Create and commit a file.
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", "hello.txt")
	run("commit", "-m", "initial")

	// Modify the file (unstaged change).
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello world\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	tc := plugin.ToolContext{Directory: dir}

	t.Run("default diff shows changes", func(t *testing.T) {
		raw := json.RawMessage(`{}`)
		result, err := executeCodeReview(ctx, raw, tc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "1 file(s) changed") {
			t.Errorf("expected 1 file changed, got: %s", result)
		}
		if !strings.Contains(result, "+hello world") {
			t.Errorf("expected diff content, got: %s", result)
		}
	})

	t.Run("staged diff", func(t *testing.T) {
		run("add", "hello.txt")
		raw := json.RawMessage(`{"staged":true}`)
		result, err := executeCodeReview(ctx, raw, tc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "1 file(s) changed") {
			t.Errorf("expected 1 file changed, got: %s", result)
		}
		run("reset", "HEAD", "hello.txt") // unstage
	})

	t.Run("no changes returns empty message", func(t *testing.T) {
		// Diff HEAD against HEAD => no changes via a ref that equals current state
		// Use a committed state with no working tree changes
		run("add", "hello.txt")
		run("commit", "-m", "update")
		raw := json.RawMessage(`{}`)
		result, err := executeCodeReview(ctx, raw, tc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != "No changes found." {
			t.Errorf("expected 'No changes found.', got: %s", result)
		}
	})

	t.Run("path filter", func(t *testing.T) {
		// Create changes in two files, filter to one.
		if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b\n"), 0644); err != nil {
			t.Fatal(err)
		}
		run("add", "a.txt", "b.txt")
		run("commit", "-m", "add files")

		if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aa\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("bb\n"), 0644); err != nil {
			t.Fatal(err)
		}

		raw := json.RawMessage(`{"path":"a.txt"}`)
		result, err := executeCodeReview(ctx, raw, tc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "1 file(s) changed") {
			t.Errorf("expected 1 file changed with path filter, got: %s", result)
		}
		if strings.Contains(result, "b.txt") {
			t.Errorf("expected b.txt to be filtered out, got: %s", result)
		}
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		raw := json.RawMessage(`not json`)
		_, err := executeCodeReview(ctx, raw, tc)
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
		if !strings.Contains(err.Error(), "invalid arguments") {
			t.Errorf("expected 'invalid arguments' error, got: %v", err)
		}
	})
}

func intPtr(n int) *int {
	return &n
}
