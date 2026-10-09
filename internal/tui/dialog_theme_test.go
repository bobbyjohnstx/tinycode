package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func makeTestThemes() []ColorTheme {
	return []ColorTheme{
		{ID: "dark", Name: "Dark"},
		{ID: "light", Name: "Light"},
		{ID: "solarized", Name: "Solarized"},
	}
}

func makeAdaptiveColor(light, dark string) lipgloss.AdaptiveColor {
	return lipgloss.AdaptiveColor{Light: light, Dark: dark}
}

func TestThemeDialog_InitiallyHidden(t *testing.T) {
	d := NewThemeDialog()
	if d.IsVisible() {
		t.Fatal("expected dialog to be hidden initially")
	}
}

func TestThemeDialog_ShowMakesVisible(t *testing.T) {
	d := NewThemeDialog()
	d.Show(makeTestThemes(), "dark")
	if !d.IsVisible() {
		t.Fatal("expected dialog to be visible after Show")
	}
}

func TestThemeDialog_ShowSelectsCurrentTheme(t *testing.T) {
	d := NewThemeDialog()
	d.Show(makeTestThemes(), "solarized")
	if d.selected != 2 {
		t.Errorf("expected selected=2 for solarized, got %d", d.selected)
	}
}

func TestThemeDialog_ShowResetsFilterAndScroll(t *testing.T) {
	d := NewThemeDialog()
	d.Show(makeTestThemes(), "dark")
	// Type a filter character
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if d.filter != "s" {
		t.Fatalf("expected filter=%q, got %q", "s", d.filter)
	}
	// Re-show should reset filter
	d.Show(makeTestThemes(), "dark")
	if d.filter != "" {
		t.Errorf("expected filter to be empty after re-Show, got %q", d.filter)
	}
	if d.scroll != 0 {
		t.Errorf("expected scroll=0 after re-Show, got %d", d.scroll)
	}
}

func TestThemeDialog_NavigateDownWraps(t *testing.T) {
	d := NewThemeDialog()
	d.Show(makeTestThemes(), "dark")
	d.selected = 0

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.selected != 1 {
		t.Errorf("expected selected=1, got %d", d.selected)
	}

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.selected != 2 {
		t.Errorf("expected selected=2, got %d", d.selected)
	}

	// Should wrap to 0
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.selected != 0 {
		t.Errorf("expected selected to wrap to 0, got %d", d.selected)
	}
}

func TestThemeDialog_NavigateUpWraps(t *testing.T) {
	d := NewThemeDialog()
	d.Show(makeTestThemes(), "dark")
	d.selected = 0

	// Should wrap to last item
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyUp})
	if d.selected != 2 {
		t.Errorf("expected selected to wrap to 2, got %d", d.selected)
	}
}

func TestThemeDialog_NavigationEmitsPreviewMsg(t *testing.T) {
	d := NewThemeDialog()
	d.Show(makeTestThemes(), "dark")
	d.selected = 0

	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd == nil {
		t.Fatal("expected preview command on navigation")
	}
	msg := cmd()
	preview, ok := msg.(ThemePreviewMsg)
	if !ok {
		t.Fatalf("expected ThemePreviewMsg, got %T", msg)
	}
	if preview.ThemeID != "light" {
		t.Errorf("expected preview ThemeID=%q, got %q", "light", preview.ThemeID)
	}
}

func TestThemeDialog_EnterSelectsTheme(t *testing.T) {
	d := NewThemeDialog()
	d.Show(makeTestThemes(), "dark")
	d.selected = 1 // "Light"

	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if d.IsVisible() {
		t.Fatal("expected dialog to close on enter")
	}
	if cmd == nil {
		t.Fatal("expected command from enter")
	}
	msg := cmd()
	selected, ok := msg.(ThemeSelectedMsg)
	if !ok {
		t.Fatalf("expected ThemeSelectedMsg, got %T", msg)
	}
	if selected.ThemeID != "light" {
		t.Errorf("expected ThemeID=%q, got %q", "light", selected.ThemeID)
	}
}

func TestThemeDialog_EscapeRevertsToInitialTheme(t *testing.T) {
	d := NewThemeDialog()
	d.Show(makeTestThemes(), "dark")
	// Navigate to change preview
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyDown})

	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if d.IsVisible() {
		t.Fatal("expected dialog to close on esc")
	}
	if cmd == nil {
		t.Fatal("expected revert command from esc")
	}
	msg := cmd()
	revert, ok := msg.(ThemeRevertMsg)
	if !ok {
		t.Fatalf("expected ThemeRevertMsg, got %T", msg)
	}
	if revert.ThemeID != "dark" {
		t.Errorf("expected revert to initial theme %q, got %q", "dark", revert.ThemeID)
	}
}

func TestThemeDialog_FilterNarrowsResults(t *testing.T) {
	d := NewThemeDialog()
	d.Show(makeTestThemes(), "dark")

	// Type "sol" to filter
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})

	filtered := d.filtered()
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered theme, got %d", len(filtered))
	}
	if filtered[0].ID != "solarized" {
		t.Errorf("expected filtered theme ID=%q, got %q", "solarized", filtered[0].ID)
	}
}

func TestThemeDialog_FilterResetsSelection(t *testing.T) {
	d := NewThemeDialog()
	d.Show(makeTestThemes(), "dark")
	d.selected = 2

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if d.selected != 0 {
		t.Errorf("expected filter to reset selected to 0, got %d", d.selected)
	}
}

func TestThemeDialog_BackspaceRemovesFilterChar(t *testing.T) {
	d := NewThemeDialog()
	d.Show(makeTestThemes(), "dark")

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	if d.filter != "ab" {
		t.Fatalf("expected filter=%q, got %q", "ab", d.filter)
	}

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if d.filter != "a" {
		t.Errorf("expected filter=%q after backspace, got %q", "a", d.filter)
	}
}

func TestThemeDialog_BackspaceOnEmptyFilterIsNoop(t *testing.T) {
	d := NewThemeDialog()
	d.Show(makeTestThemes(), "dark")

	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if d.filter != "" {
		t.Errorf("expected filter to remain empty, got %q", d.filter)
	}
	if cmd != nil {
		t.Error("backspace on empty filter should not produce a command")
	}
}

func TestThemeDialog_EnterOnFilteredResultSelectsCorrectTheme(t *testing.T) {
	d := NewThemeDialog()
	d.Show(makeTestThemes(), "dark")

	// Filter to only "Solarized"
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})

	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected command from enter")
	}
	msg := cmd()
	selected, ok := msg.(ThemeSelectedMsg)
	if !ok {
		t.Fatalf("expected ThemeSelectedMsg, got %T", msg)
	}
	if selected.ThemeID != "solarized" {
		t.Errorf("expected ThemeID=%q, got %q", "solarized", selected.ThemeID)
	}
}

func TestThemeDialog_ViewEmptyWhenHidden(t *testing.T) {
	d := NewThemeDialog()
	if d.View() != "" {
		t.Error("expected empty view when hidden")
	}
}

func TestThemeDialog_ViewNonEmptyWhenVisible(t *testing.T) {
	d := NewThemeDialog()
	d.SetSize(80, 24)
	d.Show(makeTestThemes(), "dark")
	view := d.View()
	if view == "" {
		t.Fatal("expected non-empty view")
	}
}
