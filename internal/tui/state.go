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

// MessageInfo is the metadata of a message (role, model, etc.).
type MessageInfo struct {
	ID         string    `json:"id"`
	SessionID  string    `json:"sessionID"`
	Role       string    `json:"role"`
	Agent      string    `json:"agent,omitempty"`
	ModelID    string    `json:"modelID,omitempty"`
	ProviderID string    `json:"providerID,omitempty"`
	CreatedAt  string    `json:"createdAt,omitempty"`
	Tokens     TokenInfo `json:"tokens,omitempty"`
	Cost       float64   `json:"cost,omitempty"`
}

// TokenInfo holds token usage for a single message.
type TokenInfo struct {
	Input  int `json:"input"`
	Output int `json:"output"`
}

// MessageView represents a message with its parts.
type MessageView struct {
	Info  MessageInfo
	Parts []PartView
}

// PartView represents a message part (text, tool call, etc.).
type PartView struct {
	ID        string         `json:"id"`
	SessionID string         `json:"sessionID"`
	MessageID string         `json:"messageID"`
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	ToolName  string         `json:"toolName,omitempty"`
	ToolArgs  string         `json:"toolArgs,omitempty"`
	ToolError bool           `json:"toolError,omitempty"`
	Time      map[string]any `json:"time,omitempty"`

	// Streaming is true while deltas are still arriving for this part.
	Streaming bool `json:"-"`
	// Collapsed hides tool result output when true.
	Collapsed bool `json:"-"`
	// ThoughtExpanded shows reasoning content when true.
	ThoughtExpanded bool `json:"-"`
	// SubagentLabel identifies which subagent group this part belongs to.
	SubagentLabel string `json:"-"`
}

// SubagentStatus holds completion data for a subagent.
type SubagentStatus struct {
	Label        string
	Agent        string
	InputTokens  int
	OutputTokens int
	Done         bool
}

// ProviderInfo is the TUI's view of a provider.
type ProviderInfo struct {
	ID     string      `json:"id"`
	Name   string      `json:"name"`
	Models []ModelInfo `json:"models"`
}

// ModelInfo is the TUI's view of a model.
type ModelInfo struct {
	ID           string  `json:"id"`
	ProviderID   string  `json:"providerID"`
	Name         string  `json:"name"`
	ContextLimit int     `json:"contextLimit,omitempty"`
	CostInput    float64 `json:"costInput,omitempty"`
	CostOutput   float64 `json:"costOutput,omitempty"`
}

// lookupModelDisplay returns the display name and provider name for a model,
// falling back to the raw IDs if not found.
func lookupModelDisplay(providers []ProviderInfo, providerID, modelID string) (modelName, providerName string) {
	modelName = modelID
	providerName = providerID
	for _, p := range providers {
		if p.ID != providerID {
			continue
		}
		providerName = p.Name
		for _, m := range p.Models {
			if m.ID == modelID {
				modelName = m.Name
				break
			}
		}
		break
	}
	return
}

// PermissionRequest represents a pending permission prompt.
type PermissionRequest struct {
	ID         string         `json:"id"`
	SessionID  string         `json:"sessionID"`
	Permission string         `json:"permission"`
	Metadata   map[string]any `json:"metadata"`
}

// AppState holds all shared TUI state, mutated only through the Update path.
type AppState struct {
	ActiveSession string

	// Server-synced data
	Sessions      []SessionInfo
	Messages      map[string][]MessageView
	Providers     []ProviderInfo
	Agents        []api.AgentInfo
	Commands      []api.CommandInfo
	Plugins       []api.PluginInfo
	SessionStatus map[string]SessionStatus

	// Local UI state
	CurrentAgent       string
	CurrentModel       ModelSelection
	CurrentTheme       string
	SidebarOpen        bool
	Connected          bool
	AutoApprove        bool
	PendingModelDialog bool
}

// SessionInfo is a lightweight view of session.Info for the TUI layer,
// avoiding a direct import of the storage-coupled session package in UI state.
type SessionInfo struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Agent      string `json:"agent,omitempty"`
	ModelID    string `json:"modelID,omitempty"`
	ProviderID string `json:"providerID,omitempty"`
	ParentID   string `json:"parentID,omitempty"`
	Directory  string `json:"directory"`
	CreatedAt  int64  `json:"created"`
	UpdatedAt  int64  `json:"updated"`
}

// NewAppState returns an AppState with initialized maps.
func NewAppState() *AppState {
	return &AppState{
		Messages:      make(map[string][]MessageView),
		SessionStatus: make(map[string]SessionStatus),
	}
}
