package tui

import "testing"

func TestHistory_AddDedup(t *testing.T) {
	h := NewPromptHistory()
	h.Add("hello")
	h.Add("hello") // duplicate, should be skipped
	h.Add("world")

	if h.Len() != 2 {
		t.Fatalf("expected 2 entries after dedup, got %d", h.Len())
	}
}

func TestHistory_AddEmpty(t *testing.T) {
	h := NewPromptHistory()
	h.Add("")
	if h.Len() != 0 {
		t.Fatalf("expected 0 entries after adding empty string, got %d", h.Len())
	}
}

func TestHistory_AddNonConsecutiveDuplicates(t *testing.T) {
	h := NewPromptHistory()
	h.Add("a")
	h.Add("b")
	h.Add("a") // not consecutive, should be kept

	if h.Len() != 3 {
		t.Fatalf("expected 3 entries for non-consecutive duplicates, got %d", h.Len())
	}
}

func TestHistory_PreviousNext(t *testing.T) {
	h := NewPromptHistory()
	h.Add("first")
	h.Add("second")
	h.Add("third")

	// Previous from live input stashes current and returns newest
	val, ok := h.Previous("current-input")
	if !ok {
		t.Fatal("expected ok from Previous")
	}
	if val != "third" {
		t.Errorf("expected 'third', got %q", val)
	}

	// Previous again returns second
	val, ok = h.Previous("")
	if !ok {
		t.Fatal("expected ok")
	}
	if val != "second" {
		t.Errorf("expected 'second', got %q", val)
	}

	// Previous again returns first
	val, ok = h.Previous("")
	if !ok {
		t.Fatal("expected ok")
	}
	if val != "first" {
		t.Errorf("expected 'first', got %q", val)
	}

	// Previous wraps to stash
	val, ok = h.Previous("")
	if !ok {
		t.Fatal("expected ok on wrap")
	}
	if val != "current-input" {
		t.Errorf("expected stashed 'current-input', got %q", val)
	}
}

func TestHistory_Next(t *testing.T) {
	h := NewPromptHistory()
	h.Add("first")
	h.Add("second")

	// Navigate to oldest via Previous
	h.Previous("live")  // -> "second", stashes "live"
	h.Previous("")      // -> "first"

	// Next goes forward
	val, ok := h.Next("")
	if !ok {
		t.Fatal("expected ok from Next")
	}
	if val != "second" {
		t.Errorf("expected 'second', got %q", val)
	}

	// Next past newest returns stash
	val, ok = h.Next("")
	if !ok {
		t.Fatal("expected ok")
	}
	if val != "live" {
		t.Errorf("expected stashed 'live', got %q", val)
	}
}

func TestHistory_NextWraps(t *testing.T) {
	h := NewPromptHistory()
	h.Add("a")
	h.Add("b")

	// Next from live input wraps to oldest
	val, ok := h.Next("typing")
	if !ok {
		t.Fatal("expected ok")
	}
	if val != "a" {
		t.Errorf("expected 'a', got %q", val)
	}
}

func TestHistory_StashSwap(t *testing.T) {
	h := NewPromptHistory()
	h.Add("hist1")

	// Stash saves current text
	h.Stash("in-progress")
	if h.StashValue() != "in-progress" {
		t.Errorf("expected stash 'in-progress', got %q", h.StashValue())
	}

	// Stash resets cursor
	h.Previous("") // navigates
	h.Stash("new-stash")
	// Cursor should be -1 after Stash
	if h.cursor != -1 {
		t.Errorf("expected cursor -1 after Stash, got %d", h.cursor)
	}
}

func TestHistory_EmptyHistory_PreviousNext(t *testing.T) {
	h := NewPromptHistory()

	_, ok := h.Previous("x")
	if ok {
		t.Fatal("expected Previous to return false on empty history")
	}

	_, ok = h.Next("x")
	if ok {
		t.Fatal("expected Next to return false on empty history")
	}
}

func TestHistory_MaxSize(t *testing.T) {
	h := NewPromptHistory()
	for i := 0; i < 150; i++ {
		h.Add(string(rune('a' + (i % 26))))
	}
	// Non-consecutive dedup won't kick in for all, but size should be capped.
	if h.Len() > maxHistorySize {
		t.Fatalf("expected at most %d entries, got %d", maxHistorySize, h.Len())
	}
}

func TestHistory_Reset(t *testing.T) {
	h := NewPromptHistory()
	h.Add("a")
	h.Previous("live")
	h.Reset()

	if h.cursor != -1 {
		t.Errorf("expected cursor -1 after Reset, got %d", h.cursor)
	}
	if h.stash != "" {
		t.Errorf("expected empty stash after Reset, got %q", h.stash)
	}
	// Entries should still be there
	if h.Len() != 1 {
		t.Errorf("expected 1 entry after Reset, got %d", h.Len())
	}
}
