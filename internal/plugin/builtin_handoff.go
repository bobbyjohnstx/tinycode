package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type handoffPlugin struct {
	mu           sync.Mutex
	currentState handoffState
	loadedState  *handoffState
}

type handoffState struct {
	SessionID     string   `json:"sessionId"`
	Timestamp     int64    `json:"timestamp"`
	Goal          string   `json:"goal"`
	Decisions     []string `json:"decisions"`
	OpenTasks     []string `json:"openTasks"`
	FilesModified []string `json:"filesModified"`
}

// NewHandoffPlugin creates a builtin plugin that persists session context
// (goal, decisions, open tasks, modified files) for handoff between sessions.
func NewHandoffPlugin() BuiltinPlugin {
	return &handoffPlugin{
		currentState: handoffState{
			Decisions:     []string{},
			OpenTasks:     []string{},
			FilesModified: []string{},
		},
	}
}

func (p *handoffPlugin) ID() string { return "handoff" }

func handoffDir() string {
	if dir := os.Getenv("HANDOFF_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".tinycode", "handoff")
	}
	return filepath.Join(home, ".tinycode", "handoff")
}

func loadMostRecentHandoff(dir, currentSessionID string) (*handoffState, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var best *handoffState
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var state handoffState
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

func (p *handoffPlugin) Tools() []BuiltinTool {
	return []BuiltinTool{
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
			Execute: func(_ context.Context, raw json.RawMessage) (string, error) {
				var args struct {
					Goal          *string  `json:"goal,omitempty"`
					Decisions     []string `json:"decisions,omitempty"`
					OpenTasks     []string `json:"openTasks,omitempty"`
					FilesModified []string `json:"filesModified,omitempty"`
				}
				if err := json.Unmarshal(raw, &args); err != nil {
					return "", fmt.Errorf("invalid arguments: %w", err)
				}

				p.mu.Lock()
				defer p.mu.Unlock()

				if args.Goal != nil {
					p.currentState.Goal = *args.Goal
				}
				if args.Decisions != nil {
					p.currentState.Decisions = append(p.currentState.Decisions, args.Decisions...)
				}
				if args.OpenTasks != nil {
					p.currentState.OpenTasks = append(p.currentState.OpenTasks, args.OpenTasks...)
				}
				if args.FilesModified != nil {
					p.currentState.FilesModified = append(p.currentState.FilesModified, args.FilesModified...)
				}
				return "Session context saved. Will be persisted when session ends.", nil
			},
		},
	}
}

func (p *handoffPlugin) Hooks() BuiltinHooks {
	return BuiltinHooks{
		SessionStart: func(_ context.Context, sessionID string) error {
			dir := handoffDir()
			loaded, _ := loadMostRecentHandoff(dir, sessionID)

			p.mu.Lock()
			defer p.mu.Unlock()

			p.loadedState = loaded
			p.currentState = handoffState{
				SessionID:     sessionID,
				Timestamp:     time.Now().UnixMilli(),
				Decisions:     []string{},
				OpenTasks:     []string{},
				FilesModified: []string{},
			}
			return nil
		},
		SessionEnd: func(_ context.Context, sessionID string) error {
			dir := handoffDir()
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("creating handoff dir: %w", err)
			}

			p.mu.Lock()
			state := handoffState{
				SessionID:     sessionID,
				Timestamp:     time.Now().UnixMilli(),
				Goal:          p.currentState.Goal,
				Decisions:     p.currentState.Decisions,
				OpenTasks:     p.currentState.OpenTasks,
				FilesModified: p.currentState.FilesModified,
			}
			p.mu.Unlock()

			data, err := json.MarshalIndent(state, "", "  ")
			if err != nil {
				return fmt.Errorf("marshaling state: %w", err)
			}

			filePath := filepath.Join(dir, sessionID+".json")
			return os.WriteFile(filePath, data, 0o644)
		},
		Dispose: func(_ context.Context) error {
			p.mu.Lock()
			defer p.mu.Unlock()
			p.loadedState = nil
			p.currentState = handoffState{
				Decisions:     []string{},
				OpenTasks:     []string{},
				FilesModified: []string{},
			}
			return nil
		},
	}
}
