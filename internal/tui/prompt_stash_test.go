package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestPromptStash_RoundTrip verifies ctrl+s parks the current draft and a
// second ctrl+s restores it verbatim.
func TestPromptStash_RoundTrip(t *testing.T) {
	p := NewPromptInput(80)
	p.SetValue("unsent draft")

	if p.HasDraft() {
		t.Fatal("expected no draft stashed initially")
	}

	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if !p.HasDraft() {
		t.Fatal("expected a draft to be stashed after ctrl+s")
	}
	if p.DraftValue() != "unsent draft" {
		t.Fatalf("expected stashed draft %q, got %q", "unsent draft", p.DraftValue())
	}
	if p.Value() != "" {
		t.Fatalf("expected textarea cleared after stash, got %q", p.Value())
	}

	// Second ctrl+s (textarea is empty) restores the draft.
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if p.HasDraft() {
		t.Fatal("expected draft cleared after restore")
	}
	if p.Value() != "unsent draft" {
		t.Fatalf("expected restored draft %q, got %q", "unsent draft", p.Value())
	}
}

// TestPromptStash_SurvivesReset verifies the stash is untouched by Reset(),
// which only clears the textarea (e.g. after submitting a prompt).
func TestPromptStash_SurvivesReset(t *testing.T) {
	p := NewPromptInput(80)
	p.SetValue("park me")
	if !p.StashDraft() {
		t.Fatal("expected StashDraft to succeed with non-empty textarea")
	}

	p.SetValue("something else entirely")
	p.Reset() // simulates textarea.Reset() after a submit

	if !p.HasDraft() {
		t.Fatal("expected stashed draft to survive Reset()")
	}
	if p.DraftValue() != "park me" {
		t.Fatalf("expected stash to still hold %q, got %q", "park me", p.DraftValue())
	}
	if !p.RestoreDraft() {
		t.Fatal("expected RestoreDraft to succeed")
	}
	if p.Value() != "park me" {
		t.Fatalf("expected restored value %q, got %q", "park me", p.Value())
	}
}

// TestPromptStash_EmptyNoop verifies stashing with an empty textarea is a
// no-op, and restoring with no stash is a no-op.
func TestPromptStash_EmptyNoop(t *testing.T) {
	p := NewPromptInput(80)

	if p.StashDraft() {
		t.Fatal("expected StashDraft to no-op on empty textarea")
	}
	if p.RestoreDraft() {
		t.Fatal("expected RestoreDraft to no-op with no stash")
	}
}

// TestPromptStash_OverwritesPrevious verifies stashing again overwrites any
// previously parked draft rather than merging it.
func TestPromptStash_OverwritesPrevious(t *testing.T) {
	p := NewPromptInput(80)
	p.SetValue("first draft")
	p.StashDraft()

	p.SetValue("second draft")
	p.StashDraft()

	if p.DraftValue() != "second draft" {
		t.Fatalf("expected stash overwritten to %q, got %q", "second draft", p.DraftValue())
	}
}

// TestPromptStash_MetadataHint verifies the status bar surfaces a hint once
// a draft is stashed (beyond the bare keybind).
func TestPromptStash_MetadataHint(t *testing.T) {
	p := NewPromptInput(80)
	p.SetValue("to park")
	p.StashDraft()

	meta := p.renderMetadata()
	if !strings.Contains(meta, "stash") {
		t.Fatalf("expected metadata to mention stash, got %q", meta)
	}
}

// TestPromptHistoryBrowser_ToggleOpenClose verifies ctrl+r opens the history
// browser (when entries exist) and ctrl+r/esc close it again.
func TestPromptHistoryBrowser_ToggleOpenClose(t *testing.T) {
	p := NewPromptInput(80)

	// No history yet: ctrl+r is a no-op.
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	if p.historyBrowserOn {
		t.Fatal("expected history browser to stay closed with no entries")
	}

	p.history.Add("first prompt")
	p.history.Add("second prompt")

	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	if !p.historyBrowserOn {
		t.Fatal("expected history browser to open via ctrl+r")
	}
	if p.PopoverView() == "" {
		t.Fatal("expected non-empty popover view while browser is open")
	}

	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if p.historyBrowserOn {
		t.Fatal("expected esc to close the history browser")
	}
}

// TestPromptHistoryBrowser_SelectEntry verifies navigating and selecting an
// entry fills the textarea and closes the browser.
func TestPromptHistoryBrowser_SelectEntry(t *testing.T) {
	p := NewPromptInput(80)
	p.history.Add("older prompt")
	p.history.Add("newest prompt")

	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	if !p.historyBrowserOn {
		t.Fatal("expected browser open")
	}

	// Cursor starts on the newest entry; selecting immediately should fill it.
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if p.historyBrowserOn {
		t.Fatal("expected enter to close the browser")
	}
	if p.Value() != "newest prompt" {
		t.Fatalf("expected textarea filled with %q, got %q", "newest prompt", p.Value())
	}
}

// TestPromptHistoryBrowser_NavigateUp verifies up-arrow moves the cursor to
// older entries while the browser is open.
func TestPromptHistoryBrowser_NavigateUp(t *testing.T) {
	p := NewPromptInput(80)
	p.history.Add("older prompt")
	p.history.Add("newest prompt")

	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyUp})
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if p.Value() != "older prompt" {
		t.Fatalf("expected textarea filled with %q, got %q", "older prompt", p.Value())
	}
}

// TestPromptHistory_MetadataHint verifies the status bar surfaces a hint
// about history entries beyond the bare up-arrow affordance.
func TestPromptHistory_MetadataHint(t *testing.T) {
	p := NewPromptInput(80)
	p.history.Add("one")
	p.history.Add("two")

	meta := p.renderMetadata()
	if !strings.Contains(meta, "history") {
		t.Fatalf("expected metadata to mention history, got %q", meta)
	}
	if !strings.Contains(meta, "ctrl+r") {
		t.Fatalf("expected metadata to mention ctrl+r, got %q", meta)
	}
}
