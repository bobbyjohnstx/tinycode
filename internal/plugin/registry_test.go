package plugin

import "testing"

func TestRegistry_NonEmpty(t *testing.T) {
	entries := Registry()
	if len(entries) == 0 {
		t.Fatal("expected non-empty registry")
	}
}

func TestRegistry_ReturnsCopy(t *testing.T) {
	a := Registry()
	b := Registry()
	a[0].Name = "mutated"
	if b[0].Name == "mutated" {
		t.Error("Registry should return a copy, not a shared slice")
	}
}

func TestLookupRegistry_Found(t *testing.T) {
	entry, ok := LookupRegistry("notify")
	if !ok {
		t.Fatal("expected to find 'notify' in registry")
	}
	if entry.Name != "notify" {
		t.Errorf("expected name 'notify', got %q", entry.Name)
	}
	if entry.Description == "" {
		t.Error("expected non-empty description")
	}
	if entry.Repo == "" {
		t.Error("expected non-empty repo")
	}
	if entry.Binary == "" {
		t.Error("expected non-empty binary")
	}
}

func TestLookupRegistry_NotFound(t *testing.T) {
	_, ok := LookupRegistry("nonexistent-plugin")
	if ok {
		t.Error("expected false for unknown plugin")
	}
}
