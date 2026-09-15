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
	entry, ok := LookupRegistry("log-sanitizer")
	if !ok {
		t.Fatal("expected to find 'log-sanitizer' in registry")
	}
	if entry.Name != "log-sanitizer" {
		t.Errorf("expected name 'log-sanitizer', got %q", entry.Name)
	}
	if entry.Description == "" {
		t.Error("expected non-empty description")
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

func TestRegistry_AllHaveCategory(t *testing.T) {
	for _, e := range Registry() {
		if e.Category == "" {
			t.Errorf("plugin %q has no category", e.Name)
		}
	}
}

func TestCategories_NonEmpty(t *testing.T) {
	cats := Categories()
	if len(cats) == 0 {
		t.Fatal("expected non-empty categories list")
	}
	for _, c := range cats {
		if c.Slug == "" {
			t.Error("category has empty slug")
		}
		if c.Label == "" {
			t.Errorf("category %q has empty label", c.Slug)
		}
	}
}

func TestRegistryByCategory(t *testing.T) {
	sre := RegistryByCategory(CategorySRE)
	if len(sre) == 0 {
		t.Fatal("expected non-empty SRE category")
	}
	for _, e := range sre {
		if e.Category != CategorySRE {
			t.Errorf("plugin %q in SRE results has category %q", e.Name, e.Category)
		}
	}
}

func TestRegistryByCategory_Unknown(t *testing.T) {
	results := RegistryByCategory("nonexistent-category")
	if len(results) != 0 {
		t.Errorf("expected 0 results for unknown category, got %d", len(results))
	}
}

func TestRegistryByCategory_CoversAll(t *testing.T) {
	total := 0
	for _, c := range Categories() {
		total += len(RegistryByCategory(c.Slug))
	}
	if total != len(Registry()) {
		t.Errorf("category sum (%d) != registry size (%d) — some plugins have invalid categories", total, len(Registry()))
	}
}
