package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// Info holds the merged tinycode configuration. Fields use pointer types or
// omitempty to support forward compatibility: unknown fields are silently
// ignored during unmarshaling.
type Info struct {
	Shell             string                    `json:"shell,omitempty"`
	LogLevel          string                    `json:"logLevel,omitempty"`
	Model             string                    `json:"model,omitempty"`
	SmallModel        string                    `json:"small_model,omitempty"`
	DefaultAgent      string                    `json:"default_agent,omitempty"`
	SubagentDepth     *int                      `json:"subagent_depth,omitempty"`
	Username          string                    `json:"username,omitempty"`
	Share             string                    `json:"share,omitempty"`
	Snapshot          *bool                     `json:"snapshot,omitempty"`
	DisabledProviders []string                  `json:"disabled_providers,omitempty"`
	EnabledProviders  []string                  `json:"enabled_providers,omitempty"`
	Server            *ServerConfig             `json:"server,omitempty"`
	Provider          map[string]ProviderConfig `json:"provider,omitempty"`
	Permission        *PermissionConfig         `json:"permission,omitempty"`
	ToolOutput        *ToolOutputConfig         `json:"tool_output,omitempty"`
	Compaction        *CompactionConfig         `json:"compaction,omitempty"`
	Instructions      []string                  `json:"instructions,omitempty"`
	Plugins           []json.RawMessage         `json:"plugins,omitempty"`
	MCP               map[string]MCPConfig      `json:"mcp,omitempty"`
}

