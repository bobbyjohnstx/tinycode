package tui

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// handleKeyMsg handles all keyboard input: leader keys, overlay routing, and global keys.
func (a App) handleKeyMsg(msg tea.KeyMsg) (App, tea.Cmd) {
	// Leader key state machine runs first.
	if a.leader.IsPending() {
		action, consumed := a.leader.HandleKey(msg)
		if consumed {
			if action != "" {
				cmd := a.dispatchLeaderAction(action)
				return a, cmd
			}
			return a, nil
		}
	} else {
		_, consumed := a.leader.HandleKey(msg)
		if consumed {
			// Leader key was just pressed; start timeout.
			return a, a.leader.TimeoutCmd()
		}
	}

	// Route keys to visible overlay (priority order).
	if a.permPrompt.IsVisible() {
		var cmd tea.Cmd
		a.permPrompt, cmd = a.permPrompt.Update(msg)
		return a, cmd
	}
	if a.palette.IsVisible() {
		var cmd tea.Cmd
		a.palette, cmd = a.palette.Update(msg)
		return a, cmd
	}
	if a.agentDlg.IsVisible() {
		var cmd tea.Cmd
		a.agentDlg, cmd = a.agentDlg.Update(msg)
		return a, cmd
	}
	if a.modelDlg.IsVisible() {
		var cmd tea.Cmd
		a.modelDlg, cmd = a.modelDlg.Update(msg)
		return a, cmd
	}
	if a.themeDlg.IsVisible() {
		var cmd tea.Cmd
		a.themeDlg, cmd = a.themeDlg.Update(msg)
		return a, cmd
	}

	// Global keys handled before component dispatch.
	if cmd := a.handleGlobalKey(msg); cmd != nil {
		return a, cmd
	}

	return a.dispatchToFocused(msg)
}

// handleStateMsg handles messages that update application state (session lifecycle,
// provider/agent/command data, connection status).
func (a App) handleStateMsg(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case AgentListMsg:
		m, cmd := a.handleAgentListMsg(msg)
		return m, cmd, true
	case CommandListMsg:
		m, cmd := a.handleCommandListMsg(msg)
		return m, cmd, true
	case ProvidersLoadedMsg:
		m, cmd := a.handleProvidersLoadedMsg(msg)
		return m, cmd, true
	case SessionsLoadedMsg:
		if msg.Err == nil {
			a.state.Sessions = msg.Sessions
			a.sidebar.SetSessions(msg.Sessions)
		}
		return a, nil, true
	case SessionCreatedMsg:
		a.state.Sessions = append([]SessionInfo{msg.Info}, a.state.Sessions...)
		a.sidebar.SetSessions(a.state.Sessions)
		return a, nil, true
	case SessionDeletedMsg:
		a.removeSession(msg.SessionID)
		a.sidebar.SetSessions(a.state.Sessions)
		return a, nil, true
	case SessionSwitchedMsg:
		a.state.ActiveSession = msg.SessionID
		a.state.Messages[msg.SessionID] = nil
		a.syncPromptMetadata()
		return a, nil, true
	case SessionStatusMsg:
		a.state.SessionStatus[msg.SessionID] = msg.Status
		var spinCmd tea.Cmd
		if msg.SessionID == a.state.ActiveSession {
			spinCmd = a.status.SetWorking(msg.Status.Working)
		}
		return a, spinCmd, true
	case MCPStatusMsg:
		a.sidebar.UpdateMCPServer(msg.Server)
		return a, nil, true
	case MCPStatusLoadedMsg:
		if msg.Err == nil {
			a.sidebar.SetMCPServers(msg.Servers)
		}
		return a, nil, true
	case SSEConnectedMsg:
		a.state.Connected = true
		return a, nil, true
	case SSEDisconnectedMsg:
		a.state.Connected = false
		return a, nil, true
	}
	return a, nil, false
}

