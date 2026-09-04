package tui

// Shared state types for the TUI.
// NOTE: The foundation layer (state.go) is being created by another agent.
// These types should be consolidated with the canonical definitions once available.

// FocusTarget identifies which component has keyboard focus.
type FocusTarget int

const (
	FocusPrompt FocusTarget = iota
	FocusPalette
	FocusDialog
	FocusSidebar
	FocusPermission
)

// SessionStatus tracks the working/alert state of a session.
type SessionStatus struct {
	Working bool `json:"working"`
	Alert   bool `json:"alert"`
}

// ModelSelection identifies the currently selected model.
type ModelSelection struct {
	ProviderID string `json:"providerID"`
	ModelID    string `json:"modelID"`
}

// SessionInfo is the TUI's view of a session (mirrors session.Info).
type SessionInfo struct {
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	Agent     string          `json:"agent,omitempty"`
	Model     *ModelSelection `json:"model,omitempty"`
	ParentID  string          `json:"parentID,omitempty"`
	Time      TimeInfo        `json:"time"`
}

// TimeInfo holds created/updated timestamps.
type TimeInfo struct {
	Created int64 `json:"created"`
	Updated int64 `json:"updated"`
}

// ProviderInfo is the TUI's view of a provider.
type ProviderInfo struct {
	ID     string      `json:"id"`
	Name   string      `json:"name"`
	Models []ModelInfo `json:"models"`
}

// ModelInfo is the TUI's view of a model.
type ModelInfo struct {
	ID         string `json:"id"`
	ProviderID string `json:"providerID"`
	Name       string `json:"name"`
}

// MessageInfo is the SSE-sourced message metadata.
type MessageInfo struct {
	ID         string `json:"id"`
	SessionID  string `json:"sessionID"`
	Role       string `json:"role"`
	Agent      string `json:"agent,omitempty"`
	ModelID    string `json:"modelID,omitempty"`
	ProviderID string `json:"providerID,omitempty"`
}

// MessageView is a message with its rendered parts.
type MessageView struct {
	Info  MessageInfo
	Parts []PartView
}

// PartView is the TUI's view of a message part.
type PartView struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionID"`
	MessageID string `json:"messageID"`
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	ToolName  string `json:"toolName,omitempty"`
	ToolArgs  string `json:"toolArgs,omitempty"`
	ToolError bool   `json:"toolError,omitempty"`
	Time      *struct {
		Start int64 `json:"start,omitempty"`
		End   int64 `json:"end,omitempty"`
	} `json:"time,omitempty"`
}

// AppState holds all shared state for the TUI, mutated only through Update.
type AppState struct {
	ActiveSession string
	Sessions      []SessionInfo
	Messages      map[string][]MessageView
	Providers     []ProviderInfo
	SessionStatus map[string]SessionStatus
	CurrentAgent  string
	CurrentModel  ModelSelection
	SidebarOpen   bool
	Connected     bool
}

// NewAppState creates an initialized AppState.
func NewAppState() *AppState {
	return &AppState{
		Messages:      make(map[string][]MessageView),
		SessionStatus: make(map[string]SessionStatus),
		CurrentAgent:  "build",
	}
}
