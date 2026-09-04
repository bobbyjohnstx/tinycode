package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// PaletteItem is a selectable entry in the command palette.
type PaletteItem struct {
	Label       string
	Description string
	Value       string
}

// PaletteOpenMsg signals the command palette should open.
type PaletteOpenMsg struct{}

// PaletteClosedMsg signals the command palette was dismissed.
type PaletteClosedMsg struct{}

// PaletteSelectedMsg signals an item was selected.
type PaletteSelectedMsg struct {
	Item PaletteItem
}

// CommandPalette is a filterable overlay list opened with ctrl+p.
type CommandPalette struct {
	input    textinput.Model
	items    []PaletteItem
	filtered []PaletteItem
	selected int
	visible  bool
	width    int
	height   int
}

// NewCommandPalette creates a CommandPalette.
func NewCommandPalette() CommandPalette {
	ti := textinput.New()
	ti.Placeholder = "Type to filter..."
	ti.CharLimit = 100

	return CommandPalette{
		input: ti,
	}
}

// Show opens the palette with the given items.
func (p *CommandPalette) Show(items []PaletteItem) {
	p.items = items
	p.filtered = items
	p.selected = 0
	p.visible = true
	p.input.Reset()
	p.input.Focus()
}

// Hide closes the palette.
func (p *CommandPalette) Hide() {
	p.visible = false
	p.input.Blur()
}

// IsVisible reports whether the palette is shown.
func (p CommandPalette) IsVisible() bool {
	return p.visible
}

// SetSize updates the palette dimensions.
func (p *CommandPalette) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// Update handles key events for the palette.
func (p CommandPalette) Update(msg tea.Msg) (CommandPalette, tea.Cmd) {
	if !p.visible {
		return p, nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if ok {
		switch keyMsg.String() {
		case "esc":
			p.visible = false
			p.input.Blur()
			return p, func() tea.Msg { return PaletteClosedMsg{} }

		case "enter":
			if len(p.filtered) > 0 && p.selected < len(p.filtered) {
				item := p.filtered[p.selected]
				p.visible = false
				p.input.Blur()
				return p, func() tea.Msg { return PaletteSelectedMsg{Item: item} }
			}
			return p, nil

		case "up", "ctrl+p":
			if p.selected > 0 {
				p.selected--
			}
			return p, nil

		case "down", "ctrl+n":
			if p.selected < len(p.filtered)-1 {
				p.selected++
			}
			return p, nil
		}
	}

	// Forward to text input for typing.
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)

	// Re-filter on any input change.
	p.filtered = filterItems(p.items, p.input.Value())
	if p.selected >= len(p.filtered) {
		p.selected = max(0, len(p.filtered)-1)
	}

	return p, cmd
}

// View renders the command palette.
func (p CommandPalette) View() string {
	if !p.visible {
		return ""
	}

	paletteWidth := p.width / 2
	if paletteWidth < 40 {
		paletteWidth = 40
	}
	if paletteWidth > 80 {
		paletteWidth = 80
	}

	var sb strings.Builder
	sb.WriteString(p.input.View())
	sb.WriteString("\n")

	maxVisible := p.height - 8
	if maxVisible < 3 {
		maxVisible = 3
	}

	for i, item := range p.filtered {
		if i >= maxVisible {
			break
		}
		sb.WriteString("\n")

		line := item.Label
		if item.Description != "" {
			line += "  " + styleMetadata.Render(item.Description)
		}

		if i == p.selected {
			sb.WriteString(styleSelected.Render("▸ " + line))
		} else {
			sb.WriteString("  " + line)
		}
	}

	if len(p.filtered) == 0 {
		sb.WriteString("\n")
		sb.WriteString(styleMetadata.Render("  No matches"))
	}

	content := sb.String()

	return lipgloss.Place(
		p.width, p.height,
		lipgloss.Center, lipgloss.Center,
		styleDialogBorder.Width(paletteWidth).Render(content),
	)
}

// filterItems returns items whose label contains the query (case-insensitive).
func filterItems(items []PaletteItem, query string) []PaletteItem {
	if query == "" {
		return items
	}
	q := strings.ToLower(query)
	result := make([]PaletteItem, 0, len(items))
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.Label), q) {
			result = append(result, item)
		}
	}
	return result
}
