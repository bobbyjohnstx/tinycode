package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ContextDialog displays a context window usage breakdown.
type ContextDialog struct {
	breakdown ContextBreakdown
	visible   bool
	width     int
	height    int
}

// NewContextDialog creates a ContextDialog.
func NewContextDialog() ContextDialog {
	return ContextDialog{}
}

// Show opens the dialog with the given breakdown.
func (d *ContextDialog) Show(breakdown ContextBreakdown) {
	d.breakdown = breakdown
	d.visible = true
}

// Hide closes the dialog.
func (d *ContextDialog) Hide() {
	d.visible = false
}

// IsVisible reports whether the dialog is shown.
func (d ContextDialog) IsVisible() bool {
	return d.visible
}

// SetSize updates the dialog dimensions.
func (d *ContextDialog) SetSize(width, height int) {
	d.width = width
	d.height = height
}

// Update handles key events for the context dialog.
func (d ContextDialog) Update(msg tea.Msg) (ContextDialog, tea.Cmd) {
	if !d.visible {
		return d, nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return d, nil
	}

	switch keyMsg.String() {
	case "esc", "q", "ctrl+c", "ctrl+p":
		d.visible = false
	}

	return d, nil
}

// View renders the context dialog.
func (d ContextDialog) View() string {
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

	b := d.breakdown
	barWidth := dialogWidth - 6 // account for border padding

	var sb strings.Builder
	sb.WriteString("Context Usage  ")
	sb.WriteString(styleMetadata.Render("esc=close"))
	sb.WriteString("\n\n")

	// Total usage line.
	if b.ContextLimit > 0 {
		fmt.Fprintf(&sb, "%s / %s tokens (%d%%)\n\n",
			formatTokenCount(b.TotalTokens),
			formatTokenCount(b.ContextLimit),
			b.Percent)
	} else {
		fmt.Fprintf(&sb, "%s tokens (no limit detected)\n\n",
			formatTokenCount(b.TotalTokens))
	}

	// Category bars.
	categories := []struct {
		name   string
		tokens int
		color  lipgloss.TerminalColor
	}{
		{"System", b.SystemTokens, lipgloss.AdaptiveColor{Light: "#6600CC", Dark: "#BB88FF"}},
		{"User", b.UserTokens, lipgloss.AdaptiveColor{Light: "#0070F3", Dark: "#58A6FF"}},
		{"Assistant", b.AssistantTokens, lipgloss.AdaptiveColor{Light: "#006600", Dark: "#66FF66"}},
		{"Tool calls", b.ToolCallTokens, lipgloss.AdaptiveColor{Light: "#CC6600", Dark: "#FFAA44"}},
		{"Tool results", b.ToolResultTokens, lipgloss.AdaptiveColor{Light: "#CC0000", Dark: "#FF6666"}},
	}

	for _, cat := range categories {
		if cat.tokens == 0 && b.TotalTokens == 0 {
			continue
		}
		pct := 0
		if b.TotalTokens > 0 {
			pct = cat.tokens * 100 / b.TotalTokens
		}
		filled := 0
		if b.TotalTokens > 0 {
			filled = cat.tokens * barWidth / b.TotalTokens
		}
		if filled < 0 {
			filled = 0
		}
		if filled > barWidth {
			filled = barWidth
		}

		bar := lipgloss.NewStyle().Foreground(cat.color).Render(strings.Repeat("█", filled))
		empty := strings.Repeat("░", barWidth-filled)

		fmt.Fprintf(&sb, "%-13s %s%s %s %d%%\n",
			cat.name, bar, empty,
			formatTokenCount(cat.tokens), pct)
	}

	// Largest item callout.
	if b.LargestItem != "" && b.LargestSize > 0 {
		sb.WriteString("\n")
		fmt.Fprintf(&sb, "Largest: %s (%s tokens)\n", b.LargestItem, formatTokenCount(b.LargestSize))
	}

	// Suggestion.
	if b.Suggestion != "" {
		sb.WriteString("\n")
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#CC6600", Dark: "#FFAA44"}).Render(b.Suggestion))
		sb.WriteString("\n")
	}

	content := sb.String()

	return lipgloss.Place(
		d.width, d.height,
		lipgloss.Center, lipgloss.Center,
		styleDialogBorder.Width(dialogWidth).Render(content),
	)
}

// formatTokenCount formats a token count with comma separators.
func formatTokenCount(n int) string {
	if n < 0 {
		return "0"
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}

	var result strings.Builder
	remainder := len(s) % 3
	if remainder > 0 {
		result.WriteString(s[:remainder])
	}
	for i := remainder; i < len(s); i += 3 {
		if result.Len() > 0 {
			result.WriteByte(',')
		}
		result.WriteString(s[i : i+3])
	}
	return result.String()
}
