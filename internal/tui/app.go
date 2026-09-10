package tui

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// App is the root bubbletea model composing all TUI components.
type App struct {
	chat       ChatView
	prompt     PromptInput
	status     StatusBar
	welcome    WelcomeView
	dialog     SessionDialog
	palette    CommandPalette
	agentDlg   AgentDialog
	modelDlg   ModelDialog
	themeDlg   ThemeDialog
	permPrompt PermissionPrompt
	toast      Toast
	sidebar    Sidebar
	leader     LeaderState
	themes     *ThemeRegistry
	state      *AppState
	focus      FocusTarget
	width      int
	height     int
	serverURL  string
	keys       KeyMap
	ready      bool
}

// NewApp creates a new App with the given server URL.
func NewApp(serverURL string) App {
	state := NewAppState()
	keys := DefaultKeyMap()

	themes := NewThemeRegistry()
	themes.LoadEmbedded()

	return App{
		chat:       NewChatView(80, 24),
		prompt:     NewPromptInput(80),
		status:     NewStatusBar(80),
		welcome:    NewWelcomeView(),
		dialog:     NewSessionDialog(),
		palette:    NewCommandPalette(),
		agentDlg:   NewAgentDialog(),
		modelDlg:   NewModelDialog(),
		themeDlg:   NewThemeDialog(),
		permPrompt: NewPermissionPrompt(),
		toast:      NewToast(DefaultTheme()),
		sidebar:    NewSidebar(),
		leader:     NewLeaderState(keys),
		themes:     themes,
		state:      state,
		focus:      FocusPrompt,
		serverURL:  serverURL,
		keys:       keys,
	}
}

// Init implements tea.Model. Returns initial commands to fetch data.
func (a App) Init() tea.Cmd {
	return tea.Batch(
		a.prompt.Init(),
		a.status.Init(),
	)
}

