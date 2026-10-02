package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/session"
)

func TestTaskTool_Foreground_WithRunner(t *testing.T) {
	called := false
	r := NewRegistry(&Context{
		Directory: t.TempDir(),
		SubagentRunner: func(ctx context.Context, parentSessionID string, parentDepth int, prompt, agent, directory string, autoApprove bool) (string, error) {
			called = true
			if prompt != "do something" {
				t.Errorf("expected prompt 'do something', got %q", prompt)
			}
			if agent != "debugger" {
				t.Errorf("expected agent 'debugger', got %q", agent)
			}
			return "subagent result", nil
		},
	})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"description":"test task","prompt":"do something","subagent_type":"debugger"}`)
	output, isErr, _ := r.Execute(context.Background(), "task", args, "ses-fg")

	if !called {
		t.Error("expected SubagentRunner to be called")
	}
	if isErr {
		t.Errorf("expected no error, got: %s", output)
	}
	if output != "subagent result" {
		t.Errorf("expected 'subagent result', got %q", output)
	}
}

func TestTaskTool_Foreground_NoRunner(t *testing.T) {
	r := NewRegistry(&Context{
		Directory: t.TempDir(),
	})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"description":"test","prompt":"hello"}`)
	output, isErr, _ := r.Execute(context.Background(), "task", args, "ses-nr")

	if !isErr {
		t.Error("expected error when SubagentRunner is nil")
	}
	if output == "" {
		t.Error("expected error message")
	}
}

func TestTaskTool_Foreground_DefaultAgent(t *testing.T) {
	var gotAgent string
	r := NewRegistry(&Context{
		Directory: t.TempDir(),
		SubagentRunner: func(ctx context.Context, parentSessionID string, parentDepth int, prompt, agent, directory string, autoApprove bool) (string, error) {
			gotAgent = agent
			return "ok", nil
		},
	})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"description":"test","prompt":"hello"}`)
	r.Execute(context.Background(), "task", args, "ses-da")

	if gotAgent != "executor" {
		t.Errorf("expected default agent 'executor', got %q", gotAgent)
	}
}

func TestTaskTool_Background_WithRunner(t *testing.T) {
	jm := session.NewJobManager()
	r := NewRegistry(&Context{
		Directory:  t.TempDir(),
		JobManager: jm,
		SubagentRunner: func(ctx context.Context, parentSessionID string, parentDepth int, prompt, agent, directory string, autoApprove bool) (string, error) {
			return "bg result", nil
		},
	})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"description":"bg task","prompt":"do bg","background":true}`)
	output, isErr, _ := r.Execute(context.Background(), "task", args, "ses-bg")

	if isErr {
		t.Fatalf("expected no error, got: %s", output)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	jobID, _ := result["job_id"].(string)
	if jobID == "" {
		t.Fatal("expected job_id in result")
	}
	if result["status"] != "running" {
		t.Errorf("expected status 'running', got %v", result["status"])
	}

	// Wait for the background job to complete
	job, err := jm.Wait(jobID)
	if err != nil {
		t.Fatalf("wait error: %v", err)
	}
	if job.Result != "bg result" {
		t.Errorf("expected 'bg result', got %q", job.Result)
	}
}

func TestTaskTool_Background_NoJobManager(t *testing.T) {
	r := NewRegistry(&Context{
		Directory: t.TempDir(),
		SubagentRunner: func(ctx context.Context, parentSessionID string, parentDepth int, prompt, agent, directory string, autoApprove bool) (string, error) {
			return "ok", nil
		},
	})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"description":"bg","prompt":"hello","background":true}`)
	output, isErr, _ := r.Execute(context.Background(), "task", args, "ses-nj")

	if !isErr {
		t.Error("expected error when JobManager is nil for background task")
	}
	if output == "" {
		t.Error("expected error message")
	}
}

func TestTaskTool_DepthExceeded(t *testing.T) {
	r := NewRegistry(&Context{
		Directory:     t.TempDir(),
		SubagentDepth: maxSubagentDepth,
		SubagentRunner: func(ctx context.Context, parentSessionID string, parentDepth int, prompt, agent, directory string, autoApprove bool) (string, error) {
			t.Error("SubagentRunner should not be called when depth exceeded")
			return "", nil
		},
	})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"description":"deep","prompt":"hello"}`)
	output, isErr, _ := r.Execute(context.Background(), "task", args, "ses-dep")

	if !isErr {
		t.Error("expected error when depth exceeded")
	}
	if output == "" {
		t.Error("expected error message about depth")
	}
}

