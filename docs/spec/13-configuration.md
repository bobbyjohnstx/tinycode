# 13. Configuration

Package: `internal/config/`

Configuration is loaded from multiple sources and merged into a single `Info` struct. Unknown JSON fields are silently ignored during unmarshaling (forward compatibility).

## 13.1 Config Loading

Source: `config.go`

`Load(directory)` reads and merges configuration from all sources in order:

1. **Preferred global config file** -- exactly one file from `GlobalConfigFile()`: first existing of `<configDir>/tinycode.jsonc`, `<configDir>/tinycode.json`, `<configDir>/config.json`. Other global names are not re-merged (jsonc priority is preserved). If that file exists but fails to parse, `Load` returns an error.
2. **Project config files** -- walks up from `directory`, collecting at each level `tinycode.jsonc` / `tinycode.json` and `.tinycode/tinycode.jsonc` / `.tinycode/tinycode.json`; results are reversed so outermost files load first (innermost wins in merge)
3. **Username fallback** -- if `Username` is still empty after merging, set from `$USER` env var (fallback: `"user"`)

`Load` does **not** merge process environment variables into `Info`. Runtime env vars (hosts, ports, log level, auth) are read by the `cmd/tinycode` layer after config load.

### File Parsing Pipeline

Each file goes through:

1. `os.ReadFile()` -- read raw bytes (missing files return empty `Info`, not error)
2. `SubstituteEnvVars()` -- replace `{env:VAR_NAME}` placeholders with environment or explicit values
3. `ParseJSONC()` -- strip `//` line comments, `/* */` block comments, and trailing commas
4. `json.Unmarshal()` -- decode JSON into `Info` struct
5. `Merge()` -- merge into accumulated config

## 13.2 Config File Locations

Source: `paths.go`

### Config Directories

`configDirs()` returns directories in priority order:

| Platform | Directories (in order) |
|----------|----------------------|
| macOS | `~/Library/Application Support/tinycode/`, `~/.config/tinycode/` |
| Linux | `~/.config/tinycode/` |

Overrides:
- `TINYCODE_CONFIG_DIR` -- if set, used as the sole config directory
- `XDG_CONFIG_HOME` -- if set, `$XDG_CONFIG_HOME/tinycode/` replaces the defaults

The macOS dual-path exists for backward compatibility with the TypeScript tinycode, which uses `~/.config/`.

### Data Directory

`DataDir()` returns the data storage directory:

| Platform | Default |
|----------|---------|
| macOS | `~/Library/Application Support/tinycode/` |
| Linux | `~/.local/share/tinycode/` |

Overrides: `TINYCODE_DATA_DIR`, `XDG_DATA_HOME`

### Global Config File

`GlobalConfigFile()` returns the first existing file from the primary config directory:

1. `<configDir>/tinycode.jsonc`
2. `<configDir>/tinycode.json`
3. `<configDir>/config.json`

If none exist, returns the `.jsonc` path (for creation).

### Project Config Files

`ProjectConfigFiles(name, directory)` walks up from `directory` to the filesystem root, collecting `<name>.jsonc` and `<name>.json` at each level. The result is reversed so that outermost files merge first and innermost files win.

### Project Dot Directories

`ProjectDotDirs(directory)` walks up from `directory` collecting `.tinycode/` directories. Results are returned outermost first. `Load` also merges `tinycode.jsonc` / `tinycode.json` inside each `.tinycode/` directory (same outer→inner order as project root configs).

## 13.3 Config Schema

```go
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
    Agents            map[string]json.RawMessage `json:"agents,omitempty"`
    Experimental      *ExperimentalConfig        `json:"experimental,omitempty"`
    Temperature       *float64                   `json:"temperature,omitempty"`
    TopP              *float64                   `json:"top_p,omitempty"`
    MaxTokens         *int                       `json:"max_tokens,omitempty"`
    Skills            *SkillsConfig              `json:"skills,omitempty"`
    Attachment        *AttachmentConfig           `json:"attachment,omitempty"`
    Command           map[string]string          `json:"command,omitempty"`
    Reference         map[string]string          `json:"reference,omitempty"`
    Watcher           []string                   `json:"watcher,omitempty"`
    LSP               *LSPConfig                 `json:"lsp,omitempty"`
    Theme             string                     `json:"theme,omitempty"`
}
```

### Field Reference