// Update implements tea.Model. Dispatches messages to focused component
// and handles global keys.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if !a.ready {
			a.prompt.ResetStartupGuard()
		}
		a.width = msg.Width
		a.height = msg.Height
		a.resize()
		a.ready = true
		return a, nil

	case tea.KeyMsg:
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
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			return a, tea.Batch(cmds...)
		}
		if a.palette.IsVisible() {
			var cmd tea.Cmd
			a.palette, cmd = a.palette.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			return a, tea.Batch(cmds...)
		}
		if a.agentDlg.IsVisible() {
			var cmd tea.Cmd
			a.agentDlg, cmd = a.agentDlg.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			return a, tea.Batch(cmds...)
		}
		if a.modelDlg.IsVisible() {
			var cmd tea.Cmd
			a.modelDlg, cmd = a.modelDlg.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			return a, tea.Batch(cmds...)
		}
		if a.themeDlg.IsVisible() {
			var cmd tea.Cmd
			a.themeDlg, cmd = a.themeDlg.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			return a, tea.Batch(cmds...)
		}

		// Global keys handled before component dispatch.
		if cmd := a.handleGlobalKey(msg); cmd != nil {
			return a, cmd
		}

	case LeaderTimeoutMsg:
		a.leader.HandleTimeout()
		return a, nil

	case SessionErrorMsg:
		cmd := a.toast.Show(msg.Error, true)
		if msg.SessionID == a.state.ActiveSession || msg.SessionID == "" {
			a.status.SetWorking(false)
		}
		a.state.SessionStatus[msg.SessionID] = SessionStatus{Working: false}
		return a, cmd


	case CopiedToClipboardMsg:
		if msg.Err != nil {
			return a, nil
		}
		cmd := a.toast.Show(fmt.Sprintf("Copied %d chars", msg.Chars), false)
		return a, cmd

	case ExportSessionMsg:
		if msg.Err != nil {
			cmd := a.toast.Show(fmt.Sprintf("Export failed: %v", msg.Err), true)
			return a, cmd
		}
		cmd := a.toast.Show("Exported to "+filepath.Base(msg.Path), false)
		return a, cmd

	case ToastMsg:
		cmd := a.toast.Show(msg.Text, msg.IsError)
		return a, cmd

	case ToastExpiredMsg:
		var cmd tea.Cmd
		a.toast, cmd = a.toast.Update(msg)
		return a, cmd

	case AgentListMsg:
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

	case CommandListMsg:
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

	case PermissionRequestedMsg:
		a.permPrompt.Show(msg.Request)
		return a, nil

	case PaletteClosedMsg:
		a.setFocus(FocusPrompt)
		return a, nil

	case PaletteSelectedMsg:
		a.setFocus(FocusPrompt)
		if cmd, handled := a.handleClientCommand(msg.Item.Value); handled {
			return a, cmd
		}
		return a, nil

	case AgentSelectedMsg:
		a.state.CurrentAgent = msg.Agent
		a.prompt.SetMetadata(msg.Agent, a.state.CurrentModel.ModelID, a.state.CurrentModel.ProviderID)
		a.status.SetAgent(msg.Agent)
		a.setFocus(FocusPrompt)
		return a, nil

	case ThemePreviewMsg:
		if theme := a.themes.Get(msg.ThemeID); theme != nil {
			ApplyColorTheme(theme)
		}
		return a, nil

	case ThemeRevertMsg:
		a.setFocus(FocusPrompt)
		if theme := a.themes.Get(msg.ThemeID); theme != nil {
			ApplyColorTheme(theme)
		}
		return a, nil

	case ThemeSelectedMsg:
		a.setFocus(FocusPrompt)
		a.state.CurrentTheme = msg.ThemeID
		if theme := a.themes.Get(msg.ThemeID); theme != nil {
			ApplyColorTheme(theme)
			cmd := a.toast.Show("Theme: "+theme.Name, false)
			return a, cmd
		}
		return a, nil

	case ModelSelectedMsg:
		a.setFocus(FocusPrompt)
		a.state.CurrentModel = msg.Selection
		for _, p := range a.state.Providers {
			if p.ID == msg.Selection.ProviderID {
				for _, m := range p.Models {
					if m.ID == msg.Selection.ModelID {
						a.prompt.SetMetadata(a.state.CurrentAgent, m.Name, p.Name)
						a.status.SetModel(m.Name, p.Name)
						a.updateSidebarContext()
						return a, nil
					}
				}
			}
		}
		a.prompt.SetMetadata(a.state.CurrentAgent, msg.Selection.ModelID, msg.Selection.ProviderID)
		a.status.SetModel(msg.Selection.ModelID, msg.Selection.ProviderID)
		a.updateSidebarContext()
		return a, nil

	case PromptSubmittedMsg:
		a.state.SessionStatus[a.state.ActiveSession] = SessionStatus{Working: true}
		spinCmd := a.status.SetWorking(true)
		return a, spinCmd

	case SessionStatusMsg:
		a.state.SessionStatus[msg.SessionID] = msg.Status
		var spinCmd tea.Cmd
		if msg.SessionID == a.state.ActiveSession {
			spinCmd = a.status.SetWorking(msg.Status.Working)
		}
		return a, spinCmd

	case SessionsLoadedMsg:
		if msg.Err == nil {
			a.state.Sessions = msg.Sessions
			a.sidebar.SetSessions(msg.Sessions)
		}
		return a, nil

	case SessionCreatedMsg:
		a.state.Sessions = append([]SessionInfo{msg.Info}, a.state.Sessions...)
		a.sidebar.SetSessions(a.state.Sessions)
		return a, nil

	case SessionDeletedMsg:
		a.removeSession(msg.SessionID)
		a.sidebar.SetSessions(a.state.Sessions)
		return a, nil

	case SessionSwitchedMsg:
		a.state.ActiveSession = msg.SessionID
		a.state.Messages[msg.SessionID] = nil // clear, will reload
		a.syncPromptMetadata()
		return a, nil

	case ProvidersLoadedMsg:
		if msg.Err == nil {
			a.state.Providers = msg.Providers
		}
		if a.state.CurrentModel.ModelID == "" && msg.DefaultProvider != "" && msg.DefaultModel != "" {
			a.state.CurrentModel = ModelSelection{
				ProviderID: msg.DefaultProvider,
				ModelID:    msg.DefaultModel,
			}
			modelName := msg.DefaultModel
			providerName := msg.DefaultProvider
			for _, p := range a.state.Providers {
				if p.ID == msg.DefaultProvider {
					providerName = p.Name
					for _, m := range p.Models {
						if m.ID == msg.DefaultModel {
							modelName = m.Name
							break
						}
					}
					break
				}
			}
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

	case SSEConnectedMsg:
		a.state.Connected = true
		return a, nil

	case SSEDisconnectedMsg:
		a.state.Connected = false
		return a, nil

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
		}

	case SidebarSessionSelectedMsg:
		slog.Info("sidebar session selected", "sessionID", msg.SessionID)
		return a, func() tea.Msg {
			return SessionSwitchedMsg{SessionID: msg.SessionID}
		}

	case FocusChangedMsg:
		if msg.Target == FocusPalette {
			a.showPalette()
			return a, nil
		}
		a.setFocus(msg.Target)
		return a, nil

	case MessagePartDeltaMsg, MessageUpdatedMsg, MessagePartUpdatedMsg, MessagesLoadedMsg:
		// SSE messages always go to chat view.
		var cmd tea.Cmd
		a.chat, cmd = a.chat.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		// Also forward to status bar for spinner state.
		var statusCmd tea.Cmd
		a.status, statusCmd = a.status.Update(msg)
		if statusCmd != nil {
			cmds = append(cmds, statusCmd)
		}
		a.updateSidebarContext()
		return a, tea.Batch(cmds...)

	case tea.MouseMsg:
		l := calculateLayout(a.width, a.height, a.sidebar.IsOpen())
		if msg.Y < l.chatHeight {
			var cmd tea.Cmd
			a.chat, cmd = a.chat.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			return a, tea.Batch(cmds...)
		}

	default:
		slog.Debug("unhandled msg in App.Update", "type", fmt.Sprintf("%T", msg))
	}

	// Dispatch to focused component or dialog overlay.
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
		// ChatView handles scroll keys when not in prompt focus.
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