func TestTaskTool_StatusLookup(t *testing.T) {
	jm := session.NewJobManager()
	jobID := jm.Start(context.Background(), func(ctx context.Context) (string, error) {
		return "done", nil
	})
	// Wait for it to complete
	time.Sleep(50 * time.Millisecond)

	r := NewRegistry(&Context{
		Directory:  t.TempDir(),
		JobManager: jm,
	})
	RegisterBuiltins(r)

	args := json.RawMessage(fmt.Sprintf(`{"description":"check","task_id":"%s"}`, jobID))
	output, isErr, _ := r.Execute(context.Background(), "task", args, "ses-st")

	if isErr {
		t.Fatalf("expected no error, got: %s", output)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	if result["status"] != string(session.JobCompleted) {
		t.Errorf("expected status 'completed', got %v", result["status"])
	}
	if result["result"] != "done" {
		t.Errorf("expected result 'done', got %v", result["result"])
	}
}

func TestTaskTool_StatusLookup_NotFound(t *testing.T) {
	jm := session.NewJobManager()
	r := NewRegistry(&Context{
		Directory:  t.TempDir(),
		JobManager: jm,
	})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"description":"check","task_id":"nonexistent"}`)
	output, isErr, _ := r.Execute(context.Background(), "task", args, "ses-nf")

	if !isErr {
		t.Error("expected error for nonexistent job")
	}
	if output == "" {
		t.Error("expected error message")
	}
}

func TestTaskTool_EmptyPrompt(t *testing.T) {
	r := NewRegistry(&Context{Directory: t.TempDir()})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"description":"test","prompt":""}`)
	output, isErr, _ := r.Execute(context.Background(), "task", args, "ses-ep")

	if !isErr {
		t.Error("expected error for empty prompt")
	}
	if output != "prompt is required" {
		t.Errorf("expected 'prompt is required', got %q", output)
	}
}

func TestTaskTool_RunnerError(t *testing.T) {
	r := NewRegistry(&Context{
		Directory: t.TempDir(),
		SubagentRunner: func(ctx context.Context, parentSessionID string, parentDepth int, prompt, agent, directory string, autoApprove bool) (string, error) {
			return "", fmt.Errorf("LLM failed")
		},
	})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"description":"test","prompt":"hello"}`)
	output, isErr, _ := r.Execute(context.Background(), "task", args, "ses-re")

	if !isErr {
		t.Error("expected error when runner returns error")
	}
	if output == "" {
		t.Error("expected error message")
	}
}

func TestTaskTool_DepthBelowMax_Allowed(t *testing.T) {
	var gotDepth int
	r := NewRegistry(&Context{
		Directory:     t.TempDir(),
		SubagentDepth: maxSubagentDepth - 1,
		SubagentRunner: func(ctx context.Context, parentSessionID string, parentDepth int, prompt, agent, directory string, autoApprove bool) (string, error) {
			gotDepth = parentDepth
			return "ok", nil
		},
	})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"description":"test","prompt":"hello"}`)
	output, isErr, _ := r.Execute(context.Background(), "task", args, "ses-below")

	if isErr {
		t.Errorf("expected no error at depth %d, got: %s", maxSubagentDepth-1, output)
	}
	if gotDepth != maxSubagentDepth-1 {
		t.Errorf("expected parentDepth=%d, got %d", maxSubagentDepth-1, gotDepth)
	}
}

