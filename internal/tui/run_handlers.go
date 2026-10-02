package tui

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	commandpkg "github.com/bobbyjohnstx/tinycode/internal/command"
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

	if trimmed == "/branch" || strings.HasPrefix(trimmed, "/branch ") {
		return c.handleBranchCommand(trimmed)
	}

	if trimmed == "/btw" || strings.HasPrefix(trimmed, "/btw ") {
		return c.handleBtwCommand(trimmed)
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

	if strings.HasPrefix(trimmed, "/") {
		cmdName := strings.TrimPrefix(strings.Fields(trimmed)[0], "/")
		if cmd, handled := c.app.handleClientCommand(cmdName); handled {
			slog.Info("client command handled", "cmd", cmdName)
			return c, cmd
		}
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
		return c, cmd
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
	if spinCmd != nil {
		cmds = append(cmds, spinCmd)
	}
	return c, tea.Batch(cmds...)
}

// thinkingLevelBudget maps a thinking level name to a token budget.
// Returns (budget, displayLabel, ok).
func thinkingLevelBudget(level string) (int, string, bool) {
	switch level {
	case "off":
		return 0, "off", true
	case "low":
		return 1024, "1k tokens", true
	case "medium":
		return 4096, "4k tokens", true
	case "high":
		return 16384, "16k tokens", true
	case "max":
		return 128000, "128k tokens", true
	default:
		return 0, "", false
	}
}

// handleThinkingCommand handles the /thinking slash command.
func (c *connectedApp) handleThinkingCommand(trimmed string) (tea.Model, tea.Cmd) {
	level := strings.TrimSpace(strings.TrimPrefix(trimmed, "/thinking"))

	if level == "" {
		current := c.app.state.ThinkingLevel
		if current == "" {
			current = "off"
		}
		model, cmd := c.app.Update(ToastMsg{Text: fmt.Sprintf("Thinking level: %s", current)})
		c.updateApp(model)
		return c, cmd
	}

	if _, _, ok := thinkingLevelBudget(level); !ok {
		model, cmd := c.app.Update(ToastMsg{
			Text:    fmt.Sprintf("Invalid thinking level: %s (use off, low, medium, high, max)", level),
			IsError: true,
		})
		c.updateApp(model)
		return c, cmd
	}

	c.app.state.ThinkingLevel = level
	c.app.prompt.SetThinkingLevel(level)

	_, label, _ := thinkingLevelBudget(level)
	model, cmd := c.app.Update(ToastMsg{Text: fmt.Sprintf("Thinking level: %s (%s)", level, label)})
	c.updateApp(model)
	return c, cmd
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

// handleEditorRequest opens $EDITOR. If FilePath is set, edits that file
// directly (in-place). Otherwise, creates a temp file with Content.
func (c *connectedApp) handleEditorRequest(msg EditorRequestMsg) (tea.Model, tea.Cmd) {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}

	// Edit an existing file directly — no temp file, no content return.
	if msg.FilePath != "" {
		cmd := exec.Command(editor, msg.FilePath)
		return c, tea.ExecProcess(cmd, func(err error) tea.Msg {
			return EditorDoneMsg{Err: err}
		})
	}

	// Edit prompt content via temp file.
	tmpFile, err := os.CreateTemp("", "tinycode-editor-*.txt")
	if err != nil {
		slog.Error("failed to create temp file for editor", "error", err)
		return c, tea.Batch(c.showErrorToast("Editor failed: %v", err)...)
	}

	if msg.Content != "" {
		if _, err := tmpFile.WriteString(msg.Content); err != nil {
			tmpFile.Close()
			os.Remove(tmpFile.Name())
			slog.Error("failed to write to temp file", "error", err)
			return c, tea.Batch(c.showErrorToast("Editor failed: %v", err)...)
		}
	}
	tmpFile.Close()

	tmpPath := tmpFile.Name()
	cmd := exec.Command(editor, tmpPath)
	return c, tea.ExecProcess(cmd, func(err error) tea.Msg {
		content := ""
		if err == nil {
			data, readErr := os.ReadFile(tmpPath)
			if readErr != nil {
				err = readErr
			} else {
				content = string(data)
			}
		}
		os.Remove(tmpPath)
		return EditorDoneMsg{Content: content, Err: err}
	})
}

// handleEditorDone sets the prompt textarea to the editor content.
func (c *connectedApp) handleEditorDone(msg EditorDoneMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		slog.Error("editor exited with error", "error", msg.Err)
		return c, tea.Batch(c.showErrorToast("Editor error: %v", msg.Err)...)
	}
	c.app.prompt.SetValue(msg.Content)
	return c, nil
}

// handleShellSessionRequest opens an interactive shell.
func (c *connectedApp) handleShellSessionRequest() (tea.Model, tea.Cmd) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}

	cmd := exec.Command(shell)
	cmd.Dir = c.app.status.Cwd()
	return c, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return ShellSessionDoneMsg{Err: err}
	})
}

