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
// It supports two modes: "/" for slash commands and "/ask" for agent selection.
type Autocomplete struct {
	commands []AutocompleteItem
	agents   []AutocompleteItem
	filtered []AutocompleteItem
	visible  bool
	mode     string // "/" or "/ask"
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

// SetAgents sets the available agents for /ask completion.
func (ac *Autocomplete) SetAgents(agents []AutocompleteItem) {
	ac.agents = agents
}

// SetWidth sets the popover width.
func (ac *Autocomplete) SetWidth(w int) {
	ac.width = w
}

// IsVisible returns whether the autocomplete popover is showing.
func (ac *Autocomplete) IsVisible() bool {
	return ac.visible
}

// Mode returns the current autocomplete mode ("/" or "/ask").
func (ac *Autocomplete) Mode() string {
	return ac.mode
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
		ac.mode = ""
		return
	}

	// "/ask <partial>" → agent mode
	if strings.HasPrefix(text, "/ask ") {
		afterAsk := text[5:]
		trimmed := strings.TrimSpace(afterAsk)
		// If the user typed a complete agent name followed by a space (ready to type message),
		// or typed "agent message" (space within the non-whitespace part), dismiss
		if trimmed != "" && strings.HasSuffix(afterAsk, " ") {
			ac.visible = false
			ac.cursor = 0
			ac.query = ""
			ac.mode = ""
			return
		}
		if ac.mode != "/ask" {
			ac.cursor = 0
		}
		ac.visible = true
		ac.mode = "/ask"
		ac.query = afterAsk
		ac.filter()
		return
	}

	if ac.mode == "/ask" {
		ac.cursor = 0
	}
	ac.visible = true
	ac.mode = "/"
	ac.query = strings.TrimPrefix(text, "/")
	ac.filter()
}

// filter recalculates the filtered list based on the current query.
func (ac *Autocomplete) filter() {
	q := strings.ToLower(ac.query)
	source := ac.commands
	if ac.mode == "/ask" {
		source = ac.agents
	}
	ac.filtered = ac.filtered[:0]
	for _, item := range source {
		if q == "" || strings.HasPrefix(strings.ToLower(item.Name), q) {
			ac.filtered = append(ac.filtered, item)
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

// View renders the autocomplete popover as a two-column table.
func (ac Autocomplete) View() string {
	if !ac.visible || len(ac.filtered) == 0 {
		return ""
	}

	w := ac.width
	if w <= 0 {
		w = 40
	}

	isAskMode := ac.mode == "/ask"
	prefix := "/"
	if isAskMode {
		prefix = ""
	}

	nameCol := 0
	for _, cmd := range ac.filtered {
		n := len(cmd.Name) + len(prefix)
		if n > nameCol {
			nameCol = n
		}
	}
	nameCol += 2

	descCol := w - nameCol - 6
	if descCol < 10 {
		descCol = 10
	}

	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#777777"})
	highlightStyle := lipgloss.NewStyle().
		Background(lipgloss.AdaptiveColor{Light: "#E8E8E8", Dark: "#2A2A2A"}).
		Bold(true)
	highlightDimStyle := lipgloss.NewStyle().
		Background(lipgloss.AdaptiveColor{Light: "#E8E8E8", Dark: "#2A2A2A"}).
		Foreground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#999999"})

	var lines []string
	for i, cmd := range ac.filtered {
		name := prefix + cmd.Name
		desc := cmd.Description
		if len(desc) > descCol {
			desc = desc[:descCol-1] + "…"
		}

		nameStr := lipgloss.NewStyle().Width(nameCol).Render(name)
		descStr := dimStyle.Width(descCol).Render(desc)

		if i == ac.cursor {
			nameStr = highlightStyle.Width(nameCol).Render(name)
			descStr = highlightDimStyle.Width(descCol).Render(desc)
		}

		row := lipgloss.JoinHorizontal(lipgloss.Top, nameStr, descStr)
		lines = append(lines, row)
	}

	return styleDialogBorder.Width(w).Render(
		strings.Join(lines, "\n"),
	)
}
