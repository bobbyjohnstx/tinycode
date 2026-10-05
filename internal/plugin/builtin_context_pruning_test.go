package plugin

import (
	"context"
	"strings"
	"testing"
)

func TestContextPruning_ReplacesDuplicateOutput(t *testing.T) {
	hook := NewContextPruningPlugin().Hooks().ToolExecAfter

	out, _, changed := hook(context.Background(), "read", "file contents", false)
	if changed || out != "" {
		t.Fatalf("first call should pass through, changed=%v out=%q", changed, out)
	}

	out, isErr, changed := hook(context.Background(), "read", "file contents", false)
	if !changed {
		t.Fatal("expected duplicate to be replaced")
	}
	if isErr {
		t.Fatal("expected isError to stay false")
	}
	if strings.Contains(out, "file contents") {
		t.Fatalf("duplicate note should not repeat the output, got %q", out)
	}
	if !strings.Contains(out, "duplicate of recent read call") {
		t.Fatalf("expected duplicate note, got %q", out)
	}
}

func TestContextPruning_PreservesErrorFlag(t *testing.T) {
	hook := NewContextPruningPlugin().Hooks().ToolExecAfter
	hook(context.Background(), "bash", "boom", true)

	_, isErr, changed := hook(context.Background(), "bash", "boom", true)
	if !changed || !isErr {
		t.Fatalf("changed=%v isErr=%v, want both true", changed, isErr)
	}
}
