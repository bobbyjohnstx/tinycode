package plugin

import (
	"context"
	"testing"
)

func TestContextPruning_ReturnsPreviousOutput(t *testing.T) {
	hook := NewContextPruningPlugin().Hooks().ToolExecAfter
	args := `{"file_path":"/tmp/foo.go"}`

	out, _, changed := hook(context.Background(), "read", args, "file contents", false)
	if changed || out != "" {
		t.Fatalf("first call should pass through, changed=%v out=%q", changed, out)
	}

	out, isErr, changed := hook(context.Background(), "read", args, "newer contents", false)
	if !changed {
		t.Fatal("expected the repeat to return the earlier result")
	}
	if isErr {
		t.Fatal("expected isError to stay false")
	}
	if out != "file contents" {
		t.Fatalf("repeat output = %q, want the earlier read", out)
	}
}

func TestContextPruning_DifferentRequestsPassThrough(t *testing.T) {
	hook := NewContextPruningPlugin().Hooks().ToolExecAfter

	out, _, changed := hook(context.Background(), "bash", `{"command":"git diff"}`, "", false)
	if changed || out != "" {
		t.Fatalf("first empty bash should pass through, changed=%v out=%q", changed, out)
	}

	out, _, changed = hook(context.Background(), "bash", `{"command":"git status"}`, "", false)
	if changed {
		t.Fatalf("a different command with empty output is not a duplicate, out=%q", out)
	}
}

func TestContextPruning_PreservesErrorFlag(t *testing.T) {
	hook := NewContextPruningPlugin().Hooks().ToolExecAfter
	args := `{"command":"false"}`
	hook(context.Background(), "bash", args, "boom", true)

	out, isErr, changed := hook(context.Background(), "bash", args, "other", true)
	if !changed || !isErr {
		t.Fatalf("changed=%v isErr=%v, want both true", changed, isErr)
	}
	if out != "boom" {
		t.Fatalf("repeat output = %q, want the earlier error", out)
	}
}
