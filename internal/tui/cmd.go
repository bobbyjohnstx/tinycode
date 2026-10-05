package tui

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bobbyjohnstx/tinycode/internal/session"
	"github.com/bobbyjohnstx/tinycode/internal/tui/api"
)

// sseReconnectAfter returns a tea.Cmd that fires an SSEReconnectMsg after an
// exponential backoff delay: 1s, 2s, 4s, 8s, 16s for attempts 0–4.
func sseReconnectAfter(attempt int) tea.Cmd {
	delay := time.Second << uint(attempt) // 1s, 2s, 4s, 8s, 16s
	return tea.Tick(delay, func(_ time.Time) tea.Msg {
		return SSEReconnectMsg{Attempt: attempt}
	})
}

// waitForSSE waits for the next event on an existing SSE channel.
func waitForSSE(events <-chan api.ServerEvent) tea.Cmd {
	return func() tea.Msg {
		evt, ok := <-events
		if !ok {
			return SSEDisconnectedMsg{}
		}
		return SSEEventMsg{Event: evt}
	}
}

// mapSSEToMsg converts a raw server event into a typed bubbletea message.
func mapSSEToMsg(evt api.ServerEvent) tea.Msg {
	props := evt.Properties
	sessionID, _ := props["sessionID"].(string)

	switch evt.Type {
	case "session.created":
		return SessionCreatedMsg{
			SessionID: sessionID,
			Info:      parseSessionInfo(props),
		}

	case "session.updated":
		return SessionUpdatedMsg{
			SessionID: sessionID,
			Info:      parseSessionInfo(props),
		}

	case "session.deleted":
		return SessionDeletedMsg{
			SessionID: sessionID,
		}

	case "session.status":
		status := SessionStatus{}
		if statusMap, ok := props["status"].(map[string]any); ok {
			statusType, _ := statusMap["type"].(string)
			status.Working = statusType == "busy"
			status.Alert, _ = statusMap["alert"].(bool)
		}
		return SessionStatusMsg{
			SessionID: sessionID,
			Status:    status,
		}

	case "message.updated":
		return MessageUpdatedMsg{
			SessionID: sessionID,
			Message:   parseMessageView(props),
		}

	case "message.part.updated":
		return MessagePartUpdatedMsg{
			SessionID: sessionID,
			Part:      parsePartView(props),
		}

	case "message.part.delta":
		messageID, _ := props["messageID"].(string)
		partID, _ := props["partID"].(string)
		field, _ := props["field"].(string)
		delta, _ := props["delta"].(string)
		return MessagePartDeltaMsg{
			SessionID: sessionID,
			MessageID: messageID,
			PartID:    partID,
			Field:     field,
			Delta:     delta,
		}

	case "session.error":
		errMsg := extractSessionError(props)
		if errMsg == "" {
			errMsg = "unknown error"
		}
		return SessionErrorMsg{SessionID: sessionID, Error: errMsg}

	case "permission.asked":
		metadata, _ := props["metadata"].(map[string]any)
		return PermissionRequestedMsg{
			Request: PermissionRequest{
				ID:         stringProp(props, "id"),
				SessionID:  sessionID,
				Permission: stringProp(props, "permission"),
				Metadata:   metadata,
			},
		}

	case "mcp.status":
		srv := MCPServer{}
		if server, ok := props["server"].(map[string]any); ok {
			srv.Name, _ = server["name"].(string)
			srv.Status, _ = server["status"].(string)
			srv.Error, _ = server["error"].(string)
			if tc, ok := intFromAny(server["toolCount"]); ok {
				srv.ToolCount = tc
			}
		}
		return MCPStatusMsg{Server: srv}

	case "provider.discovered", "provider.removed", "provider.reconnected":
		return ProvidersRefreshMsg{}

	case "session.reverted":
		return ToastMsg{Text: "Changes reverted", IsError: false}

	case "session.unreverted":
		return ToastMsg{Text: "Changes restored", IsError: false}

	case "session.compacted":
		compNum, _ := props["compactionNum"].(float64)
		return ToastMsg{
			Text:    fmt.Sprintf("Context compacted (#%d)", int(compNum)),
			IsError: false,
		}

	case "subagent.completed":
		label, _ := props["label"].(string)
		agent, _ := props["agent"].(string)
		inputTokens, _ := intFromAny(props["inputTokens"])
		outputTokens, _ := intFromAny(props["outputTokens"])
		return SubagentCompletedMsg{
			ParentSessionID: sessionID,
			Label:           label,
			Agent:           agent,
			InputTokens:     inputTokens,
			OutputTokens:    outputTokens,
		}

	default:
		return SSEEventMsg{Event: evt}
	}
}

