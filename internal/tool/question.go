package tool

import (
	"context"
	"encoding/json"
	"fmt"
)

type questionArgs struct {
	Question string   `json:"question"`
	Options  []string `json:"options,omitempty"`
}

func QuestionTool() *Def {
	return &Def{
		ID:          "question",
		Description: "Ask the user a question and wait for their response.",
		Permission:  "read",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"question": map[string]any{
					"type":        "string",
					"description": "The question to ask the user",
				},
				"options": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Optional list of choices",
				},
			},
			"required": []string{"question"},
		},
		Execute: executeQuestion,
	}
}

func executeQuestion(ctx context.Context, tc *Context, rawArgs json.RawMessage) (*ExecuteResult, error) {
	var args questionArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
	}

	return &ExecuteResult{
		Output: fmt.Sprintf("[question: %s]", args.Question),
	}, nil
}
