package tui

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bobbyjohnstx/tinycode/internal/config"
	"github.com/bobbyjohnstx/tinycode/internal/tui/api"
)

// RunConfig holds the configuration for starting the TUI.
type RunConfig struct {
	ServerURL       string
	Directory       string
	Theme           string
	Token           string
	Version         string
	ScopedModels    []string
	ShellHooks      map[string][]config.HookConfig
	InitialTitle    string
	SafeMode        bool
	ResumeSessionID string
}

// Run starts the bubbletea TUI program connected to the given server.
func Run(ctx context.Context, cfg RunConfig) error {
	client := api.New(cfg.ServerURL, cfg.Directory, cfg.Token)

	app := newConnectedApp(ctx, cfg.ServerURL, client, cfg.Directory, cfg.Theme, cfg.Version, cfg.ScopedModels)
	app.app.state.ShellHooks = cfg.ShellHooks
	app.initialTitle = cfg.InitialTitle
	app.resumeSessionID = cfg.ResumeSessionID
	if cfg.SafeMode {
		app.app.status.SetSafeMode(true)
	}

	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())

	go func() {
		<-ctx.Done()
		p.Quit()
	}()

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}
	return nil
}

// pendingImage holds base64-encoded image data waiting to be sent with the next prompt.
type pendingImage struct {
	Data      string // base64-encoded
	MediaType string // e.g. "image/png"
	Size      int    // raw byte count
}

// sideQA stores a single side question and answer pair.
type sideQA struct {
	Question string
	Answer   string
}

// connectedApp wraps App with an API client for server communication.
type connectedApp struct {
	app             App
	client          *api.Client
	ctx             context.Context
	sseEvents       <-chan api.ServerEvent
	pendingPrompt   string
	pendingAgent    string
	pendingImages   []pendingImage
	initialTitle    string
	resumeSessionID string
	btwHistory      []sideQA
	goal            *goalTracker
}

func newConnectedApp(ctx context.Context, serverURL string, client *api.Client, directory, themeName, version string, scopedModels []string) *connectedApp {
	app := NewApp(serverURL)
	app.status.SetCwd(directory)
	app.sidebar.SetCwd(directory)
	app.prompt.SetCwd(directory)
	if version != "" {
		app.sidebar.SetVersion(version)
	}
	app.prompt.EnableStartupGuard()
	if themeName != "" {
		if theme := app.themes.Get(themeName); theme != nil {
			app.state.CurrentTheme = themeName
			ApplyColorTheme(theme)
		}
	}
	if len(scopedModels) > 0 {
		app.state.ScopedModels = scopedModels
		app.modelDlg.SetScopedModels(scopedModels)
	}
	return &connectedApp{
		app:    app,
		client: client,
		ctx:    ctx,
	}
}

func (c *connectedApp) Init() tea.Cmd {
	events, err := c.client.Subscribe(c.ctx)
	if err != nil {
		return func() tea.Msg { return SSEDisconnectedMsg{Err: err} }
	}
	c.sseEvents = events
	c.app.welcome.MarkDone("sse")
	cmds := []tea.Cmd{
		c.app.Init(),
		waitForSSE(c.sseEvents),
		fetchSessions(c.client, 50, 0),
		fetchProviders(c.client),
		fetchAllAgents(c.client),
		fetchCommands(c.client),
		fetchPlugins(c.client),
		fetchMCPStatus(c.client),
	}
	if c.resumeSessionID != "" {
		resumeID := c.resumeSessionID
		cmds = append(cmds, func() tea.Msg {
			return SessionSwitchedMsg{SessionID: resumeID}
		})
	}
	return tea.Batch(cmds...)
}

