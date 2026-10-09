package tui

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bobbyjohnstx/tinycode/internal/config"
	"github.com/bobbyjohnstx/tinycode/internal/storage"
)

// isSkillCommand reports whether name is a discovered skill (Source == "skill").
func (a *App) isSkillCommand(name string) bool {
	for _, cmd := range a.state.Commands {
		if cmd.Name == name && cmd.Source == "skill" {
			return true
		}
	}
	return false
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
		a.debugDlg.Show("Help", helpCommandsText)
		a.setFocus(FocusDialog)
		return nil, true
	case "editor":
		content := a.prompt.Value()
		return func() tea.Msg { return EditorRequestMsg{Content: content} }, true
	case "shell":
		return func() tea.Msg { return ShellSessionRequestMsg{} }, true
	case "paste-image":
		return readClipboardImage(), true
	case "diagnostics":
		info := a.buildDebugInfo()
		a.debugDlg.Show("Diagnostics", info)
		a.setFocus(FocusDialog)
		return nil, true
	case "privacy":
		info := a.buildPrivacyInfo()
		a.privacyDlg.Show("Privacy", info)
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
	case "changes":
		return func() tea.Msg { return ChangesRequestMsg{} }, true
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
	case "effort":
		current := a.state.EffortLevel
		if current == "" {
			current = "medium"
		}
		return a.toast.Show(fmt.Sprintf("Effort level: %s (use /effort low|medium|high|max)", current), false), true
	case "rename":
		return a.toast.Show("Usage: /rename <title> — rename current session", false), true
	case "thinking":
		current := a.state.ThinkingLevel
		if current == "" {
			current = "off"
		}
		return a.toast.Show(fmt.Sprintf("Thinking: %s (use /thinking off|low|medium|high|max)", current), false), true
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
		a.debugDlg.Show("Hooks", info)
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
	modName, provName := lookupModelDisplay(
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