func parseMessageView(props map[string]any) MessageView {
	info, _ := props["info"].(map[string]any)
	if info == nil {
		return MessageView{}
	}

	mi := MessageInfo{}
	mi.ID, _ = info["id"].(string)
	mi.SessionID, _ = info["sessionID"].(string)
	mi.Role, _ = info["role"].(string)
	mi.Agent, _ = info["agent"].(string)
	if model, ok := info["model"].(map[string]any); ok {
		mi.ModelID, _ = model["modelID"].(string)
		mi.ProviderID, _ = model["providerID"].(string)
	}
	if mi.ModelID == "" {
		mi.ModelID, _ = info["modelID"].(string)
	}
	if mi.ProviderID == "" {
		mi.ProviderID, _ = info["providerID"].(string)
	}
	mi.CreatedAt, _ = info["createdAt"].(string)
	if mi.CreatedAt == "" {
		if t, ok := info["time"].(map[string]any); ok {
			if created, ok := t["created"].(float64); ok && created > 0 {
				mi.CreatedAt = time.UnixMilli(int64(created)).Format(time.RFC3339)
			}
		}
	}
	if tokens, ok := info["tokens"].(map[string]any); ok {
		mi.Tokens.Input, _ = intFromAny(tokens["input"])
		mi.Tokens.Output, _ = intFromAny(tokens["output"])
	}
	mi.Cost, _ = info["cost"].(float64)
	return MessageView{Info: mi}
}

func parsePartView(props map[string]any) PartView {
	part, _ := props["part"].(map[string]any)
	if part == nil {
		return PartView{}
	}

	pv := PartView{}
	pv.ID, _ = part["id"].(string)
	pv.SessionID, _ = part["sessionID"].(string)
	pv.MessageID, _ = part["messageID"].(string)
	pv.Type, _ = part["type"].(string)
	pv.Text, _ = part["text"].(string)

	// Unified tool part: read tool/callID and extract fields from state
	if pv.Type == "tool" {
		pv.ToolName, _ = part["tool"].(string)
		if state, ok := part["state"].(map[string]any); ok {
			status, _ := state["status"].(string)
			pv.ToolError = status == "error"
			if input, ok := state["input"].(map[string]any); ok {
				if args, ok := input["args"].(string); ok {
					pv.ToolArgs = args
				}
			}
			if output, ok := state["output"].(string); ok && pv.Text == "" {
				pv.Text = output
			}
			if status == "error" {
				if errStr, ok := state["error"].(string); ok && pv.Text == "" {
					pv.Text = errStr
				}
			}
			if t, ok := state["time"].(map[string]any); ok {
				pv.Time = t
			}
		}
	} else {
		// Legacy fields for non-tool parts
		if pv.Text == "" {
			if tr, ok := part["toolResult"].(string); ok {
				pv.Text = tr
			}
		}
		pv.ToolName, _ = part["toolName"].(string)
		pv.ToolArgs, _ = part["toolArgs"].(string)
		pv.ToolError, _ = part["toolError"].(bool)
		if t, ok := part["time"].(map[string]any); ok {
			pv.Time = t
		}
	}
	if label, ok := part["subagentLabel"].(string); ok {
		pv.SubagentLabel = label
	}
	return pv
}

