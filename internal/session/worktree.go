package session

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bobbyjohnstx/tinycode/internal/id"
)

// WorktreeResult holds the outcome of a successful worktree creation.
type WorktreeResult struct {
	WorktreePath string
	BranchName   string
}

// CreateWorktree creates a git worktree branched from the current HEAD.
// Branch name: tinycode/<short-id>. If checkDirty is true the call refuses
// when the working tree has uncommitted changes.
func CreateWorktree(directory string, checkDirty bool) (*WorktreeResult, error) {
	if !isGitRepo(directory) {
		return nil, fmt.Errorf("not a git repository: %s", directory)
	}
	if checkDirty {
		dirty, err := isGitDirty(directory)
		if err != nil {
			return nil, fmt.Errorf("checking git status: %w", err)
		}
		if dirty {
			return nil, fmt.Errorf("working tree has uncommitted changes; commit or stash first, or use a child session")
		}
	}

	shortID := generateShortID()
	branchName := "tinycode/" + shortID

	repoRoot, err := gitRepoRoot(directory)
	if err != nil {
		return nil, fmt.Errorf("finding repo root: %w", err)
	}

	worktreePath := filepath.Join(repoRoot, ".worktrees", branchName)

	cmd := exec.Command("git", "-C", directory, "worktree", "add", "-b", branchName, worktreePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git worktree add: %s: %w", strings.TrimSpace(string(output)), err)
	}

	absPath, err := filepath.Abs(worktreePath)
	if err != nil {
		absPath = worktreePath
	}

	return &WorktreeResult{
		WorktreePath: absPath,
		BranchName:   branchName,
	}, nil
}

func isGitRepo(dir string) bool {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--git-dir")
	return cmd.Run() == nil
}

func isGitDirty(dir string) (bool, error) {
	cmd := exec.Command("git", "-C", dir, "status", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return len(strings.TrimSpace(string(output))) > 0, nil
}

func gitRepoRoot(dir string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func generateShortID() string {
	fullID, err := id.Ascending("session")
	if err != nil {
		return "wt-fallback"
	}
	short := strings.TrimPrefix(fullID, "ses_")
	if len(short) > 8 {
		short = short[:8]
	}
	return short
}
