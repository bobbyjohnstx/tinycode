package tui

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bobbyjohnstx/tinycode-go/internal/session"
	"github.com/bobbyjohnstx/tinycode-go/internal/tui/api"
)

// RunConfig holds the configuration for starting the TUI.
type RunConfig struct {
	ServerURL string
	Directory string
}

// Run starts the bubbletea TUI program connected to the given server.
func Run(ctx context.Context, cfg RunConfig) error {
	client := api.New(cfg.ServerURL, cfg.Directory)

	app := newConnectedApp(ctx, cfg.ServerURL, client, cfg.Directory)

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

func newConnectedApp(ctx context.Context, serverURL string, client *api.Client, directory string) *connectedApp {
	app := NewApp(serverURL)
	app.status.SetCwd(directory)
	app.prompt.EnableStartupGuard()
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
	return tea.Batch(
		c.app.Init(),
		waitForSSE(c.sseEvents),
		fetchSessions(c.client, 50, 0),
		fetchProviders(c.client),
		fetchAgents(c.client),
		fetchCommands(c.client),
	)
}

func (c *connectedApp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case SSEEventMsg:
		slog.Debug("SSE event", "type", msg.Event.Type)
		tuiMsg := mapSSEToMsg(msg.Event)
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
		slog.Info("prompt submitted", "content", msg.Content)

		// Handle client-side commands before sending to server.
		trimmed := strings.TrimSpace(msg.Content)
		if strings.HasPrefix(trimmed, "/") {
			cmdName := strings.TrimPrefix(strings.Fields(trimmed)[0], "/")
			if cmd, handled := c.app.handleClientCommand(cmdName); handled {
				slog.Info("client command handled", "cmd", cmdName)
				return c, cmd
			}
		}

		// Parse /ask <agent> <message> into agent override + stripped text.
		promptText, agentOverride := parseAskCommand(msg.Content)
		slog.Info("parsed prompt", "text", promptText, "agent", agentOverride, "knownAgents", len(c.app.state.Agents))

		if agentOverride != "" && !c.isKnownAgent(agentOverride) {
			slog.Warn("unknown agent", "agent", agentOverride)
			model, cmd := c.app.Update(ToastMsg{
				Text:    fmt.Sprintf("Unknown agent: %s", agentOverride),
				IsError: true,
			})
			c.updateApp(model)
			c.app.status.SetWorking(false)
			return c, cmd
		}

		sessionID := c.app.state.ActiveSession
		if sessionID == "" {
			slog.Info("no active session, creating new", "agent", agentOverride)
			c.pendingPrompt = msg.Content
			c.pendingAgent = agentOverride
			input := api.SessionCreateInput{
				Title: "New Session",
				Agent: c.app.state.CurrentAgent,
			}
			if agentOverride != "" {
				input.Agent = agentOverride
			}
			if input.Agent == "" {
				input.Agent = "build"
			}
			if c.app.state.CurrentModel.ModelID != "" {
				input.Model = &session.ModelRef{
					ProviderID: c.app.state.CurrentModel.ProviderID,
					ID:         c.app.state.CurrentModel.ModelID,
				}
			}
			cmds = append(cmds, createSession(c.client, input))
		} else {
			pi := c.buildPromptInput(promptText)
			if agentOverride != "" {
				pi.Agent = agentOverride
			}
			slog.Info("sending prompt", "sessionID", sessionID, "text", promptText, "agent", pi.Agent)
			cmds = append(cmds, sendPrompt(c.client, sessionID, pi))
		}
		model, cmd := c.app.Update(msg)
		c.updateApp(model)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return c, tea.Batch(cmds...)

	case SessionCreatedLocalMsg:
		if msg.Err != nil {
			c.pendingPrompt = ""
			c.pendingAgent = ""
			model, cmd := c.app.Update(SessionErrorMsg{Error: fmt.Sprintf("Failed to create session: %v", msg.Err)})
			c.updateApp(model)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			return c, tea.Batch(cmds...)
		}
		if msg.Session != nil {
			c.app.state.ActiveSession = msg.Session.ID
			c.app.state.Sessions = append([]SessionInfo{*msg.Session}, c.app.state.Sessions...)
			c.app.syncPromptMetadata()
			if c.pendingPrompt != "" {
				promptText, agent := parseAskCommand(c.pendingPrompt)
				if c.pendingAgent != "" {
					agent = c.pendingAgent
				}
				pi := c.buildPromptInput(promptText)
				if agent != "" {
					pi.Agent = agent
				}
				cmds = append(cmds, sendPrompt(c.client, msg.Session.ID, pi))
				c.pendingPrompt = ""
				c.pendingAgent = ""
			}
		}
		return c, tea.Batch(cmds...)

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
			model, cmd := c.app.Update(ToastMsg{Text: fmt.Sprintf("Prompt failed: %v", msg.Err), IsError: true})
			c.updateApp(model)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
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
			model, cmd := c.app.Update(ToastMsg{Text: fmt.Sprintf("Permission reply failed: %v", msg.Err), IsError: true})
			c.updateApp(model)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return c, tea.Batch(cmds...)

	case AbortSentMsg:
		if msg.Err != nil {
			slog.Error("abort failed", "error", msg.Err)
			model, cmd := c.app.Update(ToastMsg{Text: fmt.Sprintf("Abort failed: %v", msg.Err), IsError: true})
			c.updateApp(model)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return c, tea.Batch(cmds...)

	case ProvidersRefreshMsg:
		cmds = append(cmds, fetchProviders(c.client))
		return c, tea.Batch(cmds...)

	case AbortRequestMsg:
		sessionID := c.app.state.ActiveSession
		if sessionID != "" {
			return c, abortSession(c.client, sessionID)
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
