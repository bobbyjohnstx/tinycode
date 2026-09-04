package tui

import "github.com/bobbyjohnstx/tinycode-go/internal/tui/api"

// --- SSE event messages ---

// SessionCreatedMsg is received when a new session is created on the server.
type SessionCreatedMsg struct {
	SessionID string
	Info      map[string]any
}

// SessionDeletedMsg is received when a session is deleted on the server.
type SessionDeletedMsg struct {
	SessionID string
}

// SessionStatusMsg is received when a session's working status changes.
type SessionStatusMsg struct {
	SessionID string
	Status    SessionStatus
}

// MessageUpdatedMsg is received when a message is created or updated.
type MessageUpdatedMsg struct {
	SessionID string
	Info      MessageView
}

// MessagePartUpdatedMsg is received when a message part is created or updated.
type MessagePartUpdatedMsg struct {
	SessionID string
	Part      PartView
}

// MessagePartDeltaMsg is received for streaming text deltas.
type MessagePartDeltaMsg struct {
	SessionID string
	MessageID string
	PartID    string
	Field     string
	Delta     string
}

// SSEConnectedMsg indicates the SSE connection was established.
type SSEConnectedMsg struct{}

// SSEDisconnectedMsg indicates the SSE connection was lost.
type SSEDisconnectedMsg struct {
	Err error
}

// SSEEventMsg wraps a raw server event for initial dispatch.
type SSEEventMsg struct {
	Event api.ServerEvent
}

// --- API response messages ---

// SessionListMsg carries the result of listing sessions.
type SessionListMsg struct {
	Sessions []SessionInfo
	Err      error
}

// SessionCreatedLocalMsg carries the result of creating a session locally.
type SessionCreatedLocalMsg struct {
	Session *SessionInfo
	Err     error
}

// ProviderListMsg carries the result of listing providers.
type ProviderListMsg struct {
	Providers *api.ProviderListResponse
	Err       error
}

// AgentListMsg carries the result of listing agents.
type AgentListMsg struct {
	Agents []api.AgentInfo
	Err    error
}

// CommandListMsg carries the result of listing commands.
type CommandListMsg struct {
	Commands []api.CommandInfo
	Err      error
}

// MessageListMsg carries the result of listing messages for a session.
type MessageListMsg struct {
	SessionID string
	Messages  []map[string]any
	Err       error
}

// PromptSentMsg indicates a prompt was submitted to the server.
type PromptSentMsg struct {
	Err error
}

// AbortSentMsg indicates an abort was submitted to the server.
type AbortSentMsg struct {
	Err error
}

// PermissionRepliedMsg indicates a permission reply was submitted.
type PermissionRepliedMsg struct {
	Err error
}

// --- UI messages ---

// NavigateMsg requests navigation to a different route.
type NavigateMsg struct {
	Route Route
}

// FocusMsg requests focus change to a different component.
type FocusMsg struct {
	Target FocusTarget
}

// ToastMsg shows an ephemeral notification.
type ToastMsg struct {
	Text    string
	IsError bool
}

// WindowSizeMsg is received when the terminal is resized.
type WindowSizeMsg struct {
	Width  int
	Height int
}
