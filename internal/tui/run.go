package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

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
	app    App
	client *api.Client
	ctx    context.Context
}

func newConnectedApp(ctx context.Context, serverURL string, client *api.Client) connectedApp {
	return connectedApp{
		app:    NewApp(serverURL),
		client: client,
		ctx:    ctx,
	}
}

func (c connectedApp) Init() tea.Cmd {
	return tea.Batch(
		c.app.Init(),
		listenSSE(c.ctx, c.client),
		fetchSessions(c.client, 50, 0),
		fetchProviders(c.client),
	)
}

func (c connectedApp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case SSEEventMsg:
		tuiMsg := mapSSEToMsg(msg.Event)
		model, cmd := c.app.Update(tuiMsg)
		c.app = model.(App)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return c, tea.Batch(append(cmds, listenSSE(c.ctx, c.client))...)

	case PromptSubmittedMsg:
		sessionID := c.app.state.ActiveSession
		if sessionID == "" {
			cmds = append(cmds, createSession(c.client, api.SessionCreateInput{
				Title: "New Session",
				Agent: "build",
			}))
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
		}
		return c, nil

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
	}

	model, cmd := c.app.Update(msg)
	c.app = model.(App)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}
	return c, tea.Batch(cmds...)
}

func (c connectedApp) View() string {
	return c.app.View()
}
