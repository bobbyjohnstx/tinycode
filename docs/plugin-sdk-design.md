# Go Plugin SDK Design

## Recommendation: JSON-RPC over stdin/stdout

Each plugin is a standalone binary. Tinycode spawns it, communicates via JSON-RPC over stdin/stdout — the same pattern already used by MCP (`internal/mcp/stdio.go`) and LSP. Language-agnostic, process-isolated, no protobuf.

## Why This Over Alternatives

| Approach | Verdict |
|----------|---------|
| **JSON-RPC/stdio** | **Recommended** — reuses MCP transport already in codebase, language-agnostic, process isolation |
| HashiCorp go-plugin | Rejected — pulls in gRPC+protobuf (contradicts migration decision), heavy deps |
| Wasm (wazero) | Rejected — TinyGo required, limited stdlib, high friction for plugin authors |
| Go native plugins | Rejected — Linux+Darwin only, must match exact Go version, fragile |
| Yaegi interpreter | Rejected — incomplete Go support, no generics, uncertain maintenance |
| Config-driven hooks | Too limited — can't register tools (the primary use case) |

## TS Plugin API Audit

The TS SDK (`@tinycode/plugin` v1.20.0) has 27 hook types, but real-world usage concentrates on ~8:

| Hook | Usage | Example |
|------|-------|---------|
| `tool` (registration) | ~80% of plugins | notify, code-review, cluster-ops |
| `session.start` / `session.end` | ~30% | handoff, telemetry |
| `permission.ask` | safety-net | blocks dangerous shell commands |
| `shell.env` | cluster-ops | injects env vars |
| `experimental.chat.system.transform` | handoff | injects prior session context |
| `tool.execute.before` / `after` | telemetry | tracks tool call duration |
| `dispose` | handoff, telemetry | cleanup on shutdown |

The auth/provider/chat-modification hooks are exclusively used by built-in provider plugins (Copilot, xAI, Azure, Cloudflare, DigitalOcean) — these will be compiled directly into the Go binary, not external plugins.

## Protocol

### Initialization (tinycode → plugin)
```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"version":"2.0.0","directory":"/path/to/project","options":{"clusterId":"xxx"}}}
```

### Plugin manifest response
```json
{"jsonrpc":"2.0","id":1,"result":{
  "id":"my-plugin",
  "tools":[{"name":"notify","description":"Send notification","inputSchema":{...}}],
  "hooks":["session.start","session.end","permission.ask"]
}}
```

### Tool calls (tinycode → plugin)
```json
{"jsonrpc":"2.0","id":2,"method":"tool/call","params":{"name":"notify","args":{"title":"Done"},"context":{"sessionID":"...","directory":"..."}}}
```

### Hook dispatch (tinycode → plugin)
```json
{"jsonrpc":"2.0","id":3,"method":"hook/permission.ask","params":{"input":{"type":"bash","pattern":"rm -rf /"},"output":{"status":"ask"}}}
```

## Go SDK (`pkg/plugin/`)

```go
// pkg/plugin/plugin.go
type Plugin struct {
    ID    string
    Tools []ToolDef
    Hooks HookHandlers
}

type ToolDef struct {
    Name        string
    Description string
    Parameters  map[string]any  // JSON Schema
    Execute     func(ctx context.Context, args json.RawMessage, tc ToolContext) (string, error)
}

type HookHandlers struct {
    SessionStart    func(SessionStartEvent) error
    SessionEnd      func(SessionEndEvent) error
    PermissionAsk   func(PermissionInput, *PermissionOutput) error
    ShellEnv        func(ShellEnvInput, *ShellEnvOutput) error
    ChatParams      func(ChatParamsInput, *ChatParamsOutput) error
    ToolExecBefore  func(ToolExecInput, *ToolExecBeforeOutput) error
    ToolExecAfter   func(ToolExecInput, *ToolExecAfterOutput) error
    SystemTransform func(SystemTransformInput, *SystemTransformOutput) error
    Dispose         func() error
}

// Run handles the stdin/stdout JSON-RPC protocol
func Run(p Plugin)
```

