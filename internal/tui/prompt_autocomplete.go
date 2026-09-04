package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// AutocompleteItem describes a slash command for the autocomplete popover.
type AutocompleteItem struct {
	Name        string
	Description string
}

// AutocompleteResultsMsg delivers filtered autocomplete items.
type AutocompleteResultsMsg struct {
	Items []string
}

// AutocompleteDismissMsg signals the autocomplete should close.
type AutocompleteDismissMsg struct{}

// Autocomplete is a popover that triggers on "/" at input start,
// filters by typed text, and allows selection via Up/Down/Tab/Enter/Escape.
type Autocomplete struct {
	commands []AutocompleteItem
	filtered []AutocompleteItem
	visible  bool
	cursor   int
	query    string
	width    int
}

// NewAutocomplete creates an Autocomplete sub-model.
func NewAutocomplete() Autocomplete {
	return Autocomplete{}
}

// SetCommands sets the available slash commands.
func (ac *Autocomplete) SetCommands(cmds []AutocompleteItem) {
	ac.commands = cmds
}

// SetWidth sets the popover width.
func (ac *Autocomplete) SetWidth(w int) {
	ac.width = w
}

// IsVisible returns whether the autocomplete popover is showing.
func (ac *Autocomplete) IsVisible() bool {
	return ac.visible
}

// Selected returns the currently highlighted command name, or "" if none.
func (ac *Autocomplete) Selected() string {
	if !ac.visible || len(ac.filtered) == 0 {
		return ""
	}
	if ac.cursor < 0 || ac.cursor >= len(ac.filtered) {
		return ""
	}
	return ac.filtered[ac.cursor].Name
}

// UpdateInput is called when the prompt input text changes.
// It decides whether to show/hide the autocomplete and filters results.
func (ac *Autocomplete) UpdateInput(text string) {
	if !strings.HasPrefix(text, "/") {
		ac.visible = false
		ac.cursor = 0
		ac.query = ""
		return
	}

	ac.visible = true
	ac.query = strings.TrimPrefix(text, "/")
	ac.filter()
}

// filter recalculates the filtered list based on the current query.
func (ac *Autocomplete) filter() {
	q := strings.ToLower(ac.query)
	ac.filtered = ac.filtered[:0]
	for _, cmd := range ac.commands {
		if q == "" || strings.Contains(strings.ToLower(cmd.Name), q) {
			ac.filtered = append(ac.filtered, cmd)
		}
	}
	if ac.cursor >= len(ac.filtered) {
		ac.cursor = max(0, len(ac.filtered)-1)
	}
}

// Update handles keyboard input when the autocomplete is visible.
// Returns the updated Autocomplete, an optional tea.Cmd, and whether
// the key was consumed (so the prompt should not process it).
func (ac Autocomplete) Update(msg tea.KeyMsg) (Autocomplete, tea.Cmd, bool) {
	if !ac.visible {
		return ac, nil, false
	}

	switch msg.String() {
	case "up":
		if ac.cursor > 0 {
			ac.cursor--
		} else {
			ac.cursor = max(0, len(ac.filtered)-1)
		}
		return ac, nil, true

	case "down":
		if ac.cursor < len(ac.filtered)-1 {
			ac.cursor++
		} else {
			ac.cursor = 0
		}
		return ac, nil, true

	case "tab", "enter":
		selected := ac.Selected()
		ac.visible = false
		ac.cursor = 0
		ac.query = ""
		if selected != "" {
			return ac, func() tea.Msg {
				return AutocompleteResultsMsg{Items: []string{selected}}
			}, true
		}
		return ac, nil, true

	case "esc":
		ac.visible = false
		ac.cursor = 0
		ac.query = ""
		return ac, func() tea.Msg {
			return AutocompleteDismissMsg{}
		}, true
	}

	return ac, nil, false
}

// View renders the autocomplete popover.
func (ac Autocomplete) View() string {
	if !ac.visible || len(ac.filtered) == 0 {
		return ""
	}

	w := ac.width
	if w <= 0 {
		w = 40
	}

	var lines []string
	for i, cmd := range ac.filtered {
		label := "/" + cmd.Name
		if cmd.Description != "" {
			label += "  " + cmd.Description
		}
		if i == ac.cursor {
			lines = append(lines, styleSelected.Width(w).Render(label))
		} else {
			lines = append(lines, lipgloss.NewStyle().Width(w).Render(label))
		}
	}

	return styleDialogBorder.Width(w).Render(
		strings.Join(lines, "\n"),
	)
}
