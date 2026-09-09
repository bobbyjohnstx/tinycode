package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Valid statuses and priorities for todo items.
var (
	validStatuses   = map[string]bool{"pending": true, "in_progress": true, "done": true}
	validPriorities = map[string]bool{"high": true, "normal": true, "low": true}
)

type todoItem struct {
	Content  string `json:"content"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
}

type todoWriteArgs struct {
	Todos []todoItem `json:"todos"`
}

// TodoWriteTool returns a tool that replaces all session todos with the
// provided list. Each todo has content, status, and priority fields.
func TodoWriteTool() *Def {
	return &Def{
		ID:          "todowrite",
		Description: "Replace all session todos with the provided list. Each todo has content, status (pending/in_progress/done), and priority (high/normal/low).",
		Permission:  "edit",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"todos": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"content": map[string]any{
								"type":        "string",
								"description": "The todo content",
							},
							"status": map[string]any{
								"type":        "string",
								"enum":        []string{"pending", "in_progress", "done"},
								"description": "Todo status",
							},
							"priority": map[string]any{
								"type":        "string",
								"enum":        []string{"high", "normal", "low"},
								"description": "Todo priority",
							},
						},
						"required": []string{"content", "status", "priority"},
					},
					"description": "The complete list of todos to set for this session",
				},
			},
			"required": []string{"todos"},
		},
		Execute: executeTodoWrite,
	}
}

func executeTodoWrite(_ context.Context, tc *Context, rawArgs json.RawMessage) (*ExecuteResult, error) {
	var args todoWriteArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
	}

	// Validate all items
	var errors []string
	for i, item := range args.Todos {
		if item.Content == "" {
			errors = append(errors, fmt.Sprintf("todo[%d]: content is required", i))
		}
		if !validStatuses[item.Status] {
			errors = append(errors, fmt.Sprintf("todo[%d]: invalid status %q (must be pending/in_progress/done)", i, item.Status))
		}
		if !validPriorities[item.Priority] {
			errors = append(errors, fmt.Sprintf("todo[%d]: invalid priority %q (must be high/normal/low)", i, item.Priority))
		}
	}
	if len(errors) > 0 {
		return &ExecuteResult{
			Output:  "Validation errors:\n" + strings.Join(errors, "\n"),
			IsError: true,
		}, nil
	}

	if tc.DB == nil {
		// No DB: return the todo list as formatted output
		return &ExecuteResult{
			Output: fmt.Sprintf("Updated %d todos for session %s (no database configured)", len(args.Todos), tc.SessionID),
		}, nil
	}

	// Persist todos: delete existing and insert new
	todosJSON, err := json.Marshal(args.Todos)
	if err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Failed to serialize todos: %v", err), IsError: true}, nil
	}

	_, err = tc.DB.Exec("DELETE FROM todo WHERE session_id = ?", tc.SessionID)
	if err != nil {
		// Table may not exist; treat as non-fatal
		return &ExecuteResult{
			Output: fmt.Sprintf("Updated %d todos for session %s (persistence skipped: %v)", len(args.Todos), tc.SessionID, err),
		}, nil
	}

	for i, item := range args.Todos {
		_, err = tc.DB.Exec(
			"INSERT INTO todo (session_id, idx, content, status, priority, data) VALUES (?, ?, ?, ?, ?, ?)",
			tc.SessionID, i, item.Content, item.Status, item.Priority, string(todosJSON),
		)
		if err != nil {
			return &ExecuteResult{
				Output: fmt.Sprintf("Updated %d todos for session %s (partial persistence: %v)", len(args.Todos), tc.SessionID, err),
			}, nil
		}
	}

	if tc.Bus != nil {
		tc.Bus.Publish("session.todos.updated", map[string]any{
			"sessionID": tc.SessionID,
			"count":     len(args.Todos),
		})
	}

	return &ExecuteResult{
		Output: fmt.Sprintf("Successfully updated %d todos for session %s", len(args.Todos), tc.SessionID),
	}, nil
}
