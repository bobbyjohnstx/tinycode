package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ModelDialogOpenMsg signals the model dialog should open.
type ModelDialogOpenMsg struct{}

// ModelSelectedMsg signals a model was selected.
type ModelSelectedMsg struct {
	Selection ModelSelection
}

// ModelDialog displays a list of providers and models for selection.
type ModelDialog struct {
	providers []ProviderInfo
	selected  int
	visible   bool
	width     int
	height    int
}

// NewModelDialog creates a ModelDialog.
func NewModelDialog() ModelDialog {
	return ModelDialog{}
}

// Show opens the dialog with the given providers.
// If current is non-empty, the cursor starts on the matching model.
func (d *ModelDialog) Show(providers []ProviderInfo, current ...ModelSelection) {
	d.providers = providers
	d.visible = true
	d.selected = 0
	items := flattenProviders(providers)

	// Try to land on the currently selected model.
	if len(current) > 0 && current[0].ModelID != "" {
		for i, item := range items {
			if !item.isHeader && item.modelID == current[0].ModelID && item.providerID == current[0].ProviderID {
				d.selected = i
				return
			}
		}
	}

	// Fall back to the first non-header item.
	for i, item := range items {
		if !item.isHeader {
			d.selected = i
			break
		}
	}
}

// Hide closes the dialog.
func (d *ModelDialog) Hide() {
	d.visible = false
}

// IsVisible reports whether the dialog is shown.
func (d ModelDialog) IsVisible() bool {
	return d.visible
}

// SetSize updates the dialog dimensions.
func (d *ModelDialog) SetSize(width, height int) {
	d.width = width
	d.height = height
}

// flatItem is a flattened entry in the model list for navigation.
type flatItem struct {
	providerID string
	modelID    string
	label      string
	isHeader   bool
}

// flattenProviders returns a flat list of navigable items.
func flattenProviders(providers []ProviderInfo) []flatItem {
	var items []flatItem
	for _, p := range providers {
		items = append(items, flatItem{
			providerID: p.ID,
			label:      p.Name,
			isHeader:   true,
		})
		for _, m := range p.Models {
			items = append(items, flatItem{
				providerID: p.ID,
				modelID:    m.ID,
				label:      m.Name,
			})
		}
	}
	return items
}

// Update handles key events for the model dialog.
func (d ModelDialog) Update(msg tea.Msg) (ModelDialog, tea.Cmd) {
	if !d.visible {
		return d, nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return d, nil
	}

	items := flattenProviders(d.providers)
	if len(items) == 0 {
		if keyMsg.String() == "esc" || keyMsg.String() == "q" {
			d.visible = false
		}
		return d, nil
	}

	switch keyMsg.String() {
	case "up", "k":
		d.selected = wrapIndex(d.selected-1, len(items))
		// Skip headers when navigating.
		if items[d.selected].isHeader {
			d.selected = wrapIndex(d.selected-1, len(items))
		}
	case "down", "j":
		d.selected = wrapIndex(d.selected+1, len(items))
		if items[d.selected].isHeader {
			d.selected = wrapIndex(d.selected+1, len(items))
		}
	case "enter":
		if d.selected < len(items) && !items[d.selected].isHeader {
			item := items[d.selected]
			d.visible = false
			return d, func() tea.Msg {
				return ModelSelectedMsg{
					Selection: ModelSelection{
						ProviderID: item.providerID,
						ModelID:    item.modelID,
					},
				}
			}
		}
	case "esc", "q":
		d.visible = false
	}

	return d, nil
}

// View renders the model dialog.
func (d ModelDialog) View() string {
	if !d.visible {
		return ""
	}

	dialogWidth := d.width / 2
	if dialogWidth < 40 {
		dialogWidth = 40
	}
	if dialogWidth > 80 {
		dialogWidth = 80
	}

	items := flattenProviders(d.providers)

	var sb strings.Builder
	sb.WriteString("Select Model\n")

	maxVisible := d.height - 6
	if maxVisible < 3 {
		maxVisible = 3
	}

	for i, item := range items {
		if i >= maxVisible {
			break
		}
		sb.WriteString("\n")

		if item.isHeader {
			sb.WriteString(styleToolName.Render(item.label))
			continue
		}

		if i == d.selected {
			sb.WriteString(styleSelected.Render("▸ " + item.label))
		} else {
			sb.WriteString("  " + item.label)
		}
	}

	content := sb.String()

	return lipgloss.Place(
		d.width, d.height,
		lipgloss.Center, lipgloss.Center,
		styleDialogBorder.Width(dialogWidth).Render(content),
	)
}

// wrapIndex wraps an index within [0, length).
func wrapIndex(i, length int) int {
	if length == 0 {
		return 0
	}
	return ((i % length) + length) % length
}
