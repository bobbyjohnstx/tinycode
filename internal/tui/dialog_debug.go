package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// DebugDialog displays diagnostic information for bug reports.
type DebugDialog struct {
	info    string
	visible bool
	width   int
	height  int
}

// NewDebugDialog creates a DebugDialog.
func NewDebugDialog() DebugDialog {
	return DebugDialog{}
}

// Show opens the dialog with the given info text.
func (d *DebugDialog) Show(info string) {
	d.info = info
	d.visible = true
}

// Hide closes the dialog.
func (d *DebugDialog) Hide() {
	d.visible = false
}

// IsVisible reports whether the dialog is shown.
func (d DebugDialog) IsVisible() bool {
	return d.visible
}

// SetSize updates the dialog dimensions.
func (d *DebugDialog) SetSize(width, height int) {
	d.width = width
	d.height = height
}

// Update handles key events for the debug dialog.
func (d DebugDialog) Update(msg tea.Msg) (DebugDialog, tea.Cmd) {
	if !d.visible {
		return d, nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return d, nil
	}

	switch keyMsg.String() {
	case "esc", "q":
		d.visible = false
	case "c":
		d.visible = false
		return d, copyToClipboard(d.info)
	}

	return d, nil
}

// View renders the debug dialog.
func (d DebugDialog) View() string {
	if !d.visible {
		return ""
	}

	dialogWidth := d.width / 2
	if dialogWidth < 50 {
		dialogWidth = 50
	}
	if dialogWidth > 80 {
		dialogWidth = 80
	}

	var sb strings.Builder
	sb.WriteString("Diagnostics  ")
	sb.WriteString(styleMetadata.Render("c=copy  esc=close"))
	sb.WriteString("\n\n")
	sb.WriteString(d.info)

	content := sb.String()

	return lipgloss.Place(
		d.width, d.height,
		lipgloss.Center, lipgloss.Center,
		styleDialogBorder.Width(dialogWidth).Render(content),
	)
}
