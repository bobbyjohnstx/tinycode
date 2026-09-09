package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestTodoWrite_ValidInput(t *testing.T) {
	def := TodoWriteTool()
	args, _ := json.Marshal(todoWriteArgs{
		Todos: []todoItem{
			{Content: "Fix bug", Status: "pending", Priority: "high"},
			{Content: "Write docs", Status: "in_progress", Priority: "normal"},
			{Content: "Deploy", Status: "done", Priority: "low"},
		},
	})

	result, err := def.Execute(context.Background(), &Context{SessionID: "test-session"}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "3 todos") {
		t.Errorf("expected todo count in output, got: %s", result.Output)
	}
}

func TestTodoWrite_InvalidStatus(t *testing.T) {
	def := TodoWriteTool()
	args, _ := json.Marshal(todoWriteArgs{
		Todos: []todoItem{
			{Content: "Task", Status: "invalid_status", Priority: "high"},
		},
	})

	result, err := def.Execute(context.Background(), &Context{SessionID: "s1"}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected validation error")
	}
	if !strings.Contains(result.Output, "invalid status") {
		t.Errorf("expected status validation message, got: %s", result.Output)
	}
}

func TestTodoWrite_InvalidPriority(t *testing.T) {
	def := TodoWriteTool()
	args, _ := json.Marshal(todoWriteArgs{
		Todos: []todoItem{
			{Content: "Task", Status: "pending", Priority: "critical"},
		},
	})

	result, err := def.Execute(context.Background(), &Context{SessionID: "s1"}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected validation error")
	}
	if !strings.Contains(result.Output, "invalid priority") {
		t.Errorf("expected priority validation message, got: %s", result.Output)
	}
}

func TestTodoWrite_EmptyContent(t *testing.T) {
	def := TodoWriteTool()
	args, _ := json.Marshal(todoWriteArgs{
		Todos: []todoItem{
			{Content: "", Status: "pending", Priority: "normal"},
		},
	})

	result, err := def.Execute(context.Background(), &Context{SessionID: "s1"}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected validation error")
	}
	if !strings.Contains(result.Output, "content is required") {
		t.Errorf("expected content validation message, got: %s", result.Output)
	}
}

func TestTodoWrite_EmptyList(t *testing.T) {
	def := TodoWriteTool()
	args, _ := json.Marshal(todoWriteArgs{Todos: []todoItem{}})

	result, err := def.Execute(context.Background(), &Context{SessionID: "s1"}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "0 todos") {
		t.Errorf("expected 0 count, got: %s", result.Output)
	}
}

func TestTodoWrite_MultipleErrors(t *testing.T) {
	def := TodoWriteTool()
	args, _ := json.Marshal(todoWriteArgs{
		Todos: []todoItem{
			{Content: "", Status: "bad", Priority: "bad"},
		},
	})

	result, err := def.Execute(context.Background(), &Context{SessionID: "s1"}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected validation error")
	}
	// Should report all three errors
	if strings.Count(result.Output, "todo[0]") < 3 {
		t.Errorf("expected 3 validation errors, got: %s", result.Output)
	}
}

func TestTodoWrite_BadArgs(t *testing.T) {
	def := TodoWriteTool()
	result, err := def.Execute(context.Background(), &Context{SessionID: "s1"}, json.RawMessage(`not json`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for bad JSON")
	}
}
