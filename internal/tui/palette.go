package tui

import (
	"fmt"
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
	scroll   int
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
	p.scroll = 0
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
			if len(p.filtered) > 0 {
				if p.selected > 0 {
					p.selected--
				} else {
					p.selected = len(p.filtered) - 1
				}
				p.ensureVisible()
			}
			return p, nil

		case "down", "ctrl+n":
			if len(p.filtered) > 0 {
				if p.selected < len(p.filtered)-1 {
					p.selected++
				} else {
					p.selected = 0
				}
				p.ensureVisible()
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
	p.scroll = 0

	return p, cmd
}

// View renders the command palette.
func (p CommandPalette) View() string {
	if !p.visible {
		return ""
	}

	paletteWidth := min(p.width*3/4, 90)
	if paletteWidth < 50 {
		paletteWidth = 50
	}

	innerWidth := paletteWidth - 6 // account for border padding

	var sb strings.Builder
	sb.WriteString("> ")
	sb.WriteString(p.input.View())
	sb.WriteString("\n")

	maxVisible := p.maxVisibleItems()

	// Calculate name column width for alignment.
	nameCol := 0
	for _, item := range p.filtered {
		if len(item.Label) > nameCol {
			nameCol = len(item.Label)
		}
	}
	nameCol += 4 // padding (2 for prefix + 2 gap)
	descCol := innerWidth - nameCol
	if descCol < 15 {
		descCol = 15
	}

	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#777777"})
	highlightBg := lipgloss.NewStyle().
		Background(lipgloss.AdaptiveColor{Light: "#E8E8E8", Dark: "#2A2A2A"})

	end := p.scroll + maxVisible
	if end > len(p.filtered) {
		end = len(p.filtered)
	}

	if p.scroll > 0 {
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render(fmt.Sprintf("  ↑ %d more", p.scroll)))
	}

	for i := p.scroll; i < end; i++ {
		item := p.filtered[i]
		sb.WriteString("\n")

		desc := item.Description
		if len(desc) > descCol {
			desc = desc[:descCol-1] + "…"
		}

		prefix := "  "
		if i == p.selected {
			prefix = "▸ "
			nameStr := highlightBg.Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "#0070F3", Dark: "#58A6FF"}).
				Width(nameCol).Render(prefix + item.Label)
			descStr := highlightBg.
				Foreground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#999999"}).
				Width(descCol).Render(desc)
			sb.WriteString(nameStr + descStr)
		} else {
			nameStr := lipgloss.NewStyle().Bold(true).Width(nameCol).Render(prefix + item.Label)
			descStr := dimStyle.Width(descCol).Render(desc)
			sb.WriteString(nameStr + descStr)
		}
	}

	if end < len(p.filtered) {
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render(fmt.Sprintf("  ↓ %d more", len(p.filtered)-end)))
	}

	if len(p.filtered) == 0 {
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render("  No matches"))
	}

	content := sb.String()

	return lipgloss.Place(
		p.width, p.height,
		lipgloss.Center, lipgloss.Center,
		styleDialogBorder.Width(paletteWidth).Render(content),
	)
}

// maxVisibleItems returns how many items fit in the palette viewport.
func (p CommandPalette) maxVisibleItems() int {
	mv := p.height - 8
	if mv < 3 {
		mv = 3
	}
	return mv
}

// ensureVisible adjusts the scroll offset so the selected item is visible.
func (p *CommandPalette) ensureVisible() {
	mv := p.maxVisibleItems()
	if p.selected < p.scroll {
		p.scroll = p.selected
	} else if p.selected >= p.scroll+mv {
		p.scroll = p.selected - mv + 1
	}
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