### Example Plugin (notify)

```go
package main

import (
    "context"
    "encoding/json"
    "os/exec"
    "runtime"

    "github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

func main() {
    plugin.Run(plugin.Plugin{
        ID: "notify",
        Tools: []plugin.ToolDef{{
            Name:        "notify",
            Description: "Send a desktop notification",
            Parameters: map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "title":   map[string]any{"type": "string", "description": "Notification title"},
                    "message": map[string]any{"type": "string", "description": "Notification body"},
                },
                "required": []string{"title", "message"},
            },
            Execute: func(ctx context.Context, raw json.RawMessage, tc plugin.ToolContext) (string, error) {
                var args struct{ Title, Message string }
                json.Unmarshal(raw, &args)
                if runtime.GOOS == "darwin" {
                    exec.Command("osascript", "-e",
                        `display notification "`+args.Message+`" with title "`+args.Title+`"`).Run()
                }
                return "Notification sent", nil
            },
        }},
    })
}
```

## File Structure

```
pkg/plugin/              — Public SDK for plugin authors
  plugin.go              — Plugin struct, Run() entry point
  tool.go                — Tool definition helpers
  hook.go                — Hook handler types
  protocol.go            — JSON-RPC message types

internal/plugin/          — Server-side plugin management
  manager.go             — Plugin lifecycle (spawn, connect, dispose)
  registry.go            — Plugin discovery and marketplace
  hook.go                — Hook dispatch to all loaded plugins
  config.go              — Plugin configuration parsing
  builtin.go             — BuiltinPlugin interface for compiled providers
```

## Plugin Configuration

```json
{
  "plugin": [
    "notify",
    ["cluster-ops", {"clusterId": "xxx", "consoleOfflineToken": "..."}]
  ]
}
```

Resolution order:
1. `~/.config/tinycode/plugins/<name>` (local binary)
2. Plugin registry (download URL)
3. `PATH` for `tinycode-plugin-<name>`

## Plugin Installation

```bash
tinycode plugin install github.com/user/tinycode-plugin-foo   # Go module
tinycode plugin install --url https://example.com/foo-arm64   # Binary download
tinycode plugin install notify                                 # From registry
```

## Migration Patterns

### Pattern A: Tool-only (notify, code-review, web-search)
- TS `tool({description, args, execute})` → Go `ToolDef{Name, Description, Parameters, Execute}`
- Replace zod schemas with JSON Schema objects
- Replace `Bun.$` with `os/exec.Command`

### Pattern B: Hook + tool (handoff, safety-net, telemetry)
- Tools translate same as Pattern A
- Hook handlers map directly: `session.start` → `HookHandlers.SessionStart`
- State: Go plugins use local files/SQLite/memory (persistent processes)

### Pattern C: Provider plugins (cluster-ops with options)
- Options schema: JSON Schema validation replaces zod
- Options passed via `initialize` params
- Shell: direct `os/exec` instead of `Bun.$`

## Existing Plugin Inventory (30+ plugins)

**General (12):** pilot, code-review, command-inject, context-pruning, handoff, notify, snippets, telemetry, documents, web-search, safety-net, log-sanitizer

> All 12 general plugins have been implemented in Go using the `pkg/plugin/` SDK.

**Red Hat (19):** cluster-ops, context-injection, oauth, obs-logging, obs-metrics, eval-trustyai, experiment-tracker, mcp-bridge, mlflow-tools, model-serving, pipelines, quay, rhdh, tekton, rhacm, aap-bridge, eda-events, api-catalog, dev-content, ecosystem-catalog, rhdp-provisioner, lightspeed, container-linter, lightwell, rhacs

## Built-in Plugins (compiled into Go binary)

Provider auth plugins (Copilot, xAI, Azure, Cloudflare, DigitalOcean) and oh-my-tiny (22 tools) will use a `BuiltinPlugin` interface — no IPC overhead:

```go
type BuiltinPlugin interface {
    ID() string
    Tools() []*tool.Def
    Hooks() HookHandlers
}
```
