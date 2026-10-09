package tui

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bobbyjohnstx/tinycode/internal/session"
	"github.com/bobbyjohnstx/tinycode/internal/tui/api"
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

	if strings.HasPrefix(trimmed, "/paste-image") || trimmed == "/image" {
		return c, readClipboardImage()
	}

	if strings.HasPrefix(trimmed, "/editor") {
		arg := strings.TrimSpace(strings.TrimPrefix(trimmed, "/editor"))
		if arg != "" {
			// Resolve file path: strip leading @ if present
			filePath := strings.TrimPrefix(arg, "@")
			cwd := c.app.status.Cwd()
			if !filepath.IsAbs(filePath) {
				filePath = filepath.Join(cwd, filePath)
			}
			if _, err := os.Stat(filePath); err != nil {
				model, cmd := c.app.Update(ToastMsg{Text: fmt.Sprintf("File not found: %s", arg), IsError: true})
				c.updateApp(model)
				return c, cmd
			}
			return c.handleEditorRequest(EditorRequestMsg{FilePath: filePath})
		}
		content := c.app.prompt.Value()
		return c.handleEditorRequest(EditorRequestMsg{Content: content})
	}

	if strings.HasPrefix(trimmed, "/thinking") {
		return c.handleThinkingCommand(trimmed)
	}

	if strings.HasPrefix(trimmed, "/effort") {
		return c.handleEffortCommand(trimmed)
	}

	if trimmed == "/branch" || strings.HasPrefix(trimmed, "/branch ") {
		return c.handleBranchCommand(trimmed)
	}

	if trimmed == "/btw" || strings.HasPrefix(trimmed, "/btw ") {
		return c.handleBtwCommand(trimmed)
	}

	if trimmed == "/init" || strings.HasPrefix(trimmed, "/init ") {
		return c.handleInitCommand()
	}

	if trimmed == "/goal" || strings.HasPrefix(trimmed, "/goal ") {
		return c.handleGoalCommand(trimmed)
	}

	// /work-loop <task> is an alias for /goal <task>.
	if strings.HasPrefix(trimmed, "/work-loop ") {
		task := strings.TrimSpace(strings.TrimPrefix(trimmed, "/work-loop"))
		if task != "" {
			var cmds []tea.Cmd
			toastCmd := c.app.toast.Show("Tip: /work-loop is now /goal", false)
			if toastCmd != nil {
				cmds = append(cmds, toastCmd)
			}
			_, goalCmd := c.handleGoalCommand("/goal " + task)
			if goalCmd != nil {
				cmds = append(cmds, goalCmd)
			}
			return c, tea.Batch(cmds...)
		}
	}

	if strings.HasPrefix(trimmed, "/rename ") {
		newTitle := strings.TrimSpace(strings.TrimPrefix(trimmed, "/rename"))
		if newTitle == "" || newTitle == "New Session" {
			model, cmd := c.app.Update(ToastMsg{Text: "Title cannot be empty or 'New Session'", IsError: true})
			c.updateApp(model)
			return c, cmd
		}
		sessionID := c.app.state.ActiveSession
		if sessionID == "" {
			model, cmd := c.app.Update(ToastMsg{Text: "No active session to rename", IsError: true})
			c.updateApp(model)
			return c, cmd
		}
		slog.Info("renaming session", "sessionID", sessionID, "title", newTitle)
		return c, renameSession(c.client, sessionID, newTitle)
	}

	if strings.HasPrefix(trimmed, "/copy") {
		arg := strings.TrimSpace(strings.TrimPrefix(trimmed, "/copy"))
		n := 1
		if arg != "" {
			parsed, err := strconv.Atoi(arg)
			if err != nil || parsed < 1 {
				model, cmd := c.app.Update(ToastMsg{Text: fmt.Sprintf("Invalid copy index: %s", arg), IsError: true})
				c.updateApp(model)
				return c, cmd
			}
			n = parsed
		}
		cmd := c.app.handleCopyCommand(n)
		if cmd != nil {
			model, updateCmd := c.app.Update(nil)
			c.updateApp(model)
			return c, tea.Batch(cmd, updateCmd)
		}
		c.updateApp(c.app)
		return c, nil
	}

	if trimmed == "/export html" || trimmed == "/export-html" {
		if cmd, handled := c.app.handleClientCommand("export-html"); handled {
			slog.Info("client command handled", "cmd", "export-html")
			return c, cmd
		}
	}

	if trimmed == "/clear-queue" || trimmed == "/queue clear" {
		c.promptQueue = nil
		c.app.status.SetQueueCount(0)
		model, cmd := c.app.Update(ToastMsg{Text: "Queue cleared", IsError: false})
		c.updateApp(model)
		return c, cmd
	}

	if strings.HasPrefix(trimmed, "/") {
		cmdName := strings.TrimPrefix(strings.Fields(trimmed)[0], "/")
		if cmd, handled := c.app.handleClientCommand(cmdName); handled {
			slog.Info("client command handled", "cmd", cmdName)
			return c, cmd
		}
	}

	// If session is working, queue the prompt instead of sending.
	if c.app.status.working && c.app.state.ActiveSession != "" {
		c.promptQueue = append(c.promptQueue, msg.Content)
		queueCount := len(c.promptQueue)
		model, cmd := c.app.Update(ToastMsg{
			Text:    fmt.Sprintf("Queued (%d pending)", queueCount),
			IsError: false,
		})
		c.updateApp(model)
		c.app.status.SetQueueCount(queueCount)
		return c, tea.Batch(cmd, c.app.reflowChrome())
	}

	// Parse /ask <agent> <message> into agent override + stripped text.
	// Slash command expansion (/swarm, /work-loop) happens server-side
	// in processPrompt for headless mode; /work-loop is intercepted above in TUI.
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
		return c, tea.Batch(cmd, c.app.reflowChrome())
	}

	if c.app.state.CurrentModel.ModelID == "" {
		c.app.state.PendingModelDialog = true
		model, toastCmd := c.app.Update(ToastMsg{
			Text:    "Select a provider and model first",
			IsError: true,
		})
		c.updateApp(model)
		c.app.status.SetWorking(false)
		return c, tea.Batch(toastCmd, c.app.reflowChrome(), func() tea.Msg { return ProvidersRefreshMsg{} })
	}

	sessionID := c.app.state.ActiveSession
	if sessionID == "" {
		slog.Info("no active session, creating new", "agent", agentOverride)
		c.pendingPrompt = promptText
		c.pendingAgent = agentOverride
		title := "New Session"
		if c.initialTitle != "" {
			title = c.initialTitle
			c.initialTitle = ""
		}
		input := api.SessionCreateInput{
			Title: title,
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
				ModelID:    c.app.state.CurrentModel.ModelID,
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
		title := "New Session"
		if c.initialTitle != "" {
			title = c.initialTitle
			c.initialTitle = ""
		}
		input := api.SessionCreateInput{
			Title: title,
			Agent: c.app.state.CurrentAgent,
		}
		if input.Agent == "" {
			input.Agent = "build"
		}
		if c.app.state.CurrentModel.ModelID != "" {
			input.Model = &session.ModelRef{
				ProviderID: c.app.state.CurrentModel.ProviderID,
				ModelID:    c.app.state.CurrentModel.ModelID,
			}
		}
		cmds = append(cmds, createSession(c.client, input))
	} else {
		pi := c.buildPromptInput(promptText)
		cmds = append(cmds, sendPrompt(c.client, sessionID, pi))
	}

	spinCmd := c.app.status.SetWorking(true)
	cmds = append(cmds, c.app.reflowChrome())
	if spinCmd != nil {
		cmds = append(cmds, spinCmd)
	}
	return c, tea.Batch(cmds...)
}
