package tui

import "github.com/bobbyjohnstx/tinycode-go/internal/tui/api"

// --- SSE event messages ---

type SessionCreatedMsg struct {
	SessionID string
	Info      SessionInfo
}

type SessionDeletedMsg struct {
	SessionID string
}

type SessionStatusMsg struct {
	SessionID string
	Status    SessionStatus
}

type MessageUpdatedMsg struct {
	SessionID string
	Message   MessageView
}

type MessagePartUpdatedMsg struct {
	SessionID string
	Part      PartView
}

type MessagePartDeltaMsg struct {
	SessionID string
	MessageID string
	PartID    string
	Field     string
	Delta     string
}

type SSEConnectedMsg struct{}

type SSEDisconnectedMsg struct {
	Err error
}

type SSEEventMsg struct {
	Event api.ServerEvent
}

// --- API response messages ---

type SessionsLoadedMsg struct {
	Sessions []SessionInfo
	Err      error
}

type SessionCreatedLocalMsg struct {
	Session *SessionInfo
	Err     error
}

type ProvidersLoadedMsg struct {
	Providers []ProviderInfo
	Err       error
}

type AgentListMsg struct {
	Agents []api.AgentInfo
	Err    error
}

type CommandListMsg struct {
	Commands []api.CommandInfo
	Err      error
}

type MessagesLoadedMsg struct {
	SessionID string
	Messages  []map[string]any
	Err       error
}

type PromptSentMsg struct {
	Err error
}

type AbortSentMsg struct {
	Err error
}

type PermissionRepliedMsg struct {
	Err error
}

// --- UI messages ---

type PromptSubmittedMsg struct {
	Content string
}

type SessionSwitchedMsg struct {
	SessionID string
}

type FocusChangedMsg struct {
	Target FocusTarget
}

type NavigateMsg struct {
	Route Route
}

type ToastMsg struct {
	Text    string
	IsError bool
}

type WindowSizeMsg struct {
	Width  int
	Height int
}

type TickMsg struct{}

// ProvidersRefreshMsg signals that the provider list should be re-fetched.
type ProvidersRefreshMsg struct{}

// Aliases for cmd.go compatibility.
type SessionListMsg = SessionsLoadedMsg
type MessageListMsg = MessagesLoadedMsg
type ProviderListMsg = ProvidersLoadedMsg
type FocusMsg = FocusChangedMsg