| Field | Type | JSON Key | Description |
|-------|------|----------|-------------|
| Shell | string | `shell` | Default shell for tool execution |
| LogLevel | string | `logLevel` | Log level: `DEBUG`, `INFO`, `WARN`, `ERROR` |
| Model | string | `model` | Default model in `provider/model` format |
| SmallModel | string | `small_model` | Small model used for titles and compaction |
| DefaultAgent | string | `default_agent` | Default primary agent name |
| SubagentDepth | *int | `subagent_depth` | Maximum subagent nesting depth |
| Username | string | `username` | Display username |
| Share | string | `share` | Session sharing: `manual`, `auto`, `disabled` |
| Snapshot | *bool | `snapshot` | Enable filesystem snapshot tracking |
| DisabledProviders | []string | `disabled_providers` | Provider blacklist |
| EnabledProviders | []string | `enabled_providers` | Provider whitelist |
| Server | *ServerConfig | `server` | Server port and host |
| Provider | map[string]ProviderConfig | `provider` | Per-provider configuration |
| Permission | *PermissionConfig | `permission` | Allow/deny permission rules |
| ToolOutput | *ToolOutputConfig | `tool_output` | Tool output truncation limits |
| Compaction | *CompactionConfig | `compaction` | Context compaction tuning |
| Instructions | []string | `instructions` | Additional instruction file paths |
| Plugins | []json.RawMessage | `plugins` | Plugin specifications (opaque JSON) |
| MCP | map[string]MCPConfig | `mcp` | MCP server configurations |
| Agents | map[string]json.RawMessage | `agents` | Agent overrides (opaque JSON per agent) |
| Experimental | *ExperimentalConfig | `experimental` | Experimental feature flags |
| Temperature | *float64 | `temperature` | Global LLM temperature |
| TopP | *float64 | `top_p` | Global LLM top-p |
| MaxTokens | *int | `max_tokens` | Global LLM max tokens |
| Skills | *SkillsConfig | `skills` | Skill discovery paths and URLs |
| Attachment | *AttachmentConfig | `attachment` | Attachment handling config |
| Command | map[string]string | `command` | Named command configurations |
| Reference | map[string]string | `reference` | Directory `@alias` references |
| Watcher | []string | `watcher` | File watcher ignore patterns |
| LSP | *LSPConfig | `lsp` | LSP client config (accepts bool or object) |
| Theme | string | `theme` | TUI theme name |

## 13.4 Sub-Configs

### ServerConfig

```go
type ServerConfig struct {
    Port *int   `json:"port,omitempty"`
    Host string `json:"host,omitempty"`
}
```

Default port: 4096 (overridden by `TINYCODE_PORT` env var).

### ProviderConfig

```go
type ProviderConfig struct {
    NPM     string                        `json:"npm,omitempty"`
    Env     []string                      `json:"env,omitempty"`
    Options map[string]interface{}        `json:"options,omitempty"`
    Models  map[string]ProviderModelConfig `json:"models,omitempty"`
}

type ProviderModelConfig struct {
    Limit *ProviderModelLimit `json:"limit,omitempty"`
}

type ProviderModelLimit struct {
    Context int `json:"context,omitempty"`
    Output  int `json:"output,omitempty"`
}
```

### PermissionConfig

```go
type PermissionConfig struct {
    Allow []string `json:"allow,omitempty"`
    Deny  []string `json:"deny,omitempty"`
}
```

See [12-permissions.md](12-permissions.md) for how these rules are evaluated.

### ToolOutputConfig

```go
type ToolOutputConfig struct {
    MaxLines *int `json:"max_lines,omitempty"`
    MaxBytes *int `json:"max_bytes,omitempty"`
}
```

Defaults: `MaxLines` = 2000, `MaxBytes` = 51200 (50 KB). See [08-tools.md](08-tools.md) for truncation behavior.

### CompactionConfig

```go
type CompactionConfig struct {
    Auto                 *bool `json:"auto,omitempty"`
    Prune                *bool `json:"prune,omitempty"`
    TailTurns            *int  `json:"tail_turns,omitempty"`
    PreserveRecentTokens *int  `json:"preserve_recent_tokens,omitempty"`
    Reserved             *int  `json:"reserved,omitempty"`
    MaskObservations     *bool `json:"mask_observations,omitempty"`
    MaxMessages          *int  `json:"max_messages,omitempty"`
}
```

See [05-context-compaction.md](05-context-compaction.md) for compaction behavior.

### ExperimentalConfig

```go
type ExperimentalConfig struct {
    DoomLoopThreshold int `json:"doom_loop_threshold,omitempty"`
    AutoContinue      int `json:"auto_continue,omitempty"`
}
```

`AutoContinue` controls maximum automatic continuation iterations (0 = disabled).

### SkillsConfig