// handleNotificationMsg handles toast, clipboard, export, and error notifications.
func (a App) handleNotificationMsg(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case LeaderTimeoutMsg:
		a.leader.HandleTimeout()
		return a, nil, true
	case SessionErrorMsg:
		cmd := a.toast.Show(msg.Error, true)
		if msg.SessionID == a.state.ActiveSession || msg.SessionID == "" {
			a.status.SetWorking(false)
		}
		a.state.SessionStatus[msg.SessionID] = SessionStatus{Working: false}
		return a, cmd, true
	case CopiedToClipboardMsg:
		if msg.Err != nil {
			return a, nil, true
		}
		cmd := a.toast.Show(fmt.Sprintf("Copied %d chars", msg.Chars), false)
		return a, cmd, true
	case ExportSessionMsg:
		if msg.Err != nil {
			cmd := a.toast.Show(fmt.Sprintf("Export failed: %v", msg.Err), true)
			return a, cmd, true
		}
		cmd := a.toast.Show("Exported to "+filepath.Base(msg.Path), false)
		return a, cmd, true
	case ToastMsg:
		cmd := a.toast.Show(msg.Text, msg.IsError)
		return a, cmd, true
	case ToastExpiredMsg:
		var cmd tea.Cmd
		a.toast, cmd = a.toast.Update(msg)
		return a, cmd, true
	case PromptSubmittedMsg:
		a.state.SessionStatus[a.state.ActiveSession] = SessionStatus{Working: true}
		spinCmd := a.status.SetWorking(true)
		return a, spinCmd, true
	}
	return a, nil, false
}

// handleDialogMsg handles dialog results, palette selections, theme/model/agent
// choices, permissions, and focus changes.
func (a App) handleDialogMsg(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case PermissionRequestedMsg:
		a.permPrompt.Show(msg.Request)
		return a, nil, true
	case PaletteClosedMsg:
		a.setFocus(FocusPrompt)
		return a, nil, true
	case PaletteSelectedMsg:
		a.setFocus(FocusPrompt)
		if cmd, handled := a.handleClientCommand(msg.Item.Value); handled {
			return a, cmd, true
		}
		return a, nil, true
	case AgentSelectedMsg:
		a.state.CurrentAgent = msg.Agent
		a.prompt.SetMetadata(msg.Agent, a.state.CurrentModel.ModelID, a.state.CurrentModel.ProviderID)
		a.status.SetAgent(msg.Agent)
		a.setFocus(FocusPrompt)
		return a, nil, true
	case ThemePreviewMsg:
		if theme := a.themes.Get(msg.ThemeID); theme != nil {
			ApplyColorTheme(theme)
		}
		return a, nil, true
	case ThemeRevertMsg:
		a.setFocus(FocusPrompt)
		if theme := a.themes.Get(msg.ThemeID); theme != nil {
			ApplyColorTheme(theme)
		}
		return a, nil, true
	case ThemeSelectedMsg:
		a.setFocus(FocusPrompt)
		a.state.CurrentTheme = msg.ThemeID
		if theme := a.themes.Get(msg.ThemeID); theme != nil {
			ApplyColorTheme(theme)
			cmd := a.toast.Show("Theme: "+theme.Name, false)
			return a, cmd, true
		}
		return a, nil, true
	case ModelSelectedMsg:
		m, cmd := a.handleModelSelectedMsg(msg)
		return m, cmd, true
	case PermissionDismissedMsg:
		a.permPrompt.Hide()
		a.setFocus(FocusPrompt)
		slog.Info("permission dismissed", "permission", msg.Request.Permission, "action", msg.Action.String())
		return a, func() tea.Msg {
			return PermissionReplyMsg{
				SessionID:    msg.Request.SessionID,
				PermissionID: msg.Request.ID,
				Action:       msg.Action.String(),
			}
		}, true
	case SidebarSessionSelectedMsg:
		slog.Info("sidebar session selected", "sessionID", msg.SessionID)
		return a, func() tea.Msg {
			return SessionSwitchedMsg{SessionID: msg.SessionID}
		}, true
	case FocusChangedMsg:
		if msg.Target == FocusPalette {
			a.showPalette()
			return a, nil, true
		}
		a.setFocus(msg.Target)
		return a, nil, true
	}
	return a, nil, false
}

// handleAgentListMsg processes the agent list response.
func (a App) handleAgentListMsg(msg AgentListMsg) (App, tea.Cmd) {
	if msg.Err == nil {
		a.state.Agents = msg.Agents
		names := make([]string, len(msg.Agents))
		agentItems := make([]AutocompleteItem, 0, len(msg.Agents))
		for i, ag := range msg.Agents {
			names[i] = ag.Name
			if ag.Mode != "primary" {
				agentItems = append(agentItems, AutocompleteItem{
					Name:        ag.Name,
					Description: ag.Description,
				})
			}
		}
		a.prompt.SetAgents(agentItems)
		a.prompt.SetCycleAgents(names)
		slog.Info("agents loaded", "count", len(msg.Agents), "names", names)
	} else {
		slog.Error("agent list fetch failed", "error", msg.Err)
	}
	return a, nil
}

