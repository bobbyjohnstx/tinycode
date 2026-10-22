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

// String returns the SDK/docs permission reply: once|always|reject.
func (a PermissionAction) String() string {
	switch a {
	case PermissionAllow:
		return "once"
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
// Requests that arrive while one is visible are queued and shown after it.
type PermissionPrompt struct {
	request       PermissionRequest
	queue         []PermissionRequest
	selected      PermissionAction
	visible       bool
	width         int
	height        int
	cancelledIDs  map[string]struct{} // remote cancel; late reply toast-once
	cancelToasted map[string]struct{}
}

// NewPermissionPrompt creates a PermissionPrompt.
func NewPermissionPrompt() PermissionPrompt {
	return PermissionPrompt{
		selected: PermissionAllow,
	}
}

// Show displays the permission prompt. A different request that arrives while
// one is already visible is queued instead of replacing it.
func (p *PermissionPrompt) Show(req PermissionRequest) {
	if p.visible {
		if req.ID == p.request.ID || p.queued(req.ID) {
			return
		}
		p.queue = append(p.queue, req)
		return
	}
	p.request = req
	p.selected = PermissionAllow
	p.visible = true
}

// ShowNext displays the next queued request. It reports whether one was shown.
func (p *PermissionPrompt) ShowNext() bool {
	if len(p.queue) == 0 {
		p.visible = false
		return false
	}
	next := p.queue[0]
	p.queue = p.queue[1:]
	p.request = next
	p.selected = PermissionAllow
	p.visible = true
	return true
}

// DropQueued removes queued requests. An empty perm drops the whole queue
// (Reject cascade). shell/bash drops only queued shell asks (Always on shell).
// Any other permission leaves the queue unchanged so edit/read Always does not
// hide prompts the server is still waiting on.
func (p *PermissionPrompt) DropQueued(perm string) {
	if perm == "" {
		p.queue = nil
		return
	}
	if !shellPermission(perm) {
		return
	}
	kept := p.queue[:0]
	for _, req := range p.queue {
		if shellPermission(req.Permission) {
			continue
		}
		kept = append(kept, req)
	}
	p.queue = kept
}

func (p *PermissionPrompt) queued(id string) bool {
	for _, req := range p.queue {
		if req.ID == id {
			return true
		}
	}
	return false
}

func shellPermission(perm string) bool {
	return perm == "shell" || perm == "bash"
}

// VisibleID returns the ID of the currently shown request, or empty if hidden.
func (p PermissionPrompt) VisibleID() string {
	if !p.visible {
		return ""
	}
	return p.request.ID
}

// removeQueued drops a queued permission by ID (SSE resolve for a non-visible item).
func (p *PermissionPrompt) removeQueued(id string) {
	if len(p.queue) == 0 {
		return
	}
	kept := p.queue[:0]
	for _, req := range p.queue {
		if req.ID == id {
			continue
		}
		kept = append(kept, req)
	}
	p.queue = kept
}

// ClearSession dismisses the visible prompt and drops queued asks for sessionID.
// Empty sessionID clears all permission UI state for the overlay.
func (p *PermissionPrompt) ClearSession(sessionID string) {
	if p.visible && (sessionID == "" || p.request.SessionID == sessionID) {
		if p.request.ID != "" {
			p.markCancelled(p.request.ID)
		}
		p.visible = false
	}
	if sessionID == "" {
		for _, req := range p.queue {
			p.markCancelled(req.ID)
		}
		p.queue = nil
		return
	}
	kept := p.queue[:0]
	for _, req := range p.queue {
		if req.SessionID == sessionID {
			p.markCancelled(req.ID)
			continue
		}
		kept = append(kept, req)
	}
	p.queue = kept
}

func (p *PermissionPrompt) markCancelled(id string) {
	if id == "" {
		return
	}
	if p.cancelledIDs == nil {
		p.cancelledIDs = make(map[string]struct{})
	}
	p.cancelledIDs[id] = struct{}{}
}

// lateReplyToast reports whether a failed HTTP reply for id should toast.
// Remotely cancelled IDs toast at most once, then clear.
func (p *PermissionPrompt) lateReplyToast(id string) bool {
	if id == "" || p.cancelledIDs == nil {
		return true
	}
	if _, ok := p.cancelledIDs[id]; !ok {
		return true
	}
	if p.cancelToasted == nil {
		p.cancelToasted = make(map[string]struct{})
	}
	if _, done := p.cancelToasted[id]; done {
		return false
	}
	p.cancelToasted[id] = struct{}{}
	return true
}

// Hide closes the permission prompt without discarding queued requests.
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
		return p.dismiss(p.selected)
	case "esc":
		return p.dismiss(PermissionReject)
	}

	return p, nil
}

// dismiss hides the prompt and replies with action.
func (p PermissionPrompt) dismiss(action PermissionAction) (PermissionPrompt, tea.Cmd) {
	req := p.request
	p.visible = false
	return p, func() tea.Msg {
		return PermissionDismissedMsg{Request: req, Action: action}
	}
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
	sb.WriteString("    " + dimStyle.Render("⇆ select  enter confirm  esc reject"))

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
