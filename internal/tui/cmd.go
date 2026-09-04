package tui

import (
	"context"

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
			Info:      props,
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
			Info:      parseMessageView(props),
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

	default:
		return SSEEventMsg{Event: evt}
	}
}

func parseMessageView(props map[string]any) MessageView {
	info, _ := props["info"].(map[string]any)
	if info == nil {
		return MessageView{}
	}

	mv := MessageView{}
	mv.ID, _ = info["id"].(string)
	mv.SessionID, _ = info["sessionID"].(string)
	mv.Role, _ = info["role"].(string)
	mv.Agent, _ = info["agent"].(string)
	mv.ParentID, _ = info["parentID"].(string)
	mv.ModelID, _ = info["modelID"].(string)
	mv.ProviderID, _ = info["providerID"].(string)
	mv.Mode, _ = info["mode"].(string)
	mv.Cost, _ = info["cost"].(float64)
	if tokens, ok := info["tokens"].(map[string]any); ok {
		mv.Tokens = tokens
	}
	if t, ok := info["time"].(map[string]any); ok {
		mv.Time = t
	}
	if e, ok := info["error"].(map[string]any); ok {
		mv.Error = e
	}
	return mv
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

// fetchProviders fetches the provider list from the server.
func fetchProviders(client *api.Client) tea.Cmd {
	return func() tea.Msg {
		providers, err := client.ListProviders()
		if err != nil {
			return ProviderListMsg{Err: err}
		}
		return ProviderListMsg{Providers: providers}
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
	return SessionInfo{
		ID:        s.ID,
		Title:     s.Title,
		Agent:     s.Agent,
		ParentID:  s.ParentID,
		Directory: s.Directory,
		CreatedAt: s.Time.Created,
		UpdatedAt: s.Time.Updated,
	}
}
