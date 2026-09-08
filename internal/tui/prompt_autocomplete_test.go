package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func testCommands() []AutocompleteItem {
	return []AutocompleteItem{
		{Name: "build", Description: "Run build agent"},
		{Name: "plan", Description: "Run plan agent"},
		{Name: "bug", Description: "Report a bug"},
		{Name: "help", Description: "Show help"},
	}
}

func TestAutocomplete_SlashTriggers(t *testing.T) {
	ac := NewAutocomplete()
	ac.SetCommands(testCommands())

	ac.UpdateInput("/")
	if !ac.IsVisible() {
		t.Fatal("expected autocomplete visible after '/'")
	}

	ac.UpdateInput("hello")
	if ac.IsVisible() {
		t.Fatal("expected autocomplete hidden without '/' prefix")
	}
}

func TestAutocomplete_TypingFilters(t *testing.T) {
	ac := NewAutocomplete()
	ac.SetCommands(testCommands())

	ac.UpdateInput("/bu")
	if !ac.IsVisible() {
		t.Fatal("expected autocomplete visible")
	}
	// "bu" should match "build" and "bug"
	if len(ac.filtered) != 2 {
		t.Fatalf("expected 2 filtered items, got %d", len(ac.filtered))
	}
	if ac.filtered[0].Name != "build" {
		t.Errorf("expected first match 'build', got %q", ac.filtered[0].Name)
	}
	if ac.filtered[1].Name != "bug" {
		t.Errorf("expected second match 'bug', got %q", ac.filtered[1].Name)
	}
}

func TestAutocomplete_TypingFilters_NoMatch(t *testing.T) {
	ac := NewAutocomplete()
	ac.SetCommands(testCommands())

	ac.UpdateInput("/xyz")
	if len(ac.filtered) != 0 {
		t.Fatalf("expected 0 filtered items for /xyz, got %d", len(ac.filtered))
	}
}

func TestAutocomplete_PrefixMatchOnly(t *testing.T) {
	ac := NewAutocomplete()
	ac.SetCommands(testCommands())

	// "el" is a substring of "help" but not a prefix — should not match
	ac.UpdateInput("/el")
	if len(ac.filtered) != 0 {
		t.Fatalf("expected 0 filtered items for /el (prefix match only), got %d", len(ac.filtered))
	}
}

func TestAutocomplete_TabSelects(t *testing.T) {
	ac := NewAutocomplete()
	ac.SetCommands(testCommands())
	ac.UpdateInput("/bu")

	// Cursor starts at 0 ("build")
	ac, cmd, consumed := ac.Update(tea.KeyMsg{Type: tea.KeyTab})
	if !consumed {
		t.Fatal("expected tab to be consumed")
	}
	if ac.IsVisible() {
		t.Fatal("expected autocomplete dismissed after tab")
	}
	if cmd == nil {
		t.Fatal("expected a command from tab selection")
	}

	msg := cmd()
	result, ok := msg.(AutocompleteResultsMsg)
	if !ok {
		t.Fatalf("expected AutocompleteResultsMsg, got %T", msg)
	}
	if len(result.Items) != 1 || result.Items[0] != "build" {
		t.Errorf("expected selected 'build', got %v", result.Items)
	}
}

func TestAutocomplete_EnterSelects(t *testing.T) {
	ac := NewAutocomplete()
	ac.SetCommands(testCommands())
	ac.UpdateInput("/")

	// Navigate down to "plan" (index 1)
	ac, _, _ = ac.Update(tea.KeyMsg{Type: tea.KeyDown})
	if ac.cursor != 1 {
		t.Fatalf("expected cursor at 1, got %d", ac.cursor)
	}

	ac, cmd, consumed := ac.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !consumed {
		t.Fatal("expected enter to be consumed")
	}
	if cmd == nil {
		t.Fatal("expected a command from enter selection")
	}
	msg := cmd()
	result, ok := msg.(AutocompleteResultsMsg)
	if !ok {
		t.Fatalf("expected AutocompleteResultsMsg, got %T", msg)
	}
	if len(result.Items) != 1 || result.Items[0] != "plan" {
		t.Errorf("expected selected 'plan', got %v", result.Items)
	}
}

func TestAutocomplete_EscapeDismisses(t *testing.T) {
	ac := NewAutocomplete()
	ac.SetCommands(testCommands())
	ac.UpdateInput("/")

	ac, cmd, consumed := ac.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if !consumed {
		t.Fatal("expected escape to be consumed")
	}
	if ac.IsVisible() {
		t.Fatal("expected autocomplete dismissed after escape")
	}
	if cmd == nil {
		t.Fatal("expected dismiss command")
	}
	msg := cmd()
	if _, ok := msg.(AutocompleteDismissMsg); !ok {
		t.Fatalf("expected AutocompleteDismissMsg, got %T", msg)
	}
}

func TestAutocomplete_UpDownNavigation(t *testing.T) {
	ac := NewAutocomplete()
	ac.SetCommands(testCommands())
	ac.UpdateInput("/")

	// Start at 0
	if ac.cursor != 0 {
		t.Fatalf("expected cursor at 0, got %d", ac.cursor)
	}

	// Down to 1
	ac, _, _ = ac.Update(tea.KeyMsg{Type: tea.KeyDown})
	if ac.cursor != 1 {
		t.Fatalf("expected cursor at 1, got %d", ac.cursor)
	}

	// Up back to 0
	ac, _, _ = ac.Update(tea.KeyMsg{Type: tea.KeyUp})
	if ac.cursor != 0 {
		t.Fatalf("expected cursor at 0, got %d", ac.cursor)
	}

	// Up wraps to last
	ac, _, _ = ac.Update(tea.KeyMsg{Type: tea.KeyUp})
	if ac.cursor != 3 {
		t.Fatalf("expected cursor to wrap to 3, got %d", ac.cursor)
	}

	// Down wraps to 0
	ac, _, _ = ac.Update(tea.KeyMsg{Type: tea.KeyDown})
	if ac.cursor != 0 {
		t.Fatalf("expected cursor to wrap to 0, got %d", ac.cursor)
	}
}

func TestAutocomplete_NotVisible_NoConsume(t *testing.T) {
	ac := NewAutocomplete()
	_, _, consumed := ac.Update(tea.KeyMsg{Type: tea.KeyTab})
	if consumed {
		t.Fatal("expected key not consumed when autocomplete is hidden")
	}
}

func TestAutocomplete_ViewEmpty_WhenHidden(t *testing.T) {
	ac := NewAutocomplete()
	if ac.View() != "" {
		t.Fatal("expected empty view when hidden")
	}
}

func TestAutocomplete_CursorClampOnFilter(t *testing.T) {
	ac := NewAutocomplete()
	ac.SetCommands(testCommands())
	ac.UpdateInput("/")

	// Move cursor to last item (index 3)
	ac.cursor = 3

	// Filter to fewer results
	ac.UpdateInput("/hel")
	// Only "help" matches, cursor should clamp to 0
	if ac.cursor != 0 {
		t.Fatalf("expected cursor clamped to 0, got %d", ac.cursor)
	}
	if len(ac.filtered) != 1 {
		t.Fatalf("expected 1 filtered item, got %d", len(ac.filtered))
	}
}
