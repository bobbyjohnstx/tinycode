package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bobbyjohnstx/tinycode-go/internal/session"
	"github.com/bobbyjohnstx/tinycode-go/internal/tui/api"
)

// listenSSE establishes the SSE connection and returns the first event.
func listenSSE(ctx context.Context, client *api.Client) tea.Cmd {
	return func() tea.Msg {
		events, err := client.Subscribe(ctx)
		if err != nil {
			return SSEDisconnectedMsg{Err: err}
		}
		evt, ok := <-events
		if !ok {
			return SSEDisconnectedMsg{}
		}
		return SSEEventMsg{Event: evt}
	}
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

	case "session.deleted":
		return SessionDeletedMsg{
			SessionID: sessionID,
		}

	case "session.status":
		status := SessionStatus{}
		if statusMap, ok := props["status"].(map[string]any); ok {
			status.Working, _ = statusMap["working"].(bool)
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
		errMsg := stringProp(props, "error")
		if errMsg == "" {
			errMsg = "unknown error"
		}
		return ToastMsg{Text: errMsg, IsError: true}

	case "permission.asked":
		return PermissionRequestedMsg{
			Request: PermissionRequest{
				ID:        stringProp(props, "id"),
				SessionID: sessionID,
				Tool:      stringProp(props, "tool"),
				Input:     props["input"],
			},
		}

	case "provider.updated":
		return ProvidersRefreshMsg{}

	case "session.compacted":
		compNum, _ := props["compactionNum"].(float64)
		return ToastMsg{
			Text:    fmt.Sprintf("Context compacted (#%d)", int(compNum)),
			IsError: false,
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
	mi.ModelID, _ = info["modelID"].(string)
	mi.ProviderID, _ = info["providerID"].(string)
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
	pv.ToolName, _ = part["toolName"].(string)
	pv.ToolArgs, _ = part["toolArgs"].(string)
	pv.ToolError, _ = part["toolError"].(bool)
	if t, ok := part["time"].(map[string]any); ok {
		pv.Time = t
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
			providers[i] = ProviderInfo{ID: p.ID, Name: p.Name}
		}
		return ProvidersLoadedMsg{Providers: providers}
	}
}

// fetchAgents fetches the agent list from the server.
func fetchAgents(client *api.Client) tea.Cmd {
	return func() tea.Msg {
		agents, err := client.ListAgents()
		if err != nil {
			return AgentListMsg{Err: err}
		}
		return AgentListMsg{Agents: agents}
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
		si.ModelID = s.Model.ID
		si.ProviderID = s.Model.ProviderID
	}
	return si
}

// stringProp extracts a string value from a property map.
func stringProp(props map[string]any, key string) string {
	v, _ := props[key].(string)
	return v
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
