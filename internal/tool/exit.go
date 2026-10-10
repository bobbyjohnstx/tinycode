package tool

import (
	"context"
	"encoding/json"
)

// ExitTool returns a tool the model can call to exit tinycode cleanly.
// It publishes an "app.exit" bus event that the TUI handles as tea.Quit.
func ExitTool() *Def {
	return &Def{
		ID:          "exit",
		Description: "Exit tinycode cleanly. Call this when your task is complete and you want to hand control back to the user or an outer script.",
		Permission:  "",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"reason": map[string]any{
					"type":        "string",
					"description": "Optional reason for exiting (e.g. 'task complete', 'PR pushed')",
				},
			},
		},
		Execute: executeExit,
	}
}

func executeExit(ctx context.Context, tc *Context, rawArgs json.RawMessage) (*ExecuteResult, error) {
	var args struct {
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(rawArgs, &args)

	reason := args.Reason
	if reason == "" {
		reason = "task complete"
	}

	if tc.Bus != nil {
		tc.Bus.Publish("app.exit", map[string]any{
			"reason": reason,
		})
	}

	return &ExecuteResult{Output: "Exiting: " + reason}, nil
}
