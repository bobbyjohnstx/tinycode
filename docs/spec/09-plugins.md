# 9. Plugins

Package: `internal/plugin/`, SDK: `pkg/plugin/`

Plugins are standalone Go binaries that communicate with tinycode over JSON-RPC 2.0 via stdin/stdout. The plugin system provides process isolation, a language-agnostic protocol, and an in-process built-in plugin interface.

## 9.1 Architecture

- **External plugins** are Go binaries in `cmd/plugin-*/` built using the `pkg/plugin/` SDK
- **Built-in plugins** implement `BuiltinPlugin` interface and run in-process
- Communication: JSON-RPC 2.0 over stdin/stdout (external) or direct function calls (built-in)
- Plugin manager spawns processes, performs initialize handshake, dispatches hooks and tool calls
- Tool IDs are namespaced: `plugin__{pluginName}__{toolName}`
- 36 plugins in the curated registry (12 general-purpose + 24 Red Hat)

---

## 9.2 Plugin Manager

Source: `manager.go`

```go
type Manager struct {
    plugins        map[string]*pluginProcess  // loaded plugins by ID
    registry       []RegistryEntry            // curated available plugins
    logger         *slog.Logger
    directory      string                     // working directory for initialize
    commandFactory CommandFactory             // exec.Cmd creator (testable)
    resolveFunc    ResolveFunc                // binary path resolver (testable)
}
```

### Constants

| Constant | Value | Description |
|----------|-------|-------------|
| `hookTimeout` | `5s` | Timeout for individual JSON-RPC calls |
| `killTimeout` | `3s` | Timeout for graceful process termination |

### Factory Functions

| Function | Description |
|----------|-------------|
| `NewManager(logger)` | Creates manager with built-in registry |
| `NewManagerWithRegistry(entries)` | Creates manager with custom registry (testing) |

### Methods

| Method | Description |
|--------|-------------|
| `Load(name)` | Resolve, spawn, handshake, register |
| `Unload(pluginID)` | Stop process, remove from map |
| `Shutdown()` | Dispose all plugins, stop all processes |
| `List()` | Return `[]PluginInfo` of loaded plugins |
| `Tools()` | Return `[]ToolManifest` from all loaded plugins |
| `CallTool(pluginID, toolName, args)` | Invoke a tool via JSON-RPC `tool/call` |
| `Registry()` | Return curated `[]RegistryEntry` |
| `SetDirectory(dir)` | Set working directory for initialize handshake |
| `SetCommandFactory(f)` | Override exec.Cmd creation (testing) |
| `SetResolveFunc(f)` | Override binary resolution (testing) |

---

## 9.3 Plugin Lifecycle

### Loading (`Manager.Load`)