// fetchSessions fetches the session list from the server.
func fetchSessions(client *api.Client, limit, offset int) tea.Cmd {
	return func() tea.Msg {
		sessions, err := client.ListSessions(limit, offset)
		if err != nil {
			return SessionListMsg{Err: err}
		}
		infos := make([]SessionInfo, len(sessions))
		for i, s := range sessions {
			infos[i] = sessionInfoFromAPI(s)
		}
		return SessionListMsg{Sessions: infos}
	}
}

func fetchProviders(client *api.Client) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.ListProviders()
		if err != nil {
			return ProvidersLoadedMsg{Err: err}
		}
		providers := make([]ProviderInfo, len(resp.All))
		for i, p := range resp.All {
			pi := ProviderInfo{ID: p.ID, Name: p.Name}
			for modelID, raw := range p.Models {
				pi.Models = append(pi.Models, parseModelInfo(modelID, p.ID, raw))
			}
			sort.Slice(pi.Models, func(a, b int) bool {
				return pi.Models[a].Name < pi.Models[b].Name
			})
			providers[i] = pi
		}
		msg := ProvidersLoadedMsg{Providers: providers}
		for providerID, modelID := range resp.Default {
			msg.DefaultProvider = providerID
			msg.DefaultModel = modelID
			break
		}
		return msg
	}
}

// parseModelInfo converts a raw model map from the API into a ModelInfo.
func parseModelInfo(modelID, providerID string, raw any) ModelInfo {
	m := ModelInfo{ID: modelID, ProviderID: providerID, Name: modelID}
	obj, ok := raw.(map[string]any)
	if !ok {
		return m
	}
	if name, ok := obj["name"].(string); ok && name != "" {
		m.Name = name
	}
	if id, ok := obj["id"].(string); ok && id != "" {
		m.ID = id
	}
	limit, _ := obj["limit"].(map[string]any)
	if ctx, ok := limit["context"].(float64); ok {
		m.ContextLimit = int(ctx)
	}
	cost, _ := obj["cost"].(map[string]any)
	m.CostInput, _ = cost["input"].(float64)
	m.CostOutput, _ = cost["output"].(float64)
	return m
}

// fetchMCPStatus fetches the MCP server status from the server.
func fetchMCPStatus(client *api.Client) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.GetMCPStatus()
		if err != nil {
			return MCPStatusLoadedMsg{Err: err}
		}
		var servers []MCPServer
		for name, raw := range resp {
			srv := MCPServer{Name: name}
			srv.Status, _ = raw["status"].(string)
			srv.Error, _ = raw["error"].(string)
			if tc, ok := intFromAny(raw["toolCount"]); ok {
				srv.ToolCount = tc
			}
			servers = append(servers, srv)
		}
		return MCPStatusLoadedMsg{Servers: servers}
	}
}

// reconnectMCP requests reconnection of a named MCP server.
func reconnectMCP(client *api.Client, name string) tea.Cmd {
	return func() tea.Msg {
		err := client.ReconnectMCP(name)
		return MCPReconnectResultMsg{Name: name, Err: err}
	}
}

// fetchAllAgents fetches all agents including disabled ones.
func fetchAllAgents(client *api.Client) tea.Cmd {
	return func() tea.Msg {
		agents, err := client.ListAllAgents()
		if err != nil {
			return AgentListMsg{Err: err}
		}
		return AgentListMsg{Agents: agents}
	}
}

// patchScopedModels sends a config patch to update the scoped models list.
func patchScopedModels(client *api.Client, models []string) tea.Cmd {
	return func() tea.Msg {
		body := map[string]any{
			"scopedModels": models,
		}
		err := client.PatchConfig(body)
		return ModelScopedDoneMsg{Err: err}
	}
}

// summarizeSession triggers manual context compaction via POST /session/{id}/summarize.
func summarizeSession(client *api.Client, sessionID string) tea.Cmd {
	return func() tea.Msg {
		err := client.SummarizeSession(sessionID)
		return CompactDoneMsg{Err: err}
	}
}