func TestTaskTool_DepthAtMax_Blocked(t *testing.T) {
	r := NewRegistry(&Context{
		Directory:     t.TempDir(),
		SubagentDepth: maxSubagentDepth,
		SubagentRunner: func(ctx context.Context, parentSessionID string, parentDepth int, prompt, agent, directory string, autoApprove bool) (string, error) {
			t.Error("SubagentRunner should not be called when depth equals max")
			return "", nil
		},
	})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"description":"deep","prompt":"hello"}`)
	output, isErr, _ := r.Execute(context.Background(), "task", args, "ses-atmax")

	if !isErr {
		t.Error("expected error when depth equals maxSubagentDepth")
	}
	if output == "" {
		t.Error("expected error message about depth")
	}
}

func TestWithDepth_SetsSubagentDepth(t *testing.T) {
	r := NewRegistry(&Context{
		Directory:     t.TempDir(),
		SubagentDepth: 0,
	})
	RegisterBuiltins(r)

	child := r.WithDepth(3)

	if child.ctx.SubagentDepth != 3 {
		t.Errorf("expected SubagentDepth=3, got %d", child.ctx.SubagentDepth)
	}
	// Original should be unchanged.
	if r.ctx.SubagentDepth != 0 {
		t.Errorf("expected original SubagentDepth=0, got %d", r.ctx.SubagentDepth)
	}
	// Child should have the same tools registered.
	if len(child.List()) != len(r.List()) {
		t.Errorf("expected same tool count, got child=%d parent=%d", len(child.List()), len(r.List()))
	}
}

func TestTaskTool_RunnerPanic(t *testing.T) {
	r := NewRegistry(&Context{
		Directory: t.TempDir(),
		SubagentRunner: func(ctx context.Context, parentSessionID string, parentDepth int, prompt, agent, directory string, autoApprove bool) (string, error) {
			panic("test panic in runner")
		},
	})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"description":"panic test","prompt":"trigger panic"}`)
	output, isErr, _ := r.Execute(context.Background(), "task", args, "ses-panic")

	if !isErr {
		t.Error("expected error after panic")
	}
	if output == "" {
		t.Error("expected error message about panic")
	}
}

func TestTaskTool_ConcurrentLimit(t *testing.T) {
	count := &atomic.Int32{}
	count.Store(int32(maxConcurrentSubagents)) // already at max
	budget := &atomic.Int32{}
	budget.Store(10)

	r := NewRegistry(&Context{
		Directory:      t.TempDir(),
		SubagentCount:  count,
		SubagentBudget: budget,
		SubagentRunner: func(ctx context.Context, parentSessionID string, parentDepth int, prompt, agent, directory string, autoApprove bool) (string, error) {
			t.Error("SubagentRunner should not be called when concurrent limit reached")
			return "", nil
		},
	})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"description":"limit test","prompt":"hello"}`)
	output, isErr, _ := r.Execute(context.Background(), "task", args, "ses-conc")

	if !isErr {
		t.Error("expected error when concurrent limit reached")
	}
	if output == "" {
		t.Error("expected error message about concurrent limit")
	}
	// Budget should have been decremented then the concurrent check fails,
	// but budget is checked first and is consumed.
	// Verify the count was not left incremented.
	if count.Load() != int32(maxConcurrentSubagents) {
		t.Errorf("expected count to remain at %d, got %d", maxConcurrentSubagents, count.Load())
	}
}

func TestTaskTool_BudgetExhausted(t *testing.T) {
	budget := &atomic.Int32{}
	budget.Store(0) // no budget left

	r := NewRegistry(&Context{
		Directory:      t.TempDir(),
		SubagentBudget: budget,
		SubagentRunner: func(ctx context.Context, parentSessionID string, parentDepth int, prompt, agent, directory string, autoApprove bool) (string, error) {
			t.Error("SubagentRunner should not be called when budget exhausted")
			return "", nil
		},
	})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"description":"budget test","prompt":"hello"}`)
	output, isErr, _ := r.Execute(context.Background(), "task", args, "ses-bud")

	if !isErr {
		t.Error("expected error when budget exhausted")
	}
	if output == "" {
		t.Error("expected error message about budget")
	}
	// Budget should remain at 0 (not go negative).
	if budget.Load() != 0 {
		t.Errorf("expected budget to remain at 0, got %d", budget.Load())
	}
}

