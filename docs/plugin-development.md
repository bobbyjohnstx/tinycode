# Plugin Development Guide

This guide covers everything you need to build, test, and distribute plugins for tinycode 2.0.

## Overview

Plugins are standalone Go binaries that communicate with tinycode over JSON-RPC 2.0 via stdin/stdout. Each plugin is a separate process spawned by the tinycode plugin manager. Plugins can:

- Register custom tools that the LLM can invoke during sessions
- Hook into session lifecycle events (start, end)
- Intercept permission requests
- Inject environment variables into shell commands
- Observe and modify tool execution (before and after)
- Clean up resources on shutdown

### Plugin SDK

The public SDK lives in `pkg/plugin/`. Import it as:

```go
import "github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
```

The SDK provides three core types:
- `plugin.Plugin` -- the plugin definition (ID, tools, hooks)
- `plugin.ToolDef` -- a tool exposed to the LLM
- `plugin.HookHandlers` -- optional lifecycle callbacks

---

## Quick start

A minimal plugin that provides a single tool:

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

func main() {
    plugin.Run(plugin.Plugin{
        ID: "greet",
        Tools: []plugin.ToolDef{{
            Name:        "greet",
            Description: "Greet someone by name",
            Parameters: map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "name": map[string]any{
                        "type":        "string",
                        "description": "The name to greet",
                    },
                },
                "required": []string{"name"},
            },
            Execute: func(ctx context.Context, args json.RawMessage, tc plugin.ToolContext) (string, error) {
                var input struct {
                    Name string `json:"name"`
                }
                if err := json.Unmarshal(args, &input); err != nil {
                    return "", fmt.Errorf("invalid arguments: %w", err)
                }
                return fmt.Sprintf("Hello, %s!", input.Name), nil
            },
        }},
    })
}
```

Build and install:

```bash
go build -o ~/.config/tinycode/plugins/greet ./cmd/my-plugin
```

Add to your tinycode config (`~/.config/tinycode/config.json`):

```json
{
  "plugins": ["greet"]
}
```

Restart tinycode. The `greet` tool is now available to the LLM.

---

## Plugin entry point

`plugin.Run()` is the entry point for all plugins. It:

1. Reads an `initialize` request from stdin
2. Responds with a manifest declaring the plugin's tools and hooks
3. Enters a dispatch loop, routing `tool/call` and `hook/invoke` requests to handlers
4. Calls the `Dispose` hook on clean shutdown (stdin closed)

```go
type Plugin struct {
    ID    string
    Tools []ToolDef
    Hooks HookHandlers
}

func Run(p Plugin)
```

`Run` blocks until stdin is closed or an unrecoverable error occurs. On error, it prints to stderr and exits with code 1.

---

## Tool plugins

Tools are functions the LLM can invoke during a session. Each tool has a name, description, JSON Schema parameters, and an execute function.

### ToolDef struct

```go
type ToolDef struct {
    Name        string
    Description string
    Parameters  map[string]any
    Execute     func(ctx context.Context, args json.RawMessage, tc ToolContext) (string, error)
}
```

| Field | Purpose |
|-------|---------|
| `Name` | Unique tool name. The LLM uses this to invoke the tool. |
| `Description` | Human-readable description shown to the LLM. |
| `Parameters` | JSON Schema describing the tool's input. Use standard JSON Schema with `type`, `properties`, and `required`. |
| `Execute` | The function called when the LLM invokes the tool. Receives raw JSON args and a `ToolContext`. Returns a string result or an error. |

### ToolContext

```go
type ToolContext struct {
    SessionID string `json:"sessionId"`
    Directory string `json:"directory"`
}
```

The `ToolContext` provides the current session ID and working directory.

### Parameter schema

Parameters use standard JSON Schema. Define them as `map[string]any`:

```go
Parameters: map[string]any{
    "type": "object",
    "properties": map[string]any{
        "query": map[string]any{
            "type":        "string",
            "description": "The search query",
        },
        "limit": map[string]any{
            "type":        "number",
            "description": "Maximum number of results",
        },
    },
    "required": []string{"query"},
},
```

### Error handling

Return an error from `Execute` to signal failure. The error message is sent back to the LLM as the tool result with `IsError: true`:

```go
Execute: func(ctx context.Context, args json.RawMessage, tc plugin.ToolContext) (string, error) {
    var input myArgs
    if err := json.Unmarshal(args, &input); err != nil {
        return "", fmt.Errorf("invalid arguments: %w", err)
    }
    result, err := doWork(input)
    if err != nil {
        return "", fmt.Errorf("operation failed: %w", err)
    }
    return result, nil
},
```

### Example: notify plugin

The `cmd/plugin-notify/` plugin demonstrates a tool-only plugin. It sends desktop notifications using platform-specific commands (osascript on macOS, notify-send on Linux):

```go
func newPlugin() plugin.Plugin {
    return plugin.Plugin{
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
            Execute: executeNotify,
        }},
    }
}
```

---

## Hook plugins

Hooks let plugins observe and react to events in the tinycode session lifecycle.

### HookHandlers struct

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

All fields are optional. Set only the hooks your plugin needs.

### Available hooks

#### SessionStart

Fires when a new session is created.

```go
type SessionStartEvent struct {
    SessionID string `json:"sessionId"`
    Directory string `json:"directory"`
}
```

```go
SessionStart: func(ctx context.Context, event plugin.SessionStartEvent) error {
    log.Printf("Session started: %s in %s", event.SessionID, event.Directory)
    return nil
},
```

#### SessionEnd

Fires when a session is destroyed.

```go
type SessionEndEvent struct {
    SessionID string `json:"sessionId"`
}
```

```go
SessionEnd: func(ctx context.Context, event plugin.SessionEndEvent) error {
    return cleanup(event.SessionID)
},
```

#### PermissionAsk

Fires when a tool requests permission. Return a `PermissionOutput` to auto-allow or auto-deny:

```go
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
```

```go
PermissionAsk: func(ctx context.Context, input plugin.PermissionInput) (*plugin.PermissionOutput, error) {
    // Auto-allow read operations
    if input.ToolName == "read" {
        return &plugin.PermissionOutput{Allowed: true}, nil
    }
    // Return nil to fall through to the default permission prompt
    return nil, nil
},
```

#### ShellEnv

Fires before shell commands execute. Return a `ShellEnvOutput` to inject environment variables:

```go
type ShellEnvInput struct {
    SessionID string            `json:"sessionId"`
    Directory string            `json:"directory"`
    Env       map[string]string `json:"env,omitempty"`
}

