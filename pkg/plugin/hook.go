package plugin

import "context"

// HookHandlers holds optional callback functions for plugin lifecycle hooks.
// Each field is nil when the plugin does not handle that hook.
type HookHandlers struct {
	SessionStart   func(ctx context.Context, event SessionStartEvent) (*SessionStartOutput, error)
	SessionEnd     func(ctx context.Context, event SessionEndEvent) error
	PermissionAsk  func(ctx context.Context, input PermissionInput) (*PermissionOutput, error)
	ShellEnv       func(ctx context.Context, input ShellEnvInput) (*ShellEnvOutput, error)
	ToolExecBefore func(ctx context.Context, input ToolExecBeforeInput) (*ToolExecBeforeOutput, error)
	ToolExecAfter  func(ctx context.Context, input ToolExecAfterInput) (*ToolExecAfterOutput, error)
	Dispose        func(ctx context.Context) error
}

// SessionStartOutput is the optional response from a session.start hook.
type SessionStartOutput struct {
	AdditionalContext []string `json:"additionalContext,omitempty"`
}

// ToolExecBeforeOutput is the optional response from a tool.execute.before hook.
type ToolExecBeforeOutput struct {
	AdditionalContext []string `json:"additionalContext,omitempty"`
}

// SessionStartEvent is emitted when a new session is created.
type SessionStartEvent struct {
	SessionID string `json:"sessionId"`
	Directory string `json:"directory"`
}

// SessionEndEvent is emitted when a session is destroyed.
type SessionEndEvent struct {
	SessionID string `json:"sessionId"`
}

// PermissionInput is the payload for permission.ask hooks.
type PermissionInput struct {
	SessionID  string `json:"sessionId"`
	ToolName   string `json:"toolName"`
	ToolArgs   string `json:"toolArgs"`
	Permission string `json:"permission"`
}

// PermissionOutput is the response from a permission.ask hook.
type PermissionOutput struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

// ShellEnvInput is the payload for shell.env hooks.
type ShellEnvInput struct {
	SessionID string            `json:"sessionId"`
	Directory string            `json:"directory"`
	Env       map[string]string `json:"env,omitempty"`
}

// ShellEnvOutput is the response from a shell.env hook.
type ShellEnvOutput struct {
	Env map[string]string `json:"env"`
}

// ToolExecBeforeInput is the payload for tool.execute.before hooks.
type ToolExecBeforeInput struct {
	SessionID string `json:"sessionId"`
	ToolName  string `json:"toolName"`
	ToolArgs  string `json:"toolArgs"`
}

// ToolExecAfterInput is the payload for tool.execute.after hooks.
type ToolExecAfterInput struct {
	SessionID string `json:"sessionId"`
	ToolName  string `json:"toolName"`
	Output    string `json:"output"`
	IsError   bool   `json:"isError"`
}

// ToolExecAfterOutput is the response from a tool.execute.after hook.
// If non-nil, the Output field replaces the original tool output.
type ToolExecAfterOutput struct {
	Output            string   `json:"output"`
	IsError           bool     `json:"isError"`
	AdditionalContext []string `json:"additionalContext,omitempty"`
}