func TestTaskTool_TaskRoundDone_BlocksSecondRound(t *testing.T) {
	roundDone := &atomic.Bool{}
	callCount := 0
	r := NewRegistry(&Context{
		Directory:     t.TempDir(),
		TaskRoundDone: roundDone,
		SubagentRunner: func(ctx context.Context, parentSessionID string, parentDepth int, prompt, agent, directory string, autoApprove bool) (string, error) {
			callCount++
			return "ok", nil
		},
	})
	RegisterBuiltins(r)

	// First call should succeed and set the flag.
	args := json.RawMessage(`{"description":"first","prompt":"hello"}`)
	output, isErr, _ := r.Execute(context.Background(), "task", args, "ses-rd1")
	if isErr {
		t.Fatalf("expected first call to succeed, got: %s", output)
	}
	if !roundDone.Load() {
		t.Error("expected TaskRoundDone to be true after first call")
	}

	// Second call should be blocked.
	args2 := json.RawMessage(`{"description":"second","prompt":"hello again"}`)
	output2, isErr2, _ := r.Execute(context.Background(), "task", args2, "ses-rd2")
	if !isErr2 {
		t.Error("expected second call to be blocked")
	}
	if output2 == "" {
		t.Error("expected error message about task delegation completed")
	}
	if callCount != 1 {
		t.Errorf("expected runner called once, got %d", callCount)
	}
}

func TestTaskTool_TaskRoundDone_ResetAllowsNewRound(t *testing.T) {
	roundDone := &atomic.Bool{}
	callCount := 0
	r := NewRegistry(&Context{
		Directory:     t.TempDir(),
		TaskRoundDone: roundDone,
		SubagentRunner: func(ctx context.Context, parentSessionID string, parentDepth int, prompt, agent, directory string, autoApprove bool) (string, error) {
			callCount++
			return "ok", nil
		},
	})
	RegisterBuiltins(r)

	// First call succeeds.
	args := json.RawMessage(`{"description":"first","prompt":"hello"}`)
	r.Execute(context.Background(), "task", args, "ses-rr1")

	// Reset the flag (simulates new prompt).
	r.ResetTaskRound()

	// Second call should now succeed.
	args2 := json.RawMessage(`{"description":"second","prompt":"hello again"}`)
	output2, isErr2, _ := r.Execute(context.Background(), "task", args2, "ses-rr2")
	if isErr2 {
		t.Fatalf("expected second call after reset to succeed, got: %s", output2)
	}
	if callCount != 2 {
		t.Errorf("expected runner called twice, got %d", callCount)
	}
}

func TestTaskTool_TaskRoundDone_StatusLookupNotBlocked(t *testing.T) {
	roundDone := &atomic.Bool{}
	roundDone.Store(true) // simulate done round
	jm := session.NewJobManager()
	jobID := jm.Start(context.Background(), func(ctx context.Context) (string, error) {
		return "done", nil
	})
	time.Sleep(50 * time.Millisecond)

	r := NewRegistry(&Context{
		Directory:     t.TempDir(),
		TaskRoundDone: roundDone,
		JobManager:    jm,
	})
	RegisterBuiltins(r)

	// Status lookup should not be blocked by TaskRoundDone.
	args := json.RawMessage(fmt.Sprintf(`{"description":"check","task_id":"%s"}`, jobID))
	output, isErr, _ := r.Execute(context.Background(), "task", args, "ses-sl")
	if isErr {
		t.Fatalf("expected status lookup to succeed even with TaskRoundDone=true, got: %s", output)
	}
}

func TestTaskTool_BudgetDecrement(t *testing.T) {
	budget := &atomic.Int32{}
	budget.Store(5)
	count := &atomic.Int32{}

	r := NewRegistry(&Context{
		Directory:      t.TempDir(),
		SubagentCount:  count,
		SubagentBudget: budget,
		SubagentRunner: func(ctx context.Context, parentSessionID string, parentDepth int, prompt, agent, directory string, autoApprove bool) (string, error) {
			return "ok", nil
		},
	})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"description":"test","prompt":"hello"}`)
	output, isErr, _ := r.Execute(context.Background(), "task", args, "ses-dec")

	if isErr {
		t.Errorf("expected no error, got: %s", output)
	}
	if budget.Load() != 4 {
		t.Errorf("expected budget=4 after one spawn, got %d", budget.Load())
	}
	// Concurrent count should be back to 0 after foreground completes.
	if count.Load() != 0 {
		t.Errorf("expected concurrent count=0 after completion, got %d", count.Load())
	}
}
