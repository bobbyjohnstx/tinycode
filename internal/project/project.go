package project

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Info struct {
	ID       string `json:"id"`
	Worktree string `json:"worktree"`
	VCSDir   string `json:"vcsDir,omitempty"`
	VCS      string `json:"vcs,omitempty"`
	Time     Time   `json:"time"`
}

type Time struct {
	Created     int64 `json:"created"`
	Initialized int64 `json:"initialized,omitempty"`
}

func IDFromDirectory(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	h := sha256.Sum256([]byte(abs))
	return fmt.Sprintf("prj_%x", h[:8])
}

func FromDirectory(dir string) *Info {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}

	now := time.Now().UnixMilli()
	p := &Info{
		ID:       IDFromDirectory(abs),
		Worktree: abs,
		Time: Time{
			Created:     now,
			Initialized: now,
		},
	}

	if gitDir := findGitDir(abs); gitDir != "" {
		p.VCS = "git"
		p.VCSDir = gitDir
	}

	return p
}

func findGitDir(dir string) string {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--git-dir")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	gitDir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}
	if fi, err := os.Stat(gitDir); err == nil && fi.IsDir() {
		return gitDir
	}
	return ""
}
