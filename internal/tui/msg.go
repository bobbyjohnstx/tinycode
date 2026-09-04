package tui

// Message types for bubbletea Update dispatch.
// NOTE: The foundation layer (msg.go) is being created by another agent.
// These types should be consolidated with the canonical definitions once available.

import tea "github.com/charmbracelet/bubbletea"

// --- SSE-sourced messages ---

// MessagePartDeltaMsg is sent when streaming text arrives for a message part.
type MessagePartDeltaMsg struct {
	SessionID string
	MessageID string
	PartID    string
	Field     string
	Delta     string
}

// MessageUpdatedMsg is sent when a full message info is received.
type MessageUpdatedMsg struct {
	SessionID string
	Info      MessageInfo
}

// MessagePartUpdatedMsg is sent when a full part is received.
type MessagePartUpdatedMsg struct {
	SessionID string
	Part      PartView
	Time      int64
}

// SessionStatusMsg reports working/alert state for a session.
type SessionStatusMsg struct {
	SessionID string
	Status    SessionStatus
}

// SessionCreatedMsg is sent when a new session is created.
type SessionCreatedMsg struct {
	SessionID string
	Info      SessionInfo
}

// SessionDeletedMsg is sent when a session is deleted.
type SessionDeletedMsg struct {
	SessionID string
}

// --- API response messages ---

// SessionsLoadedMsg carries the result of listing sessions.
type SessionsLoadedMsg struct {
	Sessions []SessionInfo
	Err      error
}

// MessagesLoadedMsg carries the result of loading messages for a session.
type MessagesLoadedMsg struct {
	SessionID string
	Messages  []MessageView
	Err       error
}

// ProvidersLoadedMsg carries discovered providers.
type ProvidersLoadedMsg struct {
	Providers []ProviderInfo
	Err       error
}

// PromptSubmittedMsg is sent when the user submits a prompt.
type PromptSubmittedMsg struct {
	Content string
}

// SSEConnectedMsg indicates the SSE connection is established.
type SSEConnectedMsg struct{}

// SSEDisconnectedMsg indicates the SSE connection dropped.
type SSEDisconnectedMsg struct {
	Err error
}

// SSEEventMsg wraps a raw SSE event for dispatch.
type SSEEventMsg struct {
	Type string
	Data []byte
}

// --- Internal UI messages ---

// FocusChangedMsg requests a focus change.
type FocusChangedMsg struct {
	Target FocusTarget
}

// SessionSwitchedMsg requests switching to a different session.
type SessionSwitchedMsg struct {
	SessionID string
}

// TickMsg is used for spinner and other animations.
type TickMsg struct{}

// errMsg wraps an error for display.
type errMsg struct {
	err error
}

// --- Cmd constructors ---

func switchSession(id string) tea.Cmd {
	return func() tea.Msg {
		return SessionSwitchedMsg{SessionID: id}
	}
}

func changeFocus(target FocusTarget) tea.Cmd {
	return func() tea.Msg {
		return FocusChangedMsg{Target: target}
	}
}
