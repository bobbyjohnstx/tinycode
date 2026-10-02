package frecency

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScore_UnknownItem(t *testing.T) {
	s := New("/dev/null")
	if score := s.Score("nonexistent"); score != 0 {
		t.Fatalf("expected 0 for unknown item, got %f", score)
	}
}

func TestRecord_IncreasesScore(t *testing.T) {
	s := New("/dev/null")
	before := s.Score("item")
	s.Record("item")
	after := s.Score("item")
	if after <= before {
		t.Fatalf("expected score to increase after Record, before=%f after=%f", before, after)
	}
}

func TestScore_RecencyWeighting(t *testing.T) {
	s := New("/dev/null")

	now := time.Now().UnixMilli()

	// Add a recent timestamp (within 4 hours).
	s.mu.Lock()
	s.data["recent"] = Entry{
		Visits:     1,
		Timestamps: []int64{now - 1000}, // 1 second ago
	}
	// Add an old timestamp (over 30 days ago).
	s.data["old"] = Entry{
		Visits:     1,
		Timestamps: []int64{now - 31*24*60*60*1000},
	}
	s.mu.Unlock()

	recentScore := s.Score("recent")
	oldScore := s.Score("old")
	if recentScore <= oldScore {
		t.Fatalf("expected recent score (%f) > old score (%f)", recentScore, oldScore)
	}
}

func TestSort_OrdersByScore(t *testing.T) {
	s := New("/dev/null")

	now := time.Now().UnixMilli()
	s.mu.Lock()
	s.data["a"] = Entry{Visits: 1, Timestamps: []int64{now - 31*24*60*60*1000}} // old
	s.data["b"] = Entry{Visits: 3, Timestamps: []int64{now - 1000, now - 2000, now - 3000}} // recent, 3 visits
	s.mu.Unlock()

	items := []string{"a", "b", "c"}
	sorted := s.Sort(items)

	if sorted[0] != "b" {
		t.Fatalf("expected 'b' first, got %v", sorted)
	}
	if sorted[1] != "a" {
		t.Fatalf("expected 'a' second, got %v", sorted)
	}
	if sorted[2] != "c" {
		t.Fatalf("expected 'c' last (unknown), got %v", sorted)
	}
}

func TestSort_UnknownsPreserveOrder(t *testing.T) {
	s := New("/dev/null")

	items := []string{"x", "y", "z"}
	sorted := s.Sort(items)

	for i, item := range items {
		if sorted[i] != item {
			t.Fatalf("expected unknowns to preserve order, got %v", sorted)
		}
	}
}

func TestTimestamps_CappedAt100(t *testing.T) {
	s := New("/dev/null")
	for i := 0; i < 120; i++ {
		s.Record("item")
	}
	s.mu.RLock()
	count := len(s.data["item"].Timestamps)
	s.mu.RUnlock()
	if count != maxTimestamps {
		t.Fatalf("expected %d timestamps, got %d", maxTimestamps, count)
	}
}

func TestEntries_CappedAt500(t *testing.T) {
	s := New("/dev/null")
	for i := 0; i < 600; i++ {
		s.Record("item" + string(rune('A'+i%26)) + string(rune('a'+i/26)))
	}
	s.mu.Lock()
	s.dirty = true
	s.pruneLocked()
	count := len(s.data)
	s.mu.Unlock()
	if count > maxEntries {
		t.Fatalf("expected at most %d entries after prune, got %d", maxEntries, count)
	}
}

func TestLoadSave_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "frecency.json")

	s1 := New(path)
	s1.Record("alpha")
	s1.Record("beta")
	s1.Record("beta")
	if err := s1.Save(); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	s2 := New(path)
	if err := s2.Load(); err != nil {
		t.Fatalf("load failed: %v", err)
	}

	s2.mu.RLock()
	alphaVisits := s2.data["alpha"].Visits
	betaVisits := s2.data["beta"].Visits
	s2.mu.RUnlock()

	if alphaVisits != 1 {
		t.Fatalf("expected alpha visits=1, got %d", alphaVisits)
	}
	if betaVisits != 2 {
		t.Fatalf("expected beta visits=2, got %d", betaVisits)
	}
}

func TestLoad_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "frecency.json")
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	s := New(path)
	if err := s.Load(); err != nil {
		t.Fatalf("load of empty file should succeed, got %v", err)
	}
}

func TestLoad_NonexistentFile(t *testing.T) {
	s := New("/nonexistent/path/frecency.json")
	if err := s.Load(); err != nil {
		t.Fatalf("load of nonexistent file should succeed, got %v", err)
	}
}

func TestSave_SkipsWhenNotDirty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "frecency.json")

	s := New(path)
	if err := s.Save(); err != nil {
		t.Fatalf("save with no dirty data should succeed, got %v", err)
	}
	// File should not exist since nothing was dirty.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected no file created when not dirty")
	}
}
