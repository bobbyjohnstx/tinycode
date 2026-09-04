package tui

import "github.com/bobbyjohnstx/tinycode-go/internal/tui/api"

// Route identifies which top-level view is active.
type Route int

const (
	RouteChat Route = iota
	RouteSessionList
)

// FocusTarget identifies which component has keyboard focus.
type FocusTarget int

const (
	FocusPrompt FocusTarget = iota
	FocusPalette
	FocusDialog
	FocusSidebar
	FocusPermission
)

// SessionStatus tracks the working state of a session.
type SessionStatus struct {
	Working bool `json:"working"`
	Alert   bool `json:"alert"`
}

// ModelSelection identifies the currently selected model.
type ModelSelection struct {
	ProviderID string
	ModelID    string
}

// MessageView represents a message in the conversation.
type MessageView struct {
	ID         string         `json:"id"`
	SessionID  string         `json:"sessionID"`
	Role       string         `json:"role"`
	Agent      string         `json:"agent,omitempty"`
	ParentID   string         `json:"parentID,omitempty"`
	ModelID    string         `json:"modelID,omitempty"`
	ProviderID string         `json:"providerID,omitempty"`
	Mode       string         `json:"mode,omitempty"`
	Cost       float64        `json:"cost,omitempty"`
	Tokens     map[string]any `json:"tokens,omitempty"`
	Time       map[string]any `json:"time,omitempty"`
	Error      map[string]any `json:"error,omitempty"`
}

// PartView represents a message part (text, tool call, etc.).
type PartView struct {
	ID        string         `json:"id"`
	SessionID string         `json:"sessionID"`
	MessageID string         `json:"messageID"`
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	Time      map[string]any `json:"time,omitempty"`
}

// PermissionRequest represents a pending permission prompt.
type PermissionRequest struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionID"`
	Tool      string `json:"tool"`
	Input     any    `json:"input"`
}

// AppState holds all shared TUI state, mutated only through the Update path.
type AppState struct {
	Route         Route
	ActiveSession string

	// Server-synced data
	Sessions      []SessionInfo
	Messages      map[string][]MessageView
	Parts         map[string][]PartView
	Providers     []api.ProviderInfo
	Agents        []api.AgentInfo
	Commands      []api.CommandInfo
	Permissions   map[string][]PermissionRequest
	SessionStatus map[string]SessionStatus

	// Local UI state
	CurrentAgent string
	CurrentModel ModelSelection
	SidebarOpen  bool
	Focus        FocusTarget
}

// SessionInfo is a lightweight view of session.Info for the TUI layer,
// avoiding a direct import of the storage-coupled session package in UI state.
type SessionInfo struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Agent     string `json:"agent,omitempty"`
	ParentID  string `json:"parentID,omitempty"`
	Directory string `json:"directory"`
	CreatedAt int64  `json:"created"`
	UpdatedAt int64  `json:"updated"`
}

// NewAppState returns an AppState with initialized maps.
func NewAppState() AppState {
	return AppState{
		Route:         RouteChat,
		Messages:      make(map[string][]MessageView),
		Parts:         make(map[string][]PartView),
		Permissions:   make(map[string][]PermissionRequest),
		SessionStatus: make(map[string]SessionStatus),
	}
}
