package server

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func setupRevertGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "git", "init")
	runGit(t, dir, "git", "config", "user.email", "test@test.com")
	runGit(t, dir, "git", "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "git", "add", ".")
	runGit(t, dir, "git", "commit", "-m", "initial")
	return dir
}

func runGit(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %s: %v", name, args, out, err)
	}
}

func TestRevertState_UntrackedRoundTrip(t *testing.T) {
	dir := setupRevertGitRepo(t)
	rs := NewRevertState()
	sessionID := "ses_untracked"

	untracked := filepath.Join(dir, "agent-new.txt")
	if err := os.WriteFile(untracked, []byte("created by agent\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := rs.Stash(dir, sessionID); err != nil {
		t.Fatalf("Stash: %v", err)
	}
	if !rs.HasStash(sessionID) {
		t.Fatal("expected HasStash after Stash")
	}
	if _, err := os.Stat(untracked); !os.IsNotExist(err) {
		t.Fatalf("untracked file should be removed after revert, err=%v", err)
	}

	if err := rs.Pop(dir, sessionID); err != nil {
		t.Fatalf("Pop: %v", err)
	}
	if rs.HasStash(sessionID) {
		t.Fatal("expected HasStash false after successful Pop")
	}
	data, err := os.ReadFile(untracked)
	if err != nil {
		t.Fatalf("untracked file should be restored after unrevert: %v", err)
	}
	if string(data) != "created by agent\n" {
		t.Fatalf("restored content = %q", data)
	}
}

func TestRevertState_FailedApplyRetainsStash(t *testing.T) {
	dir := setupRevertGitRepo(t)
	rs := NewRevertState()
	sessionID := "ses_conflict"

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("stashed change\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := rs.Stash(dir, sessionID); err != nil {
		t.Fatalf("Stash: %v", err)
	}
	if !rs.HasStash(sessionID) {
		t.Fatal("expected HasStash after Stash")
	}

	// Commit a divergent change so stash apply conflicts.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("divergent change\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "git", "add", "README.md")
	runGit(t, dir, "git", "commit", "-m", "divergent")

	err := rs.Pop(dir, sessionID)
	if err == nil {
		t.Fatal("expected Pop to fail on conflict")
	}
	if !rs.HasStash(sessionID) {
		t.Fatal("failed apply must retain HasStash so unrevert can be retried")
	}
}