1. Validate plugin name is non-empty and exists in the curated registry
2. Resolve binary path via `ResolveFunc` (see [9.8 Binary Resolution](#98-binary-resolution))
3. Generate ascending plugin ID with `plugin` prefix (e.g., `plugin_01J...`)
4. Spawn process with `exec.CommandContext(context.Background(), binPath)` -- background context so process lives until explicitly stopped
5. Pipe stdin/stdout, create JSON encoder/decoder
6. Start health monitor goroutine: `cmd.Wait()` sets `dead` flag and closes `done` channel on exit
7. Send `initialize` JSON-RPC request with `{version: "1.0", directory, options}`
8. Receive `{id, tools, hooks}` response (tool manifests and supported hook names)
9. Register plugin in the `plugins` map

### Process Health Monitor

A goroutine started at load time watches each plugin process:

```go
go func() {
    _ = cmd.Wait()
    proc.dead.Store(true)
    close(doneCh)
}()
```

The `dead` atomic flag and `done` channel are checked by `sendRPC` to short-circuit calls to crashed plugins.

### Unloading (`Manager.Unload`)

1. Remove plugin from the `plugins` map
2. Call `stopProcess()`:
   - Send `dispose` hook (best-effort via `hook/invoke`)
   - Close stdin to signal EOF
   - Wait for graceful exit via `done` channel
   - Force-kill after `killTimeout` (3s) if still running

### Shutdown (`Manager.Shutdown`)

1. Lock, snapshot all processes, clear the map
2. Call `stopProcess()` on each process

---

## 9.4 JSON-RPC Protocol

All messages use JSON-RPC 2.0. The SDK uses line-delimited JSON with a 1 MB scanner buffer.

### Wire Types (pkg/plugin/protocol.go)

```go
type JSONRPCRequest struct {
    JSONRPC string          `json:"jsonrpc"`       // always "2.0"
    ID      int64           `json:"id,omitempty"`
    Method  string          `json:"method"`
    Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
    JSONRPC string          `json:"jsonrpc"`
    ID      int64           `json:"id,omitempty"`
    Result  json.RawMessage `json:"result,omitempty"`
    Error   *JSONRPCError   `json:"error,omitempty"`
}

type JSONRPCError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
    Data    any    `json:"data,omitempty"`
}
```

### RPC Timeout Mechanism

`sendRPC()` uses a goroutine + channel pattern with `select` on three cases:

1. **Response received** -- decode and return
2. **`hookTimeout` (5s) elapsed** -- close stdout to unblock decoder goroutine, return timeout error
3. **`done` channel closed** -- process exited, return error

### Methods

#### `initialize`

First message after process spawn. Must be the first method received by the plugin.

```json
// Request
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{
  "version":"1.0","directory":"/path/to/project","options":{}
}}

// Response
{"jsonrpc":"2.0","id":1,"result":{
  "id":"my-plugin",
  "tools":[{"name":"search","description":"Web search","inputSchema":{"type":"object","properties":{"query":{"type":"string"}}}}],
  "hooks":["session.start","tool.execute.after"]
}}
```

| Request Field | Type | Description |
|---------------|------|-------------|
| `version` | string | Protocol version (currently `"1.0"`) |
| `directory` | string | Working directory |
| `options` | object | Plugin-specific options from config |

| Response Field | Type | Description |
|----------------|------|-------------|
| `id` | string | Plugin self-reported ID |
| `tools` | array | Tool manifests (name, description, inputSchema) |
| `hooks` | array | Hook names the plugin handles |

#### `hook/invoke`

Dispatches a lifecycle hook to a plugin.

```json
// Request
{"jsonrpc":"2.0","id":2,"method":"hook/invoke","params":{
  "name":"session.start","input":{"sessionId":"ses_...","directory":"/path"}
}}

// Response
{"jsonrpc":"2.0","id":2,"result":{"output":null}}
```

#### `tool/call`

Invokes a tool on the plugin that owns it.

```json
// Request
{"jsonrpc":"2.0","id":3,"method":"tool/call","params":{
  "name":"web-search","args":{"query":"golang channels"},"context":{"sessionId":"ses_...","directory":"/path"}
}}

// Response
{"jsonrpc":"2.0","id":3,"result":{"content":"search results...","isError":false}}
```

| Request Field | Type | Description |
|---------------|------|-------------|
| `name` | string | Tool name |
| `args` | object | Tool arguments (raw JSON) |
| `context.sessionId` | string | Current session ID |
| `context.directory` | string | Working directory |

| Response Field | Type | Description |
|----------------|------|-------------|
| `content` | string | Tool output text |
| `isError` | boolean | Whether the output represents an error |

### Error Codes

| Code | Meaning |
|------|---------|
| `-32601` | Unknown method |
| `-32602` | Invalid params (malformed tool call or hook params) |
| `-32000` | Hook handler error |

---

## 9.5 Hook System

Source: `hook.go`

Hooks are dispatched to all loaded plugins that declared support during initialization. The `pluginsWithHook()` method filters plugins by their declared hook list.

### Hook Types

| Hook | Semantics | Input Type | Output Type | Behavior |
|------|-----------|------------|-------------|----------|
| `session.start` | Notify | `SessionStartEvent` | -- | Notify all; log and continue on error |
| `session.end` | Notify | `SessionEndEvent` | -- | Notify all; log and continue on error |
| `permission.ask` | Query | `PermissionInput` | `PermissionOutput` | If ANY plugin denies, tool is blocked |
| `shell.env` | Accumulate | `ShellEnvInput` | `ShellEnvOutput` | Each plugin adds/overrides env vars, chains through |
| `tool.execute.before` | Notify | `ToolExecBeforeEvent` | -- | Notify all; log and continue on error |
| `tool.execute.after` | Transform | `ToolExecAfterEvent` | `ToolExecAfterOutput` | Chain through: each plugin sees previous plugin's modifications |
| `dispose` | Notify | -- | -- | Best-effort cleanup before process termination |

### Hook Input/Output Types

```go
// session.start
type SessionStartEvent struct {
    SessionID string `json:"sessionId"`
    Directory string `json:"directory"`
}

// session.end
type SessionEndEvent struct {
    SessionID string `json:"sessionId"`
}

// permission.ask
type PermissionInput struct {
    SessionID  string `json:"sessionId"`
    ToolName   string `json:"toolName"`
    ToolArgs   string `json:"toolArgs"`
    Permission string `json:"permission"`
}
type PermissionOutput struct {
    Allowed bool   `json:"allowed"`
    Reason  string `json:"reason,omitempty"`
}

// shell.env
type ShellEnvInput struct {
    SessionID string            `json:"sessionId"`
    Directory string            `json:"directory"`
    Env       map[string]string `json:"env,omitempty"`
}
type ShellEnvOutput struct {
    Env map[string]string `json:"env"`
}

// tool.execute.before
type ToolExecBeforeEvent struct {
    SessionID string `json:"sessionId"`
    ToolName  string `json:"toolName"`
    ToolArgs  string `json:"toolArgs"`
}

// tool.execute.after
type ToolExecAfterEvent struct {
    SessionID string `json:"sessionId"`
    ToolName  string `json:"toolName"`
    Output    string `json:"output"`
    IsError   bool   `json:"isError"`
}
type ToolExecAfterOutput struct {
    Output  string `json:"output"`
    IsError bool   `json:"isError"`
}
```

### Dispatch Functions

| Function | Hook | Description |
|----------|------|-------------|
| `DispatchSessionStart(mgr, evt)` | `session.start` | Notify all; errors logged as warnings |
| `DispatchSessionEnd(mgr, evt)` | `session.end` | Notify all; errors logged as warnings |
| `DispatchPermissionAsk(mgr, input)` | `permission.ask` | Query all; first deny short-circuits with reason |
| `DispatchShellEnv(mgr, input)` | `shell.env` | Accumulate env vars; later plugins override earlier ones |
| `DispatchToolExecBefore(mgr, evt)` | `tool.execute.before` | Notify all; errors logged as warnings |
| `DispatchToolExecAfter(mgr, evt)` | `tool.execute.after` | Chain output; returns nil if no plugin modified output |

All dispatch functions return nil/no-op when `mgr` is nil or no plugins handle the hook.

### Hook Wiring

Hooks are wired via bus events in `server.go:wirePluginHooks()`:

| Bus Event | Dispatch Function |
|-----------|-------------------|
| `session.created` | `DispatchSessionStart` |
| `session.deleted` | `DispatchSessionEnd` |
| `permission.ask` | `DispatchPermissionAsk` |
| `shell.env` | `DispatchShellEnv` |
| `tool.execute.before` | `DispatchToolExecBefore` |

`tool.execute.after` is dispatched synchronously via `tool.Context.AfterHook`, not through the bus.

---

## 9.6 Built-in Plugins

Source: `builtin.go`

Built-in plugins run in-process without JSON-RPC. They implement the `BuiltinPlugin` interface.

```go
type BuiltinPlugin interface {
    ID() string
    Tools() []BuiltinTool
    Hooks() BuiltinHooks
}

type BuiltinTool struct {
    Name        string
    Description string
    Parameters  map[string]any
    Execute     func(ctx context.Context, args json.RawMessage) (string, error)
}

type BuiltinHooks struct {
    SessionStart func(ctx context.Context, sessionID string) error
    SessionEnd   func(ctx context.Context, sessionID string) error
    Dispose      func(ctx context.Context) error
}
```

### BuiltinManager

```go
type BuiltinManager struct {
    plugins []BuiltinPlugin
    tools   map[string]BuiltinTool  // indexed by tool name
}
```

| Method | Description |
|--------|-------------|
| `NewBuiltinManager()` | Create empty manager |
| `Register(p)` | Add plugin, index its tools |
| `CallTool(ctx, name, args)` | Dispatch by tool name |
| `DispatchHook(name, input)` | Dispatch `session.start`, `session.end`, or `dispose` to all |

Built-in plugins support a subset of hooks: `session.start`, `session.end`, `dispose`. They do not support `permission.ask`, `shell.env`, `tool.execute.before`, or `tool.execute.after`.

---

## 9.7 Plugin Configuration

Source: `config.go`

Plugins are declared in the user's `config.json` under the `"plugins"` array.

### Config Formats

**String form** (no options):

```json
{
  "plugins": ["notify", "telemetry", "code-review"]
}
```

**Tuple form** (with options):

```json
{
  "plugins": [
    "notify",
    ["code-review", {"auto": true}],
    ["telemetry", {"endpoint": "https://example.com"}]
  ]
}
```

### PluginSpec

```go
type PluginSpec struct {
    Name    string
    Options map[string]any
}
```

`UnmarshalJSON` handles both forms:
- Bare string `"notify"` becomes `PluginSpec{Name: "notify"}`
- Tuple `["code-review", {"auto": true}]` becomes `PluginSpec{Name: "code-review", Options: {"auto": true}}`
- Tuple must have 1 or 2 elements; first must be a string

`ParsePluginConfig(raw)` converts `[]json.RawMessage` to `[]PluginSpec`.

---

## 9.8 Binary Resolution

Source: `resolve.go`

`ResolveBinary(name)` locates the plugin binary. Search order:

| Priority | Location | Example |
|----------|----------|---------|
| 1 | Config plugins directory | `~/.config/tinycode/plugins/<name>` |
| 2 | System PATH | `tinycode-plugin-<name>` |
| 3 | Registry (error with install hint) | "plugin found in registry but not installed" |

Binary naming convention: `tinycode-plugin-<name>` (e.g., `tinycode-plugin-notify`).

Executability check: `info.Mode() & 0o111 != 0`.

---

## 9.9 Plugin Registry

Source: `registry.go`

A curated hardcoded list of available plugins. Accessible via `Manager.Registry()` and `GET /plugin/registry`.

### RegistryEntry

```go
type RegistryEntry struct {
    Name        string `json:"name"`
    Description string `json:"description"`
    Package     string `json:"package,omitempty"`
    Repo        string `json:"repo,omitempty"`
    Binary      string `json:"binary,omitempty"`
}
```

### Functions

| Function | Description |
|----------|-------------|
| `Registry()` | Returns a copy of all registry entries |
| `LookupRegistry(name)` | Find entry by name; returns `(entry, bool)` |

### Registry Entries (36 plugins)

#### General-Purpose (12)

| Name | Binary | Description |
|------|--------|-------------|
| `cluster-ops` | `tinycode-plugin-cluster-ops` | Kubernetes cluster operations tools |
| `code-review` | `tinycode-plugin-code-review` | Automated code review on session end |
| `command-inject` | `tinycode-plugin-command-inject` | Custom command injection |
| `context-pruning` | `tinycode-plugin-context-pruning` | Context window pruning strategies |
| `handoff` | `tinycode-plugin-handoff` | Session handoff between agents |
| `log-sanitizer` | `tinycode-plugin-log-sanitizer` | Sanitize sensitive data from logs |
| `notify` | `tinycode-plugin-notify` | Desktop notifications for session events |
| `pilot` | `tinycode-plugin-pilot` | Autonomous agent pilot mode |
| `safety-net` | `tinycode-plugin-safety-net` | Pre-execution safety checks for destructive commands |
| `snippets` | `tinycode-plugin-snippets` | Code snippet management |
| `telemetry` | `tinycode-plugin-telemetry` | Usage telemetry and analytics |
| `web-search` | `tinycode-plugin-web-search` | Web search tool for agents |

#### Red Hat (24)

| Name | Binary | Description |
|------|--------|-------------|
| `aap-bridge` | `tinycode-plugin-aap-bridge` | Ansible Automation Platform bridge (job templates, inventories, lint) |
| `container-linter` | `tinycode-plugin-container-linter` | Containerfile linting, bootc validation, UBI base image suggestions |
| `eda-events` | `tinycode-plugin-eda-events` | Event-Driven Ansible event bridge (session lifecycle, tool events) |
| `lightwell` | `tinycode-plugin-lightwell` | Red Hat Lightwell package security (CVE checks, provenance, Containerfile scanning) |
| `ocp-context-injection` | `tinycode-plugin-ocp-context-injection` | OpenShift cluster context injection (version, nodes, operators, alerts, cost) |
| `ocp-oauth` | `tinycode-plugin-ocp-oauth` | OpenShift OAuth login and shell environment |
| `ocp-obs-logging` | `tinycode-plugin-ocp-obs-logging` | OpenShift observability logging (Loki, Tempo, NetObserv) |
| `ocp-obs-metrics` | `tinycode-plugin-ocp-obs-metrics` | OpenShift observability metrics (PromQL, alerts, silencing) |
| `quay` | `tinycode-plugin-quay` | Quay container registry (search, tags, manifests, vulnerabilities) |
| `rh-api-catalog` | `tinycode-plugin-rh-api-catalog` | Red Hat API catalog (list, spec, endpoints) |
| `rh-dev-content` | `tinycode-plugin-rh-dev-content` | Red Hat developer content (search, articles, recent posts) |
| `rh-ecosystem-catalog` | `tinycode-plugin-rh-ecosystem-catalog` | Red Hat ecosystem catalog (containers, operators via Pyxis) |
| `rhacm` | `tinycode-plugin-rhacm` | Red Hat ACM fleet management (clusters, policies, applications, observability) |
| `rhacs` | `tinycode-plugin-rhacs` | Red Hat ACS security (image scan, policy check, violations, compliance) |
| `rhdh` | `tinycode-plugin-rhdh` | Red Hat Developer Hub (catalog, APIs, TechDocs, dependencies) |
| `rhdp-provisioner` | `tinycode-plugin-rhdp-provisioner` | Red Hat Developer Platform provisioner (search, provision, status) |
| `rhoai-eval-trustyai` | `tinycode-plugin-rhoai-eval-trustyai` | RHOAI model evaluation and TrustyAI fairness metrics |
| `rhoai-experiment-tracker` | `tinycode-plugin-rhoai-experiment-tracker` | RHOAI experiment tracking via MLflow |
| `rhoai-mcp-bridge` | `tinycode-plugin-rhoai-mcp-bridge` | RHOAI Model Context Protocol bridge |
| `rhoai-mlflow-tools` | `tinycode-plugin-rhoai-mlflow-tools` | MLflow experiment, run, and model registry tools |
| `rhoai-model-serving` | `tinycode-plugin-rhoai-model-serving` | RHOAI model serving and sandbox provisioning |
| `rhoai-pipelines` | `tinycode-plugin-rhoai-pipelines` | RHOAI data science pipelines (list, run, status, create) |
| `satellite-lightspeed` | `tinycode-plugin-satellite-lightspeed` | Red Hat Satellite Lightspeed (query, hosts, errata, content views) |
| `tekton` | `tinycode-plugin-tekton` | Tekton pipelines (list, runs, status, logs, tasks) |

---

## 9.10 Plugin SDK

Package: `pkg/plugin/`

Public SDK for building external plugin binaries. Language-agnostic via JSON-RPC -- any language can implement the protocol, but the Go SDK provides type-safe helpers.

### Plugin Definition

```go
type Plugin struct {
    ID    string
    Tools []ToolDef
    Hooks HookHandlers
}
```

### Entry Point

```go
func Run(p Plugin)
```

`Run` starts the stdin/stdout JSON-RPC loop. It:

1. Reads the first message (must be `initialize`)
2. Responds with the manifest (`id`, `tools`, `hooks`)
3. Enters the dispatch loop: routes `tool/call` to tool handlers, `hook/invoke` to hook handlers
4. Calls `Hooks.Dispose` on clean shutdown (stdin EOF)
5. Exits with code 1 on fatal error

Scanner buffer: 1 MB (`1024 * 1024` bytes).

Notifications (requests with `ID == 0`) are silently dropped.

### Tool Definition

Source: `pkg/plugin/tool.go`

```go
type ToolDef struct {
    Name        string
    Description string
    Parameters  map[string]any  // JSON Schema
    Execute     func(ctx context.Context, args json.RawMessage, tc ToolContext) (string, error)
}
```

The `Execute` function receives the raw JSON arguments and a `ToolContext` with `SessionID` and `Directory`. Return the output string, or an error which sets `isError: true` on the wire.

### Hook Handlers

Source: `pkg/plugin/hook.go`

```go
type HookHandlers struct {
    SessionStart   func(ctx context.Context, event SessionStartEvent) error
    SessionEnd     func(ctx context.Context, event SessionEndEvent) error
    PermissionAsk  func(ctx context.Context, input PermissionInput) (*PermissionOutput, error)
    ShellEnv       func(ctx context.Context, input ShellEnvInput) (*ShellEnvOutput, error)
    ToolExecBefore func(ctx context.Context, input ToolExecBeforeInput) error
    ToolExecAfter  func(ctx context.Context, input ToolExecAfterInput) (*ToolExecAfterOutput, error)
    Dispose        func(ctx context.Context) error
}
```

Only non-nil handlers are advertised to the server during initialization. The `registeredHooks()` function introspects the struct to build the hooks list.

### Minimal Plugin Example

```go
package main

import (
    "context"
    "encoding/json"

    "github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

func main() {
    plugin.Run(plugin.Plugin{
        ID: "my-plugin",
        Tools: []plugin.ToolDef{{
            Name:        "greet",
            Description: "Say hello",
            Parameters:  map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "name": map[string]any{"type": "string"},
                },
            },
            Execute: func(ctx context.Context, args json.RawMessage, tc plugin.ToolContext) (string, error) {
                var input struct{ Name string `json:"name"` }
                json.Unmarshal(args, &input)
                return "Hello, " + input.Name, nil
            },
        }},
        Hooks: plugin.HookHandlers{
            SessionStart: func(ctx context.Context, evt plugin.SessionStartEvent) error {
                return nil
            },
        },
    })
}
```

---

## 9.11 Process Model

### Concurrency

- `Manager` uses `sync.RWMutex` for thread-safe access to the plugins map
- Each `pluginProcess` has its own `sync.Mutex` protecting stdin/stdout reads and writes
- Request IDs are generated via `atomic.Int64` (per-process counter)
- Plugin process liveness is tracked via `atomic.Bool` (`dead` flag)

### Error Handling

| Scenario | Behavior |
|----------|----------|
| Handshake failure | Kill process, close `done` channel, return error |
| Hook dispatch error | Log warning, continue to next plugin |
| `permission.ask` error | Log warning, skip plugin (does not deny) |
| Tool call error | Return error with `isError: true` |
| Process crash (mid-call) | `done` channel fires in `sendRPC` select, return error |
| RPC timeout (5s) | Close stdout to unblock decoder, return timeout error |
| Unknown JSON-RPC method | Return error code `-32601` |

---

[Prev: 08-tools.md](08-tools.md) | [Next: 10-skills.md](10-skills.md)
