package tui

import (
	"fmt"
	"log/slog"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// handleSessionMsg handles session, archive, rewind, and provider messages.
// The third result is false when msg belongs to another handler.
func (c *connectedApp) handleSessionMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case SessionSwitchedMsg:
		model, cmd := c.app.Update(msg)
		c.updateApp(model)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		if msg.SessionID != "" {
			cmds = append(cmds, fetchMessages(c.client, msg.SessionID))
		}
		return c, tea.Batch(cmds...), true

	case AgentSelectedMsg:
		model, cmd := c.app.Update(msg)
		c.updateApp(model)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		if sid := c.app.state.ActiveSession; sid != "" && msg.Agent != "" {
			cmds = append(cmds, patchSessionAgent(c.client, sid, msg.Agent))
		}
		return c, tea.Batch(cmds...), true

	case SessionAgentPatchedMsg:
		if msg.Err != nil {
			slog.Warn("session agent patch failed", "error", msg.Err)
		}
		return c, nil, true

	case PromptSentMsg:
		if msg.Err != nil {
			slog.Error("prompt send failed", "error", msg.Err)
			cmds = append(cmds, c.showErrorToast("Prompt failed: %v", msg.Err)...)
		} else {
			slog.Info("prompt sent successfully")
		}
		return c, tea.Batch(cmds...), true

	case PermissionReplyMsg:
		slog.Info("sending permission reply", "sessionID", msg.SessionID, "permissionID", msg.PermissionID, "action", msg.Action)
		cmds = append(cmds, replyPermission(c.client, msg.SessionID, msg.PermissionID, msg.Action))
		return c, tea.Batch(cmds...), true

	case PermissionRepliedMsg:
		if msg.Err != nil {
			slog.Error("permission reply failed", "error", msg.Err)
			cmds = append(cmds, c.showErrorToast("Permission reply failed: %v", msg.Err)...)
		}
		return c, tea.Batch(cmds...), true

	case AbortSentMsg:
		if msg.Err != nil {
			slog.Error("abort failed", "error", msg.Err)
			cmds = append(cmds, c.showErrorToast("Abort failed: %v", msg.Err)...)
		}
		return c, tea.Batch(cmds...), true

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
		return c, tea.Batch(cmds...), true

	case ModelScopedMsg:
		slog.Info("model scoping updated", "count", len(msg.ScopedModels))
		c.app.state.ScopedModels = msg.ScopedModels
		return c, patchScopedModels(c.client, msg.ScopedModels), true

	case ModelScopedDoneMsg:
		if msg.Err != nil {
			slog.Error("scoped models save failed", "error", msg.Err)
			cmds = append(cmds, c.showErrorToast("Scoped models save failed: %v", msg.Err)...)
		}
		return c, tea.Batch(cmds...), true

	case AgentToggleMsg:
		return c, toggleAgent(c.client, msg.Agent, msg.Disabled), true

	case AgentToggleDoneMsg:
		if msg.Err != nil {
			slog.Error("agent toggle failed", "error", msg.Err)
			cmds = append(cmds, c.showErrorToast("Agent toggle failed: %v", msg.Err)...)
			return c, tea.Batch(cmds...), true
		}
		return c, fetchAllAgents(c.client), true

	case MCPReconnectRequestMsg:
		return c, reconnectMCP(c.client, msg.Name), true

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
		return c, tea.Batch(cmds...), true

	case ProvidersRefreshMsg:
		cmds = append(cmds, fetchProviders(c.client))
		return c, tea.Batch(cmds...), true

	case ClipboardImageMsg:
		if msg.Err != nil {
			slog.Warn("clipboard image read failed", "error", msg.Err)
			cmds = append(cmds, c.showErrorToast("Clipboard image: %v", msg.Err)...)
			return c, tea.Batch(cmds...), true
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
		return c, tea.Batch(cmds...), true

	case CompactRequestMsg:
		sessionID := c.app.state.ActiveSession
		if sessionID == "" {
			return c, tea.Batch(c.showErrorToast("No active session to compact")...), true
		}
		return c, summarizeSession(c.client, sessionID), true

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
		return c, tea.Batch(cmds...), true

	case AbortRequestMsg:
		// Cancel any active goal when the user aborts.
		var abortCmds []tea.Cmd
		if c.goal != nil {
			c.goal = nil
			c.app.status.SetGoal("")
			abortCmds = append(abortCmds, c.app.reflowChrome())
		}
		sessionID := c.app.state.ActiveSession
		if sessionID != "" {
			abortCmds = append(abortCmds, abortSession(c.client, sessionID))
		}
		return c, tea.Batch(abortCmds...), true

	case ArchiveRequestMsg:
		sessionID := c.app.state.ActiveSession
		if sessionID == "" {
			return c, tea.Batch(c.showErrorToast("No active session to archive")...), true
		}
		return c, archiveSession(c.client, sessionID), true

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
		return c, tea.Batch(cmds...), true

	case RevertRequestMsg:
		sessionID := c.app.state.ActiveSession
		if sessionID == "" {
			return c, tea.Batch(c.showErrorToast("No active session to revert")...), true
		}
		return c, revertSession(c.client, sessionID), true

	case RevertSentMsg:
		if msg.Err != nil {
			slog.Error("revert failed", "error", msg.Err)
			cmds = append(cmds, c.showErrorToast("Revert failed: %v", msg.Err)...)
		}
		return c, tea.Batch(cmds...), true

	case UnrevertRequestMsg:
		sessionID := c.app.state.ActiveSession
		if sessionID == "" {
			return c, tea.Batch(c.showErrorToast("No active session to restore")...), true
		}
		return c, unrevertSession(c.client, sessionID), true

	case UnrevertSentMsg:
		if msg.Err != nil {
			slog.Error("unrevert failed", "error", msg.Err)
			cmds = append(cmds, c.showErrorToast("Restore failed: %v", msg.Err)...)
		}
		return c, tea.Batch(cmds...), true

	case BranchRequestMsg:
		sessionID := c.app.state.ActiveSession
		if sessionID == "" {
			return c, tea.Batch(c.showErrorToast("No active session to branch")...), true
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
		return c, branchSession(c.client, sessionID, title), true

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
		return c, tea.Batch(cmds...), true

	case SessionStatusMsg:
		wasWorking := c.app.status.working
		// Forward to App first for working state / spinner.
		model, cmd := c.app.Update(msg)
		c.updateApp(model)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		// Refresh LSP diagnostics after a turn ends (tools may have opened files).
		if msg.SessionID == c.app.state.ActiveSession && wasWorking && !msg.Status.Working {
			cmds = append(cmds, fetchLSPStatus(c.client))
		}
		// Drain prompt queue when the active session transitions to idle.
		if msg.SessionID == c.app.state.ActiveSession && wasWorking && !msg.Status.Working && len(c.promptQueue) > 0 {
			next := c.promptQueue[0]
			c.promptQueue = c.promptQueue[1:]
			c.app.status.SetQueueCount(len(c.promptQueue))
			cmds = append(cmds, func() tea.Msg {
				return PromptSubmittedMsg{Content: next}
			})
		}
		// When the active session transitions from working to idle and a goal is active,
		// trigger goal evaluation.
		if msg.SessionID == c.app.state.ActiveSession && !msg.Status.Working && c.goal != nil {
			c.goal.state.Iteration++
			c.app.status.SetGoalState(c.goal.state.Text, c.goal.state.Iteration, c.goal.state.MaxIterations)
			cmds = append(cmds, c.app.reflowChrome())
			if c.goal.state.Command != "" {
				// Shell-verifiable goal: run the command to check.
				dir := c.app.status.Cwd()
				cmds = append(cmds, evaluateGoal(c.goal.state.Command, dir, c.goal.state.Iteration))
			}
			// Self-assessment goals (empty command): the model self-terminates
			// via its prompt protocol. Iteration counter and max-iterations
			// cap still apply — they are checked on the next idle transition.
			if c.goal.state.Command == "" && c.goal.state.Iteration >= c.goal.state.MaxIterations {
				text := fmt.Sprintf("Goal not met after %d iterations: %s", c.goal.state.MaxIterations, c.goal.state.Text)
				c.goal = nil
				c.app.status.SetGoal("")
				cmds = append(cmds, c.app.reflowChrome())
				toastCmd := c.app.toast.Show(text, true)
				if toastCmd != nil {
					cmds = append(cmds, toastCmd)
				}
			}
		}
		return c, tea.Batch(cmds...), true

	case GoalEvalMsg:
		model, cmd := c.handleGoalEval(msg)
		return model, cmd, true

	case RewindSelectedMsg:
		sessionID := c.app.state.ActiveSession
		if sessionID == "" {
			return c, tea.Batch(c.showErrorToast("No active session to rewind")...), true
		}
		cmd := c.app.toast.Show("Rewinding conversation...", false)
		return c, tea.Batch(cmd, rewindSession(c.client, sessionID, msg.Turn.MessageID, msg.Turn.Index)), true

	case RewindDoneMsg:
		if msg.Err != nil {
			slog.Error("rewind failed", "error", msg.Err)
			cmds = append(cmds, c.showErrorToast("Rewind failed: %v", msg.Err)...)
		} else {
			toastCmd := c.app.toast.Show(fmt.Sprintf("Rewound to turn %d", msg.TurnIndex), false)
			if toastCmd != nil {
				cmds = append(cmds, toastCmd)
			}
			// Reload messages to reflect the truncated conversation.
			sessionID := c.app.state.ActiveSession
			if sessionID != "" {
				cmds = append(cmds, fetchMessages(c.client, sessionID))
			}
		}
		return c, tea.Batch(cmds...), true

	case RewindForkMsg:
		sessionID := c.app.state.ActiveSession
		if sessionID == "" {
			return c, tea.Batch(c.showErrorToast("No active session to fork")...), true
		}
		title := fmt.Sprintf("fork at turn %d", msg.Turn.Index)
		for _, s := range c.app.state.Sessions {
			if s.ID == sessionID {
				title = fmt.Sprintf("%s (turn %d)", s.Title, msg.Turn.Index)
				break
			}
		}
		cmd := c.app.toast.Show("Forking conversation...", false)
		return c, tea.Batch(cmd, forkSessionAtTurn(c.client, sessionID, msg.Turn.MessageID, title, msg.Turn.Index)), true

	case ForkDoneMsg:
		if msg.Err != nil {
			slog.Error("fork failed", "error", msg.Err)
			cmds = append(cmds, c.showErrorToast("Fork failed: %v", msg.Err)...)
		} else if msg.Session != nil {
			c.app.state.Sessions = append([]SessionInfo{*msg.Session}, c.app.state.Sessions...)
			c.app.sidebar.SetSessions(c.app.state.Sessions)
			model, cmd := c.app.Update(ToastMsg{Text: "Switched to fork: " + msg.Session.Title, IsError: false})
			c.updateApp(model)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			cmds = append(cmds, func() tea.Msg {
				return SessionSwitchedMsg{SessionID: msg.Session.ID}
			})
		}
		return c, tea.Batch(cmds...), true

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
		return c, tea.Batch(cmds...), true

	case ProvidersLoadedMsg:
		model, cmd := c.app.Update(msg)
		c.updateApp(model)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		if msg.Err == nil && c.app.state.CurrentModel.ProviderID != "" {
			cmds = append(cmds, fetchProviderBalance(c.client, c.app.state.CurrentModel.ProviderID))
		}
		return c, tea.Batch(cmds...), true

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
		return c, tea.Batch(cmds...), true

	case StoreOpenRouterAuthMsg:
		cmds = append(cmds, storeOpenRouterAuth(c.client, msg.APIKey))
		return c, tea.Batch(cmds...), true

	case AuthStoredMsg:
		if msg.Err != nil {
			model, cmd := c.app.Update(ToastMsg{
				Text:    fmt.Sprintf("OpenRouter auth failed: %v", msg.Err),
				IsError: true,
			})
			c.updateApp(model)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			return c, tea.Batch(cmds...), true
		}
		c.app.state.PendingModelDialog = true
		toastCmd := c.app.toast.Show("OpenRouter connected", false)
		if toastCmd != nil {
			cmds = append(cmds, toastCmd)
		}
		cmds = append(cmds, fetchProviders(c.client))
		return c, tea.Batch(cmds...), true

	case ProviderBalanceMsg:
		if msg.Err != nil {
			slog.Warn("provider balance fetch failed", "error", msg.Err)
		} else if msg.Balance != nil {
			slog.Info("provider balance received", "provider", msg.Balance.Provider, "hasLimit", msg.Balance.HasLimit, "remaining", msg.Balance.Remaining, "usage", msg.Balance.Usage)
			c.app.sidebar.SetBalance(msg.Balance)
		} else {
			slog.Info("provider balance: no data available")
		}
		return c, nil, true
	default:
		return c, nil, false
	}
}
