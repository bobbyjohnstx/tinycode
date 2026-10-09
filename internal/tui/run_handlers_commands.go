package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	commandpkg "github.com/bobbyjohnstx/tinycode/internal/command"
	"github.com/bobbyjohnstx/tinycode/internal/plugin"
	"github.com/bobbyjohnstx/tinycode/internal/safego"
	"github.com/bobbyjohnstx/tinycode/internal/scaffold"
	"github.com/bobbyjohnstx/tinycode/internal/session"
	"github.com/bobbyjohnstx/tinycode/internal/tui/api"
)

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
	reflowCmd := c.app.reflowChrome() // prompt meta row may appear/disappear

	_, label, _ := thinkingLevelBudget(level)
	model, cmd := c.app.Update(ToastMsg{Text: fmt.Sprintf("Thinking level: %s (%s)", level, label)})
	c.updateApp(model)
	return c, tea.Batch(cmd, reflowCmd)
}

// effortSettings holds the parameters for a given effort level.
type effortSettings struct {
	MaxTokens     int
	SystemPrefix  string
	MaxIterations int
}

// effortLevelSettings maps an effort level name to its settings.
// Returns (settings, ok).
func effortLevelSettings(level string) (effortSettings, bool) {
	switch level {
	case "low":
		return effortSettings{MaxTokens: 1024, SystemPrefix: "Be concise and direct.", MaxIterations: 2}, true
	case "medium":
		return effortSettings{MaxTokens: 4096, MaxIterations: 5}, true
	case "high":
		return effortSettings{MaxTokens: 8192, SystemPrefix: "Be thorough and comprehensive.", MaxIterations: 10}, true
	case "max":
		return effortSettings{MaxTokens: 0, SystemPrefix: "Be exhaustive. Use every tool at your disposal.", MaxIterations: 20}, true
	default:
		return effortSettings{}, false
	}
}

