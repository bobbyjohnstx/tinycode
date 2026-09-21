package tool

import (
	"context"
	"encoding/json"
	"fmt"
)

const (
	maxSubagentDepth       = 5
	maxConcurrentSubagents = 8
)

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
		Description: "Run a subtask via a subagent. Default is foreground (blocks and returns the result). Set background=true only if you will poll with task_id later. Prefer foreground for most work.",
		Permission:  "",
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
			"required": []string{"description"},
		},
		Execute: executeTask,
	}
}

func callSubagentRunner(ctx context.Context, tc *Context, prompt, agent string) (result string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("subagent panic: %v", r)
		}
	}()
	return tc.SubagentRunner(ctx, tc.SessionID, tc.SubagentDepth, prompt, agent, tc.Directory, tc.AutoApprove)
}

func executeTask(ctx context.Context, tc *Context, rawArgs json.RawMessage) (*ExecuteResult, error) {
	var args taskArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
	}

	// Status lookup first — doesn't need a prompt.
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

	// Prompt required for spawning new tasks.
	if args.Prompt == "" {
		return &ExecuteResult{Output: "prompt is required", IsError: true}, nil
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

	// Check concurrent subagent limit BEFORE budget so budget isn't
	// consumed when the concurrent limit blocks.
	if tc.SubagentCount != nil {
		if tc.SubagentCount.Add(1) > int32(maxConcurrentSubagents) {
			tc.SubagentCount.Add(-1) // release slot
			return &ExecuteResult{
				Output:  fmt.Sprintf("Maximum concurrent subagents (%d) reached. Wait for a running subagent to complete.", maxConcurrentSubagents),
				IsError: true,
			}, nil
		}
	}

	// Check per-session spawn budget.
	if tc.SubagentBudget != nil {
		if tc.SubagentBudget.Add(-1) < 0 {
			tc.SubagentBudget.Add(1) // restore
			releaseSubagentSlot(tc)
			return &ExecuteResult{
				Output:  "Subagent budget exhausted. No more subagents may be spawned in this session.",
				IsError: true,
			}, nil
		}
	}

	agent := args.SubagentType
	if agent == "" {
		agent = "executor"
	}

	if args.Background {
		if tc.JobManager == nil {
			releaseSubagentSlot(tc)
			return &ExecuteResult{Output: "Job manager not available for background tasks", IsError: true}, nil
		}
		runner := tc.SubagentRunner
		prompt := args.Prompt
		dir := tc.Directory
		sessionID := tc.SessionID
		depth := tc.SubagentDepth
		count := tc.SubagentCount
		autoApprove := tc.AutoApprove
		jobID := tc.JobManager.Start(ctx, func(jobCtx context.Context) (string, error) {
			defer func() {
				if count != nil {
					count.Add(-1)
				}
			}()
			return runner(jobCtx, sessionID, depth, prompt, agent, dir, autoApprove)
		})
		result, _ := json.Marshal(map[string]any{
			"job_id":  jobID,
			"status":  "running",
			"message": fmt.Sprintf("Background task started: %s", args.Description),
		})
		return &ExecuteResult{Output: string(result)}, nil
	}

	defer releaseSubagentSlot(tc)

	output, err := callSubagentRunner(ctx, tc, args.Prompt, agent)
	if err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Subagent error: %v", err), IsError: true}, nil
	}
	return &ExecuteResult{Output: output}, nil
}

// releaseSubagentSlot decrements the concurrent subagent counter if set.
func releaseSubagentSlot(tc *Context) {
	if tc.SubagentCount != nil {
		tc.SubagentCount.Add(-1)
	}
}
