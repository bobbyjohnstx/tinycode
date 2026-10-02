package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bobbyjohnstx/tinycode/internal/tui/api"
)

// AgentDialogOpenMsg signals the agent dialog should open.
type AgentDialogOpenMsg struct{}

// AgentSelectedMsg signals an agent was selected.
type AgentSelectedMsg struct {
	Agent string
}

// AgentDialog displays a list of agents for selection.
type AgentDialog struct {
	agents   []api.AgentInfo
	selected int
	visible  bool
	width    int
	height   int
}

// NewAgentDialog creates an AgentDialog.
func NewAgentDialog() AgentDialog {
	return AgentDialog{}
}

// Show opens the dialog with the given agents.
func (d *AgentDialog) Show(agents []api.AgentInfo) {
	d.agents = agents
	d.selected = 0
	d.visible = true
}

// Refresh updates the agent list while preserving the selection position.
func (d *AgentDialog) Refresh(agents []api.AgentInfo) {
	selectedName := ""
	if d.selected < len(d.agents) {
		selectedName = d.agents[d.selected].Name
	}
	d.agents = agents
	// Try to restore selection by name.
	for i, a := range agents {
		if a.Name == selectedName {
			d.selected = i
			return
		}
	}
	if d.selected >= len(agents) {
		d.selected = max(0, len(agents)-1)
	}
}

// Hide closes the dialog.
func (d *AgentDialog) Hide() {
	d.visible = false
}

// IsVisible reports whether the dialog is shown.
func (d AgentDialog) IsVisible() bool {
	return d.visible
}

// SetSize updates the dialog dimensions.
func (d *AgentDialog) SetSize(width, height int) {
	d.width = width
	d.height = height
}

// Update handles key events for the agent dialog.
func (d AgentDialog) Update(msg tea.Msg) (AgentDialog, tea.Cmd) {
	if !d.visible {
		return d, nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return d, nil
	}

	count := len(d.agents)
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
	case "enter":
		if d.selected < count {
			agent := d.agents[d.selected]
			if agent.Disabled {
				return d, nil
			}
			d.visible = false
			return d, func() tea.Msg { return AgentSelectedMsg{Agent: agent.Name} }
		}
	case "d":
		if d.selected < count {
			agent := d.agents[d.selected]
			if agent.Native {
				return d, nil
			}
			return d, func() tea.Msg {
				return AgentToggleMsg{Agent: agent.Name, Disabled: !agent.Disabled}
			}
		}
	case "esc", "q":
		d.visible = false
	}

	return d, nil
}

// View renders the agent dialog.
func (d AgentDialog) View() string {
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
	sb.WriteString("Select Agent  ")
	sb.WriteString(styleMetadata.Render("d=toggle"))
	sb.WriteString("\n")

	maxVisible := d.height - 6
	if maxVisible < 3 {
		maxVisible = 3
	}

	for i, agent := range d.agents {
		if i >= maxVisible {
			break
		}
		sb.WriteString("\n")

		line := agent.Name
		if agent.Disabled {
			line += " " + styleMetadata.Render("[off]")
		}
		if agent.Description != "" && !agent.Disabled {
			desc := truncate(agent.Description, dialogWidth-len(agent.Name)-10)
			line += "  " + styleMetadata.Render(desc)
		}

		if agent.Disabled {
			if i == d.selected {
				sb.WriteString(styleMetadata.Render("▸ " + agent.Name + " [off]"))
			} else {
				sb.WriteString(styleMetadata.Render("  " + agent.Name + " [off]"))
			}
		} else if i == d.selected {
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