```go
type SkillsConfig struct {
    Paths []string `json:"paths,omitempty"`
    URLs  []string `json:"urls,omitempty"`
}
```

Additional skill discovery locations beyond the defaults. `paths` directories are scanned for `*/SKILL.md` via `skill.DiscoverWithPaths`. `urls` are accepted in config but not fetched (not implemented). See [10-skills.md](10-skills.md).

### AttachmentConfig

```go
type AttachmentConfig struct {
    MaxSize *int         `json:"max_size,omitempty"`
    Image   *ImageConfig `json:"image,omitempty"`
}

type ImageConfig struct {
    MaxWidth  int    `json:"max_width,omitempty"`
    MaxHeight int    `json:"max_height,omitempty"`
    Quality   int    `json:"quality,omitempty"`
    Format    string `json:"format,omitempty"`
}
```

### LSPConfig

```go
type LSPConfig struct {
    Enabled *bool                       `json:"enabled,omitempty"`
    Servers map[string]LSPServerConfig  `json:"servers,omitempty"`
    Timeout *int                        `json:"timeout,omitempty"`
}

type LSPServerConfig struct {
    Command  string            `json:"command"`
    Args     []string          `json:"args,omitempty"`
    Disabled *bool             `json:"disabled,omitempty"`
    Env      map[string]string `json:"env,omitempty"`
}
```

`LSPConfig` has a custom `UnmarshalJSON` that accepts both a boolean and an object. LSP is **enabled by default**; use `"lsp": false` (or `"enabled": false`) to disable. `timeout` is in **seconds** (request timeout per LSP call). Server map keys are **language names** (`go`, `typescript`, `python`, `rust`, …), not binary names.

```jsonc
// Boolean form -- disable LSP entirely (default is enabled)
"lsp": false

// Object form -- configure per-language server
"lsp": {
  "enabled": true,
  "timeout": 30,
  "servers": {
    "go": { "command": "gopls", "args": ["serve"] },
    "typescript": { "disabled": true }
  }
}
```

### MCPConfig

```go
type MCPConfig struct {
    Command   string            `json:"-"`
    Args      []string          `json:"-"`
    Env       map[string]string `json:"env,omitempty"`
    URL       string            `json:"url,omitempty"`
    Transport string            `json:"transport,omitempty"`
    Headers   map[string]string `json:"headers,omitempty"`
    OAuth     *MCPOAuthConfig   `json:"oauth,omitempty"`
}

type MCPOAuthConfig struct {
    ClientID    string   `json:"client_id"`
    AuthURL     string   `json:"auth_url"`
    TokenURL    string   `json:"token_url"`
    Scopes      []string `json:"scopes,omitempty"`
    CallbackURL string   `json:"callback_url,omitempty"`
}
```

`MCPConfig` stores `Command`/`Args` with `json:"-"` and uses custom `MarshalJSON` / `UnmarshalJSON` so GET `/config` includes them. Unmarshal accepts `command` as either a string or an array. When an array, the first element becomes `Command` and the rest become `Args`:

```jsonc
// String form
"my-server": { "command": "npx my-tool" }

// Array form
"my-server": { "command": ["npx", "my-tool", "--port", "3000"] }
```

See [11-mcp.md](11-mcp.md) for MCP client behavior.

## 13.5 Config Merging

Source: `config.go:Merge()`

`Merge(dst, src)` combines two `Info` structs. Returns a new struct (does not mutate either input).

### Scalar Fields

Non-zero values in `src` override `dst`:

- String fields: `Shell`, `LogLevel`, `Model`, `SmallModel`, `DefaultAgent`, `Username`, `Share`, `Theme`
- Scalar pointer fields: `SubagentDepth`, `Snapshot`, `Temperature`, `TopP`, `MaxTokens`
- Nested pointer structs (`Server`, `ToolOutput`, `Compaction`, `Experimental`, `Skills`, `Attachment`, `LSP`) are **deep-merged** field-by-field so a project override of `server.port` keeps a global `server.host`

### Collection Fields

| Field | Merge Strategy |
|-------|---------------|
| `Instructions` | Concatenated, deduplicated |
| `DisabledProviders` | Concatenated, deduplicated |
| `EnabledProviders` | Concatenated, deduplicated |
| `Watcher` | Concatenated, deduplicated |
| `Permission` | `Allow` and `Deny` lists concatenated and deduplicated independently |
| `Provider` | Map merged key-by-key (src keys override dst) |
| `Command` | Map merged key-by-key |
| `Reference` | Map merged key-by-key |
| `MCP` | Map merged key-by-key |
| `Agents` | Map merged key-by-key |
| `Plugins` | Replaced entirely (src wins, not merged) |