// View implements tea.Model. Composes all component views.
func (a App) View() string {
	if !a.ready {
		return "Loading..."
	}

	l := calculateLayout(a.width, a.height, a.sidebar.IsOpen())

	// Show welcome screen when no messages exist yet.
	var chatView string
	if a.hasMessages() {
		chatView = a.chat.View()
	} else {
		chatView = a.welcome.View(l.chatWidth, l.chatHeight)
	}
	promptView := a.prompt.View()
	statusView := a.status.View()

	// Render highest-priority overlay if one is visible.
	if a.permPrompt.IsVisible() {
		return a.permPrompt.View()
	}
	if a.palette.IsVisible() {
		return a.palette.View()
	}
	if a.agentDlg.IsVisible() {
		return a.agentDlg.View()
	}
	if a.modelDlg.IsVisible() {
		return a.modelDlg.View()
	}
	if a.themeDlg.IsVisible() {
		return a.themeDlg.View()
	}
	if a.dialog.IsVisible() {
		return a.dialog.View()
	}

	sidebarView := ""
	if a.sidebar.IsOpen() {
		sidebarView = a.sidebar.View()
	}

	base := composeView(chatView, promptView, statusView, sidebarView, l)

	// Overlay toast at the bottom if visible.
	if a.toast.IsVisible() {
		toastLine := toastOverlay(a.toast.View(), a.width)
		if toastLine != "" {
			return base + "\n" + toastLine
		}
	}

	return base
}

// handleGlobalKey handles keys that work regardless of focus.
func (a *App) handleGlobalKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c":
		if a.prompt.Value() == "" {
			return tea.Quit
		}
		a.prompt.Reset()
		return nil
	case "ctrl+d":
		return tea.Quit
	case "ctrl+p":
		a.showPalette()
		return nil
	case "esc":
		if a.isSessionWorking() {
			return func() tea.Msg { return AbortRequestMsg{} }
		}
		return nil
	}
	return nil
}

func (a *App) isSessionWorking() bool {
	if a.state.ActiveSession == "" {
		return false
	}
	status, ok := a.state.SessionStatus[a.state.ActiveSession]
	return ok && status.Working
}

// resize recalculates all component sizes.
func (a *App) resize() {
	l := calculateLayout(a.width, a.height, a.sidebar.IsOpen())

	a.chat.SetSize(l.chatWidth, l.chatHeight)
	a.prompt.SetSize(l.promptWidth)
	a.status.SetSize(l.statusWidth)
	a.dialog.SetSize(a.width, a.height)
	a.palette.SetSize(a.width, a.height)
	a.agentDlg.SetSize(a.width, a.height)
	a.modelDlg.SetSize(a.width, a.height)
	a.themeDlg.SetSize(a.width, a.height)
	a.permPrompt.SetSize(a.width, a.height)
	a.toast.SetSize(a.width)
	a.sidebar.SetSize(l.sidebarWidth, l.chatHeight)
}

// setFocus changes the focused component.
func (a *App) setFocus(target FocusTarget) {
	a.focus = target
	switch target {
	case FocusPrompt:
		a.prompt.Focus()
	default:
		a.prompt.Blur()
	}
}

// removeSession removes a session from the state by ID.
func (a *App) removeSession(id string) {
	sessions := make([]SessionInfo, 0, len(a.state.Sessions))
	for _, s := range a.state.Sessions {
		if s.ID != id {
			sessions = append(sessions, s)
		}
	}
	a.state.Sessions = sessions
	delete(a.state.Messages, id)
	delete(a.state.SessionStatus, id)
}

