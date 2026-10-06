package plugin

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodeReview_UsesContextDirectory(t *testing.T) {
	repo := t.TempDir()
	runCmd(t, repo, "git", "init")
	runCmd(t, repo, "git", "config", "user.email", "test@test.com")
	runCmd(t, repo, "git", "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, repo, "git", "add", ".")
	runCmd(t, repo, "git", "commit", "-m", "initial")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# changed\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Process cwd is elsewhere; directory comes from context.
	outside := t.TempDir()
	t.Chdir(outside)

	bm := NewBuiltinManager()
	bm.Register(NewCodeReviewPlugin())
	ctx := WithDirectory(context.Background(), repo)
	out, err := bm.CallTool(ctx, "code_review", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if strings.Contains(out, "No changes found.") {
		t.Fatalf("expected diff from project dir, got %q", out)
	}
	if !strings.Contains(out, "```diff") {
		t.Fatalf("expected markdown diff block, got %q", out)
	}
}

func runCmd(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %s: %v", name, args, out, err)
	}
}