// handleCommandListMsg processes the command list response.
func (a App) handleCommandListMsg(msg CommandListMsg) (App, tea.Cmd) {
	if msg.Err == nil {
		a.state.Commands = msg.Commands
		items := []AutocompleteItem{
			{Name: "exit", Description: "Exit the app"},
			{Name: "connect", Description: "Select provider and model"},
			{Name: "export", Description: "Export session as Markdown"},
			{Name: "theme", Description: "Change color theme"},
		}
		for _, cmd := range msg.Commands {
			if strings.HasPrefix(cmd.Description, "Switch to ") {
				continue
			}
			items = append(items, AutocompleteItem{
				Name:        cmd.Name,
				Description: cmd.Description,
			})
		}
		a.prompt.SetCommands(items)
	}
	return a, nil
}

// handleProvidersLoadedMsg processes the providers/models discovery response.
func (a App) handleProvidersLoadedMsg(msg ProvidersLoadedMsg) (App, tea.Cmd) {
	if msg.Err == nil {
		a.state.Providers = msg.Providers
	}
	if a.state.CurrentModel.ModelID == "" && msg.DefaultProvider != "" && msg.DefaultModel != "" {
		a.state.CurrentModel = ModelSelection{
			ProviderID: msg.DefaultProvider,
			ModelID:    msg.DefaultModel,
		}
		modelName, providerName := lookupModelDisplay(a.state.Providers, msg.DefaultProvider, msg.DefaultModel)
		a.prompt.SetMetadata(a.state.CurrentAgent, modelName, providerName)
		a.status.SetModel(modelName, providerName)
	}
	if a.state.PendingModelDialog {
		a.state.PendingModelDialog = false
		a.modelDlg.Show(a.state.Providers, a.state.CurrentModel)
		a.setFocus(FocusDialog)
	}
	a.updateSidebarContext()
	return a, nil
}

// handleModelSelectedMsg processes a model selection from the model dialog.
func (a App) handleModelSelectedMsg(msg ModelSelectedMsg) (App, tea.Cmd) {
	a.setFocus(FocusPrompt)
	a.state.CurrentModel = msg.Selection
	modelName, providerName := lookupModelDisplay(a.state.Providers, msg.Selection.ProviderID, msg.Selection.ModelID)
	a.prompt.SetMetadata(a.state.CurrentAgent, modelName, providerName)
	a.status.SetModel(modelName, providerName)
	a.updateSidebarContext()
	return a, nil
}

// forwardSSEMessages dispatches SSE-originated messages to the chat view and status bar.
func (a App) forwardSSEMessages(msg tea.Msg) (App, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd
	a.chat, cmd = a.chat.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}
	var statusCmd tea.Cmd
	a.status, statusCmd = a.status.Update(msg)
	if statusCmd != nil {
		cmds = append(cmds, statusCmd)
	}
	a.updateSidebarContext()
	return a, tea.Batch(cmds...)
}

// handleMouseMsg routes mouse events to the appropriate component.
func (a App) handleMouseMsg(msg tea.MouseMsg) (App, tea.Cmd) {
	l := calculateLayout(a.width, a.height, a.sidebar.IsOpen())
	if msg.Y < l.chatHeight {
		var cmd tea.Cmd
		a.chat, cmd = a.chat.Update(msg)
		return a, cmd
	}
	return a.dispatchToFocused(msg)
}

// dispatchToFocused routes messages to the currently focused component or dialog overlay,
// and always forwards spinner ticks to the status bar.
func (a App) dispatchToFocused(msg tea.Msg) (App, tea.Cmd) {
	var cmds []tea.Cmd

	if a.dialog.IsVisible() {
		var cmd tea.Cmd
		a.dialog, cmd = a.dialog.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return a, tea.Batch(cmds...)
	}

	switch a.focus {
	case FocusPrompt:
		var cmd tea.Cmd
		a.prompt, cmd = a.prompt.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	default:
		var cmd tea.Cmd
		a.chat, cmd = a.chat.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	// Always forward spinner ticks to status bar.
	var statusCmd tea.Cmd
	a.status, statusCmd = a.status.Update(msg)
	if statusCmd != nil {
		cmds = append(cmds, statusCmd)
	}

	return a, tea.Batch(cmds...)
}
