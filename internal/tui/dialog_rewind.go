package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// RewindTurn represents a single user turn for the rewind picker.
type RewindTurn struct {
	Index     int
	MessageID string
	Preview   string
	CreatedAt int64 // milliseconds
}

// RewindSelectedMsg is emitted when the user selects a turn to rewind to.
type RewindSelectedMsg struct {
	Turn RewindTurn
}

// RewindForkMsg is emitted when the user chooses to fork the session at a
// selected turn instead of rewinding in place.
type RewindForkMsg struct {
	Turn RewindTurn
}

// RewindDialog displays a picker for rewinding to a previous conversation turn.
type RewindDialog struct {
	turns    []RewindTurn
	selected int
	visible  bool
	width    int
	height   int
}

// NewRewindDialog creates a RewindDialog.
func NewRewindDialog() RewindDialog {
	return RewindDialog{}
}

// Show opens the dialog with the given turns (most recent first).
func (d *RewindDialog) Show(turns []RewindTurn) {
	d.turns = turns
	d.selected = 0
	d.visible = true
}

// Hide closes the dialog.
func (d *RewindDialog) Hide() {
	d.visible = false
}

// IsVisible reports whether the dialog is shown.
func (d RewindDialog) IsVisible() bool {
	return d.visible
}

// SetSize updates the dialog dimensions.
func (d *RewindDialog) SetSize(width, height int) {
	d.width = width
	d.height = height
}

// Update handles key events for the rewind picker.
func (d RewindDialog) Update(msg tea.Msg) (RewindDialog, tea.Cmd) {
	if !d.visible {
		return d, nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return d, nil
	}

	switch keyMsg.String() {
	case "up", "k":
		if d.selected > 0 {
			d.selected--
		}
	case "down", "j":
		if d.selected < len(d.turns)-1 {
			d.selected++
		}
	case "enter":
		if d.selected < len(d.turns) {
			turn := d.turns[d.selected]
			d.visible = false
			return d, func() tea.Msg {
				return RewindSelectedMsg{Turn: turn}
			}
		}
	case "f":
		if d.selected < len(d.turns) {
			turn := d.turns[d.selected]
			d.visible = false
			return d, func() tea.Msg {
				return RewindForkMsg{Turn: turn}
			}
		}
	case "esc", "q":
		d.visible = false
	}

	return d, nil
}

// View renders the rewind picker dialog.
func (d RewindDialog) View() string {
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
	sb.WriteString("Rewind to Turn  ")
	sb.WriteString(styleMetadata.Render("enter=rewind  f=fork  esc=close"))
	sb.WriteString("\n\n")

	if len(d.turns) == 0 {
		sb.WriteString("  No user turns found")
	}

	contentWidth := dialogWidth - 8
	if contentWidth < 20 {
		contentWidth = 20
	}

	maxVisible := d.height - 6
	if maxVisible < 3 {
		maxVisible = 3
	}

	for i, turn := range d.turns {
		if i >= maxVisible {
			sb.WriteString(fmt.Sprintf("\n  ... %d more", len(d.turns)-maxVisible))
			break
		}

		if i > 0 {
			sb.WriteString("\n")
		}

		preview := turn.Preview
		maxPreview := contentWidth - 12
		if maxPreview < 10 {
			maxPreview = 10
		}
		if len(preview) > maxPreview {
			preview = preview[:maxPreview-3] + "..."
		}

		label := fmt.Sprintf("Turn %d", turn.Index)
		line := fmt.Sprintf("%s: %s", label, styleMetadata.Render(preview))

		if d.selected == i {
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

// ExtractTurns builds a list of RewindTurn from chat messages.
// User messages define turns. Most recent turn appears first.
// The current (latest) turn is excluded since rewinding to it is a no-op.
func ExtractTurns(messages []MessageView) []RewindTurn {
	var turns []RewindTurn
	turnNum := 0

	for _, msg := range messages {
		if msg.Info.Role != "user" {
			continue
		}
		turnNum++
		preview := extractUserPreview(msg)
		turns = append(turns, RewindTurn{
			Index:     turnNum,
			MessageID: msg.Info.ID,
			Preview:   preview,
			CreatedAt: parseCreatedAtMs(msg.Info.CreatedAt),
		})
	}

	// Exclude the latest turn (rewinding to current is a no-op).
	if len(turns) > 0 {
		turns = turns[:len(turns)-1]
	}

	// Reverse so most recent appears first.
	for i, j := 0, len(turns)-1; i < j; i, j = i+1, j-1 {
		turns[i], turns[j] = turns[j], turns[i]
	}

	return turns
}

// extractUserPreview returns the first ~80 chars of the user message text.
func extractUserPreview(msg MessageView) string {
	for _, part := range msg.Parts {
		if part.Type == "text" && part.Text != "" {
			text := strings.TrimSpace(part.Text)
			// Collapse newlines to spaces for preview.
			text = strings.ReplaceAll(text, "\n", " ")
			if len(text) > 80 {
				return text[:77] + "..."
			}
			return text
		}
	}
	return "(no text)"
}

// parseCreatedAtMs parses the createdAt string to milliseconds.
// Falls back to 0 if parsing fails.
func parseCreatedAtMs(createdAt string) int64 {
	if createdAt == "" {
		return 0
	}
	// The field is an ISO string or numeric string; we only need
	// it for the server call, which uses the message ID instead.
	return 0
}