// branchSession creates a branch of the session via POST /session/{id}/fork.
func branchSession(client *api.Client, sessionID, title string) tea.Cmd {
	return func() tea.Msg {
		info, err := client.BranchSession(sessionID, title)
		if err != nil {
			return BranchDoneMsg{Err: err}
		}
		si := sessionInfoFromAPI(*info)
		return BranchDoneMsg{Session: &si}
	}
}

// rewindSession rewinds a session to a specific message via POST /session/{id}/rewind.
func rewindSession(client *api.Client, sessionID, messageID string, turnIndex int) tea.Cmd {
	return func() tea.Msg {
		err := client.RewindSession(sessionID, messageID)
		return RewindDoneMsg{TurnIndex: turnIndex, Err: err}
	}
}

// archiveSession archives a session via POST /session/{id}/archive.
func archiveSession(client *api.Client, sessionID string) tea.Cmd {
	return func() tea.Msg {
		err := client.ArchiveSession(sessionID)
		return ArchiveSentMsg{Err: err}
	}
}

// toggleAgent sends a config patch to toggle an agent's disabled state.
func toggleAgent(client *api.Client, name string, disabled bool) tea.Cmd {
	return func() tea.Msg {
		body := map[string]any{
			"agents": map[string]any{
				name: map[string]any{
					"disable": disabled,
				},
			},
		}
		err := client.PatchConfig(body)
		return AgentToggleDoneMsg{Err: err}
	}
}

// renameSession updates the session title via PATCH /session/{id}.
func renameSession(client *api.Client, sessionID, title string) tea.Cmd {
	return func() tea.Msg {
		err := client.UpdateSessionTitle(sessionID, title)
		return SessionRenamedMsg{Err: err}
	}
}

// fetchCommands fetches the command list from the server.
func fetchCommands(client *api.Client) tea.Cmd {
	return func() tea.Msg {
		commands, err := client.ListCommands()
		if err != nil {
			return CommandListMsg{Err: err}
		}
		return CommandListMsg{Commands: commands}
	}
}

// fetchProviderBalance fetches the balance for a provider.
func fetchProviderBalance(client *api.Client, providerID string) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.GetProviderBalance(providerID)
		if err != nil {
			return ProviderBalanceMsg{Err: err}
		}
		bal := &ProviderBalance{Provider: resp.Provider}
		if resp.Remaining != nil {
			bal.Remaining = *resp.Remaining
			bal.HasLimit = true
		}
		if resp.Usage != nil {
			bal.Usage = *resp.Usage
		}
		if !bal.HasLimit && bal.Usage == 0 {
			return ProviderBalanceMsg{}
		}
		return ProviderBalanceMsg{Balance: bal}
	}
}

func fetchPlugins(client *api.Client) tea.Cmd {
	return func() tea.Msg {
		plugins, err := client.ListPlugins()
		if err != nil {
			return PluginListMsg{Err: err}
		}
		return PluginListMsg{Plugins: plugins}
	}
}

// fetchMessages fetches messages for a session.
func fetchMessages(client *api.Client, sessionID string) tea.Cmd {
	return func() tea.Msg {
		messages, err := client.ListMessages(sessionID)
		if err != nil {
			return MessageListMsg{SessionID: sessionID, Err: err}
		}
		return MessageListMsg{SessionID: sessionID, Messages: messages}
	}
}

// createSession creates a new session on the server.
func createSession(client *api.Client, input api.SessionCreateInput) tea.Cmd {
	return func() tea.Msg {
		info, err := client.CreateSession(input)
		if err != nil {
			return SessionCreatedLocalMsg{Err: err}
		}
		si := sessionInfoFromAPI(*info)
		return SessionCreatedLocalMsg{Session: &si}
	}
}

// sendPrompt sends a prompt to the server.
func sendPrompt(client *api.Client, sessionID string, input api.PromptInput) tea.Cmd {
	return func() tea.Msg {
		err := client.SendPrompt(sessionID, input)
		return PromptSentMsg{Err: err}
	}
}

