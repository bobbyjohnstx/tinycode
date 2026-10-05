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
		// Ignore Esc so an accidental press does not reject the request.
		// Reject requires selecting Reject and confirming with Enter.
	}

	return p, nil
}

// View renders the permission prompt.
func (p PermissionPrompt) View() string {
	if !p.visible {
		return ""
	}

	dialogWidth := p.width - 4
	if dialogWidth < 50 {
		dialogWidth = 50
	}
	if dialogWidth > 100 {
		dialogWidth = 100
	}

	warningColor := lipgloss.AdaptiveColor{Light: "#CC8800", Dark: "#FFAA33"}
	warningStyle := lipgloss.NewStyle().Foreground(warningColor)
	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#777777"})

	icon, title, body := p.toolDescription()

	var sb strings.Builder
	sb.WriteString(warningStyle.Render("△") + " Permission required\n")
	sb.WriteString("  " + dimStyle.Render(icon) + " " + title + "\n")

	if body != "" {
		sb.WriteString("\n" + dimStyle.Render(body) + "\n")
	}

	sb.WriteString("\n")

	actions := []struct {
		label  string
		action PermissionAction
	}{
		{"Allow once", PermissionAllow},
		{"Allow always", PermissionAlways},
		{"Reject", PermissionReject},
	}

	rejectStyle := lipgloss.NewStyle().Bold(true).Foreground(colorError)

	var buttons []string
	for _, a := range actions {
		label := fmt.Sprintf(" %s ", a.label)
		if p.selected == a.action {
			if a.action == PermissionReject {
				buttons = append(buttons, rejectStyle.Render("["+label+"]"))
			} else {
				buttons = append(buttons, warningStyle.Render("["+label+"]"))
			}
		} else {
			buttons = append(buttons, " "+label+" ")
		}
	}
	sb.WriteString(strings.Join(buttons, "  "))
	sb.WriteString("    " + dimStyle.Render("⇆ select  enter confirm"))

	content := sb.String()

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(warningColor).
		Padding(1, 2)

	return lipgloss.Place(
		p.width, p.height,
		lipgloss.Center, lipgloss.Center,
		borderStyle.Width(dialogWidth).Render(content),
	)
}

// toolDescription returns an icon, title, and body for the permission request
// based on the permission type and metadata, matching the TS TUI's contextual display.
func (p PermissionPrompt) toolDescription() (icon, title, body string) {
	args := p.parseArgs()
	toolName, _ := p.request.Metadata["tool"].(string)

	switch p.request.Permission {
	case "shell", "destructive-shell":
		title = "Shell command"
		if desc, ok := args["description"].(string); ok && desc != "" {
			title = desc
		}
		if cmd, ok := args["command"].(string); ok && cmd != "" {
			body = "  $ " + cmd
		}
		return "#", title, body

	case "edit":
		fp := p.argPath(args, "file_path")
		if fp != "" {
			title = "Edit " + fp
		} else {
			title = "Edit file"
		}
		return "→", title, body

	case "read":
		fp := p.argPath(args, "file_path")
		if fp == "" {
			fp = p.argPath(args, "filePath")
		}
		if fp != "" {
			title = "Read " + fp
		} else {
			title = "Read file"
		}
		return "→", title, body

	case "glob":
		if pattern, ok := args["pattern"].(string); ok && pattern != "" {
			title = fmt.Sprintf("Glob \"%s\"", pattern)
		} else {
			title = "Glob"
		}
		return "✱", title, body

	case "grep":
		if pattern, ok := args["pattern"].(string); ok && pattern != "" {
			title = fmt.Sprintf("Grep \"%s\"", pattern)
		} else {
			title = "Grep"
		}
		return "✱", title, body

	default:
		if toolName != "" {
			title = fmt.Sprintf("Call tool %s", toolName)
		} else {
			title = fmt.Sprintf("Permission: %s", p.request.Permission)
		}
		if len(args) == 0 {
			return "⚙", title, body
		}
		raw, err := json.MarshalIndent(args, "  ", "  ")
		if err != nil {
			return "⚙", title, body
		}
		s := string(raw)
		if len(s) > 200 {
			s = s[:197] + "..."
		}
		body = "  " + s
		return "⚙", title, body
	}
}

// parseArgs extracts the tool arguments from metadata. The server stores
// args as a JSON string in metadata["args"].
func (p PermissionPrompt) parseArgs() map[string]any {
	if p.request.Metadata == nil {
		return nil
	}
	raw, ok := p.request.Metadata["args"].(string)
	if !ok || raw == "" {
		return nil
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil
	}
	return args
}

// argPath extracts a file path from args and shortens it for display.
func (p PermissionPrompt) argPath(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	if fp, ok := args[key].(string); ok && fp != "" {
		return shortenCwd(fp)
	}
	return ""
}
