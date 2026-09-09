package main

import (
	"context"
	"os"
	"testing"

	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

func TestPluginID(t *testing.T) {
	p := newPlugin()
	if p.ID != "context-pruning" {
		t.Errorf("expected plugin ID 'context-pruning', got %q", p.ID)
	}
}

func TestPluginHasToolExecAfterHook(t *testing.T) {
	p := newPlugin()
	if p.Hooks.ToolExecAfter == nil {
		t.Fatal("expected ToolExecAfter hook to be set")
	}
}

func TestPluginHasNoTools(t *testing.T) {
	p := newPlugin()
	if len(p.Tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(p.Tools))
	}
}

func TestHashCallDeterministic(t *testing.T) {
	h1 := hashCall("read_file", "/etc/hosts")
	h2 := hashCall("read_file", "/etc/hosts")
	if h1 != h2 {
		t.Errorf("expected identical hashes, got %q and %q", h1, h2)
	}
}

func TestHashCallDifferentInputs(t *testing.T) {
	h1 := hashCall("read_file", "/etc/hosts")
	h2 := hashCall("read_file", "/etc/passwd")
	if h1 == h2 {
		t.Errorf("expected different hashes for different args")
	}

	h3 := hashCall("write_file", "/etc/hosts")
	if h1 == h3 {
		t.Errorf("expected different hashes for different tool names")
	}
}

func TestTrackerFirstCallNotDuplicate(t *testing.T) {
	tr := newTracker(5)
	if tr.record("read_file", "/etc/hosts") {
		t.Error("first call should not be a duplicate")
	}
}

func TestTrackerImmediateDuplicate(t *testing.T) {
	tr := newTracker(5)
	tr.record("read_file", "/etc/hosts")
	if !tr.record("read_file", "/etc/hosts") {
		t.Error("immediate repeat should be a duplicate")
	}
}

func TestTrackerDuplicateWithinThreshold(t *testing.T) {
	tr := newTracker(3)
	tr.record("read_file", "/etc/hosts")
	tr.record("other_tool", "args1")
	tr.record("other_tool", "args2")
	// 3 calls after the first — still within threshold of 3
	if !tr.record("read_file", "/etc/hosts") {
		t.Error("call within threshold should be a duplicate")
	}
}

func TestTrackerNotDuplicateOutsideThreshold(t *testing.T) {
	tr := newTracker(2)
	tr.record("read_file", "/etc/hosts") // seq 1
	tr.record("other_tool", "a")         // seq 2
	tr.record("other_tool", "b")         // seq 3
	// seq 4 - prev was at 1, diff is 3 > threshold 2
	if tr.record("read_file", "/etc/hosts") {
		t.Error("call outside threshold should not be a duplicate")
	}
}

func TestTrackerDifferentArgsNotDuplicate(t *testing.T) {
	tr := newTracker(5)
	tr.record("read_file", "/etc/hosts")
	if tr.record("read_file", "/etc/passwd") {
		t.Error("different args should not be a duplicate")
	}
}

func TestHookReturnsDuplicateNote(t *testing.T) {
	p := newPlugin()
	ctx := context.Background()

	// First call
	out1, err := p.Hooks.ToolExecAfter(ctx, plugin.ToolExecAfterInput{
		ToolName: "read_file",
		Output:   "file contents here",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out1 != nil {
		t.Error("first call should return nil (no modification)")
	}

	// Duplicate call
	out2, err := p.Hooks.ToolExecAfter(ctx, plugin.ToolExecAfterInput{
		ToolName: "read_file",
		Output:   "file contents here",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out2 == nil {
		t.Fatal("duplicate call should return non-nil output")
	}
	if out2.Output == "" {
		t.Error("duplicate output should not be empty")
	}
	expected := "[note: duplicate of recent read_file call]\nfile contents here"
	if out2.Output != expected {
		t.Errorf("expected %q, got %q", expected, out2.Output)
	}
}

func TestHookPreservesIsError(t *testing.T) {
	p := newPlugin()
	ctx := context.Background()

	p.Hooks.ToolExecAfter(ctx, plugin.ToolExecAfterInput{
		ToolName: "shell",
		Output:   "error output",
		IsError:  true,
	})

	out, err := p.Hooks.ToolExecAfter(ctx, plugin.ToolExecAfterInput{
		ToolName: "shell",
		Output:   "error output",
		IsError:  true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil output for duplicate")
	}
	if !out.IsError {
		t.Error("expected IsError to be preserved as true")
	}
}

func TestGetThresholdDefault(t *testing.T) {
	os.Unsetenv("CONTEXT_PRUNE_THRESHOLD")
	n := getThreshold()
	if n != 20 {
		t.Errorf("expected default threshold 20, got %d", n)
	}
}

func TestGetThresholdFromEnv(t *testing.T) {
	os.Setenv("CONTEXT_PRUNE_THRESHOLD", "10")
	defer os.Unsetenv("CONTEXT_PRUNE_THRESHOLD")
	n := getThreshold()
	if n != 10 {
		t.Errorf("expected threshold 10, got %d", n)
	}
}

func TestGetThresholdInvalidFallsBack(t *testing.T) {
	os.Setenv("CONTEXT_PRUNE_THRESHOLD", "abc")
	defer os.Unsetenv("CONTEXT_PRUNE_THRESHOLD")
	n := getThreshold()
	if n != 20 {
		t.Errorf("expected fallback threshold 20, got %d", n)
	}
}

func TestGetThresholdZeroFallsBack(t *testing.T) {
	os.Setenv("CONTEXT_PRUNE_THRESHOLD", "0")
	defer os.Unsetenv("CONTEXT_PRUNE_THRESHOLD")
	n := getThreshold()
	if n != 20 {
		t.Errorf("expected fallback threshold 20 for zero, got %d", n)
	}
}
