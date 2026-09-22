package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// WhichKeyEntry represents a single keybinding in the which-key panel.
type WhichKeyEntry struct {
	Key         string
	Description string
	Category    string
}

// WhichKeyPanel is a non-modal overlay that displays available leader key
// follow-up bindings grouped by category. It does not consume input — any
// keypress hides it and is forwarded normally.
type WhichKeyPanel struct {
	visible bool
	entries []WhichKeyEntry
	width   int
	height  int
}

// Show populates the panel with entries and makes it visible.
func (w *WhichKeyPanel) Show(entries []WhichKeyEntry) {
	w.entries = entries
	w.visible = true
}

// Hide dismisses the panel.
func (w *WhichKeyPanel) Hide() {
	w.visible = false
}

// IsVisible reports whether the panel is currently shown.
func (w WhichKeyPanel) IsVisible() bool {
	return w.visible
}

// SetSize updates the available dimensions for rendering.
func (w *WhichKeyPanel) SetSize(width, height int) {
	w.width = width
	w.height = height
}

// View renders the which-key panel as a bordered floating box.
func (w WhichKeyPanel) View() string {
	if !w.visible || len(w.entries) == 0 {
		return ""
	}

	keyStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.AdaptiveColor{Light: "#0070F3", Dark: "#58A6FF"})
	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#666666", Dark: "#999999"})
	catStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.AdaptiveColor{Light: "#333333", Dark: "#CCCCCC"})

	// Group entries by category, preserving insertion order.
	var categories []string
	groups := make(map[string][]WhichKeyEntry)
	for _, e := range w.entries {
		if _, seen := groups[e.Category]; !seen {
			categories = append(categories, e.Category)
		}
		groups[e.Category] = append(groups[e.Category], e)
	}

	var lines []string
	for i, cat := range categories {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, catStyle.Render(cat))
		for _, e := range groups[cat] {
			line := "  " + keyStyle.Render(e.Key) + "  " + descStyle.Render(e.Description)
			lines = append(lines, line)
		}
	}

	content := strings.Join(lines, "\n")

	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.AdaptiveColor{Light: "#0070F3", Dark: "#58A6FF"}).
		Padding(1, 2).
		Render(content)

	return box
}

// placeWhichKey composites the which-key panel over the base view,
// positioned at the bottom-right above the status bar.
func placeWhichKey(base, panel string, width, height int) string {
	return lipgloss.Place(
		width, height,
		lipgloss.Right, lipgloss.Bottom,
		panel,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(lipgloss.NoColor{}),
	)
}

// LeaderKeyEntries builds WhichKeyEntry items from the KeyMap, grouped by
// category. This is the canonical source for the which-key panel content.
func LeaderKeyEntries(keys KeyMap) []WhichKeyEntry {
	return []WhichKeyEntry{
		{Key: "b", Description: "toggle sidebar", Category: "Navigation"},
		{Key: "o", Description: "session list", Category: "Navigation"},
		{Key: "n", Description: "new session", Category: "Navigation"},

		{Key: "e", Description: "open $EDITOR", Category: "Edit"},
		{Key: "d", Description: "diff viewer", Category: "Edit"},
		{Key: "u", Description: "undo", Category: "Edit"},
		{Key: "r", Description: "redo", Category: "Edit"},

		{Key: "a", Description: "agent list", Category: "Tools"},
		{Key: "m", Description: "model list", Category: "Tools"},
		{Key: "t", Description: "theme picker", Category: "Tools"},
		{Key: "i", Description: "MCP servers", Category: "Tools"},

		{Key: "y", Description: "copy last response", Category: "Actions"},
		{Key: "x", Description: "export session", Category: "Actions"},
	}
}