func (c *connectedApp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case SSEEventMsg:
		slog.Debug("SSE event", "type", msg.Event.Type)
		tuiMsg := mapSSEToMsg(msg.Event)
		if _, ok := tuiMsg.(ProvidersRefreshMsg); ok {
			cmds = append(cmds, fetchProviders(c.client))
		}
		model, cmd := c.app.Update(tuiMsg)
		c.updateApp(model)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return c, tea.Batch(append(cmds, waitForSSE(c.sseEvents))...)

	case SSEDisconnectedMsg:
		events, err := c.client.Subscribe(c.ctx)
		if err != nil {
			return c, nil
		}
		c.sseEvents = events
		return c, waitForSSE(c.sseEvents)

	case PromptSubmittedMsg:
		return c.handlePromptSubmission(msg)

	case SessionCreatedLocalMsg:
		return c.handleSessionCreatedLocal(msg)

	case SessionSwitchedMsg:
		model, cmd := c.app.Update(msg)
		c.updateApp(model)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		if msg.SessionID != "" {
			cmds = append(cmds, fetchMessages(c.client, msg.SessionID))
		}
		return c, tea.Batch(cmds...)

	case PromptSentMsg:
		if msg.Err != nil {
			slog.Error("prompt send failed", "error", msg.Err)
			cmds = append(cmds, c.showErrorToast("Prompt failed: %v", msg.Err)...)
		} else {
			slog.Info("prompt sent successfully")
		}
		return c, tea.Batch(cmds...)

	case PermissionReplyMsg:
		slog.Info("sending permission reply", "sessionID", msg.SessionID, "permissionID", msg.PermissionID, "action", msg.Action)
		cmds = append(cmds, replyPermission(c.client, msg.SessionID, msg.PermissionID, msg.Action))
		return c, tea.Batch(cmds...)

	case PermissionRepliedMsg:
		if msg.Err != nil {
			slog.Error("permission reply failed", "error", msg.Err)
			cmds = append(cmds, c.showErrorToast("Permission reply failed: %v", msg.Err)...)
		}
		return c, tea.Batch(cmds...)

	case AbortSentMsg:
		if msg.Err != nil {
			slog.Error("abort failed", "error", msg.Err)
			cmds = append(cmds, c.showErrorToast("Abort failed: %v", msg.Err)...)
		}
		return c, tea.Batch(cmds...)

	case SessionRenamedMsg:
		if msg.Err != nil {
			slog.Error("session rename failed", "error", msg.Err)
			cmds = append(cmds, c.showErrorToast("Rename failed: %v", msg.Err)...)
		} else {
			model, cmd := c.app.Update(ToastMsg{Text: "Session renamed", IsError: false})
			c.updateApp(model)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return c, tea.Batch(cmds...)

	case ModelScopedMsg:
		slog.Info("model scoping updated", "count", len(msg.ScopedModels))
		c.app.state.ScopedModels = msg.ScopedModels
		return c, patchScopedModels(c.client, msg.ScopedModels)

	case ModelScopedDoneMsg:
		if msg.Err != nil {
			slog.Error("scoped models save failed", "error", msg.Err)
			cmds = append(cmds, c.showErrorToast("Scoped models save failed: %v", msg.Err)...)
		}
		return c, tea.Batch(cmds...)

	case AgentToggleMsg:
		return c, toggleAgent(c.client, msg.Agent, msg.Disabled)

	case AgentToggleDoneMsg:
		if msg.Err != nil {
			slog.Error("agent toggle failed", "error", msg.Err)
			cmds = append(cmds, c.showErrorToast("Agent toggle failed: %v", msg.Err)...)
			return c, tea.Batch(cmds...)
		}
		return c, fetchAllAgents(c.client)

	case MCPReconnectRequestMsg:
		return c, reconnectMCP(c.client, msg.Name)

	case MCPReconnectResultMsg:
		if msg.Err != nil {
			slog.Error("MCP reconnect failed", "name", msg.Name, "error", msg.Err)
			cmds = append(cmds, c.showErrorToast("MCP reconnect failed: %v", msg.Err)...)
		} else {
			model, cmd := c.app.Update(ToastMsg{Text: "Reconnecting " + msg.Name + "...", IsError: false})
			c.updateApp(model)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return c, tea.Batch(cmds...)

	case ProvidersRefreshMsg:
		cmds = append(cmds, fetchProviders(c.client))
		return c, tea.Batch(cmds...)

	case ShellResultMsg:
		return c.handleShellResult(msg)

	case EditorRequestMsg:
		return c.handleEditorRequest(msg)

	case EditorDoneMsg:
		return c.handleEditorDone(msg)

	case ShellSessionRequestMsg:
		return c.handleShellSessionRequest()

	case ShellSessionDoneMsg:
		return c.handleShellSessionDone(msg)

	case DiffRequestMsg:
		return c.handleDiffRequest(msg)

	case DiffDoneMsg:
		return c.handleDiffDone(msg)

	case ClipboardImageMsg:
		if msg.Err != nil {
			slog.Warn("clipboard image read failed", "error", msg.Err)
			cmds = append(cmds, c.showErrorToast("Clipboard image: %v", msg.Err)...)
			return c, tea.Batch(cmds...)
		}
		c.pendingImages = append(c.pendingImages, pendingImage{
			Data:      msg.Data,
			MediaType: msg.MediaType,
			Size:      msg.Size,
		})
		c.app.prompt.AddImage(msg.Size)
		slog.Info("clipboard image attached", "size", msg.Size, "count", len(c.pendingImages))
		model, cmd := c.app.Update(ToastMsg{Text: fmt.Sprintf("Image attached (%s)", formatImageSize(msg.Size))})
		c.updateApp(model)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return c, tea.Batch(cmds...)

	case CompactRequestMsg:
		sessionID := c.app.state.ActiveSession
		if sessionID == "" {
			return c, tea.Batch(c.showErrorToast("No active session to compact")...)
		}
		return c, summarizeSession(c.client, sessionID)

	case CompactDoneMsg:
		if msg.Err != nil {
			slog.Error("compact failed", "error", msg.Err)
			cmds = append(cmds, c.showErrorToast("Compact failed: %v", msg.Err)...)
		} else {
			model, cmd := c.app.Update(ToastMsg{Text: "Compacting context...", IsError: false})
			c.updateApp(model)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return c, tea.Batch(cmds...)

	case AbortRequestMsg:
		// Cancel any active goal when the user aborts.
		if c.goal != nil {
			c.goal = nil
			c.app.status.SetGoal("")
		}
		sessionID := c.app.state.ActiveSession
		if sessionID != "" {
			return c, abortSession(c.client, sessionID)
		}
		return c, nil

	case ArchiveRequestMsg:
		sessionID := c.app.state.ActiveSession
		if sessionID == "" {
			return c, tea.Batch(c.showErrorToast("No active session to archive")...)
		}
		return c, archiveSession(c.client, sessionID)

	case ArchiveSentMsg:
		if msg.Err != nil {
			slog.Error("archive failed", "error", msg.Err)
			cmds = append(cmds, c.showErrorToast("Archive failed: %v", msg.Err)...)
		} else {
			model, cmd := c.app.Update(ToastMsg{Text: "Session archived", IsError: false})
			c.updateApp(model)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			cmds = append(cmds, func() tea.Msg { return SessionSwitchedMsg{SessionID: ""} })
		}
		return c, tea.Batch(cmds...)

	case RevertRequestMsg:
		sessionID := c.app.state.ActiveSession
		if sessionID == "" {
			return c, tea.Batch(c.showErrorToast("No active session to revert")...)
		}
		return c, revertSession(c.client, sessionID)

	case RevertSentMsg:
		if msg.Err != nil {
			slog.Error("revert failed", "error", msg.Err)
			cmds = append(cmds, c.showErrorToast("Revert failed: %v", msg.Err)...)
		}
		return c, tea.Batch(cmds...)

	case UnrevertRequestMsg:
		sessionID := c.app.state.ActiveSession
		if sessionID == "" {
			return c, tea.Batch(c.showErrorToast("No active session to restore")...)
		}
		return c, unrevertSession(c.client, sessionID)

	case UnrevertSentMsg:
		if msg.Err != nil {
			slog.Error("unrevert failed", "error", msg.Err)
			cmds = append(cmds, c.showErrorToast("Restore failed: %v", msg.Err)...)
		}
		return c, tea.Batch(cmds...)

	case BranchRequestMsg:
		sessionID := c.app.state.ActiveSession
		if sessionID == "" {
			return c, tea.Batch(c.showErrorToast("No active session to branch")...)
		}
		title := msg.Name
		if title == "" {
			for _, s := range c.app.state.Sessions {
				if s.ID == sessionID {
					title = "branch of " + s.Title
					break
				}
			}
			if title == "" {
				title = "branch"
			}
		}
		return c, branchSession(c.client, sessionID, title)

	case BranchDoneMsg:
		if msg.Err != nil {
			slog.Error("branch failed", "error", msg.Err)
			cmds = append(cmds, c.showErrorToast("Branch failed: %v", msg.Err)...)
		} else if msg.Session != nil {
			c.app.state.Sessions = append([]SessionInfo{*msg.Session}, c.app.state.Sessions...)
			c.app.sidebar.SetSessions(c.app.state.Sessions)
			model, cmd := c.app.Update(ToastMsg{Text: "Switched to branch: " + msg.Session.Title, IsError: false})
			c.updateApp(model)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			cmds = append(cmds, func() tea.Msg {
				return SessionSwitchedMsg{SessionID: msg.Session.ID}
			})
		}
		return c, tea.Batch(cmds...)

	case SessionStatusMsg:
		// Forward to App first for working state / spinner.
		model, cmd := c.app.Update(msg)
		c.updateApp(model)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		// When the active session transitions from working to idle and a goal is active,
		// trigger goal evaluation.
		if msg.SessionID == c.app.state.ActiveSession && !msg.Status.Working && c.goal != nil {
			dir := c.app.status.Cwd()
			c.goal.state.Iteration++
			c.app.status.SetGoal(c.goal.statusText())
			cmds = append(cmds, evaluateGoal(c.goal.state.Command, dir, c.goal.state.Iteration))
		}
		return c, tea.Batch(cmds...)

	case GoalEvalMsg:
		return c.handleGoalEval(msg)

	case BtwResponseMsg:
		if msg.Err != nil {
			slog.Error("btw failed", "error", msg.Err)
			cmds = append(cmds, c.showErrorToast("Side question failed: %v", msg.Err)...)
		} else {
			c.btwHistory = append(c.btwHistory, sideQA{Question: msg.Question, Answer: msg.Answer})
			text := "Side answer: " + msg.Answer
			cmd := c.app.toast.ShowWithDuration(text, false, 10*time.Second)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return c, tea.Batch(cmds...)

	case ProvidersLoadedMsg:
		model, cmd := c.app.Update(msg)
		c.updateApp(model)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		if msg.Err == nil && c.app.state.CurrentModel.ProviderID != "" {
			cmds = append(cmds, fetchProviderBalance(c.client, c.app.state.CurrentModel.ProviderID))
		}
		return c, tea.Batch(cmds...)

	case ModelSelectedMsg:
		slog.Info("model selected", "provider", msg.Selection.ProviderID, "model", msg.Selection.ModelID)
		model, cmd := c.app.Update(msg)
		c.updateApp(model)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		if c.app.state.CurrentModel.ProviderID != "" {
			slog.Info("fetching provider balance", "provider", c.app.state.CurrentModel.ProviderID)
			cmds = append(cmds, fetchProviderBalance(c.client, c.app.state.CurrentModel.ProviderID))
		}
		return c, tea.Batch(cmds...)

	case ProviderBalanceMsg:
		if msg.Err != nil {
			slog.Warn("provider balance fetch failed", "error", msg.Err)
		} else if msg.Balance != nil {
			slog.Info("provider balance received", "provider", msg.Balance.Provider, "hasLimit", msg.Balance.HasLimit, "remaining", msg.Balance.Remaining, "usage", msg.Balance.Usage)
			c.app.sidebar.SetBalance(msg.Balance)
		} else {
			slog.Info("provider balance: no data available")
		}
		return c, nil
	}

	model, cmd := c.app.Update(msg)
	c.updateApp(model)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}
	return c, tea.Batch(cmds...)
}

func (c *connectedApp) updateApp(model tea.Model) {
	if app, ok := model.(App); ok {
		c.app = app
	}
}

// isKnownAgent checks if the agent name exists in the loaded agent list.
func (c *connectedApp) isKnownAgent(name string) bool {
	for _, a := range c.app.state.Agents {
		if a.Name == name {
			return true
		}
	}
	return false
}

// parseAskCommand checks if text is a "/ask <agent> <message>" command.
// Returns (message, agent) if matched, or (original text, "") if not.
func parseAskCommand(text string) (string, string) {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "/ask ") {
		return text, ""
	}
	rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "/ask"))
	fields := strings.SplitN(rest, " ", 2)
	if len(fields) == 0 || fields[0] == "" {
		return text, ""
	}
	agent := fields[0]
	message := ""
	if len(fields) > 1 {
		message = strings.TrimSpace(fields[1])
	}
	if message == "" {
		return text, ""
	}
	return message, agent
}

