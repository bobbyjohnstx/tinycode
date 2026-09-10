package vcs

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGitInfo_ValidRepo(t *testing.T) {
	dir := setupGitRepo(t)
	info, err := GitInfo(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Type != "git" {
		t.Errorf("Type = %q, want \"git\"", info.Type)
	}
	if info.Branch == "" {
		t.Error("Branch should not be empty")
	}
}

func TestGitInfo_NonGitDir(t *testing.T) {
	dir := t.TempDir()
	_, err := GitInfo(dir)
	if err == nil {
		t.Error("expected error for non-git directory")
	}
}

func TestGitStatus_CleanRepo(t *testing.T) {
	dir := setupGitRepo(t)
	status, err := GitStatus(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.Clean {
		t.Errorf("expected clean repo, got %d changes", len(status.Changes))
	}
	if len(status.Changes) != 0 {
		t.Errorf("expected 0 changes, got %d", len(status.Changes))
	}
}

func TestGitStatus_UntrackedFile(t *testing.T) {
	dir := setupGitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}

	status, err := GitStatus(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Clean {
		t.Error("expected dirty repo with untracked file")
	}

	found := false
	for _, c := range status.Changes {
		if c.File == "new.txt" {
			found = true
			if c.Status != "??" {
				t.Errorf("untracked file status = %q, want \"??\"", c.Status)
			}
		}
	}
	if !found {
		t.Error("untracked file not found in changes")
	}
}

func TestGitStatus_ModifiedFile(t *testing.T) {
	dir := setupGitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("modified"), 0644); err != nil {
		t.Fatal(err)
	}

	status, err := GitStatus(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Clean {
		t.Error("expected dirty repo with modified file")
	}

	found := false
	for _, c := range status.Changes {
		if c.File == "README.md" && c.Status == "M" {
			found = true
		}
	}
	if !found {
		t.Errorf("modified README.md not found in changes: %+v", status.Changes)
	}
}

func TestGitStatus_StagedFile(t *testing.T) {
	dir := setupGitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("modified"), 0644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "add", "README.md")

	status, err := GitStatus(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Clean {
		t.Error("expected dirty repo with staged file")
	}

	found := false
	for _, c := range status.Changes {
		if c.File == "README.md" && c.Status == "M" {
			found = true
		}
	}
	if !found {
		t.Errorf("staged README.md not found in changes: %+v", status.Changes)
	}
}

func TestGitDiff_CleanRepo(t *testing.T) {
	dir := setupGitRepo(t)
	diff, err := GitDiff(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff != "" {
		t.Errorf("expected empty diff, got %q", diff)
	}
}

func TestGitDiff_ModifiedFile(t *testing.T) {
	dir := setupGitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("modified content"), 0644); err != nil {
		t.Fatal(err)
	}

	diff, err := GitDiff(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff == "" {
		t.Error("expected non-empty diff for modified file")
	}
}

func TestChangeStruct(t *testing.T) {
	c := Change{Status: "M", File: "foo.go"}
	if c.Status != "M" {
		t.Errorf("Status = %q, want \"M\"", c.Status)
	}
	if c.File != "foo.go" {
		t.Errorf("File = %q, want \"foo.go\"", c.File)
	}
}

func TestStatusStruct(t *testing.T) {
	s := Status{
		Clean:   true,
		Changes: []Change{{Status: "A", File: "a.go"}},
	}
	if !s.Clean {
		t.Error("Clean should be true")
	}
	if len(s.Changes) != 1 {
		t.Errorf("expected 1 change, got %d", len(s.Changes))
	}
}

func setupGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@test.com")
	run(t, dir, "git", "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test"), 0644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-m", "initial")
	return dir
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %s: %v", name, args, out, err)
	}
}