// handleDiffRequest runs git diff HEAD and either shows a toast (no changes)
// or opens the output in a pager.
func (c *connectedApp) handleDiffRequest(msg DiffRequestMsg) (tea.Model, tea.Cmd) {
	dir := msg.Dir
	if dir == "" {
		dir, _ = os.Getwd()
	}

	// Check for changes first.
	checkCmd := exec.Command("git", "diff", "--quiet", "HEAD")
	checkCmd.Dir = dir
	if err := checkCmd.Run(); err == nil {
		// Exit code 0 means no changes.
		model, cmd := c.app.Update(ToastMsg{Text: "No uncommitted changes", IsError: false})
		c.updateApp(model)
		return c, cmd
	}

	// Changes exist — open pager.
	pagerCmd := exec.Command("git", "diff", "HEAD")
	pagerCmd.Dir = dir
	// GIT_PAGER ensures git uses less even if the user overrode it.
	pagerCmd.Env = append(os.Environ(), "GIT_PAGER=less -R")
	return c, tea.ExecProcess(pagerCmd, func(err error) tea.Msg {
		return DiffDoneMsg{Err: err}
	})
}

// handleDiffDone shows a toast when the diff pager exits.
func (c *connectedApp) handleDiffDone(msg DiffDoneMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		slog.Error("diff pager exited with error", "error", msg.Err)
		return c, tea.Batch(c.showErrorToast("Diff error: %v", msg.Err)...)
	}
	return c, nil
}

// handleShellSessionDone shows a toast when the shell exits.
func (c *connectedApp) handleShellSessionDone(msg ShellSessionDoneMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		slog.Error("shell session exited with error", "error", msg.Err)
		return c, tea.Batch(c.showErrorToast("Shell error: %v", msg.Err)...)
	}
	model, cmd := c.app.Update(ToastMsg{Text: "Shell session ended", IsError: false})
	c.updateApp(model)
	return c, cmd
}

// handleBranchCommand processes a /branch [name] command.
func (c *connectedApp) handleBranchCommand(trimmed string) (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(strings.TrimPrefix(trimmed, "/branch"))

	sessionID := c.app.state.ActiveSession
	if sessionID == "" {
		model, cmd := c.app.Update(ToastMsg{Text: "No active session to branch", IsError: true})
		c.updateApp(model)
		return c, cmd
	}

	// Auto-generate title if none provided.
	title := name
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

	cmd := c.app.toast.Show("Branching conversation...", false)
	return c, tea.Batch(cmd, branchSession(c.client, sessionID, title))
}

// handleBtwCommand processes a /btw side question command.
func (c *connectedApp) handleBtwCommand(trimmed string) (tea.Model, tea.Cmd) {
	question := strings.TrimSpace(strings.TrimPrefix(trimmed, "/btw"))

	// /btw with no argument: show the last side answer.
	if question == "" {
		if len(c.btwHistory) == 0 {
			model, cmd := c.app.Update(ToastMsg{Text: "No side questions yet", IsError: true})
			c.updateApp(model)
			return c, cmd
		}
		last := c.btwHistory[len(c.btwHistory)-1]
		text := "Side Q: " + last.Question + "\n\nSide A: " + last.Answer
		cmd := c.app.toast.ShowWithDuration(text, false, 10*time.Second)
		return c, cmd
	}

	sessionID := c.app.state.ActiveSession
	if sessionID == "" {
		model, cmd := c.app.Update(ToastMsg{Text: "No active session for side question", IsError: true})
		c.updateApp(model)
		return c, cmd
	}

	cmd := c.app.toast.Show("Asking side question...", false)
	return c, tea.Batch(cmd, askBtw(c.client, sessionID, question))
}

// askBtw sends a side question to the server and returns the answer.
func askBtw(client *api.Client, sessionID, question string) tea.Cmd {
	return func() tea.Msg {
		answer, err := client.Btw(sessionID, question)
		return BtwResponseMsg{Question: question, Answer: answer, Err: err}
	}
}

