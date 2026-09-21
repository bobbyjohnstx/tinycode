package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/session"
)

func TestTaskTool_Foreground_WithRunner(t *testing.T) {
	called := false
	r := NewRegistry(&Context{
		Directory: t.TempDir(),
		SubagentRunner: func(ctx context.Context, prompt, agent, directory string) (string, error) {
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
		SubagentRunner: func(ctx context.Context, prompt, agent, directory string) (string, error) {
			gotAgent = agent
			return "ok", nil
		},
	})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"description":"test","prompt":"hello"}`)
	r.Execute(context.Background(), "task", args, "ses-da")

	if gotAgent != "build" {
		t.Errorf("expected default agent 'build', got %q", gotAgent)
	}
}

func TestTaskTool_Background_WithRunner(t *testing.T) {
	jm := session.NewJobManager()
	r := NewRegistry(&Context{
		Directory:  t.TempDir(),
		JobManager: jm,
		SubagentRunner: func(ctx context.Context, prompt, agent, directory string) (string, error) {
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
		SubagentRunner: func(ctx context.Context, prompt, agent, directory string) (string, error) {
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
		SubagentRunner: func(ctx context.Context, prompt, agent, directory string) (string, error) {
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
	jobID := jm.Start(func(ctx context.Context) (string, error) {
		return "done", nil
	})
	// Wait for it to complete
	time.Sleep(50 * time.Millisecond)

	r := NewRegistry(&Context{
		Directory:  t.TempDir(),
		JobManager: jm,
	})
	RegisterBuiltins(r)

	args := json.RawMessage(fmt.Sprintf(`{"description":"check","prompt":"x","task_id":"%s"}`, jobID))
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

	args := json.RawMessage(`{"description":"check","prompt":"x","task_id":"nonexistent"}`)
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
		SubagentRunner: func(ctx context.Context, prompt, agent, directory string) (string, error) {
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
