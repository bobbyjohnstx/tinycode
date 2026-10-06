package server

import (
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
)

// RevertState tracks git stashes created by session reverts so they can be
// applied on unrevert.
type RevertState struct {
	mu      sync.Mutex
	stashes map[string]string // sessionID -> stash commit SHA
}

// NewRevertState creates an empty RevertState.
func NewRevertState() *RevertState {
	return &RevertState{stashes: make(map[string]string)}
}

// Stash captures tracked and untracked working-tree changes for the session,
// then clears the worktree so revert leaves a clean tree.
//
// git stash create does not reliably include untracked-only files (even with -u
// on some git versions / trees). We use git stash push -u, which stashes
// untracked files and resets/cleans the worktree in one step. The stash commit
// SHA is then recorded so Unrevert can apply by SHA.
func (rs *RevertState) Stash(dir, sessionID string) error {
	msg := "tinycode-revert-" + sessionID
	pushCmd := exec.Command("git", "stash", "push", "-u", "-m", msg)
	pushCmd.Dir = dir
	out, err := pushCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git stash push: %s: %w", string(out), err)
	}
	if strings.Contains(string(out), "No local changes to save") {
		return fmt.Errorf("no changes to stash for session %s", sessionID)
	}

	shaCmd := exec.Command("git", "rev-parse", "stash@{0}")
	shaCmd.Dir = dir
	shaOut, err := shaCmd.Output()
	if err != nil {
		return fmt.Errorf("git rev-parse stash@{0}: %w", err)
	}
	sha := strings.TrimSpace(string(shaOut))
	if sha == "" {
		return fmt.Errorf("no changes to stash for session %s", sessionID)
	}

	rs.mu.Lock()
	rs.stashes[sessionID] = sha
	rs.mu.Unlock()

	slog.Debug("revert stash created", "sessionID", sessionID, "sha", sha)
	return nil
}

// Pop restores the stash associated with the given session using the stored
// SHA, which is immune to stash stack index shifts. The session→SHA mapping
// is retained until apply succeeds so a failed unrevert can be retried.
func (rs *RevertState) Pop(dir, sessionID string) error {
	rs.mu.Lock()
	sha, ok := rs.stashes[sessionID]
	rs.mu.Unlock()

	if !ok {
		return fmt.Errorf("no stash found for session %s", sessionID)
	}

	cmd := exec.Command("git", "stash", "apply", sha)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git stash apply: %s: %w", string(out), err)
	}

	rs.mu.Lock()
	delete(rs.stashes, sessionID)
	rs.mu.Unlock()

	// Drop the stash entry from the reflog by message lookup.
	msg := "tinycode-revert-" + sessionID
	if ref, findErr := findStashByMessage(dir, msg); findErr == nil {
		dropCmd := exec.Command("git", "stash", "drop", ref)
		dropCmd.Dir = dir
		_ = dropCmd.Run()
	}

	slog.Debug("revert stash applied", "sessionID", sessionID, "sha", sha)
	return nil
}

func findStashByMessage(dir, message string) (string, error) {
	cmd := exec.Command("git", "stash", "list")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git stash list: %w", err)
	}

	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, message) {
			colon := strings.Index(line, ":")
			if colon > 0 {
				return line[:colon], nil
			}
		}
	}
	return "", fmt.Errorf("stash with message %q not found", message)
}

// HasStash reports whether a revert stash exists for the session.
func (rs *RevertState) HasStash(sessionID string) bool {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	_, ok := rs.stashes[sessionID]
	return ok
}
