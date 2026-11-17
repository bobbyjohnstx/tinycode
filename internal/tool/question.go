package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bobbyjohnstx/tinycode/internal/id"
	"github.com/bobbyjohnstx/tinycode/internal/permission"
)

type questionArgs struct {
	Question  string             `json:"question"`
	Options   []string           `json:"options,omitempty"`
	Questions []structuredQ      `json:"questions,omitempty"`
}

type structuredQ struct {
	ID       string            `json:"id"`
	Header   string            `json:"header"`
	Question string            `json:"question"`
	Options  []structuredQOpt  `json:"options"`
}

type structuredQOpt struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

const maxStructuredQuestions = 3

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
					"description": "The question to ask the user (legacy single-question mode)",
				},
				"options": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Optional list of choices (legacy single-question mode)",
				},
				"questions": map[string]any{
					"type":        "array",
					"description": "1-3 structured questions. Each has id, header (max 12 chars), question, and 2-3 options with label and description. Put recommended option first.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":       map[string]any{"type": "string", "description": "Unique identifier for this question"},
							"header":   map[string]any{"type": "string", "description": "Short header (max 12 characters)"},
							"question": map[string]any{"type": "string", "description": "The question text (one sentence)"},
							"options": map[string]any{
								"type": "array",
								"items": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"label":       map[string]any{"type": "string"},
										"description": map[string]any{"type": "string"},
									},
									"required": []string{"label"},
								},
								"description": "2-3 options with label and description",
							},
						},
						"required": []string{"id", "question", "options"},
					},
				},
			},
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
		return &ExecuteResult{
			Output:  "question tool unavailable: event bus not configured",
			IsError: true,
		}, nil
	}

	// Structured multi-question mode.
	if len(args.Questions) > 0 {
		if len(args.Questions) > maxStructuredQuestions {
			return &ExecuteResult{
				Output:  fmt.Sprintf("Too many questions: %d (max %d)", len(args.Questions), maxStructuredQuestions),
				IsError: true,
			}, nil
		}
		return executeStructuredQuestions(ctx, tc, args.Questions)
	}

	// Legacy single-question mode.
	if args.Question == "" {
		return &ExecuteResult{Output: "question or questions field is required", IsError: true}, nil
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

	return waitForQuestionReply(ctx, tc, questionID)
}

func executeStructuredQuestions(ctx context.Context, tc *Context, questions []structuredQ) (*ExecuteResult, error) {
	questionID, err := id.Ascending("question")
	if err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Error generating question ID: %v", err), IsError: true}, nil
	}

	items := make([]map[string]any, 0, len(questions))
	for _, q := range questions {
		opts := make([]map[string]any, 0, len(q.Options))
		for _, o := range q.Options {
			opt := map[string]any{"label": o.Label}
			if o.Description != "" {
				opt["description"] = o.Description
			}
			opts = append(opts, opt)
		}
		item := map[string]any{
			"id":       q.ID,
			"question": q.Question,
			"options":  opts,
		}
		if q.Header != "" {
			item["header"] = q.Header
		}
		items = append(items, item)
	}

	tc.Bus.Publish("question.asked", map[string]any{
		"id":        questionID,
		"sessionID": tc.SessionID,
		"questions": items,
	})

	return waitForQuestionReply(ctx, tc, questionID)
}

func waitForQuestionReply(ctx context.Context, tc *Context, questionID string) (*ExecuteResult, error) {
	replied := tc.Bus.Subscribe("question.replied")
	defer replied.Unsubscribe()
	rejected := tc.Bus.Subscribe("question.rejected")
	defer rejected.Unsubscribe()

	askCtx, cancel := context.WithTimeout(ctx, permission.AskTimeout)
	defer cancel()

	publishReject := func() {
		tc.Bus.Publish("question.rejected", map[string]any{
			"sessionID": tc.SessionID,
			"requestID": questionID,
		})
	}

	for {
		select {
		case evt := <-replied.C:
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
		case evt := <-rejected.C:
			props, ok := evt.Properties.(map[string]any)
			if !ok {
				continue
			}
			rid, _ := props["requestID"].(string)
			if rid != questionID {
				continue
			}
			return &ExecuteResult{Output: "Question cancelled", IsError: true}, nil
		case <-askCtx.Done():
			publishReject()
			return &ExecuteResult{Output: "Question cancelled", IsError: true}, nil
		}
	}
}
