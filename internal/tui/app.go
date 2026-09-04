package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// App is the root bubbletea model composing all TUI components.
type App struct {
	chat       ChatView
	prompt     PromptInput
	status     StatusBar
	dialog     SessionDialog
	palette    CommandPalette
	agentDlg   AgentDialog
	modelDlg   ModelDialog
	permPrompt PermissionPrompt
	toast      Toast
	sidebar    Sidebar
	leader     LeaderState
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

	return App{
		chat:       NewChatView(80, 24),
		prompt:     NewPromptInput(80),
		status:     NewStatusBar(80),
		dialog:     NewSessionDialog(),
		palette:    NewCommandPalette(),
		agentDlg:   NewAgentDialog(),
		modelDlg:   NewModelDialog(),
		permPrompt: NewPermissionPrompt(),
		toast:      NewToast(DefaultTheme()),
		sidebar:    NewSidebar(),
		leader:     NewLeaderState(keys),
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

		// Global keys handled before component dispatch.
		if cmd := a.handleGlobalKey(msg); cmd != nil {
			return a, cmd
		}

	case LeaderTimeoutMsg:
		a.leader.HandleTimeout()
		return a, nil

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
		}
		return a, nil

	case CommandListMsg:
		if msg.Err == nil {
			a.state.Commands = msg.Commands
			items := make([]AutocompleteItem, len(msg.Commands))
			for i, cmd := range msg.Commands {
				items[i] = AutocompleteItem{
					Name:        cmd.Name,
					Description: cmd.Description,
				}
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

	case PromptSubmittedMsg:
		a.state.SessionStatus[a.state.ActiveSession] = SessionStatus{Working: true}
		a.status.SetWorking(true)
		return a, nil

	case SessionStatusMsg:
		a.state.SessionStatus[msg.SessionID] = msg.Status
		if msg.SessionID == a.state.ActiveSession {
			a.status.SetWorking(msg.Status.Working)
		}
		return a, nil

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
		return a, nil

	case ProvidersLoadedMsg:
		if msg.Err == nil {
			a.state.Providers = msg.Providers
		}
		return a, nil

	case SSEConnectedMsg:
		a.state.Connected = true
		return a, nil

	case SSEDisconnectedMsg:
		a.state.Connected = false
		return a, nil

	case FocusChangedMsg:
		if msg.Target == FocusPalette {
			a.showPalette()
			return a, nil
		}
		a.setFocus(msg.Target)
		return a, nil
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

	// Always forward SSE messages to chat.
	switch msg.(type) {
	case MessagePartDeltaMsg, MessageUpdatedMsg, MessagePartUpdatedMsg, MessagesLoadedMsg:
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

	chatView := a.chat.View()
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
	}
	return nil
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

// showPalette opens the command palette with available commands.
func (a *App) showPalette() {
	items := make([]PaletteItem, 0, len(a.state.Commands))
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
		a.modelDlg.Show(a.state.Providers)
		return nil
	case LeaderActionSessionList:
		a.dialog.Show(a.state.Sessions)
		return nil
	case LeaderActionNewSession:
		// Emit a SessionSwitchedMsg with empty ID to trigger new session creation.
		return func() tea.Msg {
			return SessionSwitchedMsg{SessionID: ""}
		}
	}
	return nil
}
