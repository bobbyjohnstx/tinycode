package lsp

// Config holds user-configurable LSP settings.
type Config struct {
	Enabled *bool                   `json:"enabled,omitempty"`
	Servers map[string]ServerConfig `json:"servers,omitempty"`
	Timeout *int                    `json:"timeout,omitempty"`
}

// ServerConfig holds per-language server overrides.
type ServerConfig struct {
	Command  string            `json:"command"`
	Args     []string          `json:"args,omitempty"`
	Disabled *bool             `json:"disabled,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
}
