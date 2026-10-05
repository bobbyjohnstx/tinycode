package tui

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bobbyjohnstx/tinycode/internal/tui/api"
)

// handleKeyMsg handles all keyboard input: leader keys, overlay routing, and global keys.
func (a App) handleKeyMsg(msg tea.KeyMsg) (App, tea.Cmd) {
	// Leader key state machine runs first.
	if a.leader.IsPending() {
		action, consumed := a.leader.HandleKey(msg)
		if consumed {
			a.status.SetLeaderPending(false)
			a.whichKey.Hide()
			if action != "" {
				cmd := a.dispatchLeaderAction(action)
				return a, cmd
			}
			return a, nil
		}
	} else {
		_, consumed := a.leader.HandleKey(msg)
		if consumed {
			// Leader key was just pressed; start timeout and show which-key panel.
			a.status.SetLeaderPending(true)
			a.whichKey.Show(LeaderKeyEntries(a.keys))
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
		if !a.modelDlg.IsVisible() {
			a.setFocus(FocusPrompt)
		}
		return a, cmd
	}
	if a.themeDlg.IsVisible() {
		var cmd tea.Cmd
		a.themeDlg, cmd = a.themeDlg.Update(msg)
		return a, cmd
	}
	if a.debugDlg.IsVisible() {
		var cmd tea.Cmd
		a.debugDlg, cmd = a.debugDlg.Update(msg)
		if !a.debugDlg.IsVisible() {
			a.setFocus(FocusPrompt)
		}
		return a, cmd
	}
	if a.privacyDlg.IsVisible() {
		var cmd tea.Cmd
		a.privacyDlg, cmd = a.privacyDlg.Update(msg)
		if !a.privacyDlg.IsVisible() {
			a.setFocus(FocusPrompt)
		}
		return a, cmd
	}
	if a.contextDlg.IsVisible() {
		var cmd tea.Cmd
		a.contextDlg, cmd = a.contextDlg.Update(msg)
		if !a.contextDlg.IsVisible() {
			a.setFocus(FocusPrompt)
		}
		return a, cmd
	}
	if a.copyDlg.IsVisible() {
		var cmd tea.Cmd
		a.copyDlg, cmd = a.copyDlg.Update(msg)
		if !a.copyDlg.IsVisible() {
			a.setFocus(FocusPrompt)
		}
		return a, cmd
	}
	if a.mcpDlg.IsVisible() {
		var cmd tea.Cmd
		a.mcpDlg, cmd = a.mcpDlg.Update(msg)
		if !a.mcpDlg.IsVisible() {
			a.setFocus(FocusPrompt)
		}
		return a, cmd
	}
	if a.rewindDlg.IsVisible() {
		var cmd tea.Cmd
		a.rewindDlg, cmd = a.rewindDlg.Update(msg)
		if !a.rewindDlg.IsVisible() {
			a.setFocus(FocusPrompt)
		}
		return a, cmd
	}

	// Route to chat search if active.
	if a.chat.IsSearching() {
		var cmd tea.Cmd
		a.chat, cmd = a.chat.Update(msg)
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
	case bootTickMsg:
		cmd := a.welcome.Tick()
		return a, cmd, true
	case AgentListMsg:
		m, cmd := a.handleAgentListMsg(msg)
		if msg.Err == nil {
			m.welcome.MarkDone("agents")
		} else {
			m.welcome.MarkFailed("agents")
		}
		return m, cmd, true
	case CommandListMsg:
		m, cmd := a.handleCommandListMsg(msg)
		return m, cmd, true
	case PluginListMsg:
		if msg.Err == nil {
			a.state.Plugins = msg.Plugins
			a.welcome.MarkDone("plugins")
		} else {
			a.welcome.MarkDone("plugins")
		}
		return a, nil, true
	case ProvidersLoadedMsg:
		m, cmd := a.handleProvidersLoadedMsg(msg)
		if msg.Err == nil {
			m.welcome.MarkDone("providers")
		} else {
			m.welcome.MarkFailed("providers")
		}
		return m, cmd, true
	case SessionsLoadedMsg:
		if msg.Err == nil {
			a.state.Sessions = msg.Sessions
			a.sidebar.SetSessions(msg.Sessions)
			a.welcome.MarkDone("sessions")
		} else {
			a.welcome.MarkFailed("sessions")
		}
		return a, nil, true
	case SessionUpdatedMsg:
		for i, s := range a.state.Sessions {
			if s.ID == msg.SessionID {
				a.state.Sessions[i].Title = msg.Info.Title
				break
			}
		}
		a.sidebar.SetSessions(a.state.Sessions)
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
		prev, hadPrev := a.state.SessionStatus[msg.SessionID]
		a.state.SessionStatus[msg.SessionID] = msg.Status
		var spinCmd tea.Cmd
		if msg.SessionID == a.state.ActiveSession {
			spinCmd = a.status.SetWorking(msg.Status.Working)
			if hadPrev && prev.Working && !msg.Status.Working {
				fmt.Print("\a")
			}
		}
		return a, spinCmd, true
	case MCPStatusMsg:
		a.sidebar.UpdateMCPServer(msg.Server)
		if a.mcpDlg.IsVisible() {
			a.mcpDlg.Refresh(a.sidebar.mcpServers)
		}
		return a, nil, true
	case MCPStatusLoadedMsg:
		if msg.Err == nil {
			a.sidebar.SetMCPServers(msg.Servers)
			a.welcome.MarkDone("mcp")
			if a.mcpDlg.IsVisible() {
				a.mcpDlg.Refresh(msg.Servers)
			}
		} else {
			a.welcome.MarkFailed("mcp")
		}
		return a, nil, true
	case SSEConnectedMsg:
		a.state.Connected = true
		a.welcome.MarkDone("sse")
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
		a.status.SetLeaderPending(false)
		a.whichKey.Hide()
		return a, nil, true
	case SessionErrorMsg:
		cmd := a.toast.Show(msg.Error, true)
		if msg.SessionID == a.state.ActiveSession || msg.SessionID == "" {
			a.status.SetWorking(false)
		}
		a.state.SessionStatus[msg.SessionID] = SessionStatus{Working: false}
		if strings.Contains(strings.ToLower(msg.Error), "no model") {
			a.state.PendingModelDialog = true
			return a, tea.Batch(cmd, func() tea.Msg { return ProvidersRefreshMsg{} }), true
		}
		return a, cmd, true
	case CopiedToClipboardMsg:
		if msg.Err != nil {
			return a, nil, true
		}
		cmd := a.toast.Show(fmt.Sprintf("Copied %d chars", msg.Chars), false)
		return a, cmd, true
	case CodeBlockWrittenMsg:
		if msg.Err != nil {
			cmd := a.toast.Show(fmt.Sprintf("Write failed: %v", msg.Err), true)
			return a, cmd, true
		}
		cmd := a.toast.Show("Written to "+filepath.Base(msg.Path), false)
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
		fmt.Print("\a")
		return a, nil, true
	case PaletteClosedMsg:
		a.setFocus(FocusPrompt)
		return a, nil, true
	case PaletteSelectedMsg:
		a.setFocus(FocusPrompt)
		if a.frecStore != nil && msg.Item.Value != "" {
			a.frecStore.Record("command:" + msg.Item.Value)
		}
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
	case RewindSelectedMsg:
		a.setFocus(FocusPrompt)
		return a, func() tea.Msg { return msg }, true
	case SidebarSessionSelectedMsg:
		slog.Info("sidebar session selected", "sessionID", msg.SessionID)
		return a, func() tea.Msg { return SessionSwitchedMsg(msg) }, true
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
		// Store the full list (may include disabled agents from ListAll).
		a.state.AllAgents = msg.Agents

		// Filter enabled agents for autocomplete and cycle.
		var enabled []api.AgentInfo
		for _, ag := range msg.Agents {
			if !ag.Disabled {
				enabled = append(enabled, ag)
			}
		}
		a.state.Agents = enabled

		names := make([]string, len(enabled))
		agentItems := make([]AutocompleteItem, 0, len(enabled))
		for i, ag := range enabled {
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
		slog.Info("agents loaded", "count", len(enabled), "names", names)

		// Re-populate the agent dialog if it's open (e.g., after a toggle).
		if a.agentDlg.IsVisible() {
			a.agentDlg.Refresh(a.state.AllAgents)
		}
	} else {
		slog.Error("agent list fetch failed", "error", msg.Err)
	}
	return a, nil
}

// handleCommandListMsg processes the command list response.
func (a App) handleCommandListMsg(msg CommandListMsg) (App, tea.Cmd) {
	if msg.Err == nil {
		a.state.Commands = msg.Commands
		clientItems := make([]AutocompleteItem, 0, len(clientCommandDefs))
		clientNames := make(map[string]bool, len(clientCommandDefs))
		for _, def := range clientCommandDefs {
			clientItems = append(clientItems, AutocompleteItem{
				Name:        def.Name,
				Description: def.Description,
			})
			clientNames[def.Name] = true
		}
		items := append([]AutocompleteItem{}, clientItems...)
		for _, cmd := range msg.Commands {
			if strings.HasPrefix(cmd.Description, "Switch to ") {
				continue
			}
			if clientNames[cmd.Name] {
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
		a.modelDlg.SetScopedModels(a.state.ScopedModels)
		if a.state.PendingScopingMode {
			a.state.PendingScopingMode = false
			a.modelDlg.ShowScoping(a.state.Providers)
		} else {
			a.modelDlg.Show(a.state.Providers, a.state.CurrentModel)
		}
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
	l := calculateLayout(a.width, a.height, a.sidebar.IsOpen(), a.status.Height())
	if msg.Y < l.chatHeight && msg.X < l.chatWidth {
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
