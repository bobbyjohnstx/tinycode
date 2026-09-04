package server

import (
	"fmt"
	"log/slog"
	"os/exec"
	"sync"
)

// RevertState tracks git stashes created by session reverts so they can be
// popped on unrevert.
type RevertState struct {
	mu      sync.Mutex
	stashes map[string]string // sessionID -> stash ref (e.g. "stash@{0}")
}

// NewRevertState creates an empty RevertState.
func NewRevertState() *RevertState {
	return &RevertState{stashes: make(map[string]string)}
}

// Stash pushes the current working tree changes onto the git stash with a
// tinycode-specific message, and records the sessionID association.
func (rs *RevertState) Stash(dir, sessionID string) error {
	msg := "tinycode-revert-" + sessionID
	cmd := exec.Command("git", "stash", "push", "-m", msg)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git stash push: %s: %w", string(out), err)
	}

	rs.mu.Lock()
	rs.stashes[sessionID] = msg
	rs.mu.Unlock()

	slog.Debug("revert stash created", "sessionID", sessionID, "message", msg)
	return nil
}

// Pop restores the most recent stash associated with the given session.
func (rs *RevertState) Pop(dir, sessionID string) error {
	rs.mu.Lock()
	_, ok := rs.stashes[sessionID]
	if ok {
		delete(rs.stashes, sessionID)
	}
	rs.mu.Unlock()

	if !ok {
		return fmt.Errorf("no stash found for session %s", sessionID)
	}

	cmd := exec.Command("git", "stash", "pop")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git stash pop: %s: %w", string(out), err)
	}

	slog.Debug("revert stash popped", "sessionID", sessionID)
	return nil
}

// HasStash reports whether a revert stash exists for the session.
func (rs *RevertState) HasStash(sessionID string) bool {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	_, ok := rs.stashes[sessionID]
	return ok
}
