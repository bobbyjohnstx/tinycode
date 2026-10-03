package tui

import (
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bobbyjohnstx/tinycode/internal/config"
	"github.com/bobbyjohnstx/tinycode/internal/frecency"
	"github.com/bobbyjohnstx/tinycode/internal/storage"
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
		a.resize()
		a.ready = true
		return a, nil
	case tea.KeyMsg:
		return a.handleKeyMsg(msg)
	case tea.MouseMsg:
		return a.handleMouseMsg(msg)
	case MessagePartDeltaMsg, MessageUpdatedMsg, MessagePartUpdatedMsg, MessagesLoadedMsg, SubagentCompletedMsg:
		return a.forwardSSEMessages(msg)
	case goalFadeMsg:
		a.status, _ = a.status.Update(msg)
		a.resize()
		return a, nil
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

	l := calculateLayout(a.width, a.height, a.sidebar.IsOpen(), a.status.Height())

	// Show welcome screen when no messages exist yet.
	var chatView string
	if a.hasMessages() {
		chatView = a.chat.View()
	} else {
		provName, modName := lookupModelDisplay(
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

	// Overlay which-key panel when leader is pending.
	if a.whichKey.IsVisible() {
		panel := a.whichKey.View()
		if panel != "" {
			base = placeWhichKey(base, panel, a.width, a.height)
		}
	}

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
	l := calculateLayout(a.width, a.height, a.sidebar.IsOpen(), a.status.Height())

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
	clientNames := map[string]bool{
		"branch":        true,
		"connect":       true,
		"compact":       true,
		"copy":          true,
		"diff":          true,
		"export":        true,
		"export-html":   true,
		"theme":         true,
		"help":          true,
		"rename":        true,
		"auto-approve":  true,
		"editor":        true,
		"shell":         true,
		"debug":         true,
		"thinking":      true,
		"mcp":           true,
		"paste-image":   true,
		"scoped-models": true,
		"archive":       true,
		"btw":           true,
		"goal":          true,
		"rewind":        true,
		"hooks":         true,
		"context":       true,
	}
	items := []PaletteItem{
		{Label: "branch", Description: "Branch conversation to try a different approach", Value: "branch"},
		{Label: "compact", Description: "Compact context (summarize session)", Value: "compact"},
		{Label: "connect", Description: "Select provider and model", Value: "connect"},
		{Label: "copy", Description: "Copy response to clipboard (/copy N for Nth)", Value: "copy"},
		{Label: "diff", Description: "Show uncommitted changes", Value: "diff"},
		{Label: "export", Description: "Export session as Markdown", Value: "export"},
		{Label: "export-html", Description: "Export session as HTML", Value: "export-html"},
		{Label: "theme", Description: "Change color theme", Value: "theme"},
		{Label: "rename", Description: "Rename current session", Value: "rename"},
		{Label: "help", Description: "Show keybindings and commands", Value: "help"},
		{Label: "auto-approve", Description: "Toggle auto-approve for session", Value: "auto-approve"},
		{Label: "editor", Description: "Open prompt or file in $EDITOR (/editor @file)", Value: "editor"},
		{Label: "shell", Description: "Open interactive shell session", Value: "shell"},
		{Label: "debug", Description: "Show diagnostics for bug reports", Value: "debug"},
		{Label: "mcp", Description: "Manage MCP servers", Value: "mcp"},
		{Label: "thinking", Description: "Set reasoning level (off/low/medium/high/max)", Value: "thinking"},
		{Label: "paste-image", Description: "Paste image from clipboard", Value: "paste-image"},
		{Label: "scoped-models", Description: "Toggle model scoping (favorites)", Value: "scoped-models"},
		{Label: "archive", Description: "Archive current session", Value: "archive"},
		{Label: "btw", Description: "Side question without polluting context", Value: "btw"},
		{Label: "goal", Description: "Autonomous execution until condition met", Value: "goal"},
		{Label: "rewind", Description: "Rewind conversation to a previous turn", Value: "rewind"},
		{Label: "hooks", Description: "Show configured hooks (plugin and shell)", Value: "hooks"},
		{Label: "context", Description: "Show context window usage breakdown", Value: "context"},
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

// handleClientCommand handles a client-side slash command by name.
// Returns (cmd, true) if the command was handled, (nil, false) otherwise.
func (a *App) handleClientCommand(name string) (tea.Cmd, bool) {
	switch name {
	case "exit":
		return tea.Quit, true
	case "compact":
		if a.state.ActiveSession == "" {
			return a.toast.Show("No active session to compact", true), true
		}
		return func() tea.Msg { return CompactRequestMsg{} }, true
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
	case "export-html":
		session := a.activeSessionInfo()
		if session == nil {
			return a.toast.Show("No active session to export", true), true
		}
		msgs := a.chat.Messages()
		return exportSessionHTML(msgs, *session, a.status.Cwd()), true
	case "copy":
		return a.handleCopyCommand(1), true
	case "auto-approve":
		a.state.AutoApprove = !a.state.AutoApprove
		label := "disabled"
		if a.state.AutoApprove {
			label = "enabled"
		}
		return a.toast.Show("Auto-approve "+label+" for this session", false), true
	case "help":
		a.debugDlg.Show(helpCommandsText)
		a.setFocus(FocusDialog)
		return nil, true
	case "editor":
		content := a.prompt.Value()
		return func() tea.Msg { return EditorRequestMsg{Content: content} }, true
	case "shell":
		return func() tea.Msg { return ShellSessionRequestMsg{} }, true
	case "paste-image":
		return readClipboardImage(), true
	case "debug":
		info := a.buildDebugInfo()
		a.debugDlg.Show(info)
		a.setFocus(FocusDialog)
		return nil, true
	case "privacy":
		info := a.buildPrivacyInfo()
		a.privacyDlg.Show(info)
		a.setFocus(FocusDialog)
		return nil, true
	case "mcp":
		a.mcpDlg.Show(a.sidebar.mcpServers)
		a.setFocus(FocusDialog)
		return nil, true
	case "scoped-models":
		a.state.PendingModelDialog = true
		a.state.PendingScopingMode = true
		return func() tea.Msg { return ProvidersRefreshMsg{} }, true
	case "undo":
		return func() tea.Msg { return RevertRequestMsg{} }, true
	case "redo":
		return func() tea.Msg { return UnrevertRequestMsg{} }, true
	case "diff":
		dir := a.status.Cwd()
		return func() tea.Msg { return DiffRequestMsg{Dir: dir} }, true
	case "archive":
		if a.state.ActiveSession == "" {
			return a.toast.Show("No active session to archive", true), true
		}
		return func() tea.Msg { return ArchiveRequestMsg{} }, true
	case "branch":
		if a.state.ActiveSession == "" {
			return a.toast.Show("No active session to branch", true), true
		}
		return func() tea.Msg { return BranchRequestMsg{} }, true
	case "btw":
		return a.toast.Show("Usage: /btw <question> — ask without polluting context", false), true
	case "goal":
		return a.toast.Show("Usage: /goal <condition> — autonomous execution until condition met", false), true
	case "rewind":
		if a.state.ActiveSession == "" {
			return a.toast.Show("No active session to rewind", true), true
		}
		turns := ExtractTurns(a.chat.Messages())
		if len(turns) == 0 {
			return a.toast.Show("No turns to rewind to", true), true
		}
		a.rewindDlg.Show(turns)
		a.setFocus(FocusDialog)
		return nil, true
	case "hooks":
		info := a.buildHooksInfo()
		a.debugDlg.Show(info)
		a.setFocus(FocusDialog)
		return nil, true
	case "context":
		contextLimit := a.sidebar.context.ContextLimit
		apiTokens := a.sidebar.context.Tokens
		breakdown := AnalyzeContext(a.chat.Messages(), contextLimit, apiTokens)
		a.contextDlg.Show(breakdown)
		a.setFocus(FocusDialog)
		return nil, true
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
		a.agentDlg.Show(a.state.AllAgents)
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
	case LeaderActionCopyResponse:
		if cmd, handled := a.handleClientCommand("copy"); handled {
			return cmd
		}
	case LeaderActionUndo:
		return func() tea.Msg { return RevertRequestMsg{} }
	case LeaderActionRedo:
		return func() tea.Msg { return UnrevertRequestMsg{} }
	case LeaderActionEditor:
		content := a.prompt.Value()
		return func() tea.Msg { return EditorRequestMsg{Content: content} }
	case LeaderActionDiffView:
		return func() tea.Msg { return DiffRequestMsg{Dir: a.status.Cwd()} }
	case LeaderActionThemePicker:
		a.themeDlg.Show(a.themes.List(), a.state.CurrentTheme)
		a.setFocus(FocusDialog)
		return nil
	case LeaderActionMCPList:
		a.mcpDlg.Show(a.sidebar.mcpServers)
		a.setFocus(FocusDialog)
		return nil
	}
	return nil
}

// handleCopyCommand handles /copy with an optional Nth-response index.
// If the selected response has code blocks, a picker dialog is shown.
// Otherwise the full text is copied to clipboard.
func (a *App) handleCopyCommand(n int) tea.Cmd {
	text := nthAssistantText(a.chat.Messages(), n)
	if text == "" {
		return a.toast.Show("No assistant response to copy", true)
	}
	blocks := extractCodeBlocks(text)
	if len(blocks) > 0 {
		a.copyDlg.Show(blocks, text, a.status.Cwd())
		a.setFocus(FocusDialog)
		return nil
	}
	return copyToClipboard(text)
}

// lastAssistantText returns the concatenated text parts from the last assistant
// message, or "" if none exists.
func lastAssistantText(messages []MessageView) string {
	return nthAssistantText(messages, 1)
}

// buildDebugInfo collects environment and state diagnostics for bug reports.
func (a *App) buildDebugInfo() string {
	var sb strings.Builder

	// Version
	version := a.sidebar.version
	if version == "" {
		version = "dev"
	}
	fmt.Fprintf(&sb, "Version:    %s\n", version)
	fmt.Fprintf(&sb, "Go:         %s\n", runtime.Version())
	fmt.Fprintf(&sb, "OS/Arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&sb, "Terminal:   %s\n", os.Getenv("TERM"))

	// Model and provider
	provName, modName := lookupModelDisplay(
		a.state.Providers,
		a.state.CurrentModel.ProviderID,
		a.state.CurrentModel.ModelID,
	)
	fmt.Fprintf(&sb, "Model:      %s (%s)\n", modName, provName)

	// Agent
	agent := a.state.CurrentAgent
	if agent == "" {
		agent = "(default)"
	}
	fmt.Fprintf(&sb, "Agent:      %s\n", agent)

	// Paths
	fmt.Fprintf(&sb, "Config:     %s\n", config.GlobalConfigFile())
	fmt.Fprintf(&sb, "Data dir:   %s\n", config.DataDir())

	// Counts
	fmt.Fprintf(&sb, "Agents:     %d\n", len(a.state.Agents))
	fmt.Fprintf(&sb, "Plugins:    %d\n", len(a.state.Plugins))

	// MCP servers
	mcpCount := len(a.sidebar.mcpServers)
	if mcpCount == 0 {
		fmt.Fprintf(&sb, "MCP:        none")
	} else {
		connected := 0
		for _, srv := range a.sidebar.mcpServers {
			if srv.Status == "connected" {
				connected++
			}
		}
		fmt.Fprintf(&sb, "MCP:        %d servers (%d connected)", mcpCount, connected)
	}

	return sb.String()
}

// buildHooksInfo collects configured hooks (plugin and shell) for display.
func (a *App) buildHooksInfo() string {
	var sb strings.Builder

	// Plugin hooks
	pluginCount := len(a.state.Plugins)
	fmt.Fprintf(&sb, "PLUGIN HOOKS\n\n")
	if pluginCount == 0 {
		sb.WriteString("  (none loaded)\n")
	} else {
		for _, p := range a.state.Plugins {
			fmt.Fprintf(&sb, "  %s\n", p.Name)
		}
	}

	// Shell hooks from config
	sb.WriteString("\nSHELL HOOKS\n\n")
	if len(a.state.ShellHooks) == 0 {
		sb.WriteString("  (none configured)\n")
		sb.WriteString("\n  Add hooks in settings.json:\n")
		sb.WriteString("  \"hooks\": {\n")
		sb.WriteString("    \"session.start\": [\n")
		sb.WriteString("      {\"command\": \"echo started\"}\n")
		sb.WriteString("    ]\n")
		sb.WriteString("  }")
	} else {
		for event, hooks := range a.state.ShellHooks {
			fmt.Fprintf(&sb, "  %s:\n", event)
			for _, h := range hooks {
				cmd := h.Command
				if len(cmd) > 50 {
					cmd = cmd[:47] + "..."
				}
				fmt.Fprintf(&sb, "    %s", cmd)
				if len(h.Match) > 0 {
					fmt.Fprintf(&sb, " (match: %v)", h.Match)
				}
				if h.Timeout > 0 {
					fmt.Fprintf(&sb, " [%ds]", h.Timeout)
				}
				sb.WriteString("\n")
			}
		}
	}

	return sb.String()
}

func isLocalProvider(id string) bool {
	switch id {
	case "ollama", "lmstudio", "vllm", "llamacpp":
		return true
	}
	return strings.HasPrefix(id, "localhost") || strings.HasPrefix(id, "127.0.0.1")
}

func (a *App) buildPrivacyInfo() string {
	var sb strings.Builder

	sb.WriteString("DATA STORED LOCALLY\n\n")

	fmt.Fprintf(&sb, "Config:       %s\n", config.ConfigDir())
	fmt.Fprintf(&sb, "Database:     %s\n", storage.DefaultPath())
	fmt.Fprintf(&sb, "Data dir:     %s\n", config.DataDir())

	sb.WriteString("\n  Sessions, messages, and conversation history\n")
	sb.WriteString("  are stored in the local SQLite database.\n")
	sb.WriteString("  Config, agents, skills, and themes are in\n")
	sb.WriteString("  the config directory. No other locations.\n")

	sb.WriteString("\nWHAT LEAVES YOUR MACHINE\n\n")

	sb.WriteString("  Nothing — unless you configure a cloud\n")
	sb.WriteString("  provider. When you do, only the current\n")
	sb.WriteString("  prompt and conversation context are sent\n")
	sb.WriteString("  to that provider's API endpoint.\n")

	sb.WriteString("\nWHAT IS NOT COLLECTED\n\n")

	sb.WriteString("  No telemetry. No analytics. No crash\n")
	sb.WriteString("  reports. No usage tracking. No sign-up.\n")
	sb.WriteString("  No account required. No phone-home.\n")

	sb.WriteString("\nCONFIGURED PROVIDERS\n\n")

	if len(a.state.Providers) == 0 {
		sb.WriteString("  (none discovered)\n")
	} else {
		for _, p := range a.state.Providers {
			local := "cloud"
			if isLocalProvider(p.ID) {
				local = "local"
			}
			fmt.Fprintf(&sb, "  %-14s %s\n", p.ID, local)
		}
	}

	return sb.String()
}
