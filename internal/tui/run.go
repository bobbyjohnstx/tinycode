package tui

import (
	"context"
	"fmt"

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

	app := newConnectedApp(ctx, cfg.ServerURL, client)

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
}

func newConnectedApp(ctx context.Context, serverURL string, client *api.Client) *connectedApp {
	return &connectedApp{
		app:    NewApp(serverURL),
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
		tuiMsg := mapSSEToMsg(msg.Event)
		model, cmd := c.app.Update(tuiMsg)
		c.app = model.(App)
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
		sessionID := c.app.state.ActiveSession
		if sessionID == "" {
			c.pendingPrompt = msg.Content
			input := api.SessionCreateInput{
				Title: "New Session",
				Agent: c.app.state.CurrentAgent,
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
			cmds = append(cmds, sendPrompt(c.client, sessionID, api.PromptInput{
				Parts: []api.PromptPart{{Type: "text", Text: msg.Content}},
			}))
		}
		model, cmd := c.app.Update(msg)
		c.app = model.(App)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return c, tea.Batch(cmds...)

	case SessionCreatedLocalMsg:
		if msg.Err == nil && msg.Session != nil {
			c.app.state.ActiveSession = msg.Session.ID
			c.app.state.Sessions = append([]SessionInfo{*msg.Session}, c.app.state.Sessions...)
			c.app.syncPromptMetadata()
			if c.pendingPrompt != "" {
				cmds = append(cmds, sendPrompt(c.client, msg.Session.ID, api.PromptInput{
					Parts: []api.PromptPart{{Type: "text", Text: c.pendingPrompt}},
				}))
				c.pendingPrompt = ""
			}
		}
		return c, tea.Batch(cmds...)

	case SessionSwitchedMsg:
		model, cmd := c.app.Update(msg)
		c.app = model.(App)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		if msg.SessionID != "" {
			cmds = append(cmds, fetchMessages(c.client, msg.SessionID))
		}
		return c, tea.Batch(cmds...)

	case PromptSentMsg:
		if msg.Err != nil {
			model, cmd := c.app.Update(ToastMsg{Text: fmt.Sprintf("Prompt failed: %v", msg.Err), IsError: true})
			c.app = model.(App)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return c, tea.Batch(cmds...)

	case ProvidersRefreshMsg:
		cmds = append(cmds, fetchProviders(c.client))
		return c, tea.Batch(cmds...)
	}

	model, cmd := c.app.Update(msg)
	c.app = model.(App)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}
	return c, tea.Batch(cmds...)
}

func (c *connectedApp) View() string {
	return c.app.View()
}
