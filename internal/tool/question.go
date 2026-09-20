package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bobbyjohnstx/tinycode-go/internal/id"
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

	if tc.Bus == nil {
		return &ExecuteResult{Output: fmt.Sprintf("[question: %s]", args.Question)}, nil
	}

	questionID, err := id.Ascending("question")
	if err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Error generating question ID: %v", err), IsError: true}, nil
	}

	// Build SDK-compatible QuestionRequest event:
	// { id, sessionID, questions: [{ question, options: [{ label }] }] }
	opts := make([]map[string]any, 0, len(args.Options))
	for _, o := range args.Options {
		opts = append(opts, map[string]any{"label": o})
	}
	questionItem := map[string]any{
		"question": args.Question,
	}
	if len(opts) > 0 {
		questionItem["options"] = opts
	}

	tc.Bus.Publish("question.asked", map[string]any{
		"id":        questionID,
		"sessionID": tc.SessionID,
		"questions": []map[string]any{questionItem},
	})

	sub := tc.Bus.Subscribe("question.replied")
	defer sub.Unsubscribe()

	for {
		select {
		case evt := <-sub.C:
			props, ok := evt.Properties.(map[string]any)
			if !ok {
				continue
			}
			rid, _ := props["requestID"].(string)
			if rid != questionID {
				continue
			}
			answer, _ := props["answer"].(string)
			return &ExecuteResult{Output: answer}, nil
		case <-ctx.Done():
			return &ExecuteResult{Output: "Question cancelled", IsError: true}, nil
		}
	}
}