// syncPromptMetadata updates the prompt's agent/model/provider display
// and the status bar from the active session info.
func (a *App) syncPromptMetadata() {
	sid := a.state.ActiveSession
	if sid == "" {
		a.prompt.SetMetadata("build", "", "")
		a.status.SetModel("", "")
		a.status.SetAgent("build")
		return
	}
	for _, s := range a.state.Sessions {
		if s.ID == sid {
			agent := s.Agent
			if agent == "" {
				agent = "build"
			}
			a.prompt.SetMetadata(agent, s.ModelID, s.ProviderID)
			a.status.SetModel(s.ModelID, s.ProviderID)
			a.status.SetAgent(agent)
			return
		}
	}
	a.prompt.SetMetadata("build", "", "")
	a.status.SetModel("", "")
	a.status.SetAgent("build")
}

// updateSidebarContext recomputes token stats from chat messages and pushes them to the sidebar.
func (a *App) updateSidebarContext() {
	msgs := a.chat.Messages()
	var lastTokens int
	var totalCost float64
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Info.Role == "assistant" {
			t := m.Info.Tokens
			total := t.Input + t.Output
			if total > 0 && lastTokens == 0 {
				lastTokens = total
			}
		}
		totalCost += m.Info.Cost
	}

	contextLimit := 0
	for _, p := range a.state.Providers {
		for _, m := range p.Models {
			if m.ID == a.state.CurrentModel.ModelID && m.ProviderID == a.state.CurrentModel.ProviderID {
				contextLimit = m.ContextLimit
				break
			}
		}
		if contextLimit > 0 {
			break
		}
	}

	pct := 0
	if contextLimit > 0 && lastTokens > 0 {
		pct = lastTokens * 100 / contextLimit
	}

	a.sidebar.SetContext(ContextStats{
		Tokens:       lastTokens,
		ContextLimit: contextLimit,
		Percent:      pct,
		Cost:         totalCost,
	})
}

// activeSessionInfo returns the SessionInfo for the currently active session, or nil.
func (a *App) activeSessionInfo() *SessionInfo {
	if a.state.ActiveSession == "" {
		return nil
	}
	for _, s := range a.state.Sessions {
		if s.ID == a.state.ActiveSession {
			return &s
		}
	}
	return nil
}

// hasMessages returns true if the active session has any messages.
func (a *App) hasMessages() bool {
	return a.state.ActiveSession != "" && a.chat.HasMessages()
}

// showPalette opens the command palette with available commands.
func (a *App) showPalette() {
	items := []PaletteItem{
		{Label: "connect", Description: "Select provider and model", Value: "connect"},
		{Label: "export", Description: "Export session as Markdown", Value: "export"},
		{Label: "theme", Description: "Change color theme", Value: "theme"},
	}
	for _, cmd := range a.state.Commands {
		items = append(items, PaletteItem{
			Label:       cmd.Name,
			Description: cmd.Description,
			Value:       cmd.Name,
		})
	}
	a.palette.Show(items)
	a.focus = FocusPalette
}

// handleClientCommand handles a client-side slash command by name.
// Returns (cmd, true) if the command was handled, (nil, false) otherwise.
func (a *App) handleClientCommand(name string) (tea.Cmd, bool) {
	switch name {
	case "exit":
		return tea.Quit, true
	case "connect":
		a.state.PendingModelDialog = true
		return func() tea.Msg { return ProvidersRefreshMsg{} }, true
	case "theme":
		a.themeDlg.Show(a.themes.List(), a.state.CurrentTheme)
		a.setFocus(FocusDialog)
		return nil, true
	case "export":
		session := a.activeSessionInfo()
		if session == nil {
			return a.toast.Show("No active session to export", true), true
		}
		msgs := a.chat.Messages()
		return exportSession(msgs, *session, a.status.Cwd()), true
	}
	return nil, false
}

// dispatchLeaderAction handles a resolved leader key action.
func (a *App) dispatchLeaderAction(action string) tea.Cmd {
	switch action {
	case LeaderActionSidebar:
		a.sidebar.Toggle()
		a.state.SidebarOpen = a.sidebar.IsOpen()
		a.resize()
		return nil
	case LeaderActionAgentList:
		a.agentDlg.Show(a.state.Agents)
		return nil
	case LeaderActionModelList:
		a.state.PendingModelDialog = true
		return func() tea.Msg { return ProvidersRefreshMsg{} }
	case LeaderActionSessionList:
		a.dialog.Show(a.state.Sessions)
		return nil
	case LeaderActionNewSession:
		return func() tea.Msg {
			return SessionSwitchedMsg{SessionID: ""}
		}
	case LeaderActionExportSession:
		if cmd, handled := a.handleClientCommand("export"); handled {
			return cmd
		}
	}
	return nil
}
