package tui

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bobbyjohnstx/tinycode/internal/config"
	"github.com/bobbyjohnstx/tinycode/internal/plugin"
	"github.com/bobbyjohnstx/tinycode/internal/safego"
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
	CycleAgents     []string
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
	app.app.state.CycleAgents = cfg.CycleAgents
	app.initialTitle = cfg.InitialTitle
	app.resumeSessionID = cfg.ResumeSessionID
	if cfg.SafeMode {
		app.app.status.SetSafeMode(true)
	}

	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())

	safego.Go(func() {
		<-ctx.Done()
		p.Quit()
	})

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	// Save frecency data on graceful shutdown.
	if app.app.frecStore != nil {
		app.app.frecStore.Shutdown()
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
	startHead       string   // git HEAD at session start, for /changes
	promptQueue     []string // queued prompts waiting for current run to finish
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
	// Capture git HEAD at session start for /changes.
	dir := c.app.status.Cwd()
	if out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output(); err == nil {
		c.startHead = strings.TrimSpace(string(out))
	}

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
		fetchLSPStatus(c.client),
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
	if model, cmd, handled := c.handleSessionMsg(msg); handled {
		return model, cmd
	}
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case SSEEventMsg:
		slog.Debug("SSE event", "type", msg.Event.Type)
		tuiMsg := mapSSEToMsg(msg.Event)
		if _, ok := tuiMsg.(ProvidersRefreshMsg); ok {
			cmds = append(cmds, fetchProviders(c.client))
		}
		if scMsg, ok := tuiMsg.(SubagentCompletedMsg); ok {
			safego.Go(func() {
				title := "Subagent complete"
				body := scMsg.Label
				if scMsg.Agent != "" {
					body = scMsg.Agent + ": " + scMsg.Label
				}
				_, _ = plugin.SendNotification(context.Background(), title, body, "low")
			})
		}
		model, cmd := c.app.Update(tuiMsg)
		c.updateApp(model)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return c, tea.Batch(append(cmds, waitForSSE(c.sseEvents))...)

	case SSEDisconnectedMsg:
		slog.Warn("SSE disconnected", "error", msg.Err)
		events, err := c.client.Subscribe(c.ctx)
		if err != nil {
			// Start reconnection with exponential backoff.
			model, cmd := c.app.Update(ToastMsg{Text: "Reconnecting...", IsError: false})
			c.updateApp(model)
			return c, tea.Batch(cmd, sseReconnectAfter(0))
		}
		c.sseEvents = events
		return c, waitForSSE(c.sseEvents)

	case SSEReconnectMsg:
		events, err := c.client.Subscribe(c.ctx)
		if err != nil {
			if msg.Attempt >= 4 {
				slog.Error("SSE reconnection failed after 5 attempts", "error", err)
				cmds = append(cmds, c.showErrorToast("Connection lost")...)
				return c, tea.Batch(cmds...)
			}
			slog.Warn("SSE reconnection attempt failed", "attempt", msg.Attempt+1, "error", err)
			model, cmd := c.app.Update(ToastMsg{Text: "Reconnecting...", IsError: false})
			c.updateApp(model)
			return c, tea.Batch(cmd, sseReconnectAfter(msg.Attempt+1))
		}
		slog.Info("SSE reconnected", "attempt", msg.Attempt+1)
		c.sseEvents = events
		return c, waitForSSE(c.sseEvents)

	case PromptSubmittedMsg:
		return c.handlePromptSubmission(msg)

	case SessionCreatedLocalMsg:
		return c.handleSessionCreatedLocal(msg)

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

	case ChangesRequestMsg:
		return c.handleChangesRequest()

	case ChangesDoneMsg:
		return c.handleChangesDone(msg)

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
	if c.app.state.CurrentAgent != "" {
		input.Agent = c.app.state.CurrentAgent
	}
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
	if level := c.app.state.EffortLevel; level != "" && level != "medium" {
		if settings, ok := effortLevelSettings(level); ok {
			if settings.MaxTokens > 0 {
				input.MaxTokens = &settings.MaxTokens
			}
			input.SystemPrefix = settings.SystemPrefix
			input.MaxIterations = &settings.MaxIterations
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
