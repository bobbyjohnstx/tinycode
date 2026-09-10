package tui

import (
	"fmt"
	"log/slog"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bobbyjohnstx/tinycode-go/internal/session"
	"github.com/bobbyjohnstx/tinycode-go/internal/tui/api"
)

// handlePromptSubmission processes a PromptSubmittedMsg: parses slash/ask
// commands, validates agents, creates sessions if needed, and sends the prompt.
func (c *connectedApp) handlePromptSubmission(msg PromptSubmittedMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	slog.Info("prompt submitted", "content", msg.Content)

	trimmed := strings.TrimSpace(msg.Content)

	// ! shell command: run locally and feed output to model.
	if strings.HasPrefix(trimmed, "!") {
		shellCmd := strings.TrimSpace(trimmed[1:])
		if shellCmd != "" {
			slog.Info("user shell command", "cmd", shellCmd)
			dir := c.app.status.Cwd()
			return c, runUserShell(shellCmd, dir)
		}
	}

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
}

// handleSessionCreatedLocal processes a SessionCreatedLocalMsg: sets the active
// session and sends any pending prompt.
func (c *connectedApp) handleSessionCreatedLocal(msg SessionCreatedLocalMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
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
}

// handleShellResult processes a ShellResultMsg: formats the output and sends it
// as a prompt (creating a new session if needed).
func (c *connectedApp) handleShellResult(msg ShellResultMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var promptText string
	if msg.Err != nil {
		promptText = fmt.Sprintf("$ %s\n%s\n[exit error: %v]", msg.Command, msg.Output, msg.Err)
	} else {
		promptText = fmt.Sprintf("$ %s\n%s", msg.Command, msg.Output)
	}

	sessionID := c.app.state.ActiveSession
	if sessionID == "" {
		c.pendingPrompt = promptText
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
		pi := c.buildPromptInput(promptText)
		cmds = append(cmds, sendPrompt(c.client, sessionID, pi))
	}

	spinCmd := c.app.status.SetWorking(true)
	if spinCmd != nil {
		cmds = append(cmds, spinCmd)
	}
	return c, tea.Batch(cmds...)
}

// showErrorToast sends a ToastMsg to the app and returns any resulting cmds.
func (c *connectedApp) showErrorToast(format string, args ...any) []tea.Cmd {
	model, cmd := c.app.Update(ToastMsg{Text: fmt.Sprintf(format, args...), IsError: true})
	c.updateApp(model)
	if cmd != nil {
		return []tea.Cmd{cmd}
	}
	return nil
}