// buildPromptInput creates an api.PromptInput with the user's text and current model selection.
// It resolves any @file references by reading the files and appending their contents as
// additional text parts.
func (c *connectedApp) buildPromptInput(text string) api.PromptInput {
	parts := []api.PromptPart{{Type: "text", Text: text}}

	// Resolve @file references.
	cwd := c.app.status.Cwd()
	refs := findAllAtTokens(text)
	for _, ref := range refs {
		absPath := ref.path
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(cwd, ref.path)
		}
		content, err := readFileForPrompt(absPath)
		if err != nil {
			slog.Debug("@file read failed", "path", ref.path, "error", err)
			continue
		}
		parts = append(parts, api.PromptPart{
			Type: "text",
			Text: fmt.Sprintf("--- File: %s ---\n%s\n--- End ---", ref.path, content),
		})
	}

	// Include pending images.
	for _, img := range c.pendingImages {
		parts = append(parts, api.PromptPart{
			Type:      "image",
			Content:   img.Data,
			MediaType: img.MediaType,
		})
	}
	c.pendingImages = nil
	c.app.prompt.ClearImages()

	input := api.PromptInput{Parts: parts}
	if c.app.state.CurrentModel.ModelID != "" {
		input.Model = &api.PromptModel{
			ProviderID: c.app.state.CurrentModel.ProviderID,
			ModelID:    c.app.state.CurrentModel.ModelID,
		}
	}
	if level := c.app.state.ThinkingLevel; level != "" && level != "off" {
		if budget, _, ok := thinkingLevelBudget(level); ok && budget > 0 {
			input.ThinkingBudget = &budget
		}
	}
	return input
}

// readFileForPrompt reads a file up to 100KB for inclusion in a prompt.
func readFileForPrompt(path string) (string, error) {
	const maxSize = 100 * 1024
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("is a directory")
	}
	if info.Size() > maxSize {
		f, err := os.Open(path)
		if err != nil {
			return "", err
		}
		defer f.Close()
		buf := make([]byte, maxSize)
		n, _ := f.Read(buf)
		return string(buf[:n]) + "\n... (truncated)", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (c *connectedApp) View() string {
	return c.app.View()
}
