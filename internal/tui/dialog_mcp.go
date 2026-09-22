package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// MCPDialogOpenMsg signals the MCP dialog should open.
type MCPDialogOpenMsg struct{}

// MCPReconnectRequestMsg requests reconnection of a specific MCP server.
type MCPReconnectRequestMsg struct {
	Name string
}

// MCPDialog displays MCP server status with reconnect controls.
type MCPDialog struct {
	servers  []MCPServer
	selected int
	visible  bool
	width    int
	height   int
}

// NewMCPDialog creates an MCPDialog.
func NewMCPDialog() MCPDialog {
	return MCPDialog{}
}

// Show opens the dialog with the given servers.
func (d *MCPDialog) Show(servers []MCPServer) {
	d.servers = servers
	d.selected = 0
	d.visible = true
}

// Refresh updates the server list while preserving the selection position.
func (d *MCPDialog) Refresh(servers []MCPServer) {
	selectedName := ""
	if d.selected < len(d.servers) {
		selectedName = d.servers[d.selected].Name
	}
	d.servers = servers
	for i, s := range servers {
		if s.Name == selectedName {
			d.selected = i
			return
		}
	}
	if d.selected >= len(servers) {
		d.selected = max(0, len(servers)-1)
	}
}

// Hide closes the dialog.
func (d *MCPDialog) Hide() {
	d.visible = false
}

// IsVisible reports whether the dialog is shown.
func (d MCPDialog) IsVisible() bool {
	return d.visible
}

// SetSize updates the dialog dimensions.
func (d *MCPDialog) SetSize(width, height int) {
	d.width = width
	d.height = height
}

// Update handles key events for the MCP dialog.
func (d MCPDialog) Update(msg tea.Msg) (MCPDialog, tea.Cmd) {
	if !d.visible {
		return d, nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return d, nil
	}

	count := len(d.servers)
	if count == 0 {
		if keyMsg.String() == "esc" || keyMsg.String() == "q" {
			d.visible = false
		}
		return d, nil
	}

	switch keyMsg.String() {
	case "up", "k":
		d.selected = wrapIndex(d.selected-1, count)
	case "down", "j":
		d.selected = wrapIndex(d.selected+1, count)
	case "r":
		if d.selected < count {
			srv := d.servers[d.selected]
			if srv.Status == "error" || srv.Status == "disconnected" {
				name := srv.Name
				return d, func() tea.Msg { return MCPReconnectRequestMsg{Name: name} }
			}
		}
	case "esc", "q":
		d.visible = false
	}

	return d, nil
}

// View renders the MCP dialog.
func (d MCPDialog) View() string {
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
	sb.WriteString("MCP Servers  ")
	sb.WriteString(styleMetadata.Render("r=reconnect  esc=close"))
	sb.WriteString("\n")

	if len(d.servers) == 0 {
		sb.WriteString("\n")
		sb.WriteString(styleMetadata.Render("No MCP servers configured"))
	}

	maxVisible := d.height - 6
	if maxVisible < 3 {
		maxVisible = 3
	}

	for i, srv := range d.servers {
		if i >= maxVisible {
			break
		}
		sb.WriteString("\n")

		indicator := "○"
		indicatorStyle := styleMetadata
		switch srv.Status {
		case "connected":
			indicator = "●"
			indicatorStyle = lipgloss.NewStyle().Foreground(colorSuccess)
		case "error":
			indicator = "●"
			indicatorStyle = lipgloss.NewStyle().Foreground(colorError)
		case "connecting", "reconnecting":
			indicator = "◌"
		}

		detail := ""
		if srv.Status == "connected" && srv.ToolCount > 0 {
			detail = fmt.Sprintf(" (%d tools)", srv.ToolCount)
		} else if srv.Status != "connected" && srv.Status != "" {
			detail = " [" + srv.Status + "]"
		}

		line := indicatorStyle.Render(indicator) + " " + srv.Name + styleMetadata.Render(detail)

		if i == d.selected {
			sb.WriteString(styleSelected.Render("▸ ") + line)
		} else {
			sb.WriteString("  " + line)
		}

		if srv.Error != "" {
			sb.WriteString("\n    " + styleMetadata.Render(truncate(srv.Error, dialogWidth-8)))
		}
	}

	content := sb.String()

	return lipgloss.Place(
		d.width, d.height,
		lipgloss.Center, lipgloss.Center,
		styleDialogBorder.Width(dialogWidth).Render(content),
	)
}
