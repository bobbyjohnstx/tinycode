package tool

import (
	"context"
	"encoding/json"
	"fmt"
)

const maxSubagentDepth = 5

type taskArgs struct {
	Description  string `json:"description"`
	Prompt       string `json:"prompt"`
	SubagentType string `json:"subagent_type,omitempty"`
	TaskID       string `json:"task_id,omitempty"`
	Background   bool   `json:"background,omitempty"`
}

func TaskTool() *Def {
	return &Def{
		ID:          "task",
		Description: "Create a subagent task. Foreground tasks block until completion. Background tasks return a job ID for later retrieval.",
		Permission:  "shell",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"description": map[string]any{
					"type":        "string",
					"description": "A short description of the task",
				},
				"prompt": map[string]any{
					"type":        "string",
					"description": "The prompt to send to the subagent",
				},
				"subagent_type": map[string]any{
					"type":        "string",
					"description": "The type of subagent to use (e.g., 'executor', 'debugger')",
				},
				"task_id": map[string]any{
					"type":        "string",
					"description": "Task ID for status/cancel operations",
				},
				"background": map[string]any{
					"type":        "boolean",
					"description": "Run the task in the background",
				},
			},
			"required": []string{"description", "prompt"},
		},
		Execute: executeTask,
	}
}

func executeTask(ctx context.Context, tc *Context, rawArgs json.RawMessage) (*ExecuteResult, error) {
	var args taskArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
	}

	if args.Prompt == "" {
		return &ExecuteResult{Output: "prompt is required", IsError: true}, nil
	}

	if args.TaskID != "" {
		if tc.JobManager == nil {
			return &ExecuteResult{Output: "Job manager not available", IsError: true}, nil
		}
		job := tc.JobManager.Get(args.TaskID)
		if job == nil {
			return &ExecuteResult{Output: fmt.Sprintf("Job %s not found", args.TaskID), IsError: true}, nil
		}
		result, _ := json.Marshal(map[string]any{
			"id":     job.ID,
			"status": job.Status,
			"result": job.Result,
			"error":  job.Error,
		})
		return &ExecuteResult{Output: string(result)}, nil
	}

	if tc.SubagentDepth >= maxSubagentDepth {
		return &ExecuteResult{
			Output:  fmt.Sprintf("Maximum subagent depth (%d) exceeded. Cannot create nested subagent.", maxSubagentDepth),
			IsError: true,
		}, nil
	}

	if tc.SubagentRunner == nil {
		return &ExecuteResult{Output: "Subagent execution not available in this environment", IsError: true}, nil
	}

	agent := args.SubagentType
	if agent == "" {
		agent = "build"
	}

	if args.Background {
		if tc.JobManager == nil {
			return &ExecuteResult{Output: "Job manager not available for background tasks", IsError: true}, nil
		}
		runner := tc.SubagentRunner
		prompt := args.Prompt
		dir := tc.Directory
		jobID := tc.JobManager.Start(func(jobCtx context.Context) (string, error) {
			return runner(jobCtx, prompt, agent, dir)
		})
		result, _ := json.Marshal(map[string]any{
			"job_id":  jobID,
			"status":  "running",
			"message": fmt.Sprintf("Background task started: %s", args.Description),
		})
		return &ExecuteResult{Output: string(result)}, nil
	}

	output, err := tc.SubagentRunner(ctx, args.Prompt, agent, tc.Directory)
	if err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Subagent error: %v", err), IsError: true}, nil
	}
	return &ExecuteResult{Output: output}, nil
}