type MCPConfig struct {
	Command   string            `json:"-"`
	Args      []string          `json:"-"`
	Env       map[string]string `json:"env,omitempty"`
	URL       string            `json:"url,omitempty"`
	Transport string            `json:"transport,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	OAuth     *MCPOAuthConfig   `json:"oauth,omitempty"`
}

func (m *MCPConfig) UnmarshalJSON(data []byte) error {
	type Alias MCPConfig
	aux := &struct {
		*Alias
		Command json.RawMessage `json:"command,omitempty"`
	}{Alias: (*Alias)(m)}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	if len(aux.Command) == 0 {
		return nil
	}

	var cmdStr string
	if err := json.Unmarshal(aux.Command, &cmdStr); err == nil {
		m.Command = cmdStr
		return nil
	}

	var cmdArr []string
	if err := json.Unmarshal(aux.Command, &cmdArr); err == nil && len(cmdArr) > 0 {
		m.Command = cmdArr[0]
		m.Args = cmdArr[1:]
		return nil
	}

	return nil
}

type MCPOAuthConfig struct {
	ClientID    string   `json:"client_id"`
	AuthURL     string   `json:"auth_url"`
	TokenURL    string   `json:"token_url"`
	Scopes      []string `json:"scopes,omitempty"`
	CallbackURL string   `json:"callback_url,omitempty"`
}

type ServerConfig struct {
	Port *int   `json:"port,omitempty"`
	Host string `json:"host,omitempty"`
}

type ProviderConfig struct {
	NPM     string                 `json:"npm,omitempty"`
	Env     []string               `json:"env,omitempty"`
	Options map[string]interface{} `json:"options,omitempty"`
}

type PermissionConfig struct {
	Allow []string `json:"allow,omitempty"`
	Deny  []string `json:"deny,omitempty"`
}

type ToolOutputConfig struct {
	MaxLines *int `json:"max_lines,omitempty"`
	MaxBytes *int `json:"max_bytes,omitempty"`
}

type CompactionConfig struct {
	Auto                 *bool `json:"auto,omitempty"`
	Prune                *bool `json:"prune,omitempty"`
	TailTurns            *int  `json:"tail_turns,omitempty"`
	PreserveRecentTokens *int  `json:"preserve_recent_tokens,omitempty"`
	Reserved             *int  `json:"reserved,omitempty"`
	MaskObservations     *bool `json:"mask_observations,omitempty"`
}

// Load reads and merges config from all sources: global config dir, project
// config files walking up from directory, and environment overrides. Unknown
// JSON fields are silently ignored (forward compatibility).
func Load(directory string) (*Info, error) {
	result := &Info{}

	globalFile := GlobalConfigFile()
	if err := loadAndMerge(result, globalFile, nil); err != nil {
		slog.Warn("failed to load global config", "path", globalFile, "error", err)
	}

	configDir := ConfigDir()
	for _, name := range []string{"config.json", "tinycode.json", "tinycode.jsonc"} {
		f := filepath.Join(configDir, name)
		if f == globalFile {
			continue
		}
		if err := loadAndMerge(result, f, nil); err != nil {
			slog.Warn("failed to load config file", "path", f, "error", err)
		}
	}

	if directory != "" {
		projectFiles := ProjectConfigFiles("tinycode", directory)
		for _, f := range projectFiles {
			if err := loadAndMerge(result, f, nil); err != nil {
				slog.Warn("failed to load project config", "path", f, "error", err)
			}
		}
	}

	if result.Username == "" {
		if u := os.Getenv("USER"); u != "" {
			result.Username = u
		} else {
			result.Username = "user"
		}
	}

	return result, nil
}

// LoadFile loads and parses a single config file with JSONC support and env
// var substitution.
func LoadFile(path string, env map[string]string) (*Info, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Info{}, nil
		}
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	return ParseConfig(string(data), env)
}

// ParseConfig parses JSONC text with env var substitution into an Info struct.
func ParseConfig(text string, env map[string]string) (*Info, error) {
	if env == nil {
		env = make(map[string]string)
	}
	substituted := SubstituteEnvVars(text, env)

	jsonStr, err := ParseJSONC(substituted)
	if err != nil {
		return nil, fmt.Errorf("parsing JSONC: %w", err)
	}

	info := &Info{}
	if err := json.Unmarshal([]byte(jsonStr), info); err != nil {
		return nil, fmt.Errorf("parsing config JSON: %w", err)
	}
	return info, nil
}

// Merge combines src into dst. Non-zero fields in src override dst.
// Slice fields (Instructions, providers lists) are concatenated and deduplicated.
// Map fields (Provider) are merged key-by-key.
func Merge(dst, src *Info) *Info {
	if src == nil {
		return dst
	}
	if dst == nil {
		return src
	}

	result := *dst

	if src.Shell != "" {
		result.Shell = src.Shell
	}
	if src.LogLevel != "" {
		result.LogLevel = src.LogLevel
	}
	if src.Model != "" {
		result.Model = src.Model
	}
	if src.SmallModel != "" {
		result.SmallModel = src.SmallModel
	}
	if src.DefaultAgent != "" {
		result.DefaultAgent = src.DefaultAgent
	}
	if src.SubagentDepth != nil {
		result.SubagentDepth = src.SubagentDepth
	}
	if src.Username != "" {
		result.Username = src.Username
	}
	if src.Share != "" {
		result.Share = src.Share
	}
	if src.Snapshot != nil {
		result.Snapshot = src.Snapshot
	}
	if src.Server != nil {
		result.Server = src.Server
	}
	if src.ToolOutput != nil {
		result.ToolOutput = src.ToolOutput
	}
	if src.Compaction != nil {
		result.Compaction = src.Compaction
	}
	if src.Permission != nil {
		if result.Permission == nil {
			result.Permission = src.Permission
		} else {
			merged := *result.Permission
			merged.Allow = dedup(append(merged.Allow, src.Permission.Allow...))
			merged.Deny = dedup(append(merged.Deny, src.Permission.Deny...))
			result.Permission = &merged
		}
	}

	if len(src.DisabledProviders) > 0 {
		result.DisabledProviders = dedup(append(result.DisabledProviders, src.DisabledProviders...))
	}
	if len(src.EnabledProviders) > 0 {
		result.EnabledProviders = dedup(append(result.EnabledProviders, src.EnabledProviders...))
	}
	if len(src.Instructions) > 0 {
		result.Instructions = dedup(append(result.Instructions, src.Instructions...))
	}
	if len(src.Plugins) > 0 {
		result.Plugins = src.Plugins
	}

	if len(src.Provider) > 0 {
		if result.Provider == nil {
			result.Provider = make(map[string]ProviderConfig)
		}
		for k, v := range src.Provider {
			result.Provider[k] = v
		}
	}

	if len(src.MCP) > 0 {
		if result.MCP == nil {
			result.MCP = make(map[string]MCPConfig)
		}
		for k, v := range src.MCP {
			result.MCP[k] = v
		}
	}

	return &result
}

func loadAndMerge(dst *Info, path string, env map[string]string) error {
	loaded, err := LoadFile(path, env)
	if err != nil {
		return err
	}
	*dst = *Merge(dst, loaded)
	return nil
}

func dedup(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item]; !ok {
			seen[item] = struct{}{}
			result = append(result, item)
		}
	}
	return result
}
