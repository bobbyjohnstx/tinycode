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
	sessions  []SessionInfo
	selected  int
	scrollTop int
	width     int
	height    int
	visible   bool
	hasMore   bool
	loading   bool
}

// NewSessionDialog creates a SessionDialog.
func NewSessionDialog() SessionDialog {
	return SessionDialog{}
}

// Show makes the dialog visible and resets selection.
func (d *SessionDialog) Show(sessions []SessionInfo, hasMore bool) {
	d.sessions = sessions
	d.selected = 0
	d.scrollTop = 0
	d.visible = true
	d.hasMore = hasMore
	d.loading = false
}

// AppendSessions adds paginated sessions and updates the has-more flag.
func (d *SessionDialog) AppendSessions(sessions []SessionInfo, hasMore bool) {
	d.sessions = append(d.sessions, sessions...)
	d.hasMore = hasMore
	d.loading = false
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
		} else if d.hasMore && !d.loading {
			// At the bottom with more sessions available — request next page.
			d.loading = true
			d.adjustScroll()
			return d, func() tea.Msg { return SessionsLoadMoreMsg{} }
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

	d.adjustScroll()
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

	end := d.scrollTop + maxVisible
	if end > len(d.sessions) {
		end = len(d.sessions)
	}

	if d.scrollTop > 0 {
		sb.WriteString(fmt.Sprintf("  ... %d above\n", d.scrollTop))
	}

	for i := d.scrollTop; i < end; i++ {
		sess := d.sessions[i]

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

	remaining := len(d.sessions) - end
	if remaining > 0 {
		sb.WriteString(fmt.Sprintf("\n  ... %d more", remaining))
	}
	if d.loading {
		sb.WriteString("\n  Loading...")
	}

	content := sb.String()

	return lipgloss.Place(
		d.width, d.height,
		lipgloss.Center, lipgloss.Center,
		styleDialogBorder.Width(dialogWidth).Render(content),
	)
}

// adjustScroll keeps the selected item within the visible window.
func (d *SessionDialog) adjustScroll() {
	maxVisible := d.height - 6
	if maxVisible < 3 {
		maxVisible = 3
	}
	// selected 0 = "New Session", 1..N = sessions[0..N-1]
	sessionIdx := d.selected - 1
	if sessionIdx < 0 {
		d.scrollTop = 0
		return
	}
	if sessionIdx < d.scrollTop {
		d.scrollTop = sessionIdx
	}
	if sessionIdx >= d.scrollTop+maxVisible {
		d.scrollTop = sessionIdx - maxVisible + 1
	}
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
