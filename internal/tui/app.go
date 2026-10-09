package tui

import (
	_ "embed"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bobbyjohnstx/tinycode/internal/config"
	"github.com/bobbyjohnstx/tinycode/internal/frecency"
)

//go:embed help-commands.md
var helpCommandsText string

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
	debugDlg   DebugDialog
	privacyDlg DebugDialog
	contextDlg ContextDialog
	mcpDlg     MCPDialog
	copyDlg    CodeBlockDialog
	rewindDlg  RewindDialog
	permPrompt PermissionPrompt
	toast      Toast
	sidebar    Sidebar
	whichKey   WhichKeyPanel
	leader     LeaderState
	themes     *ThemeRegistry
	frecStore  *frecency.Store
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

	fs := frecency.New(filepath.Join(config.DataDir(), "frecency.json"))
	_ = fs.Load() // ignore error on first run

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
		debugDlg:   NewDebugDialog(),
		privacyDlg: NewDebugDialog(),
		contextDlg: NewContextDialog(),
		mcpDlg:     NewMCPDialog(),
		copyDlg:    NewCodeBlockDialog(),
		rewindDlg:  NewRewindDialog(),
		permPrompt: NewPermissionPrompt(),
		toast:      NewToast(DefaultTheme()),
		sidebar:    NewSidebar(),
		leader:     NewLeaderState(keys),
		themes:     themes,
		frecStore:  fs,
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
		a.welcome.Tick(),
	)
}

// Update implements tea.Model. Dispatches messages to focused component
// and handles global keys. Handler methods are in app_update.go.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if !a.ready {
			a.prompt.ResetStartupGuard()
		}
		a.width = msg.Width
		a.height = msg.Height
		a.ready = true
		// Clear only on real resize — ClearScreen mid-turn can leak CPR (;1R) into the frame.
		return a, a.reflowChromeClear()
	case tea.KeyMsg:
		return a.handleKeyMsg(msg)
	case tea.MouseMsg:
		return a.handleMouseMsg(msg)
	case MessagePartDeltaMsg, MessageUpdatedMsg, MessagePartUpdatedMsg, MessagesLoadedMsg, SubagentCompletedMsg:
		return a.forwardSSEMessages(msg)
	case goalFadeMsg:
		a.status, _ = a.status.Update(msg)
		return a, a.reflowChrome()
	}

	if model, cmd, ok := a.handleStateMsg(msg); ok {
		return model, cmd
	}
	if model, cmd, ok := a.handleNotificationMsg(msg); ok {
		return model, cmd
	}
	if model, cmd, ok := a.handleDialogMsg(msg); ok {
		return model, cmd
	}

	slog.Debug("unhandled msg in App.Update", "type", fmt.Sprintf("%T", msg))
	return a.dispatchToFocused(msg)
}

