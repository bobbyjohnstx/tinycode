package tool

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
)

// TestRegistry_ConcurrentAccess verifies that concurrent Register, Execute,
// ToolDefs, Get, and List calls do not race. Run with -race to confirm.
func TestRegistry_ConcurrentAccess(t *testing.T) {
	r := NewRegistry(&Context{Directory: t.TempDir()})

	// Seed one tool so Execute has something to call.
	r.Register(&Def{
		ID:         "seed",
		Permission: "read",
		Parameters: map[string]any{"type": "object"},
		Execute: func(_ context.Context, _ *Context, _ json.RawMessage) (*ExecuteResult, error) {
			return &ExecuteResult{Output: "ok"}, nil
		},
	})

	var wg sync.WaitGroup
	const goroutines = 20

	// Concurrent registers
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			r.Register(&Def{
				ID:         "dynamic",
				Permission: "read",
				Parameters: map[string]any{"type": "object"},
				Execute: func(_ context.Context, _ *Context, _ json.RawMessage) (*ExecuteResult, error) {
					return &ExecuteResult{Output: "ok"}, nil
				},
			})
		}(i)
	}

	// Concurrent reads
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.ToolDefs(nil)
			r.List()
			r.Get("seed")
			r.Execute(context.Background(), "seed", json.RawMessage(`{}`), "s1")
		}()
	}

	wg.Wait()
}

func TestRegistry_Snapshot(t *testing.T) {
	r := NewRegistry(&Context{Directory: t.TempDir()})
	r.Register(&Def{ID: "a", Permission: "read", Parameters: map[string]any{"type": "object"}})
	r.Register(&Def{ID: "b", Permission: "edit", Parameters: map[string]any{"type": "object"}})

	snap := r.Snapshot()

	// Verify snapshot has same tools
	if len(snap.List()) != 2 {
		t.Fatalf("expected 2 tools in snapshot, got %d", len(snap.List()))
	}

	// Mutate original
	r.Register(&Def{ID: "c", Permission: "shell", Parameters: map[string]any{"type": "object"}})

	// Snapshot should be unaffected
	if len(snap.List()) != 2 {
		t.Errorf("snapshot was affected by original mutation: got %d tools", len(snap.List()))
	}

	// Original should have 3
	if len(r.List()) != 3 {
		t.Errorf("expected 3 tools in original, got %d", len(r.List()))
	}
}

func TestRegistry_SnapshotDisabled(t *testing.T) {
	r := NewRegistry(&Context{Directory: t.TempDir()})
	r.Register(&Def{ID: "a", Permission: "read", Parameters: map[string]any{"type": "object"}})
	r.SetDisabled(map[string]bool{"a": true})

	snap := r.Snapshot()

	// Snapshot preserves disabled state
	names := snap.List()
	if len(names) != 0 {
		t.Errorf("expected 0 enabled tools in snapshot, got %d", len(names))
	}

	// Change disabled on original
	r.SetDisabled(map[string]bool{})

	// Snapshot should still have it disabled
	names = snap.List()
	if len(names) != 0 {
		t.Errorf("snapshot disabled state was affected by original mutation, got %d tools", len(names))
	}
}
