package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func newNotepadContext() *Context {
	return &Context{
		SessionID: "test-session",
		Notepad:   NewSafeNotepad(),
	}
}

func execNotepad(t *testing.T, tc *Context, args any) *ExecuteResult {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	def := NotepadTool()
	result, err := def.Execute(context.Background(), tc, raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return result
}

func TestNotepad_WriteAndRead(t *testing.T) {
	tc := newNotepadContext()

	r := execNotepad(t, tc, map[string]string{"action": "write", "key": "plan", "content": "step 1: do the thing"})
	if r.IsError {
		t.Fatalf("write failed: %s", r.Output)
	}
	if !strings.Contains(r.Output, `"plan"`) {
		t.Errorf("expected key in output, got: %s", r.Output)
	}

	r = execNotepad(t, tc, map[string]string{"action": "read", "key": "plan"})
	if r.IsError {
		t.Fatalf("read failed: %s", r.Output)
	}
	if r.Output != "step 1: do the thing" {
		t.Errorf("expected stored content, got: %s", r.Output)
	}
}

func TestNotepad_ListShowsKeysWithPreviews(t *testing.T) {
	tc := newNotepadContext()

	execNotepad(t, tc, map[string]string{"action": "write", "key": "alpha", "content": "first line\nsecond line"})
	execNotepad(t, tc, map[string]string{"action": "write", "key": "beta", "content": "hello world"})

	r := execNotepad(t, tc, map[string]string{"action": "list"})
	if r.IsError {
		t.Fatalf("list failed: %s", r.Output)
	}
	if !strings.Contains(r.Output, "2 notes") {
		t.Errorf("expected count, got: %s", r.Output)
	}
	if !strings.Contains(r.Output, "alpha: first line") {
		t.Errorf("expected alpha preview (first line only), got: %s", r.Output)
	}
	if !strings.Contains(r.Output, "beta: hello world") {
		t.Errorf("expected beta preview, got: %s", r.Output)
	}
}

func TestNotepad_Delete(t *testing.T) {
	tc := newNotepadContext()

	execNotepad(t, tc, map[string]string{"action": "write", "key": "tmp", "content": "data"})

	r := execNotepad(t, tc, map[string]string{"action": "delete", "key": "tmp"})
	if r.IsError {
		t.Fatalf("delete failed: %s", r.Output)
	}
	if !strings.Contains(r.Output, "Deleted") {
		t.Errorf("expected deleted confirmation, got: %s", r.Output)
	}

	r = execNotepad(t, tc, map[string]string{"action": "read", "key": "tmp"})
	if r.IsError {
		t.Fatalf("read after delete should not be error: %s", r.Output)
	}
	if !strings.Contains(r.Output, "not found") {
		t.Errorf("expected not found, got: %s", r.Output)
	}
}

func TestNotepad_KeyNotFound(t *testing.T) {
	tc := newNotepadContext()

	r := execNotepad(t, tc, map[string]string{"action": "read", "key": "missing"})
	if r.IsError {
		t.Errorf("key not found should not be an error result")
	}
	if !strings.Contains(r.Output, "not found") {
		t.Errorf("expected not found message, got: %s", r.Output)
	}

	r = execNotepad(t, tc, map[string]string{"action": "delete", "key": "missing"})
	if r.IsError {
		t.Errorf("delete missing should not be an error result")
	}
	if !strings.Contains(r.Output, "not found") {
		t.Errorf("expected not found message, got: %s", r.Output)
	}
}

func TestNotepad_MaxEntriesEnforcement(t *testing.T) {
	tc := newNotepadContext()

	for i := range maxNotepadEntries {
		r := execNotepad(t, tc, map[string]string{
			"action":  "write",
			"key":     strings.Repeat("k", 3) + string(rune('A'+i%26)) + string(rune('0'+i/26)),
			"content": "v",
		})
		if r.IsError {
			t.Fatalf("write %d failed: %s", i, r.Output)
		}
	}

	r := execNotepad(t, tc, map[string]string{"action": "write", "key": "overflow", "content": "v"})
	if !r.IsError {
		t.Error("expected error when exceeding max entries")
	}
	if !strings.Contains(r.Output, "notepad full") {
		t.Errorf("expected full message, got: %s", r.Output)
	}
}

func TestNotepad_MaxSizeEnforcement(t *testing.T) {
	tc := newNotepadContext()

	large := strings.Repeat("x", maxNotepadValue+1)
	r := execNotepad(t, tc, map[string]string{"action": "write", "key": "big", "content": large})
	if !r.IsError {
		t.Error("expected error for oversized content")
	}
	if !strings.Contains(r.Output, "exceeds maximum size") {
		t.Errorf("expected size error, got: %s", r.Output)
	}
}

func TestNotepad_WriteUpdatesExistingKey(t *testing.T) {
	tc := newNotepadContext()

	execNotepad(t, tc, map[string]string{"action": "write", "key": "k", "content": "v1"})
	execNotepad(t, tc, map[string]string{"action": "write", "key": "k", "content": "v2"})

	r := execNotepad(t, tc, map[string]string{"action": "read", "key": "k"})
	if r.Output != "v2" {
		t.Errorf("expected updated value, got: %s", r.Output)
	}
}

func TestNotepad_EmptyAction(t *testing.T) {
	tc := newNotepadContext()

	r := execNotepad(t, tc, map[string]string{"action": ""})
	if !r.IsError {
		t.Error("expected error for empty action")
	}
	if !strings.Contains(r.Output, "invalid action") {
		t.Errorf("expected invalid action message, got: %s", r.Output)
	}
}

func TestWithAutoApprove_PropagatesFindings(t *testing.T) {
	reg := NewRegistry(&Context{
		SessionID: "test",
		Findings:  NewSafeFindings(),
		Notepad:   NewSafeNotepad(),
	})

	autoReg := reg.WithAutoApprove()

	if autoReg.ctx.Findings == nil {
		t.Fatal("WithAutoApprove did not propagate Findings")
	}
	if autoReg.ctx.Findings != reg.ctx.Findings {
		t.Error("WithAutoApprove should share the same Findings pointer")
	}
}

func TestWithAutoApprove_PropagatesNotepad(t *testing.T) {
	reg := NewRegistry(&Context{
		SessionID: "test",
		Findings:  NewSafeFindings(),
		Notepad:   NewSafeNotepad(),
	})

	autoReg := reg.WithAutoApprove()

	if autoReg.ctx.Notepad == nil {
		t.Fatal("WithAutoApprove did not propagate Notepad")
	}
	if autoReg.ctx.Notepad != reg.ctx.Notepad {
		t.Error("WithAutoApprove should share the same Notepad pointer")
	}
}
