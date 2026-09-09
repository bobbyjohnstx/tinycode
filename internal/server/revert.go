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

// Stash pushes the current working tree changes onto the git stash with a
// tinycode-specific message, captures the stash commit SHA for reliable
// retrieval, and records the sessionID association.
func (rs *RevertState) Stash(dir, sessionID string) error {
	// Create a stash commit object and capture its SHA (does not modify
	// the working tree or the stash reflog).
	createCmd := exec.Command("git", "stash", "create")
	createCmd.Dir = dir
	createOut, err := createCmd.Output()
	if err != nil {
		return fmt.Errorf("git stash create: %w", err)
	}
	sha := strings.TrimSpace(string(createOut))
	if sha == "" {
		return fmt.Errorf("no changes to stash for session %s", sessionID)
	}

	// Store the commit in the stash reflog for visibility in git stash list.
	msg := "tinycode-revert-" + sessionID
	storeCmd := exec.Command("git", "stash", "store", "-m", msg, sha)
	storeCmd.Dir = dir
	if storeOut, err := storeCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git stash store: %s: %w", string(storeOut), err)
	}

	// Reset tracked files to HEAD (equivalent to what git stash push does).
	resetCmd := exec.Command("git", "checkout", "--", ".")
	resetCmd.Dir = dir
	if _, resetErr := resetCmd.CombinedOutput(); resetErr != nil {
		slog.Warn("failed to reset working tree after stash", "error", resetErr)
	}

	rs.mu.Lock()
	rs.stashes[sessionID] = sha
	rs.mu.Unlock()

	slog.Debug("revert stash created", "sessionID", sessionID, "sha", sha)
	return nil
}

// Pop restores the stash associated with the given session using the stored
// SHA, which is immune to stash stack index shifts.
func (rs *RevertState) Pop(dir, sessionID string) error {
	rs.mu.Lock()
	sha, ok := rs.stashes[sessionID]
	if ok {
		delete(rs.stashes, sessionID)
	}
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