type ShellEnvOutput struct {
    Env map[string]string `json:"env"`
}
```

```go
ShellEnv: func(ctx context.Context, input plugin.ShellEnvInput) (*plugin.ShellEnvOutput, error) {
    return &plugin.ShellEnvOutput{
        Env: map[string]string{
            "MY_PLUGIN_VAR": "some-value",
        },
    }, nil
},
```

#### ToolExecBefore

Fires before any tool executes. Observe-only (cannot modify args from this hook).

```go
type ToolExecBeforeInput struct {
    SessionID string `json:"sessionId"`
    ToolName  string `json:"toolName"`
    ToolArgs  string `json:"toolArgs"`
}
```

```go
ToolExecBefore: func(ctx context.Context, input plugin.ToolExecBeforeInput) error {
    log.Printf("Tool %s called in session %s", input.ToolName, input.SessionID)
    return nil
},
```

#### ToolExecAfter

Fires after any tool executes. Can modify the output by returning a `ToolExecAfterOutput`:

```go
type ToolExecAfterInput struct {
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

```go
ToolExecAfter: func(ctx context.Context, input plugin.ToolExecAfterInput) (*plugin.ToolExecAfterOutput, error) {
    // Truncate very long outputs
    if len(input.Output) > 10000 {
        return &plugin.ToolExecAfterOutput{
            Output: input.Output[:10000] + "\n[truncated]",
        }, nil
    }
    // Return nil to pass through the original output unchanged
    return nil, nil
},
```

#### Dispose

Fires on clean shutdown (stdin closed). Use this to flush data, close connections, or release resources:

```go
Dispose: func(ctx context.Context) error {
    return db.Close()
},
```

---

## Combined plugins: tools + hooks

Most real plugins combine tools and hooks. The telemetry plugin (`cmd/plugin-telemetry/`) is a good example:

```go
func newPlugin() plugin.Plugin {
    s := &state{}
    return plugin.Plugin{
        ID:    "telemetry",
        Tools: buildTools(s),
        Hooks: buildHooks(s),
    }
}
```

It provides two tools (`telemetry_report`, `telemetry_query`) and uses hooks (`SessionStart`, `ToolExecAfter`, `SessionEnd`, `Dispose`) to track tool call metrics in a SQLite database:

- **SessionStart** -- records when sessions begin
- **ToolExecAfter** -- buffers tool call records
- **SessionEnd** -- flushes buffered records to the database
- **Dispose** -- closes the database connection

This pattern of shared mutable state (`&state{}`) passed to both tool and hook builders is common in plugins that need coordination between tools and lifecycle events.

---

## Wire protocol

Plugins communicate with tinycode over JSON-RPC 2.0 via stdin/stdout. Each message is a single JSON line.

### Handshake

1. tinycode sends an `initialize` request:

```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"version":"2.0","directory":"/path/to/project"}}
```

2. The plugin responds with its manifest:

```json
{"jsonrpc":"2.0","id":1,"result":{"id":"my-plugin","tools":[{"name":"greet","description":"Greet someone","inputSchema":{...}}],"hooks":["session.start"]}}
```

### Tool calls

```json
{"jsonrpc":"2.0","id":2,"method":"tool/call","params":{"name":"greet","args":{"name":"World"},"context":{"sessionId":"s1","directory":"/tmp"}}}
```

Response:

```json
{"jsonrpc":"2.0","id":2,"result":{"content":"Hello, World!","isError":false}}
```

### Hook invocations

```json
{"jsonrpc":"2.0","id":3,"method":"hook/invoke","params":{"name":"session.start","input":{"sessionId":"s1","directory":"/tmp"}}}
```

Response:

```json
{"jsonrpc":"2.0","id":3,"result":{"output":null}}
```

Hooks that return output (PermissionAsk, ShellEnv, ToolExecAfter) include the output in the `output` field.

### Notifications

Messages with no `id` field are notifications and receive no response.

---

## Testing

The SDK includes test patterns in `pkg/plugin/plugin_test.go`. The core approach is to call `run()` directly with mock stdin/stdout buffers.

### Test pattern

```go
func TestMyPlugin(t *testing.T) {
    p := newPlugin()

    // Build JSON-RPC requests
    initParams, _ := json.Marshal(plugin.InitializeParams{
        Version:   "2.0",
        Directory: "/tmp",
    })
    toolParams, _ := json.Marshal(plugin.ToolCallParams{
        Name:    "greet",
        Args:    json.RawMessage(`{"name":"World"}`),
        Context: plugin.ToolContext{SessionID: "s1", Directory: "/tmp"},
    })

    // Write requests to stdin buffer
    var input bytes.Buffer
    writeRequest(&input, plugin.JSONRPCRequest{
        JSONRPC: "2.0", ID: 1, Method: "initialize", Params: initParams,
    })
    writeRequest(&input, plugin.JSONRPCRequest{
        JSONRPC: "2.0", ID: 2, Method: "tool/call", Params: toolParams,
    })

    // Run the plugin
    var output bytes.Buffer
    err := run(context.Background(), p, &input, &output)
    if err != nil {
        t.Fatalf("run error: %v", err)
    }

    // Parse and verify responses
    responses := parseResponses(t, output.String())
    // ... assert on responses
}

func writeRequest(buf *bytes.Buffer, req plugin.JSONRPCRequest) {
    data, _ := json.Marshal(req)
    buf.Write(data)
    buf.WriteByte('\n')
}
```

### Testing hooks

```go
func TestSessionStartHook(t *testing.T) {
    var receivedID string
    p := plugin.Plugin{
        ID: "test",
        Hooks: plugin.HookHandlers{
            SessionStart: func(_ context.Context, event plugin.SessionStartEvent) error {
                receivedID = event.SessionID
                return nil
            },
        },
    }

    hookInput, _ := json.Marshal(plugin.SessionStartEvent{
        SessionID: "ses-123",
        Directory: "/tmp",
    })
    hookParams, _ := json.Marshal(plugin.HookParams{
        Name:  "session.start",
        Input: hookInput,
    })

    // Send initialize + hook/invoke, verify receivedID == "ses-123"
}
```

---

## Installation and binary resolution

When tinycode loads a plugin by name, it searches for the binary in this order:

1. **Config directory**: `~/.config/tinycode/plugins/<name>`
2. **PATH**: looks for `tinycode-plugin-<name>` on the system PATH
3. **Registry**: checks the built-in registry for install instructions

### Installing a plugin

**Option 1: Build to the config directory**

```bash
go build -o ~/.config/tinycode/plugins/my-plugin ./cmd/plugin-my-plugin
```

**Option 2: Install to PATH**

```bash
go install github.com/example/tinycode-plugin-my-plugin@latest
```

The binary name must follow the `tinycode-plugin-<name>` convention.

**Option 3: Registry plugins**

Some plugins are listed in the built-in registry. If a plugin is in the registry but not installed, tinycode reports the error with the repository URL for installation.

---

## Available plugins

These plugins ship with tinycode in `cmd/plugin-*/`:

**General (12)**

| Plugin | ID | Type | Description |
|--------|----|------|-------------|
| `plugin-notify` | `notify` | Tool | Desktop notifications (macOS/Linux) |
| `plugin-safety-net` | `safety-net` | Hook | Blocks destructive shell commands via PermissionAsk |
| `plugin-telemetry` | `telemetry` | Tool + Hook | Session and tool-call analytics with SQLite storage |
| `plugin-code-review` | `code-review` | Tool | Git diff formatted as markdown for code review |
| `plugin-handoff` | `handoff` | Tool + Hook | Save/restore session context for handoff between sessions |
| `plugin-web-search` | `web-search` | Tool | Web search via DuckDuckGo |
| `plugin-pilot` | `pilot` | Tool | Issue tracker integration (GitHub, GitLab, Gitea) |
| `plugin-cluster-ops` | `cluster-ops` | Tool | OpenShift cluster authentication via `oc login` |
| `plugin-snippets` | `snippets` | Tool | Kubernetes resource templates |
| `plugin-context-pruning` | `context-pruning` | Hook | Deduplicates repeated tool outputs via ToolExecAfter |
| `plugin-log-sanitizer` | `log-sanitizer` | Hook | Strips secrets and sensitive data from tool output |
| `plugin-command-inject` | `command-inject` | Hook | Custom slash command injection |

**Red Hat — OpenShift (4)**

| Plugin | ID | Type | Description |
|--------|----|------|-------------|
| `plugin-ocp-context-injection` | `ocp-context-injection` | Hook | Cluster context injection |
| `plugin-ocp-oauth` | `ocp-oauth` | Tool | OAuth login + shell env |
| `plugin-ocp-obs-logging` | `ocp-obs-logging` | Tool | Loki/Tempo/NetObserv |
| `plugin-ocp-obs-metrics` | `ocp-obs-metrics` | Tool | PromQL/alerts/silencing |

**Red Hat — Ansible (2)**

| Plugin | ID | Type | Description |
|--------|----|------|-------------|
| `plugin-aap-bridge` | `aap-bridge` | Tool | Job templates/inventories/lint |
| `plugin-eda-events` | `eda-events` | Tool | Event-Driven Ansible bridge |

**Red Hat — RHOAI (6)**

| Plugin | ID | Type | Description |
|--------|----|------|-------------|
| `plugin-rhoai-eval-trustyai` | `rhoai-eval-trustyai` | Tool | Model evaluation/fairness |
| `plugin-rhoai-experiment-tracker` | `rhoai-experiment-tracker` | Tool | MLflow experiments |
| `plugin-rhoai-mcp-bridge` | `rhoai-mcp-bridge` | Tool | Model Context Protocol |
| `plugin-rhoai-mlflow-tools` | `rhoai-mlflow-tools` | Tool | MLflow tools |
| `plugin-rhoai-model-serving` | `rhoai-model-serving` | Tool | Model serving/sandbox |
| `plugin-rhoai-pipelines` | `rhoai-pipelines` | Tool | Data science pipelines |

**Red Hat — Platform (12)**

| Plugin | ID | Type | Description |
|--------|----|------|-------------|
| `plugin-satellite` | `satellite` | Tool | Satellite hosts/errata/services/REX |
| `plugin-quay` | `quay` | Tool | Registry search/tags/vulns |
| `plugin-rhdh` | `rhdh` | Tool | Developer Hub catalog/APIs |
| `plugin-tekton` | `tekton` | Tool | Pipelines/runs/logs |
| `plugin-rhacm` | `rhacm` | Tool | ACM fleet management |
| `plugin-rhacs` | `rhacs` | Tool | ACS security scanning |
| `plugin-rh-api-catalog` | `rh-api-catalog` | Tool | API catalog |
| `plugin-rh-dev-content` | `rh-dev-content` | Tool | Developer content |
| `plugin-rh-ecosystem-catalog` | `rh-ecosystem-catalog` | Tool | Ecosystem catalog via Pyxis |
| `plugin-rhdp-provisioner` | `rhdp-provisioner` | Tool | Developer platform provisioner |
| `plugin-container-linter` | `container-linter` | Tool | Containerfile linting/bootc |
| `plugin-lightwell` | `lightwell` | Tool | Package security/CVEs |

Build all plugins:

```bash
for dir in cmd/plugin-*/; do
    name=$(basename "$dir")
    go build -o ~/.config/tinycode/plugins/${name#plugin-} ./$dir
done
```

---

## Key types reference

### Protocol types (pkg/plugin/protocol.go)

```go
type JSONRPCRequest struct {
    JSONRPC string          `json:"jsonrpc"`
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

type InitializeParams struct {
    Version   string         `json:"version"`
    Directory string         `json:"directory"`
    Options   map[string]any `json:"options,omitempty"`
}

type InitializeResult struct {
    ID    string         `json:"id"`
    Tools []ToolManifest `json:"tools"`
    Hooks []string       `json:"hooks"`
}

type ToolManifest struct {
    Name        string         `json:"name"`
    Description string         `json:"description"`
    InputSchema map[string]any `json:"inputSchema,omitempty"`
}
```

### JSON-RPC error codes

| Code | Meaning |
|------|---------|
| `-32601` | Unknown method |
| `-32602` | Invalid params (bad tool call params, unknown tool) |
| `-32000` | Hook execution error |
