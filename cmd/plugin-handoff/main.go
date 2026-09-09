package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

// sessionState holds the handoff context for a session.
type sessionState struct {
	SessionID     string   `json:"sessionId"`
	Timestamp     int64    `json:"timestamp"`
	Goal          string   `json:"goal"`
	Decisions     []string `json:"decisions"`
	OpenTasks     []string `json:"openTasks"`
	FilesModified []string `json:"filesModified"`
}

func emptyState(sessionID string) sessionState {
	return sessionState{
		SessionID:     sessionID,
		Timestamp:     time.Now().UnixMilli(),
		Goal:          "",
		Decisions:     []string{},
		OpenTasks:     []string{},
		FilesModified: []string{},
	}
}

// getHandoffDir returns the directory for handoff state files.
func getHandoffDir() string {
	if dir := os.Getenv("HANDOFF_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".tinycode", "handoff")
	}
	return filepath.Join(home, ".tinycode", "handoff")
}

// loadMostRecentState loads the most recent state from another session.
func loadMostRecentState(dir, currentSessionID string) (*sessionState, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var best *sessionState
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var state sessionState
		if err := json.Unmarshal(data, &state); err != nil {
			continue
		}
		if state.SessionID == currentSessionID {
			continue
		}
		if best == nil || state.Timestamp > best.Timestamp {
			s := state
			best = &s
		}
	}
	return best, nil
}

// formatStateBlock formats a state as a summary string.
func formatStateBlock(state *sessionState) string {
	var lines []string
	if state.Goal != "" {
		lines = append(lines, fmt.Sprintf("Goal: %s", state.Goal))
	}
	if len(state.Decisions) > 0 {
		lines = append(lines, fmt.Sprintf("Key decisions: %s", strings.Join(state.Decisions, "; ")))
	}
	if len(state.OpenTasks) > 0 {
		lines = append(lines, fmt.Sprintf("Open tasks: %s", strings.Join(state.OpenTasks, "; ")))
	}
	if len(state.FilesModified) > 0 {
		lines = append(lines, fmt.Sprintf("Files modified: %s", strings.Join(state.FilesModified, ", ")))
	}
	if len(lines) == 0 {
		return ""
	}
	return "<previous-session>\n" + strings.Join(lines, "\n") + "\n</previous-session>"
}

// handoffSaveArgs is the input schema for the handoff_save tool.
type handoffSaveArgs struct {
	Goal          *string  `json:"goal,omitempty"`
	Decisions     []string `json:"decisions,omitempty"`
	OpenTasks     []string `json:"openTasks,omitempty"`
	FilesModified []string `json:"filesModified,omitempty"`
}

// newPlugin returns the handoff plugin definition.
func newPlugin() plugin.Plugin {
	var (
		mu           sync.Mutex
		currentState = emptyState("")
		loadedState  *sessionState
	)

	return plugin.Plugin{
		ID: "handoff",
		Tools: []plugin.ToolDef{
			{
				Name:        "handoff_save",
				Description: "Save session context for handoff to the next session. Call this to persist goal, decisions, open tasks, and modified files.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"goal":          map[string]any{"type": "string", "description": "High-level goal of this session"},
						"decisions":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Key decisions made during this session"},
						"openTasks":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Unfinished work items to carry over"},
						"filesModified": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "File paths touched during this session"},
					},
				},
				Execute: func(_ context.Context, raw json.RawMessage, _ plugin.ToolContext) (string, error) {
					var args handoffSaveArgs
					if err := json.Unmarshal(raw, &args); err != nil {
						return "", fmt.Errorf("invalid arguments: %w", err)
					}

					mu.Lock()
					defer mu.Unlock()

					if args.Goal != nil {
						currentState.Goal = *args.Goal
					}
					if args.Decisions != nil {
						currentState.Decisions = append(currentState.Decisions, args.Decisions...)
					}
					if args.OpenTasks != nil {
						currentState.OpenTasks = append(currentState.OpenTasks, args.OpenTasks...)
					}
					if args.FilesModified != nil {
						currentState.FilesModified = append(currentState.FilesModified, args.FilesModified...)
					}

					return "Session context saved. Will be persisted when session ends.", nil
				},
			},
		},
		Hooks: plugin.HookHandlers{
			SessionStart: func(_ context.Context, event plugin.SessionStartEvent) error {
				dir := getHandoffDir()
				loaded, _ := loadMostRecentState(dir, event.SessionID)

				mu.Lock()
				defer mu.Unlock()

				loadedState = loaded
				currentState = emptyState(event.SessionID)

				if loadedState != nil {
					block := formatStateBlock(loadedState)
					if block != "" {
						fmt.Fprintf(os.Stderr, "handoff: loaded previous session context for %s\n", loadedState.SessionID)
					}
				}
				return nil
			},
			SessionEnd: func(_ context.Context, event plugin.SessionEndEvent) error {
				dir := getHandoffDir()
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return fmt.Errorf("creating handoff dir: %w", err)
				}

				mu.Lock()
				state := sessionState{
					SessionID:     event.SessionID,
					Timestamp:     time.Now().UnixMilli(),
					Goal:          currentState.Goal,
					Decisions:     currentState.Decisions,
					OpenTasks:     currentState.OpenTasks,
					FilesModified: currentState.FilesModified,
				}
				mu.Unlock()

				data, err := json.MarshalIndent(state, "", "  ")
				if err != nil {
					return fmt.Errorf("marshaling state: %w", err)
				}

				filePath := filepath.Join(dir, event.SessionID+".json")
				if err := os.WriteFile(filePath, data, 0o644); err != nil {
					return fmt.Errorf("writing state file: %w", err)
				}
				return nil
			},
			Dispose: func(_ context.Context) error {
				mu.Lock()
				defer mu.Unlock()
				loadedState = nil
				currentState = emptyState("")
				return nil
			},
		},
	}
}

func main() {
	plugin.Run(newPlugin())
}