// handleGoalCommand processes a /goal [condition|clear|stop|off|cancel] command.
func (c *connectedApp) handleGoalCommand(trimmed string) (tea.Model, tea.Cmd) {
	arg := strings.TrimSpace(strings.TrimPrefix(trimmed, "/goal"))

	// /goal with no argument: show current goal status.
	if arg == "" {
		if c.goal == nil {
			model, cmd := c.app.Update(ToastMsg{Text: "No active goal", IsError: false})
			c.updateApp(model)
			return c, cmd
		}
		model, cmd := c.app.Update(ToastMsg{Text: c.goal.statusText(), IsError: false})
		c.updateApp(model)
		return c, cmd
	}

	// /goal clear|stop|off|cancel: cancel active goal.
	switch arg {
	case "clear", "stop", "off", "cancel":
		if c.goal == nil {
			model, cmd := c.app.Update(ToastMsg{Text: "No active goal to cancel", IsError: false})
			c.updateApp(model)
			return c, cmd
		}
		c.goal = nil
		c.app.status.SetGoal("")
		model, cmd := c.app.Update(ToastMsg{Text: "Goal cancelled", IsError: false})
		c.updateApp(model)
		return c, cmd
	}

	// /goal <condition>: set and start a new goal.
	sessionID := c.app.state.ActiveSession
	if sessionID == "" {
		model, cmd := c.app.Update(ToastMsg{Text: "No active session for goal", IsError: true})
		c.updateApp(model)
		return c, cmd
	}

	command, _ := session.ResolveGoalCommand(arg)

	c.goal = newGoalTracker(arg, command)
	c.app.status.SetGoal(c.goal.statusText())

	// Send the initial prompt to the model.
	var promptText string
	if command != "" {
		// Shell-verifiable goal: tell the model which command to make pass.
		promptText = fmt.Sprintf("Goal: %s\nCommand to verify: `%s`\nPlease work toward making this command succeed (exit code 0). Start by running it to see the current state.", arg, command)
	} else {
		// Self-assessment goal: use work-loop prefix for autonomous iteration.
		promptText = commandpkg.WorkLoopPrefix + arg
	}
	pi := c.buildPromptInput(promptText)

	spinCmd := c.app.status.SetWorking(true)
	var cmds []tea.Cmd
	if spinCmd != nil {
		cmds = append(cmds, spinCmd)
	}
	cmds = append(cmds, sendPrompt(c.client, sessionID, pi))
	toastCmd := c.app.toast.Show(fmt.Sprintf("Goal set: %s", arg), false)
	if toastCmd != nil {
		cmds = append(cmds, toastCmd)
	}

	// Notify the app about prompt submission for working state.
	model, cmd := c.app.Update(PromptSubmittedMsg{Content: "/goal " + arg})
	c.updateApp(model)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	return c, tea.Batch(cmds...)
}

// handleGoalEval processes the result of a goal condition evaluation.
func (c *connectedApp) handleGoalEval(msg GoalEvalMsg) (tea.Model, tea.Cmd) {
	if c.goal == nil {
		return c, nil
	}

	var cmds []tea.Cmd

	// Goal met: stop and show success.
	if msg.Met {
		text := fmt.Sprintf("Goal met: %s (after %d iterations)", c.goal.state.Text, c.goal.state.Iteration)
		c.goal = nil
		c.app.status.SetGoal("")
		toastCmd := c.app.toast.Show(text, false)
		if toastCmd != nil {
			cmds = append(cmds, toastCmd)
		}
		return c, tea.Batch(cmds...)
	}

	// Max iterations reached: stop with failure toast.
	if c.goal.state.Iteration >= c.goal.state.MaxIterations {
		text := fmt.Sprintf("Goal not met after %d iterations: %s", c.goal.state.MaxIterations, c.goal.state.Text)
		c.goal = nil
		c.app.status.SetGoal("")
		toastCmd := c.app.toast.Show(text, true)
		if toastCmd != nil {
			cmds = append(cmds, toastCmd)
		}
		return c, tea.Batch(cmds...)
	}

	// Stuck detection: same output N times in a row.
	if c.goal.isStuck(msg.Output) {
		text := fmt.Sprintf("Goal appears stuck: %s (same output %d times)", c.goal.state.Text, goalStuckThreshold)
		c.goal = nil
		c.app.status.SetGoal("")
		toastCmd := c.app.toast.Show(text, true)
		if toastCmd != nil {
			cmds = append(cmds, toastCmd)
		}
		return c, tea.Batch(cmds...)
	}

	// Goal not met, iterations remaining: inject follow-up prompt.
	sessionID := c.app.state.ActiveSession
	if sessionID == "" {
		c.goal = nil
		c.app.status.SetGoal("")
		return c, nil
	}

	c.app.status.SetGoal(c.goal.statusText())

	// Truncate output for the follow-up prompt to avoid overwhelming context.
	output := msg.Output
	const maxOutputLen = 4000
	if len(output) > maxOutputLen {
		output = output[:maxOutputLen] + "\n... (truncated)"
	}

	followUp := fmt.Sprintf("The goal '%s' is not met yet (iteration %d/%d).\nVerification command: `%s`\nOutput:\n```\n%s\n```\nPlease continue working toward making this command succeed.",
		c.goal.state.Text, c.goal.state.Iteration, c.goal.state.MaxIterations,
		c.goal.state.Command, output)

	pi := c.buildPromptInput(followUp)
	cmds = append(cmds, sendPrompt(c.client, sessionID, pi))

	spinCmd := c.app.status.SetWorking(true)
	if spinCmd != nil {
		cmds = append(cmds, spinCmd)
	}

	return c, tea.Batch(cmds...)
}
