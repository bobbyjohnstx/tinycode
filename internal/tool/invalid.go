package tool

import (
	"context"
	"encoding/json"
	"fmt"
)

type invalidArgs struct {
	Error        string `json:"error"`
	OriginalName string `json:"original_name"`
	OriginalArgs string `json:"original_args"`
}

// InvalidTool returns a tool definition for the "invalid" fallback tool.
// It is invoked when the LLM produces an unparseable tool call. The tool
// returns a helpful error instructing the LLM to retry with valid JSON.
func InvalidTool() *Def {
	return &Def{
		ID:          "invalid",
		Description: "Fallback for malformed tool calls. Returns an error prompting the LLM to retry.",
		Permission:  "read",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"error": map[string]any{
					"type":        "string",
					"description": "The JSON parse error",
				},
				"original_name": map[string]any{
					"type":        "string",
					"description": "The original tool name that was called",
				},
				"original_args": map[string]any{
					"type":        "string",
					"description": "The raw arguments that failed to parse",
				},
			},
			"required": []string{"error", "original_name"},
		},
		Execute: executeInvalid,
	}
}

func executeInvalid(_ context.Context, _ *Context, rawArgs json.RawMessage) (*ExecuteResult, error) {
	var args invalidArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return &ExecuteResult{
			Output:  "Tool call had invalid JSON arguments. Please retry with valid JSON.",
			IsError: true,
		}, nil
	}

	msg := fmt.Sprintf(
		"Tool call to %q failed: the arguments were not valid JSON.\nError: %s\nPlease retry with correctly formatted JSON arguments.",
		args.OriginalName, args.Error,
	)
	return &ExecuteResult{Output: msg, IsError: true}, nil
}
