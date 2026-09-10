package server

import (
	"sync"
	"testing"
)

func TestNewPermissionStore(t *testing.T) {
	ps := NewPermissionStore()
	if ps == nil {
		t.Fatal("expected non-nil PermissionStore")
	}
	if got := ps.List(); len(got) != 0 {
		t.Errorf("expected empty list, got %d items", len(got))
	}
}

func TestPermissionStore_AddAndList(t *testing.T) {
	ps := NewPermissionStore()
	ps.Add(PendingPermission{ID: "p1", SessionID: "s1", Tool: "shell", Description: "run ls"})

	list := ps.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 item, got %d", len(list))
	}
	if list[0].ID != "p1" {
		t.Errorf("ID = %q, want %q", list[0].ID, "p1")
	}
	if list[0].Tool != "shell" {
		t.Errorf("Tool = %q, want %q", list[0].Tool, "shell")
	}
}

func TestPermissionStore_AddMultiple(t *testing.T) {
	ps := NewPermissionStore()
	ps.Add(PendingPermission{ID: "p1", Tool: "shell"})
	ps.Add(PendingPermission{ID: "p2", Tool: "write"})
	ps.Add(PendingPermission{ID: "p3", Tool: "edit"})

	list := ps.List()
	if len(list) != 3 {
		t.Fatalf("expected 3 items, got %d", len(list))
	}

	ids := make(map[string]bool)
	for _, p := range list {
		ids[p.ID] = true
	}
	for _, id := range []string{"p1", "p2", "p3"} {
		if !ids[id] {
			t.Errorf("missing ID %q in list", id)
		}
	}
}

func TestPermissionStore_Remove(t *testing.T) {
	ps := NewPermissionStore()
	ps.Add(PendingPermission{ID: "p1", Tool: "shell"})
	ps.Add(PendingPermission{ID: "p2", Tool: "write"})

	ps.Remove("p1")

	list := ps.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 item after remove, got %d", len(list))
	}
	if list[0].ID != "p2" {
		t.Errorf("remaining ID = %q, want %q", list[0].ID, "p2")
	}
}

func TestPermissionStore_RemoveNonExistent(t *testing.T) {
	ps := NewPermissionStore()
	ps.Add(PendingPermission{ID: "p1"})
	ps.Remove("nonexistent")

	if len(ps.List()) != 1 {
		t.Error("removing nonexistent ID should not affect store")
	}
}

func TestPermissionStore_ListEmpty(t *testing.T) {
	ps := NewPermissionStore()
	list := ps.List()
	if list == nil {
		t.Error("List should return non-nil empty slice")
	}
	if len(list) != 0 {
		t.Errorf("expected 0 items, got %d", len(list))
	}
}

func TestPermissionStore_Concurrent(t *testing.T) {
	ps := NewPermissionStore()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(3)
		id := "p" + string(rune('A'+i%26))
		go func() {
			defer wg.Done()
			ps.Add(PendingPermission{ID: id, Tool: "shell"})
		}()
		go func() {
			defer wg.Done()
			ps.Remove(id)
		}()
		go func() {
			defer wg.Done()
			ps.List()
		}()
	}
	wg.Wait()
}

func TestNewQuestionStore(t *testing.T) {
	qs := NewQuestionStore()
	if qs == nil {
		t.Fatal("expected non-nil QuestionStore")
	}
	if got := qs.List(); len(got) != 0 {
		t.Errorf("expected empty list, got %d items", len(got))
	}
}

func TestQuestionStore_AddAndList(t *testing.T) {
	qs := NewQuestionStore()
	qs.Add(PendingQuestion{ID: "q1", SessionID: "s1", Question: "proceed?"})

	list := qs.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 item, got %d", len(list))
	}
	if list[0].ID != "q1" {
		t.Errorf("ID = %q, want %q", list[0].ID, "q1")
	}
	if list[0].Question != "proceed?" {
		t.Errorf("Question = %q, want %q", list[0].Question, "proceed?")
	}
}

func TestQuestionStore_Remove(t *testing.T) {
	qs := NewQuestionStore()
	qs.Add(PendingQuestion{ID: "q1", Question: "yes?"})
	qs.Add(PendingQuestion{ID: "q2", Question: "no?"})

	qs.Remove("q1")

	list := qs.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 item, got %d", len(list))
	}
	if list[0].ID != "q2" {
		t.Errorf("remaining ID = %q, want %q", list[0].ID, "q2")
	}
}

func TestQuestionStore_ListEmpty(t *testing.T) {
	qs := NewQuestionStore()
	list := qs.List()
	if list == nil {
		t.Error("List should return non-nil empty slice")
	}
	if len(list) != 0 {
		t.Errorf("expected 0 items, got %d", len(list))
	}
}
