package tui

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bobbyjohnstx/tinycode-go/internal/tui/api"
)

// RunConfig holds the configuration for starting the TUI.
type RunConfig struct {
	ServerURL string
	Directory string
	Theme     string
	Token     string
	Version   string
}

// Run starts the bubbletea TUI program connected to the given server.
func Run(ctx context.Context, cfg RunConfig) error {
	client := api.New(cfg.ServerURL, cfg.Directory, cfg.Token)

	app := newConnectedApp(ctx, cfg.ServerURL, client, cfg.Directory, cfg.Theme, cfg.Version)

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

// connectedApp wraps App with an API client for server communication.
type connectedApp struct {
	app           App
	client        *api.Client
	ctx           context.Context
	sseEvents     <-chan api.ServerEvent
	pendingPrompt string
	pendingAgent  string
}

func newConnectedApp(ctx context.Context, serverURL string, client *api.Client, directory, themeName, version string) *connectedApp {
	app := NewApp(serverURL)
	app.status.SetCwd(directory)
	app.sidebar.SetCwd(directory)
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
	return tea.Batch(
		c.app.Init(),
		waitForSSE(c.sseEvents),
		fetchSessions(c.client, 50, 0),
		fetchProviders(c.client),
		fetchAgents(c.client),
		fetchCommands(c.client),
		fetchPlugins(c.client),
		fetchMCPStatus(c.client),
	)
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

	case ProvidersRefreshMsg:
		cmds = append(cmds, fetchProviders(c.client))
		return c, tea.Batch(cmds...)

	case ShellResultMsg:
		return c.handleShellResult(msg)

	case AbortRequestMsg:
		sessionID := c.app.state.ActiveSession
		if sessionID != "" {
			return c, abortSession(c.client, sessionID)
		}
		return c, nil

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
		model, cmd := c.app.Update(msg)
		c.updateApp(model)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		if c.app.state.CurrentModel.ProviderID != "" {
			cmds = append(cmds, fetchProviderBalance(c.client, c.app.state.CurrentModel.ProviderID))
		}
		return c, tea.Batch(cmds...)

	case ProviderBalanceMsg:
		if msg.Err == nil && msg.Balance != nil {
			c.app.sidebar.SetBalance(msg.Balance)
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
func (c *connectedApp) buildPromptInput(text string) api.PromptInput {
	input := api.PromptInput{
		Parts: []api.PromptPart{{Type: "text", Text: text}},
	}
	if c.app.state.CurrentModel.ModelID != "" {
		input.Model = &api.PromptModel{
			ProviderID: c.app.state.CurrentModel.ProviderID,
			ModelID:    c.app.state.CurrentModel.ModelID,
		}
	}
	return input
}

func (c *connectedApp) View() string {
	return c.app.View()
}
