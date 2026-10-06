package plugin

import "encoding/json"

// JSONRPCRequest is a JSON-RPC 2.0 request message sent between the tinycode
// server and plugin processes over stdin/stdout.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse is a JSON-RPC 2.0 response message.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError represents a JSON-RPC 2.0 error object.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// InitializeParams is sent by the server to a plugin during the initialize
// handshake.
type InitializeParams struct {
	Version   string         `json:"version"`
	Directory string         `json:"directory"`
	Options   map[string]any `json:"options,omitempty"`
}

// InitializeResult is the plugin's response to the initialize request,
// declaring the tools and hooks it provides.
type InitializeResult struct {
	ID    string         `json:"id"`
	Tools []ToolManifest `json:"tools"`
	Hooks []string       `json:"hooks"`
}

// ToolManifest describes a tool exposed by a plugin.
type ToolManifest struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
	Permission  string         `json:"permission,omitempty"`
}

// ToolCallParams is sent by the server to invoke a plugin tool.
type ToolCallParams struct {
	Name    string          `json:"name"`
	Args    json.RawMessage `json:"args"`
	Context ToolContext      `json:"context"`
}

// ToolContext provides session context to a tool execution.
type ToolContext struct {
	SessionID string `json:"sessionId"`
	Directory string `json:"directory"`
}

// ToolCallResult is the plugin's response to a tool call.
type ToolCallResult struct {
	Content string `json:"content"`
	IsError bool   `json:"isError,omitempty"`
}

// HookParams is sent by the server to invoke a plugin hook.
type HookParams struct {
	Name   string          `json:"name"`
	Input  json.RawMessage `json:"input,omitempty"`
	Output json.RawMessage `json:"output,omitempty"`
}

// HookResult is the plugin's response to a hook invocation.
type HookResult struct {
	Output json.RawMessage `json:"output,omitempty"`
}
