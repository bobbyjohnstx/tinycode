package server

import (
	"sync"
	"testing"
	"time"
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

func TestQuestionStore_GetExistingEntry(t *testing.T) {
	qs := NewQuestionStore()
	qs.Add(PendingQuestion{ID: "q1", SessionID: "s1", Question: "yes?"})

	got, ok := qs.Get("q1")
	if !ok {
		t.Fatal("expected Get to return true for existing entry")
	}
	if got.ID != "q1" {
		t.Errorf("ID = %q, want 'q1'", got.ID)
	}
	if got.Question != "yes?" {
		t.Errorf("Question = %q, want 'yes?'", got.Question)
	}
}

func TestQuestionStore_GetNonExistentEntry(t *testing.T) {
	qs := NewQuestionStore()
	_, ok := qs.Get("nonexistent")
	if ok {
		t.Error("expected Get to return false for nonexistent entry")
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

func TestPermissionStore_CleanupRemovesExpiredEntries(t *testing.T) {
	ps := NewPermissionStore()

	// Manually add an expired entry by setting createdAt in the past.
	ps.mu.Lock()
	ps.pending["old"] = PendingPermission{
		ID:        "old",
		Tool:      "shell",
		createdAt: time.Now().Add(-11 * time.Minute), // past pendingTTL (10 minutes)
	}
	ps.mu.Unlock()

	// Add a fresh entry — this triggers cleanupLocked.
	ps.Add(PendingPermission{ID: "new", Tool: "read"})

	list := ps.List()
	for _, p := range list {
		if p.ID == "old" {
			t.Error("expected expired 'old' entry to be cleaned up")
		}
	}

	found := false
	for _, p := range list {
		if p.ID == "new" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'new' entry to remain after cleanup")
	}
}

func TestQuestionStore_CleanupRemovesExpiredEntries(t *testing.T) {
	qs := NewQuestionStore()

	// Manually add an expired entry.
	qs.mu.Lock()
	qs.pending["old"] = PendingQuestion{
		ID:        "old",
		Question:  "expired?",
		createdAt: time.Now().Add(-11 * time.Minute),
	}
	qs.mu.Unlock()

	// Add a fresh entry triggers cleanup.
	qs.Add(PendingQuestion{ID: "new", Question: "fresh?"})

	list := qs.List()
	for _, q := range list {
		if q.ID == "old" {
			t.Error("expected expired 'old' entry to be cleaned up")
		}
	}

	found := false
	for _, q := range list {
		if q.ID == "new" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'new' entry to remain after cleanup")
	}
}

func TestQuestionStore_Concurrent(t *testing.T) {
	qs := NewQuestionStore()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(4)
		id := "q" + string(rune('A'+i%26))
		go func() {
			defer wg.Done()
			qs.Add(PendingQuestion{ID: id, Question: "q?"})
		}()
		go func() {
			defer wg.Done()
			qs.Get(id)
		}()
		go func() {
			defer wg.Done()
			qs.Remove(id)
		}()
		go func() {
			defer wg.Done()
			qs.List()
		}()
	}
	wg.Wait()
}