// handleEffortCommand handles the /effort slash command.
func (c *connectedApp) handleEffortCommand(trimmed string) (tea.Model, tea.Cmd) {
	level := strings.TrimSpace(strings.TrimPrefix(trimmed, "/effort"))

	if level == "" {
		current := c.app.state.EffortLevel
		if current == "" {
			current = "medium"
		}
		model, cmd := c.app.Update(ToastMsg{Text: fmt.Sprintf("Effort level: %s", current)})
		c.updateApp(model)
		return c, cmd
	}

	if _, ok := effortLevelSettings(level); !ok {
		model, cmd := c.app.Update(ToastMsg{
			Text:    fmt.Sprintf("Invalid effort level: %s (use low, medium, high, max)", level),
			IsError: true,
		})
		c.updateApp(model)
		return c, cmd
	}

	c.app.state.EffortLevel = level
	c.app.prompt.SetEffortLevel(level)
	c.app.status.SetEffort(level)
	reflowCmd := c.app.reflowChrome() // prompt meta row may appear/disappear

	model, cmd := c.app.Update(ToastMsg{Text: fmt.Sprintf("Effort level: %s", level)})
	c.updateApp(model)
	return c, tea.Batch(cmd, reflowCmd)
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

// extractModifiedFiles walks chat messages and returns deduplicated file paths
// from write/edit tool calls.
func extractModifiedFiles(messages []MessageView) []string {
	seen := make(map[string]bool)
	var files []string
	for _, msg := range messages {
		for _, part := range msg.Parts {
			name := strings.ToLower(part.ToolName)
			if name != "write" && name != "edit" {
				continue
			}
			var args map[string]any
			if err := json.Unmarshal([]byte(part.ToolArgs), &args); err != nil {
				continue
			}
			fp, ok := args["file_path"].(string)
			if !ok || fp == "" {
				continue
			}
			if !seen[fp] {
				seen[fp] = true
				files = append(files, fp)
			}
		}
	}
	return files
}

// handleChangesRequest runs git diff from startHead scoped to files modified
// in this session and opens the output in a pager.
func (c *connectedApp) handleChangesRequest() (tea.Model, tea.Cmd) {
	if c.startHead == "" {
		model, cmd := c.app.Update(ToastMsg{Text: "Session diff requires git", IsError: true})
		c.updateApp(model)
		return c, cmd
	}

	files := extractModifiedFiles(c.app.chat.Messages())
	if len(files) == 0 {
		model, cmd := c.app.Update(ToastMsg{Text: "No changes in this session", IsError: false})
		c.updateApp(model)
		return c, cmd
	}

	dir := c.app.status.Cwd()

	// Build args: git diff <startHead> -- file1 file2 ...
	args := []string{"diff", c.startHead, "--"}
	args = append(args, files...)

	pagerCmd := exec.Command("git", args...)
	pagerCmd.Dir = dir
	pagerCmd.Env = append(os.Environ(), "GIT_PAGER=less -R")
	return c, tea.ExecProcess(pagerCmd, func(err error) tea.Msg {
		return ChangesDoneMsg{Err: err}
	})
}

// handleChangesDone shows a toast when the changes pager exits.
func (c *connectedApp) handleChangesDone(msg ChangesDoneMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		slog.Error("changes pager exited with error", "error", msg.Err)
		return c, tea.Batch(c.showErrorToast("Changes error: %v", msg.Err)...)
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

// handleInitCommand generates a root AGENTS.md (if missing) then submits an
// improved setup prompt to the LLM.
func (c *connectedApp) handleInitCommand() (tea.Model, tea.Cmd) {
	dir := c.app.status.Cwd()
	if dir == "" {
		dir, _ = os.Getwd()
	}

	created, path, err := scaffold.EnsureRootAgentsMD(dir)
	var toastCmd tea.Cmd
	prompt := scaffold.InitPrompt
	switch {
	case err != nil:
		model, cmd := c.app.Update(ToastMsg{Text: "Failed to write AGENTS.md: " + err.Error(), IsError: true})
		c.updateApp(model)
		return c, cmd
	case created:
		toastCmd = c.app.toast.Show("Created "+filepath.Base(path), false)
	default:
		toastCmd = c.app.toast.Show("AGENTS.md already exists", false)
		prompt = scaffold.InitPromptExisting
	}

	model, cmd := c.handlePromptSubmission(PromptSubmittedMsg{Content: prompt})
	if toastCmd != nil {
		return model, tea.Batch(toastCmd, cmd)
	}
	return model, cmd
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
		reflowCmd := c.app.reflowChrome()
		model, cmd := c.app.Update(ToastMsg{Text: "Goal cancelled", IsError: false})
		c.updateApp(model)
		return c, tea.Batch(cmd, reflowCmd)
	}

	// /goal <condition>: set and start a new goal.
	sessionID := c.app.state.ActiveSession
	if sessionID == "" {
		model, cmd := c.app.Update(ToastMsg{Text: "No active session for goal", IsError: true})
		c.updateApp(model)
		return c, cmd
	}

	command, _ := session.ResolveGoalCommand(arg, c.app.status.Cwd())

	c.goal = newGoalTracker(arg, command)
	c.app.status.SetGoalState(c.goal.state.Text, c.goal.state.Iteration, c.goal.state.MaxIterations)
	var cmds []tea.Cmd
	cmds = append(cmds, c.app.reflowChrome())

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
	cmds = append(cmds, c.app.reflowChrome())
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

	// Goal met: stop and show success with auto-fade.
	if msg.Met {
		text := c.goal.state.Text
		iterations := c.goal.state.Iteration
		c.goal = nil
		fadeCmd := c.app.status.SetGoalComplete(text, iterations)
		cmds = append(cmds, c.app.reflowChrome())
		if fadeCmd != nil {
			cmds = append(cmds, fadeCmd)
		}
		toastText := fmt.Sprintf("Goal met: %s (after %d iterations)", text, iterations)
		toastCmd := c.app.toast.Show(toastText, false)
		if toastCmd != nil {
			cmds = append(cmds, toastCmd)
		}
		safego.Go(func() {
			_, _ = plugin.SendNotification(context.Background(), "Goal met", toastText, "normal")
		})
		return c, tea.Batch(cmds...)
	}

	// Max iterations reached: stop with failure toast.
	if c.goal.state.Iteration >= c.goal.state.MaxIterations {
		text := fmt.Sprintf("Goal not met after %d iterations: %s", c.goal.state.MaxIterations, c.goal.state.Text)
		c.goal = nil
		c.app.status.SetGoal("")
		cmds = append(cmds, c.app.reflowChrome())
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
		cmds = append(cmds, c.app.reflowChrome())
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
		return c, c.app.reflowChrome()
	}

	c.app.status.SetGoalState(c.goal.state.Text, c.goal.state.Iteration, c.goal.state.MaxIterations)
	cmds = append(cmds, c.app.reflowChrome())

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
	cmds = append(cmds, c.app.reflowChrome())
	if spinCmd != nil {
		cmds = append(cmds, spinCmd)
	}

	return c, tea.Batch(cmds...)
}
