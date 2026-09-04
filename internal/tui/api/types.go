package api

import "github.com/bobbyjohnstx/tinycode-go/internal/session"

// PromptInput is the request body for POST /session/{id}/prompt_async.
type PromptInput struct {
	MessageID string       `json:"messageID,omitempty"`
	Model     *PromptModel `json:"model,omitempty"`
	Agent     string       `json:"agent,omitempty"`
	Parts     []PromptPart `json:"parts"`
	Variant   string       `json:"variant,omitempty"`
}

// PromptModel identifies a model for the prompt request.
type PromptModel struct {
	ProviderID string `json:"providerID"`
	ModelID    string `json:"modelID"`
}

// PromptPart is a content part within a prompt request.
type PromptPart struct {
	Type    string `json:"type"`
	Content string `json:"content,omitempty"`
	Text    string `json:"text,omitempty"`
}

// SessionCreateInput is the request body for POST /session.
type SessionCreateInput struct {
	ParentID string            `json:"parentID,omitempty"`
	Title    string            `json:"title,omitempty"`
	Agent    string            `json:"agent,omitempty"`
	Model    *session.ModelRef `json:"model,omitempty"`
}

// ProviderListResponse is the response from GET /provider.
type ProviderListResponse struct {
	All       []ProviderInfo    `json:"all"`
	Connected []string          `json:"connected"`
	Default   map[string]string `json:"default"`
}

// ProviderInfo mirrors provider.Info for client-side use.
type ProviderInfo struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Source  string            `json:"source"`
	Env     []string          `json:"env"`
	Options map[string]any    `json:"options"`
	Models  map[string]any    `json:"models"`
}

// AgentInfo is a single agent returned by GET /agent.
type AgentInfo struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Mode        string         `json:"mode"`
	Native      bool           `json:"native,omitempty"`
	Permission  map[string]any `json:"permission,omitempty"`
}

// CommandInfo is a single command returned by GET /command.
type CommandInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Source      string   `json:"source,omitempty"`
	Template    string   `json:"template,omitempty"`
	Subtask     bool     `json:"subtask,omitempty"`
	Hints       []string `json:"hints,omitempty"`
}

// PermissionReplyInput is the request body for POST /session/{sessionID}/permissions/{permissionID}.
type PermissionReplyInput struct {
	Action string `json:"action"`
}
