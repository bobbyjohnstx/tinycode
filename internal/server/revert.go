package server

import (
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
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

// Pop restores the stash associated with the given session, identified by its
// message to avoid popping the wrong entry when the stack has shifted.
func (rs *RevertState) Pop(dir, sessionID string) error {
	rs.mu.Lock()
	msg, ok := rs.stashes[sessionID]
	if ok {
		delete(rs.stashes, sessionID)
	}
	rs.mu.Unlock()

	if !ok {
		return fmt.Errorf("no stash found for session %s", sessionID)
	}

	ref, err := findStashByMessage(dir, msg)
	if err != nil {
		return fmt.Errorf("locating stash for session %s: %w", sessionID, err)
	}

	cmd := exec.Command("git", "stash", "pop", ref)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git stash pop: %s: %w", string(out), err)
	}

	slog.Debug("revert stash popped", "sessionID", sessionID, "ref", ref)
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