// abortSession sends an abort request to the server.
func abortSession(client *api.Client, sessionID string) tea.Cmd {
	return func() tea.Msg {
		err := client.AbortSession(sessionID)
		return AbortSentMsg{Err: err}
	}
}

// revertSession sends a revert request to the server.
func revertSession(client *api.Client, sessionID string) tea.Cmd {
	return func() tea.Msg {
		err := client.RevertSession(sessionID)
		return RevertSentMsg{Err: err}
	}
}

// unrevertSession sends an unrevert request to the server.
func unrevertSession(client *api.Client, sessionID string) tea.Cmd {
	return func() tea.Msg {
		err := client.UnrevertSession(sessionID)
		return UnrevertSentMsg{Err: err}
	}
}

// replyPermission sends a permission reply to the server.
func replyPermission(client *api.Client, sessionID, permissionID, action string) tea.Cmd {
	return func() tea.Msg {
		err := client.ReplyPermission(sessionID, permissionID, action)
		return PermissionRepliedMsg{Err: err}
	}
}

func sessionInfoFromAPI(s session.Info) SessionInfo {
	si := SessionInfo{
		ID:        s.ID,
		Title:     s.Title,
		Agent:     s.Agent,
		ParentID:  s.ParentID,
		Directory: s.Directory,
		CreatedAt: s.Time.Created,
		UpdatedAt: s.Time.Updated,
	}
	if s.Model != nil {
		si.ModelID = s.Model.ModelID
		si.ProviderID = s.Model.ProviderID
	}
	return si
}

// stringProp extracts a string value from a property map.
func stringProp(props map[string]any, key string) string {
	v, _ := props[key].(string)
	return v
}

// extractSessionError extracts a human-readable error message from a session.error
// SSE event. The server sends errors in two forms:
//   - plain string: props["error"] = "some message"
//   - structured payload: props["error"] = {"name": "...", "data": {"message": "..."}}
//
// This function handles both, preferring the structured data.message when available.
func extractSessionError(props map[string]any) string {
	raw := props["error"]
	if raw == nil {
		return ""
	}
	if s, ok := raw.(string); ok {
		return s
	}
	if obj, ok := raw.(map[string]any); ok {
		if data, ok := obj["data"].(map[string]any); ok {
			if msg, ok := data["message"].(string); ok && msg != "" {
				return msg
			}
		}
		// Fallback: try the name field
		if name, ok := obj["name"].(string); ok && name != "" {
			return name
		}
	}
	return ""
}

// intFromAny extracts an int from a JSON number (float64) or int.
func intFromAny(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	}
	return 0, false
}

func parseSessionInfo(props map[string]any) SessionInfo {
	info, _ := props["info"].(map[string]any)
	if info == nil {
		return SessionInfo{}
	}
	si := SessionInfo{}
	si.ID, _ = info["id"].(string)
	si.Title, _ = info["title"].(string)
	si.Agent, _ = info["agent"].(string)
	si.ParentID, _ = info["parentID"].(string)
	si.Directory, _ = info["directory"].(string)
	if m, ok := info["model"].(map[string]any); ok {
		si.ModelID, _ = m["id"].(string)
		si.ProviderID, _ = m["providerID"].(string)
	}
	if t, ok := info["time"].(map[string]any); ok {
		if c, ok := t["created"].(float64); ok {
			si.CreatedAt = int64(c)
		}
		if u, ok := t["updated"].(float64); ok {
			si.UpdatedAt = int64(u)
		}
	}
	return si
}

// runUserShell executes a shell command and returns a ShellResultMsg.
func runUserShell(command, dir string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "sh", "-c", command)
		cmd.Dir = dir
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		output := stdout.String()
		if stderr.Len() > 0 {
			if output != "" {
				output += "\n"
			}
			output += stderr.String()
		}
		return ShellResultMsg{Command: command, Output: output, Err: err}
	}
}
