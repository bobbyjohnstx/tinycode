package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SessionDialog displays a selectable list of sessions.
type SessionDialog struct {
	sessions []SessionInfo
	selected int
	width    int
	height   int
	visible  bool
}

// NewSessionDialog creates a SessionDialog.
func NewSessionDialog() SessionDialog {
	return SessionDialog{}
}

// Show makes the dialog visible and resets selection.
func (d *SessionDialog) Show(sessions []SessionInfo) {
	d.sessions = sessions
	d.selected = 0
	d.visible = true
}

// Hide closes the dialog.
func (d *SessionDialog) Hide() {
	d.visible = false
}

// IsVisible reports whether the dialog is shown.
func (d SessionDialog) IsVisible() bool {
	return d.visible
}

// SetSize updates the dialog dimensions.
func (d *SessionDialog) SetSize(width, height int) {
	d.width = width
	d.height = height
}

// Init implements tea.Model.
func (d SessionDialog) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (d SessionDialog) Update(msg tea.Msg) (SessionDialog, tea.Cmd) {
	if !d.visible {
		return d, nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return d, nil
	}

	itemCount := len(d.sessions) + 1 // +1 for "New Session"
	switch keyMsg.String() {
	case "up", "k":
		if d.selected > 0 {
			d.selected--
		}
	case "down", "j":
		if d.selected < itemCount-1 {
			d.selected++
		}
	case "enter":
		if d.selected == 0 {
			// "New Session" — caller handles creation.
			d.visible = false
			return d, func() tea.Msg {
				return SessionSwitchedMsg{SessionID: ""}
			}
		}
		idx := d.selected - 1
		if idx < len(d.sessions) {
			id := d.sessions[idx].ID
			d.visible = false
			return d, func() tea.Msg { return SessionSwitchedMsg{SessionID: id} }
		}
	case "esc", "q":
		d.visible = false
	}

	return d, nil
}

// View implements tea.Model.
func (d SessionDialog) View() string {
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

	var sb strings.Builder
	sb.WriteString("Sessions\n\n")

	// "New Session" option at top.
	if d.selected == 0 {
		sb.WriteString(styleSelected.Render("▸ + New Session"))
	} else {
		sb.WriteString("  + New Session")
	}
	sb.WriteString("\n")

	maxVisible := d.height - 6
	if maxVisible < 3 {
		maxVisible = 3
	}

	for i, sess := range d.sessions {
		if i >= maxVisible {
			sb.WriteString(fmt.Sprintf("  ... %d more", len(d.sessions)-maxVisible))
			break
		}

		sb.WriteString("\n")

		title := truncate(sess.Title, dialogWidth-10)
		age := relativeTime(sess.UpdatedAt)

		line := fmt.Sprintf("%s  %s", title, styleMetadata.Render(age))

		if d.selected == i+1 {
			sb.WriteString(styleSelected.Render("▸ " + line))
		} else {
			sb.WriteString("  " + line)
		}
	}

	content := sb.String()

	return lipgloss.Place(
		d.width, d.height,
		lipgloss.Center, lipgloss.Center,
		styleDialogBorder.Width(dialogWidth).Render(content),
	)
}

// truncate shortens a string to maxLen, adding ellipsis if needed.
func truncate(s string, maxLen int) string {
	if maxLen < 4 {
		maxLen = 4
	}
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// relativeTime returns a human-readable relative time string.
func relativeTime(ms int64) string {
	if ms == 0 {
		return ""
	}
	t := time.UnixMilli(ms)
	d := time.Since(t)

	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	}
}