Deduplication preserves insertion order -- first occurrence wins.

## 13.6 JSONC Support

Source: `jsonc.go`

`ParseJSONC()` converts JSONC (JSON with Comments) to valid JSON:

1. **Line comments** -- `// ...` stripped to end of line
2. **Block comments** -- `/* ... */` stripped entirely
3. **Trailing commas** -- removed before `}` and `]`

Comments inside JSON strings are preserved. The parser handles escaped quotes within strings correctly.

## 13.7 Environment Variable Substitution

Source: `jsonc.go:SubstituteEnvVars()`

`SubstituteEnvVars(text, env)` replaces `{env:VAR_NAME}` patterns in config text:

1. Check the explicit `env` map first
2. Fall back to `os.Getenv()`
3. Log a warning for unresolved placeholders (placeholder is removed from output)

Variable names are validated to contain only letters, digits, and underscores.

```jsonc
{
  "model": "{env:TINYCODE_DEFAULT_MODEL}",
  "server": { "host": "{env:TINYCODE_HOST}" }
}
```

## 13.8 Environment Variables

These are applied by `cmd/tinycode` (discovery, serve bind, logger, auth), **not** by `config.Load` into `Info`. Config file fields such as `logLevel` / `model` are separate; when both apply, env typically wins for the specific cmd concern (e.g. `TINYCODE_LOG_LEVEL` overrides `logLevel` from file).

| Variable | Applied by | Description |
|----------|------------|-------------|
| `TINYCODE_PORT` | cmd (serve/web) | Override server port |
| `TINYCODE_HOST` | cmd (serve/web) | Override bind address |
| `TINYCODE_DB` | storage | Override database path |
| `TINYCODE_LOG_LEVEL` | cmd `setupLogger` | Set log level (overrides config `logLevel`) |
| `TINYCODE_WEB_DIR` | cmd (web) | Serve web UI from directory (dev mode) |
| `TINYCODE_DATA_DIR` | `config.DataDir` | Override data directory |
| `TINYCODE_CONFIG_DIR` | `config.configDirs` | Override config directory |
| `TINYCODE_OLLAMA_HOST` | cmd `startDiscovery` | Override Ollama URL (preferred over `OLLAMA_HOST`) |
| `OLLAMA_HOST` | cmd `startDiscovery` | Ollama URL fallback when `TINYCODE_OLLAMA_HOST` unset |
| `TINYCODE_VLLM_HOST` | cmd `startDiscovery` | Override vLLM URL |
| `TINYCODE_LMSTUDIO_HOST` | cmd `startDiscovery` | Override LM Studio URL |
| `TINYCODE_MAAS_HOST` | provider | MaaS endpoint |
| `TINYCODE_MAAS_API_KEY` | provider | MaaS auth key |
| `OPENROUTER_API_KEY` | cmd `startDiscovery` | OpenRouter auth key |
| `TINYCODE_AUTH_TOKEN` | cmd (serve/web) | Server bearer auth token |
| `TINYCODE_NO_AUTH` | cmd (serve/web) | Disable auth entirely |
| `TINYCODE_SERVER_PASSWORD` | cmd (serve/web) | Deprecated alias for `TINYCODE_AUTH_TOKEN` |

## 13.9 Example Config

```jsonc
{
  // Global settings
  "model": "ollama/llama3.1",
  "small_model": "ollama/llama3.2",
  "logLevel": "INFO",
  "username": "alice",
  "theme": "dark",

  // Server
  "server": {
    "port": 8080,
    "host": "127.0.0.1"
  },

  // Permission rules
  "permission": {
    "allow": ["read *", "glob *"],
    "deny": ["shell rm -rf *"]
  },

  // Tool output limits
  "tool_output": {
    "max_lines": 1000,
    "max_bytes": 25600
  },

  // MCP servers
  "mcp": {
    "my-server": {
      "command": ["npx", "my-mcp-server"],
      "env": { "API_KEY": "{env:MY_API_KEY}" }
    }
  },

  // LSP configuration (enabled by default; timeout in seconds; keys are languages)
  "lsp": {
    "enabled": true,
    "timeout": 30,
    "servers": {
      "go": { "command": "gopls", "args": ["serve"] }
    }
  },

  // Experimental flags
  "experimental": {
    "doom_loop_threshold": 5,
    "auto_continue": 0
  }
}
```

---

Prev: [12-permissions.md](12-permissions.md) | Next: [14-storage.md](14-storage.md)
