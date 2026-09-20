package project

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIDFromDirectory_Prefix(t *testing.T) {
	id := IDFromDirectory("/some/path")
	if !strings.HasPrefix(id, "prj_") {
		t.Errorf("IDFromDirectory should start with prj_, got %q", id)
	}
}

func TestIDFromDirectory_Deterministic(t *testing.T) {
	dir := t.TempDir()
	id1 := IDFromDirectory(dir)
	id2 := IDFromDirectory(dir)
	if id1 != id2 {
		t.Errorf("same dir should produce same ID: %q != %q", id1, id2)
	}
}

func TestIDFromDirectory_DifferentDirs(t *testing.T) {
	a := IDFromDirectory("/path/a")
	b := IDFromDirectory("/path/b")
	if a == b {
		t.Errorf("different dirs should produce different IDs: both %q", a)
	}
}

func TestIDFromDirectory_Length(t *testing.T) {
	id := IDFromDirectory("/some/path")
	// "prj_" (4) + 16 hex chars = 20
	if len(id) != 20 {
		t.Errorf("expected ID length 20, got %d (%q)", len(id), id)
	}
}

func TestFromDirectory_NonNil(t *testing.T) {
	dir := t.TempDir()
	info := FromDirectory(dir)
	if info == nil {
		t.Fatal("FromDirectory returned nil")
	}
}

func TestFromDirectory_Worktree(t *testing.T) {
	dir := t.TempDir()
	info := FromDirectory(dir)
	abs, _ := filepath.Abs(dir)
	if info.Worktree != abs {
		t.Errorf("Worktree = %q, want %q", info.Worktree, abs)
	}
}

func TestFromDirectory_IDMatchesIDFromDirectory(t *testing.T) {
	dir := t.TempDir()
	info := FromDirectory(dir)
	abs, _ := filepath.Abs(dir)
	expected := IDFromDirectory(abs)
	if info.ID != expected {
		t.Errorf("ID = %q, want %q", info.ID, expected)
	}
}

func TestFromDirectory_Timestamps(t *testing.T) {
	before := time.Now().UnixMilli()
	dir := t.TempDir()
	info := FromDirectory(dir)
	after := time.Now().UnixMilli()

	if info.Time.Created < before || info.Time.Created > after {
		t.Errorf("Created %d not in range [%d, %d]", info.Time.Created, before, after)
	}
	if info.Time.Initialized < before || info.Time.Initialized > after {
		t.Errorf("Initialized %d not in range [%d, %d]", info.Time.Initialized, before, after)
	}
}

func TestFromDirectory_GitRepo(t *testing.T) {
	dir := setupGitRepo(t)
	info := FromDirectory(dir)
	if info.VCS != "git" {
		t.Errorf("VCS = %q, want \"git\"", info.VCS)
	}
	if info.VCSDir == "" {
		t.Error("VCSDir should be non-empty for a git repo")
	}
}

func TestFromDirectory_NonGitDir(t *testing.T) {
	dir := t.TempDir()
	info := FromDirectory(dir)
	if info.VCS != "" {
		t.Errorf("VCS = %q, want empty for non-git dir", info.VCS)
	}
	if info.VCSDir != "" {
		t.Errorf("VCSDir = %q, want empty for non-git dir", info.VCSDir)
	}
}

func TestInfo_JSONRoundTrip(t *testing.T) {
	original := &Info{
		ID:       "prj_abc123",
		Worktree: "/tmp/test",
		VCSDir:   "/tmp/test/.git",
		VCS:      "git",
		Time: Time{
			Created:     1000,
			Initialized: 2000,
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Info
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, original.ID)
	}
	if decoded.Worktree != original.Worktree {
		t.Errorf("Worktree = %q, want %q", decoded.Worktree, original.Worktree)
	}
	if decoded.VCS != original.VCS {
		t.Errorf("VCS = %q, want %q", decoded.VCS, original.VCS)
	}
	if decoded.VCSDir != original.VCSDir {
		t.Errorf("VCSDir = %q, want %q", decoded.VCSDir, original.VCSDir)
	}
	if decoded.Time.Created != original.Time.Created {
		t.Errorf("Time.Created = %d, want %d", decoded.Time.Created, original.Time.Created)
	}
	if decoded.Time.Initialized != original.Time.Initialized {
		t.Errorf("Time.Initialized = %d, want %d", decoded.Time.Initialized, original.Time.Initialized)
	}
}

func TestInfo_JSONSandboxesIsArray(t *testing.T) {
	info := &Info{
		ID:        "prj_abc123",
		Worktree:  "/tmp/test",
		Sandboxes: []string{},
		Time: Time{
			Created: 1000,
			Updated: 1000,
		},
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	sandboxes, ok := raw["sandboxes"]
	if !ok {
		t.Fatal("expected 'sandboxes' key in JSON output")
	}
	arr, ok := sandboxes.([]any)
	if !ok {
		t.Fatalf("expected sandboxes to be an array, got %T", sandboxes)
	}
	if len(arr) != 0 {
		t.Errorf("expected empty array, got %v", arr)
	}
}

func TestInfo_JSONSandboxesNotNull(t *testing.T) {
	info := FromDirectory(t.TempDir())

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	s := string(data)
	if strings.Contains(s, `"sandboxes":null`) {
		t.Error("sandboxes should not be null, expected empty array")
	}
}

func TestTime_JSONUpdatedField(t *testing.T) {
	tm := Time{
		Created:     1000,
		Updated:     2000,
		Initialized: 3000,
	}

	data, err := json.Marshal(tm)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if raw["updated"] != float64(2000) {
		t.Errorf("expected updated=2000, got %v", raw["updated"])
	}
}

func TestFromDirectory_SetsUpdatedTime(t *testing.T) {
	before := time.Now().UnixMilli()
	info := FromDirectory(t.TempDir())
	after := time.Now().UnixMilli()

	if info.Time.Updated < before || info.Time.Updated > after {
		t.Errorf("Updated %d not in range [%d, %d]", info.Time.Updated, before, after)
	}
	if info.Time.Updated != info.Time.Created {
		t.Errorf("Updated (%d) should equal Created (%d) on new project", info.Time.Updated, info.Time.Created)
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
