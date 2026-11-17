package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func initTestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s: %v", args, out, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "initial"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s: %v", args, out, err)
		}
	}
	return dir
}

func TestCreateWorktree_InGitRepo(t *testing.T) {
	dir := initTestGitRepo(t)
	result, err := CreateWorktree(dir, false)
	if err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}
	if _, err := os.Stat(result.WorktreePath); err != nil {
		t.Fatalf("worktree path does not exist: %v", err)
	}
	if !strings.HasPrefix(result.BranchName, "tinycode/") {
		t.Errorf("branch %q missing tinycode/ prefix", result.BranchName)
	}
}

func TestCreateWorktree_ChildSessionDirectory(t *testing.T) {
	dir := initTestGitRepo(t)
	result, err := CreateWorktree(dir, false)
	if err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}
	cmd := exec.Command("git", "-C", result.WorktreePath, "rev-parse", "--git-dir")
	if err := cmd.Run(); err != nil {
		t.Error("worktree should be a valid git directory")
	}
}

func TestCreateWorktree_NonRepo_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	_, err := CreateWorktree(dir, false)
	if err == nil {
		t.Fatal("expected error for non-git directory")
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("expected 'not a git repository', got: %v", err)
	}
}

func TestCreateWorktree_DirtyTree_WithCheckRefuses(t *testing.T) {
	dir := initTestGitRepo(t)
	os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("dirty"), 0o644)
	_, err := CreateWorktree(dir, true)
	if err == nil {
		t.Fatal("expected error for dirty tree")
	}
	if !strings.Contains(err.Error(), "uncommitted changes") {
		t.Errorf("expected 'uncommitted changes', got: %v", err)
	}
}

func TestCreateWorktree_DirtyTree_ChildSessionOK(t *testing.T) {
	dir := initTestGitRepo(t)
	os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("dirty"), 0o644)
	result, err := CreateWorktree(dir, false)
	if err != nil {
		t.Fatalf("child session should work from dirty tree: %v", err)
	}
	if result.WorktreePath == "" {
		t.Error("expected non-empty worktree path")
	}
}

func TestCreateWorktree_ParentUnchanged(t *testing.T) {
	dir := initTestGitRepo(t)
	before, _ := os.ReadDir(dir)
	_, err := CreateWorktree(dir, false)
	if err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}
	after, _ := os.ReadDir(dir)
	countBefore, countAfter := 0, 0
	for _, e := range before {
		if e.Name() != ".git" && e.Name() != ".worktrees" {
			countBefore++
		}
	}
	for _, e := range after {
		if e.Name() != ".git" && e.Name() != ".worktrees" {
			countAfter++
		}
	}
	if countBefore != countAfter {
		t.Errorf("parent changed: %d files before, %d after", countBefore, countAfter)
	}
}
