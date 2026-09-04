package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// PermissionAction identifies the user's response to a permission prompt.
type PermissionAction int

const (
	PermissionAllow PermissionAction = iota
	PermissionAlways
	PermissionReject
)

func (a PermissionAction) String() string {
	switch a {
	case PermissionAllow:
		return "allow"
	case PermissionAlways:
		return "always"
	case PermissionReject:
		return "reject"
	default:
		return "reject"
	}
}

// PermissionRequestedMsg signals a new permission prompt should be shown.
type PermissionRequestedMsg struct {
	Request PermissionRequest
}

// PermissionDismissedMsg signals the permission prompt was answered.
type PermissionDismissedMsg struct {
	Request PermissionRequest
	Action  PermissionAction
}

// PermissionPrompt shows a tool permission request with Allow/Always/Reject.
type PermissionPrompt struct {
	request  PermissionRequest
	selected PermissionAction
	visible  bool
	width    int
	height   int
}

// NewPermissionPrompt creates a PermissionPrompt.
func NewPermissionPrompt() PermissionPrompt {
	return PermissionPrompt{
		selected: PermissionAllow,
	}
}

// Show displays the permission prompt for the given request.
func (p *PermissionPrompt) Show(req PermissionRequest) {
	p.request = req
	p.selected = PermissionAllow
	p.visible = true
}

// Hide closes the permission prompt.
func (p *PermissionPrompt) Hide() {
	p.visible = false
}

// IsVisible reports whether the prompt is shown.
func (p PermissionPrompt) IsVisible() bool {
	return p.visible
}

// SetSize updates the prompt dimensions.
func (p *PermissionPrompt) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// Update handles key events for the permission prompt.
func (p PermissionPrompt) Update(msg tea.Msg) (PermissionPrompt, tea.Cmd) {
	if !p.visible {
		return p, nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return p, nil
	}

	switch keyMsg.String() {
	case "left", "h":
		if p.selected > PermissionAllow {
			p.selected--
		}
	case "right", "l":
		if p.selected < PermissionReject {
			p.selected++
		}
	case "enter":
		req := p.request
		action := p.selected
		p.visible = false
		return p, func() tea.Msg {
			return PermissionDismissedMsg{Request: req, Action: action}
		}
	case "esc":
		req := p.request
		p.visible = false
		return p, func() tea.Msg {
			return PermissionDismissedMsg{Request: req, Action: PermissionReject}
		}
	}

	return p, nil
}

// View renders the permission prompt.
func (p PermissionPrompt) View() string {
	if !p.visible {
		return ""
	}

	dialogWidth := p.width / 2
	if dialogWidth < 50 {
		dialogWidth = 50
	}
	if dialogWidth > 80 {
		dialogWidth = 80
	}

	var sb strings.Builder
	sb.WriteString("Permission Required\n\n")
	sb.WriteString(fmt.Sprintf("Tool: %s\n", styleToolName.Render(p.request.Tool)))

	if p.request.Input != nil {
		if data, err := json.MarshalIndent(p.request.Input, "", "  "); err == nil {
			args := string(data)
			if len(args) > 200 {
				args = args[:197] + "..."
			}
			sb.WriteString(fmt.Sprintf("\n%s\n", styleMetadata.Render(args)))
		}
	}

	sb.WriteString("\n")

	actions := []struct {
		label  string
		action PermissionAction
	}{
		{"Allow", PermissionAllow},
		{"Always", PermissionAlways},
		{"Reject", PermissionReject},
	}

	var buttons []string
	for _, a := range actions {
		label := fmt.Sprintf(" %s ", a.label)
		if p.selected == a.action {
			if a.action == PermissionReject {
				buttons = append(buttons, styleSelected.Render("["+label+"]"))
			} else {
				buttons = append(buttons, styleSelected.Render("["+label+"]"))
			}
		} else {
			buttons = append(buttons, " "+label+" ")
		}
	}
	sb.WriteString(strings.Join(buttons, "  "))

	content := sb.String()

	return lipgloss.Place(
		p.width, p.height,
		lipgloss.Center, lipgloss.Center,
		styleDialogBorder.Width(dialogWidth).Render(content),
	)
}
