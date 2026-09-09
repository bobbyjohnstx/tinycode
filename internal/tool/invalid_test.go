package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestInvalidTool_ReturnsError(t *testing.T) {
	def := InvalidTool()
	args, _ := json.Marshal(invalidArgs{
		Error:        "unexpected EOF",
		OriginalName: "shell",
		OriginalArgs: `{"command": "echo`,
	})

	result, err := def.Execute(context.Background(), &Context{}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError to be true")
	}
	if !strings.Contains(result.Output, "shell") {
		t.Errorf("expected original tool name in output, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "unexpected EOF") {
		t.Errorf("expected error message in output, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "retry") {
		t.Errorf("expected retry instruction in output, got: %s", result.Output)
	}
}

func TestInvalidTool_BadArgs(t *testing.T) {
	def := InvalidTool()
	result, err := def.Execute(context.Background(), &Context{}, json.RawMessage(`not json`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError to be true")
	}
	if !strings.Contains(result.Output, "valid JSON") {
		t.Errorf("expected generic error message, got: %s", result.Output)
	}
}

func TestInvalidTool_ExcludedFromToolDefs(t *testing.T) {
	r := NewRegistry(&Context{Directory: t.TempDir()})
	r.Register(InvalidTool())
	r.Register(&Def{ID: "shell", Permission: "shell", Parameters: map[string]any{"type": "object"}})

	defs := r.ToolDefs(nil)
	for _, d := range defs {
		if d.Function.Name == "invalid" {
			t.Error("invalid tool should not appear in ToolDefs")
		}
	}
	if len(defs) != 1 {
		t.Errorf("expected 1 tool def, got %d", len(defs))
	}
}

func TestInvalidTool_ExecutableViaRegistry(t *testing.T) {
	r := NewRegistry(&Context{Directory: t.TempDir()})
	r.Register(InvalidTool())

	args, _ := json.Marshal(map[string]string{
		"error":         "parse error",
		"original_name": "read",
	})
	output, isErr, err := r.Execute(context.Background(), "invalid", args, "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isErr {
		t.Error("expected tool to report error")
	}
	if !strings.Contains(output, "read") {
		t.Errorf("expected original tool name, got: %s", output)
	}
}
