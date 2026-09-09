package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
	"github.com/bobbyjohnstx/tinycode-go/internal/permission"
)

func collectEvents(sub *bus.Subscription, count int, timeout time.Duration) []bus.Event {
	var events []bus.Event
	deadline := time.After(timeout)
	for range count {
		select {
		case evt := <-sub.C:
			events = append(events, evt)
		case <-deadline:
			return events
		}
	}
	return events
}

func TestExecuteBeforeEvent(t *testing.T) {
	b := bus.New()
	defer b.Close()

	sub := b.Subscribe("tool.execute.before")
	defer sub.Unsubscribe()

	r := NewRegistry(&Context{Directory: t.TempDir(), Bus: b})
	r.Register(&Def{
		ID:         "noop",
		Permission: "read",
		Parameters: map[string]any{"type": "object"},
		Execute: func(ctx context.Context, tc *Context, args json.RawMessage) (*ExecuteResult, error) {
			return &ExecuteResult{Output: "ok"}, nil
		},
	})

	r.Execute(context.Background(), "noop", json.RawMessage(`{}`), "sess-1")

	events := collectEvents(sub, 1, time.Second)
	if len(events) != 1 {
		t.Fatalf("expected 1 before event, got %d", len(events))
	}

	props := events[0].Properties.(map[string]any)
	if props["sessionID"] != "sess-1" {
		t.Errorf("expected sessionID 'sess-1', got %v", props["sessionID"])
	}
	if props["tool"] != "noop" {
		t.Errorf("expected tool 'noop', got %v", props["tool"])
	}
}

func TestExecuteAfterEvent_Success(t *testing.T) {
	b := bus.New()
	defer b.Close()

	sub := b.Subscribe("tool.execute.after")
	defer sub.Unsubscribe()

	r := NewRegistry(&Context{Directory: t.TempDir(), Bus: b})
	r.Register(&Def{
		ID:         "ok-tool",
		Permission: "read",
		Parameters: map[string]any{"type": "object"},
		Execute: func(ctx context.Context, tc *Context, args json.RawMessage) (*ExecuteResult, error) {
			return &ExecuteResult{Output: "done"}, nil
		},
	})

	r.Execute(context.Background(), "ok-tool", json.RawMessage(`{}`), "sess-2")

	events := collectEvents(sub, 1, time.Second)
	if len(events) != 1 {
		t.Fatalf("expected 1 after event, got %d", len(events))
	}

	props := events[0].Properties.(map[string]any)
	if props["isError"] != false {
		t.Errorf("expected isError=false, got %v", props["isError"])
	}
	if props["output"] != "done" {
		t.Errorf("expected output='done', got %v", props["output"])
	}
}

func TestExecuteAfterEvent_ToolError(t *testing.T) {
	b := bus.New()
	defer b.Close()

	sub := b.Subscribe("tool.execute.after")
	defer sub.Unsubscribe()

	r := NewRegistry(&Context{Directory: t.TempDir(), Bus: b})
	r.Register(&Def{
		ID:         "fail-tool",
		Permission: "read",
		Parameters: map[string]any{"type": "object"},
		Execute: func(ctx context.Context, tc *Context, args json.RawMessage) (*ExecuteResult, error) {
			return &ExecuteResult{Output: "bad", IsError: true}, nil
		},
	})

	r.Execute(context.Background(), "fail-tool", json.RawMessage(`{}`), "sess-3")

	events := collectEvents(sub, 1, time.Second)
	if len(events) != 1 {
		t.Fatalf("expected 1 after event, got %d", len(events))
	}

	props := events[0].Properties.(map[string]any)
	if props["isError"] != true {
		t.Errorf("expected isError=true, got %v", props["isError"])
	}
	if props["output"] != "bad" {
		t.Errorf("expected output='bad', got %v", props["output"])
	}
}

