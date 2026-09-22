package tui

import "github.com/bobbyjohnstx/tinycode-go/internal/tui/api"

// --- SSE event messages ---

type SessionCreatedMsg struct {
	SessionID string
	Info      SessionInfo
}

type SessionUpdatedMsg struct {
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
	Providers       []ProviderInfo
	DefaultProvider string
	DefaultModel    string
	Err             error
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

type AbortRequestMsg struct{}

type PermissionRepliedMsg struct {
	Err error
}

// PermissionReplyMsg is a request to send a permission reply to the server.
type PermissionReplyMsg struct {
	SessionID    string
	PermissionID string
	Action       string
}

// SessionErrorMsg is emitted when a session-scoped error arrives via SSE.
// It carries both an error message (for toast) and session ID (to clear working state).
type SessionErrorMsg struct {
	SessionID string
	Error     string
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

// ToggleThoughtMsg toggles the expanded state of a reasoning part.
// If PartID is empty, toggles all reasoning parts in the active session.
type ToggleThoughtMsg struct {
	PartID string
}

// ToggleSubagentMsg toggles the expanded state of a subagent group.
// If Label is empty, toggles all subagent groups.
type ToggleSubagentMsg struct {
	Label string
}

// SubagentCompletedMsg carries completion data for a subagent.
type SubagentCompletedMsg struct {
	ParentSessionID string
	Label           string
	Agent           string
	InputTokens     int
	OutputTokens    int
}

type TickMsg struct{}

// ShellResultMsg carries the output of a user-initiated `! command`.
type ShellResultMsg struct {
	Command string
	Output  string
	Err     error
}

// MCPStatusMsg carries a single MCP server status update from an SSE event.
type MCPStatusMsg struct {
	Server MCPServer
}

// MCPStatusLoadedMsg carries the full MCP status from the initial API fetch.
type MCPStatusLoadedMsg struct {
	Servers []MCPServer
	Err     error
}

// MCPReconnectResultMsg carries the result of an MCP reconnect request.
type MCPReconnectResultMsg struct {
	Name string
	Err  error
}

// PluginListMsg carries the loaded plugin list from the server.
type PluginListMsg struct {
	Plugins []api.PluginInfo
	Err     error
}

// ProvidersRefreshMsg signals that the provider list should be re-fetched.
type ProvidersRefreshMsg struct{}

// ProviderBalanceMsg carries the balance response for a provider.
type ProviderBalanceMsg struct {
	Balance *ProviderBalance
	Err     error
}

// AgentToggleMsg requests toggling an agent's disabled state.
type AgentToggleMsg struct {
	Agent    string
	Disabled bool
}

// AgentToggleDoneMsg reports that the toggle config write completed.
type AgentToggleDoneMsg struct {
	Err error
}

// SessionRenamedMsg reports that a session rename completed.
type SessionRenamedMsg struct {
	Err error
}

// EditorRequestMsg requests opening $EDITOR. If FilePath is set, open that file
// directly (changes are saved in-place). Otherwise, open a temp file with Content.
type EditorRequestMsg struct {
	Content  string
	FilePath string
}

// EditorDoneMsg reports that the external editor process has exited.
type EditorDoneMsg struct {
	Content string
	Err     error
}

// ShellSessionRequestMsg requests opening an interactive shell.
type ShellSessionRequestMsg struct{}

// ShellSessionDoneMsg reports that the interactive shell has exited.
type ShellSessionDoneMsg struct {
	Err error
}

// DiffRequestMsg requests showing uncommitted changes via git diff.
type DiffRequestMsg struct {
	Dir string
}

// DiffDoneMsg reports that the diff pager has exited.
type DiffDoneMsg struct {
	Err error
}

// ModelScopedMsg requests updating the scoped models list.
type ModelScopedMsg struct {
	ScopedModels []string
}

// ModelScopedDoneMsg reports that the scoped models config write completed.
type ModelScopedDoneMsg struct {
	Err error
}

// RevertRequestMsg requests reverting the active session's file changes.
type RevertRequestMsg struct{}

// RevertSentMsg reports that the revert API call completed.
type RevertSentMsg struct {
	Err error
}

// UnrevertRequestMsg requests restoring previously reverted file changes.
type UnrevertRequestMsg struct{}

// UnrevertSentMsg reports that the unrevert API call completed.
type UnrevertSentMsg struct {
	Err error
}

// Aliases for cmd.go compatibility.
type SessionListMsg = SessionsLoadedMsg
type MessageListMsg = MessagesLoadedMsg
type ProviderListMsg = ProvidersLoadedMsg
type FocusMsg = FocusChangedMsg