// View implements tea.Model. Composes all component views.
func (a App) View() string {
	if !a.ready {
		return "Loading..."
	}

	l := calculateLayout(a.width, a.height, a.sidebar.IsOpen(), a.prompt.Height(), a.status.Height())

	// Show welcome screen when no messages exist yet.
	var chatView string
	if a.hasMessages() {
		chatView = a.chat.View()
	} else {
		modName, provName := lookupModelDisplay(
			a.state.Providers,
			a.state.CurrentModel.ProviderID,
			a.state.CurrentModel.ModelID,
		)
		chatView = a.welcome.View(l.chatWidth, l.chatHeight, provName, modName,
			len(a.state.Sessions), len(a.state.Agents), len(a.state.Plugins), len(a.sidebar.mcpServers))
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
	if a.debugDlg.IsVisible() {
		return a.debugDlg.View()
	}
	if a.privacyDlg.IsVisible() {
		return a.privacyDlg.View()
	}
	if a.contextDlg.IsVisible() {
		return a.contextDlg.View()
	}
	if a.copyDlg.IsVisible() {
		return a.copyDlg.View()
	}
	if a.mcpDlg.IsVisible() {
		return a.mcpDlg.View()
	}
	if a.rewindDlg.IsVisible() {
		return a.rewindDlg.View()
	}
	if a.dialog.IsVisible() {
		return a.dialog.View()
	}

	sidebarView := ""
	if a.sidebar.IsOpen() {
		sidebarView = a.sidebar.View()
	}

	base := composeView(chatView, promptView, statusView, sidebarView, l)

	// Slash/file popovers overlay the chat above the prompt (not inside prompt.View).
	if pop := a.prompt.PopoverView(); pop != "" {
		leftInset := 0
		if l.hasSidebar {
			leftInset = l.sidebarWidth
		}
		base = placePromptPopover(base, pop, a.width, a.height, l.statusHeight, l.promptHeight, leftInset)
	}

	// Overlay which-key panel when leader is pending.
	if a.whichKey.IsVisible() {
		panel := a.whichKey.View()
		if panel != "" {
			base = placeWhichKey(base, panel, a.width, a.height)
		}
	}

	// Overlay toast onto the last chrome row (never append — that overflows the terminal).
	if a.toast.IsVisible() {
		toastLine := toastOverlay(a.toast.View(), a.width)
		if toastLine != "" {
			base = overlayLastLine(base, toastLine)
		}
	}

	return fitTerminal(base, a.width, a.height)
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
	case "ctrl+k":
		a.showUnifiedPalette()
		return nil
	case "ctrl+f":
		if a.hasMessages() {
			a.chat.ActivateSearch()
		}
		return func() tea.Msg { return nil }
	case "esc":
		if a.isSessionWorking() {
			return func() tea.Msg { return AbortRequestMsg{} }
		}
		if a.toast.IsVisible() {
			return a.toast.Dismiss()
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
	l := calculateLayout(a.width, a.height, a.sidebar.IsOpen(), a.prompt.Height(), a.status.Height())

	a.chat.SetSize(l.chatWidth, l.chatHeight)
	a.prompt.SetSize(l.promptWidth)
	a.status.SetSize(l.statusWidth)
	a.dialog.SetSize(a.width, a.height)
	a.palette.SetSize(a.width, a.height)
	a.agentDlg.SetSize(a.width, a.height)
	a.modelDlg.SetSize(a.width, a.height)
	a.themeDlg.SetSize(a.width, a.height)
	a.debugDlg.SetSize(a.width, a.height)
	a.privacyDlg.SetSize(a.width, a.height)
	a.contextDlg.SetSize(a.width, a.height)
	a.mcpDlg.SetSize(a.width, a.height)
	a.copyDlg.SetSize(a.width, a.height)
	a.rewindDlg.SetSize(a.width, a.height)
	a.permPrompt.SetSize(a.width, a.height)
	a.toast.SetSize(a.width)
	a.sidebar.SetSize(l.sidebarWidth, l.chatHeight)
}

// reflowChrome resizes after status/prompt height changes.
// Prefer this during a turn — fitTerminal already clamps the frame.
func (a *App) reflowChrome() tea.Cmd {
	a.resize()
	return nil
}

// reflowChromeClear resizes and clears the alt screen. Use for window resize
// and working→idle (to wipe residual ghosts), not when a turn starts.
func (a *App) reflowChromeClear() tea.Cmd {
	a.resize()
	a.prompt.ArmNoiseGuard(2 * time.Second)
	return tea.ClearScreen
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

// syncPromptMetadata updates the prompt's agent/model/provider display,
// status bar, and CurrentAgent from the active session info.
func (a *App) syncPromptMetadata() {
	sid := a.state.ActiveSession
	if sid == "" {
		a.state.CurrentAgent = "build"
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
			a.state.CurrentAgent = agent
			a.prompt.SetMetadata(agent, s.ModelID, s.ProviderID)
			a.status.SetModel(s.ModelID, s.ProviderID)
			a.status.SetAgent(agent)
			return
		}
	}
	a.state.CurrentAgent = "build"
	a.prompt.SetMetadata("build", "", "")
	a.status.SetModel("", "")
	a.status.SetAgent("build")
}

// updateSidebarContext recomputes token stats from chat messages and pushes them to the sidebar.
func (a *App) updateSidebarContext() {
	msgs := a.chat.Messages()
	var lastTokens int
	var totalCost float64
	var totalInput, totalOutput int
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Info.Role == "assistant" {
			t := m.Info.Tokens
			total := t.Input + t.Output
			if total > 0 && lastTokens == 0 {
				lastTokens = total
			}
		}
		totalInput += m.Info.Tokens.Input
		totalOutput += m.Info.Tokens.Output
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
		InputTokens:  totalInput,
		OutputTokens: totalOutput,
		ContextLimit: contextLimit,
		Percent:      pct,
		Cost:         totalCost,
	})
	a.status.SetContextPercent(pct)
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
	clientNames := make(map[string]bool, len(clientCommandDefs))
	var items []PaletteItem
	for _, def := range clientCommandDefs {
		clientNames[def.Name] = true
		if def.InPalette {
			items = append(items, PaletteItem{
				Label:       def.Name,
				Description: def.Description,
				Value:       def.Name,
			})
		}
	}
	for _, cmd := range a.state.Commands {
		if clientNames[cmd.Name] {
			continue
		}
		items = append(items, PaletteItem{
			Label:       cmd.Name,
			Description: cmd.Description,
			Value:       cmd.Name,
		})
	}
	// Sort command items by frecency score before appending informational keybindings.
	if a.frecStore != nil {
		sort.SliceStable(items, func(i, j int) bool {
			si := a.frecStore.Score("command:" + items[i].Value)
			sj := a.frecStore.Score("command:" + items[j].Value)
			return si > sj
		})
	}

	items = append(items, keybindingPaletteItems(a.keys)...)
	a.palette.Show(items)
	a.focus = FocusPalette
}

// showUnifiedPalette opens the unified picker with commands, files, and sessions.
func (a *App) showUnifiedPalette() {
	// Build command items (reuse showPalette logic).
	clientNames := make(map[string]bool, len(clientCommandDefs))
	var commands []PaletteItem
	for _, def := range clientCommandDefs {
		clientNames[def.Name] = true
		if def.InPalette {
			commands = append(commands, PaletteItem{
				Label:       def.Name,
				Description: def.Description,
				Value:       def.Name,
				Category:    "command",
			})
		}
	}
	for _, cmd := range a.state.Commands {
		if clientNames[cmd.Name] {
			continue
		}
		commands = append(commands, PaletteItem{
			Label:       cmd.Name,
			Description: cmd.Description,
			Value:       cmd.Name,
			Category:    "command",
		})
	}
	if a.frecStore != nil {
		sort.SliceStable(commands, func(i, j int) bool {
			si := a.frecStore.Score("command:" + commands[i].Value)
			sj := a.frecStore.Score("command:" + commands[j].Value)
			return si > sj
		})
	}

	files := buildFilePaletteItems(a.status.Cwd())
	sessions := buildSessionPaletteItems(a.state.Sessions, a.state.ActiveSession)

	a.palette.ShowUnified(commands, files, sessions)
	a.focus = FocusPalette
}