func TestWriteTool_FileModifiedEvent(t *testing.T) {
	b := bus.New()
	defer b.Close()

	sub := b.Subscribe("file.modified")
	defer sub.Unsubscribe()

	dir := t.TempDir()
	path := filepath.Join(dir, "event-test.txt")

	r := NewRegistry(&Context{Directory: dir, Bus: b})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"file_path": "` + path + `", "content": "hello"}`)
	output, isErr, _ := r.Execute(context.Background(), "write", args, "sess-w")
	if isErr {
		t.Fatalf("write failed: %s", output)
	}

	events := collectEvents(sub, 1, time.Second)
	if len(events) != 1 {
		t.Fatalf("expected 1 file.modified event, got %d", len(events))
	}

	props := events[0].Properties.(map[string]any)
	if props["operation"] != "write" {
		t.Errorf("expected operation 'write', got %v", props["operation"])
	}
	if props["path"] != path {
		t.Errorf("expected path %q, got %v", path, props["path"])
	}
	if props["sessionID"] != "sess-w" {
		t.Errorf("expected sessionID 'sess-w', got %v", props["sessionID"])
	}
}

func TestEditTool_FileModifiedEvent(t *testing.T) {
	b := bus.New()
	defer b.Close()

	sub := b.Subscribe("file.modified")
	defer sub.Unsubscribe()

	dir := t.TempDir()
	path := filepath.Join(dir, "edit-event.txt")
	os.WriteFile(path, []byte("old content"), 0644)

	r := NewRegistry(&Context{Directory: dir, Bus: b})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"file_path": "` + path + `", "old_string": "old", "new_string": "new"}`)
	output, isErr, _ := r.Execute(context.Background(), "edit", args, "sess-e")
	if isErr {
		t.Fatalf("edit failed: %s", output)
	}

	events := collectEvents(sub, 1, time.Second)
	if len(events) != 1 {
		t.Fatalf("expected 1 file.modified event, got %d", len(events))
	}

	props := events[0].Properties.(map[string]any)
	if props["operation"] != "edit" {
		t.Errorf("expected operation 'edit', got %v", props["operation"])
	}
	if props["path"] != path {
		t.Errorf("expected path %q, got %v", path, props["path"])
	}
}

func waitForPermission(t *testing.T, perms *permission.Service, timeout time.Duration) permission.Request {
	t.Helper()
	deadline := time.After(timeout)
	for {
		reqs := perms.List()
		if len(reqs) > 0 {
			return reqs[0]
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for permission request")
			return permission.Request{}
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestShellTool_DestructiveRoutesPermission(t *testing.T) {
	b := bus.New()
	defer b.Close()

	perms := permission.NewService(b)
	defer perms.Close()

	dir := t.TempDir()
	r := NewRegistry(&Context{Directory: dir, Bus: b, Perms: perms})
	RegisterBuiltins(r)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	var output string
	var isErr bool

	go func() {
		output, isErr, _ = r.Execute(ctx, "shell", json.RawMessage(`{"command": "rm -rf /tmp/test"}`), "sess-d")
		close(done)
	}()

	// First: the registry-level "shell" permission check fires
	req := waitForPermission(t, perms, 2*time.Second)
	if req.Permission != "shell" {
		t.Errorf("expected first permission 'shell', got %q", req.Permission)
	}
	perms.RespondToAsk(permission.ReplyInput{
		RequestID: req.ID,
		Reply:     permission.ReplyOnce,
	})

	// Second: the destructive-specific "destructive-shell" permission check fires
	req = waitForPermission(t, perms, 2*time.Second)
	if req.Permission != "destructive-shell" {
		t.Errorf("expected second permission 'destructive-shell', got %q", req.Permission)
	}

	// Reject the destructive permission
	perms.RespondToAsk(permission.ReplyInput{
		RequestID: req.ID,
		Reply:     permission.ReplyReject,
	})

	<-done

	if !isErr {
		t.Error("expected error after permission rejection")
	}
	if output == "" {
		t.Error("expected non-empty error output")
	}
}

func TestShellTool_DestructiveHardBlocksWithoutPerms(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(&Context{Directory: dir})
	RegisterBuiltins(r)

	args := json.RawMessage(`{"command": "rm -rf /tmp/test"}`)
	output, isErr, _ := r.Execute(context.Background(), "shell", args, "sess-nb")
	if !isErr {
		t.Error("expected destructive command to be hard-blocked without perms")
	}
	if output == "" {
		t.Error("expected non-empty error output")
	}
}
