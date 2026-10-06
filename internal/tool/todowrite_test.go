package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/storage"
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

func TestTodoWrite_PersistsAndReloads(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	now := int64(1700000000000)
	if _, err := db.Exec(
		`INSERT INTO project (id, worktree, sandboxes, time_created, time_updated) VALUES (?, ?, ?, ?, ?)`,
		"prj_1", "/tmp", "[]", now, now,
	); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO session (id, project_id, slug, directory, title, version, time_created, time_updated)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"ses_todo", "prj_1", "s", "/tmp", "T", "1", now, now,
	); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	b := bus.New()
	defer b.Close()
	sub := b.Subscribe("session.todos.updated")
	defer sub.Unsubscribe()

	def := TodoWriteTool()
	args, _ := json.Marshal(todoWriteArgs{
		Todos: []todoItem{
			{Content: "Fix bug", Status: "pending", Priority: "high"},
			{Content: "Write docs", Status: "in_progress", Priority: "normal"},
		},
	})
	result, err := def.Execute(context.Background(), &Context{
		SessionID: "ses_todo",
		DB:        db.DB,
		Bus:       b,
	}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Successfully updated 2 todos") {
		t.Fatalf("expected success output, got: %s", result.Output)
	}

	select {
	case <-sub.C:
	default:
		t.Fatal("expected session.todos.updated bus event")
	}

	rows, err := db.Query(
		`SELECT content, status, priority, position FROM todo WHERE session_id = ? ORDER BY position`,
		"ses_todo",
	)
	if err != nil {
		t.Fatalf("select todos: %v", err)
	}
	defer rows.Close()

	var got []todoItem
	var positions []int
	for rows.Next() {
		var item todoItem
		var pos int
		if err := rows.Scan(&item.Content, &item.Status, &item.Priority, &pos); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, item)
		positions = append(positions, pos)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 persisted todos, got %d", len(got))
	}
	if got[0].Content != "Fix bug" || got[0].Status != "pending" || got[0].Priority != "high" {
		t.Errorf("unexpected first todo: %+v", got[0])
	}
	if got[1].Content != "Write docs" || positions[0] != 0 || positions[1] != 1 {
		t.Errorf("unexpected second todo/positions: %+v %v", got[1], positions)
	}

	// Replace with a single todo and confirm prior rows are gone.
	args2, _ := json.Marshal(todoWriteArgs{
		Todos: []todoItem{{Content: "Only one", Status: "done", Priority: "low"}},
	})
	result, err = def.Execute(context.Background(), &Context{SessionID: "ses_todo", DB: db.DB}, args2)
	if err != nil || result.IsError {
		t.Fatalf("replace failed: err=%v out=%v", err, result)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM todo WHERE session_id = ?`, "ses_todo").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 todo after replace, got %d", count)
	}
}
