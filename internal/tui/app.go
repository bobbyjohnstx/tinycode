package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// App is the root bubbletea model composing all TUI components.
type App struct {
	chat      ChatView
	prompt    PromptInput
	status    StatusBar
	dialog    SessionDialog
	state     *AppState
	focus     FocusTarget
	width     int
	height    int
	serverURL string
	keys      KeyMap
	ready     bool
}

// NewApp creates a new App with the given server URL.
func NewApp(serverURL string) App {
	state := NewAppState()

	return App{
		chat:      NewChatView(80, 24),
		prompt:    NewPromptInput(80),
		status:    NewStatusBar(80),
		dialog:    NewSessionDialog(),
		state:     state,
		focus:     FocusPrompt,
		serverURL: serverURL,
		keys:      DefaultKeyMap(),
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
		// Global keys handled before component dispatch.
		if cmd := a.handleGlobalKey(msg); cmd != nil {
			return a, cmd
		}

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
		}
		return a, nil

	case SessionCreatedMsg:
		a.state.Sessions = append([]SessionInfo{msg.Info}, a.state.Sessions...)
		return a, nil

	case SessionDeletedMsg:
		a.removeSession(msg.SessionID)
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

	l := calculateLayout(a.width, a.height, a.state.SidebarOpen)

	chatView := a.chat.View()
	promptView := a.prompt.View()
	statusView := a.status.View()

	// Overlay dialog if visible.
	if a.dialog.IsVisible() {
		overlay := a.dialog.View()
		return lipgloss.Place(
			a.width, a.height,
			lipgloss.Center, lipgloss.Center,
			overlay,
		)
	}

	return composeView(chatView, promptView, statusView, "", l)
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
	}
	return nil
}

// resize recalculates all component sizes.
func (a *App) resize() {
	l := calculateLayout(a.width, a.height, a.state.SidebarOpen)

	a.chat.SetSize(l.chatWidth, l.chatHeight)
	a.prompt.SetSize(l.promptWidth)
	a.status.SetSize(l.statusWidth)
	a.dialog.SetSize(a.width, a.height)
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
