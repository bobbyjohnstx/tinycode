# Plugin Migration Guide: TypeScript to Go

**Start here only if you already have a TypeScript plugin.** New plugins start from [plugin-development.md](plugin-development.md). This guide maps the old TypeScript SDK onto `pkg/plugin`.

## Overview of differences

| Aspect | TypeScript (v1) | Go (v2) |
|--------|-----------------|---------|
| Runtime | Node.js / Bun | Native binary |
| Package format | npm package | Go binary |
| SDK import | `@tinycode/plugin` or `tinycode-plugin` | `github.com/bobbyjohnstx/tinycode/pkg/plugin` |
| Entry point | `export default { server: ... }` | `plugin.Run(plugin.Plugin{...})` |
| Tool definition | `tool({ ... })` helper | `plugin.ToolDef{}` struct |
| Parameter schema | Zod (`tool.schema.string()`) | JSON Schema (`map[string]any`) |
| Hook registration | Named fields on Hooks object | Fields on `plugin.HookHandlers` struct |
| Effect system | `Effect.gen()` / Promises | Plain Go functions |
| Communication | In-process function calls | JSON-RPC 2.0 over stdin/stdout |
| Distribution | `npm publish` | `go build` + binary in PATH or config dir |

---

## Concept mapping

### Plugin definition

**TypeScript:**

```typescript
import type { PluginModule } from "tinycode-plugin"

export default {
  server: async (input, options) => {
    return {
      tool: { ... },
      "session.start": async (input) => { ... },
    }
  },
} satisfies PluginModule
```

**Go:**

```go
package main

import "github.com/bobbyjohnstx/tinycode/pkg/plugin"

func main() {
    plugin.Run(plugin.Plugin{
        ID:    "my-plugin",
        Tools: []plugin.ToolDef{ ... },
        Hooks: plugin.HookHandlers{
            SessionStart: func(ctx context.Context, event plugin.SessionStartEvent) error { ... },
        },
    })
}
```

### Tool definition

**TypeScript:**

```typescript
import { tool } from "tinycode-plugin/tool"

tool({
  description: "Search a knowledge base",
  args: {
    query: tool.schema.string().describe("The search query"),
    limit: tool.schema.number().optional().describe("Max results"),
  },
  async execute(args, context) {
    return `Found results for: ${args.query}`
  },
})
```

**Go:**

```go
plugin.ToolDef{
    Name:        "search",
    Description: "Search a knowledge base",
    Parameters: map[string]any{
        "type": "object",
        "properties": map[string]any{
            "query": map[string]any{
                "type":        "string",
                "description": "The search query",
            },
            "limit": map[string]any{
                "type":        "number",
                "description": "Max results",
            },
        },
        "required": []string{"query"},
    },
    Execute: func(ctx context.Context, args json.RawMessage, tc plugin.ToolContext) (string, error) {
        var input struct {
            Query string `json:"query"`
            Limit int    `json:"limit"`
        }
        if err := json.Unmarshal(args, &input); err != nil {
            return "", fmt.Errorf("invalid arguments: %w", err)
        }
        return fmt.Sprintf("Found results for: %s", input.Query), nil
    },
}
```

### Parameter schemas

The TypeScript SDK uses Zod for schema validation. The Go SDK uses raw JSON Schema as `map[string]any`.

| Zod (TypeScript) | JSON Schema (Go) |
|-------------------|------------------|
| `tool.schema.string()` | `map[string]any{"type": "string"}` |
| `tool.schema.number()` | `map[string]any{"type": "number"}` |
| `tool.schema.number().int()` | `map[string]any{"type": "integer"}` |
| `tool.schema.boolean()` | `map[string]any{"type": "boolean"}` |
| `tool.schema.enum(["a", "b"])` | `map[string]any{"type": "string", "enum": []string{"a", "b"}}` |
| `tool.schema.array(tool.schema.string())` | `map[string]any{"type": "array", "items": map[string]any{"type": "string"}}` |
| `.optional()` | Omit from `"required"` array |
| `.describe("text")` | `"description": "text"` in the property map |

### Hooks

| TypeScript hook | Go field |
|----------------|----------|
| `"session.start": async (input, output) => {}` | `SessionStart: func(ctx context.Context, event SessionStartEvent) error` |
| `"session.end": async (input, output) => {}` | `SessionEnd: func(ctx context.Context, event SessionEndEvent) error` |
| `"permission.ask": async (input, output) => {}` | `PermissionAsk: func(ctx context.Context, input PermissionInput) (*PermissionOutput, error)` |
| `"shell.env": async (input, output) => {}` | `ShellEnv: func(ctx context.Context, input ShellEnvInput) (*ShellEnvOutput, error)` |
| `"tool.execute.before": async (input, output) => {}` | `ToolExecBefore: func(ctx context.Context, input ToolExecBeforeInput) error` |
| `"tool.execute.after": async (input, output) => {}` | `ToolExecAfter: func(ctx context.Context, input ToolExecAfterInput) (*ToolExecAfterOutput, error)` |
| `dispose: async () => {}` | `Dispose: func(ctx context.Context) error` |

Key difference: The TypeScript SDK uses an observer pattern where hooks receive `(input, output)` and mutate `output`. The Go SDK uses return values -- hooks that need to send data back return a pointer to an output struct. Return `nil` to pass through without modification.

### Tool results

**TypeScript** tools can return a string or a structured `{ title, output, metadata, attachments }` object.

**Go** tools return `(string, error)`. The string is the tool output sent to the LLM. Errors are surfaced as error results.

### Tool context

**TypeScript** `ToolContext` includes `sessionID`, `messageID`, `agent`, `directory`, `worktree`, `abort`, `metadata()`, `ask()`, `progress()`, `messages()`, and `sessionInfo()`.

**Go** `ToolContext` is simpler:

```go
type ToolContext struct {
    SessionID string `json:"sessionId"`
    Directory string `json:"directory"`
}
```

The reduced surface is intentional -- Go plugins run as separate processes and do not have direct access to the tinycode server's internal state.

---

## Step-by-step conversion process

### 1. Create the Go module

```bash
mkdir my-plugin && cd my-plugin
go mod init github.com/example/tinycode-plugin-my-plugin
go get github.com/bobbyjohnstx/tinycode/pkg/plugin
```

### 2. Translate the plugin definition

Start with the `main()` function calling `plugin.Run()`:

```go
package main

import "github.com/bobbyjohnstx/tinycode/pkg/plugin"

func main() {
    plugin.Run(plugin.Plugin{
        ID:    "my-plugin",
        Tools: []plugin.ToolDef{ /* ... */ },
        Hooks: plugin.HookHandlers{ /* ... */ },
    })
}
```

### 3. Convert each tool

For each TypeScript `tool({...})`:

1. Create a `plugin.ToolDef` struct
2. Set `Name` -- TypeScript tools get their name from the object key; Go tools declare it explicitly
3. Copy the `Description`
4. Convert Zod args to JSON Schema `Parameters` (see the mapping table above)
5. Convert the `execute` function:
   - Replace `async execute(args, context)` with `func(ctx context.Context, args json.RawMessage, tc plugin.ToolContext) (string, error)`
   - Define a struct for your args and unmarshal from `json.RawMessage`
   - Replace `return "string"` with `return "string", nil`
   - Replace `throw new Error(...)` with `return "", fmt.Errorf(...)`

### 4. Convert each hook

For each TypeScript hook:

1. Find the corresponding Go field in `HookHandlers` (see mapping table)
2. Hooks that mutate `output` in TypeScript become hooks that return a pointer in Go:
   - `permission.ask`: mutating `output.status` becomes returning `*PermissionOutput`
   - `shell.env`: mutating `output.env` becomes returning `*ShellEnvOutput`
   - `tool.execute.after`: mutating `output.output` becomes returning `*ToolExecAfterOutput`
3. Observer-only hooks (`session.start`, `session.end`, `tool.execute.before`) return `error`

### 5. Handle shared state

TypeScript plugins use closures for shared state between tools and hooks. Go plugins use the same pattern:

```go
type state struct {
    mu   sync.Mutex
    data map[string]string
}

func newPlugin() plugin.Plugin {
    s := &state{data: make(map[string]string)}
    return plugin.Plugin{
        ID:    "my-plugin",
        Tools: buildTools(s),
        Hooks: buildHooks(s),
    }
}
```

Use a mutex to protect shared state -- hooks and tools may be called concurrently.

### 6. Build and install

```bash
# Build to config directory
go build -o ~/.config/tinycode/plugins/my-plugin .

# Or install to PATH with conventional name
go build -o $GOPATH/bin/tinycode-plugin-my-plugin .
```

### 7. Update config

Replace the npm package name in your tinycode config with the Go plugin name:

```json
{
  "plugins": ["my-plugin"]
}
```

---

## Complete before/after example: notify plugin

### TypeScript (before)

```typescript
import type { PluginModule } from "tinycode-plugin"
import { tool } from "tinycode-plugin/tool"
import { exec } from "child_process"

export default {
  server: async (input, options) => {
    return {
      tool: {
        notify: tool({
          description: "Send a desktop notification",
          args: {
            title: tool.schema.string().describe("Notification title"),
            message: tool.schema.string().describe("Notification body"),
          },
          async execute(args) {
            if (process.platform === "darwin") {
              await execPromise(
                `osascript -e 'display notification "${args.message}" with title "${args.title}"'`
              )
            } else {
              await execPromise(`notify-send "${args.title}" "${args.message}"`)
            }
            return "Notification sent"
          },
        }),
      },
    }
  },
} satisfies PluginModule
```

### Go (after)

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "os/exec"
    "runtime"

    "github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

type notifyArgs struct {
    Title   string `json:"title"`
    Message string `json:"message"`
}

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
            Execute: func(ctx context.Context, raw json.RawMessage, _ plugin.ToolContext) (string, error) {
                var args notifyArgs
                if err := json.Unmarshal(raw, &args); err != nil {
                    return "", fmt.Errorf("invalid arguments: %w", err)
                }
                if args.Title == "" || args.Message == "" {
                    return "", fmt.Errorf("title and message are required")
                }

                var cmd *exec.Cmd
                switch runtime.GOOS {
                case "darwin":
                    script := fmt.Sprintf(
                        `display notification "%s" with title "%s"`,
                        args.Message, args.Title,
                    )
                    cmd = exec.CommandContext(ctx, "osascript", "-e", script)
                case "linux":
                    cmd = exec.CommandContext(ctx, "notify-send", args.Title, args.Message)
                default:
                    return "", fmt.Errorf("unsupported platform: %s", runtime.GOOS)
                }

                if err := cmd.Run(); err != nil {
                    return "", fmt.Errorf("notification failed: %w", err)
                }
                return "Notification sent", nil
            },
        }},
    })
}
```

---

## Common gotchas

### 1. No Effect runtime

TypeScript plugins using the Effect library (`Effect.gen()`, `Effect.map()`, etc.) need to be rewritten as plain Go functions. There is no equivalent runtime. Replace Effect pipelines with sequential Go code and explicit error returns.

### 2. Error handling

TypeScript: `throw new Error("message")` or Effect failures.
Go: Return `error` as the second value. Always wrap errors with context:

```go
return "", fmt.Errorf("failed to connect: %w", err)
```

### 3. JSON argument parsing

TypeScript tools receive validated, typed args thanks to Zod schemas. Go tools receive `json.RawMessage` and must unmarshal and validate manually:

```go
var args myArgs
if err := json.Unmarshal(raw, &args); err != nil {
    return "", fmt.Errorf("invalid arguments: %w", err)
}
```

### 4. Async vs synchronous

TypeScript hooks are async (`async (input, output) => {}`). Go hook functions are synchronous but receive a `context.Context` for cancellation. For long-running work, respect `ctx.Done()`.

### 5. Output mutation vs return values

TypeScript hooks mutate an `output` object:

```typescript
"permission.ask": async (input, output) => {
  output.status = "allow"
}
```

Go hooks return a pointer (or nil to pass through):

```go
PermissionAsk: func(ctx context.Context, input plugin.PermissionInput) (*plugin.PermissionOutput, error) {
    return &plugin.PermissionOutput{Allowed: true}, nil
},
```

### 6. Process isolation

TypeScript plugins run in-process with access to the tinycode server's internal APIs (client, shell, project info). Go plugins run as separate processes and communicate only via JSON-RPC. This means:

- No direct access to the server's HTTP client
- No `PluginInput` with `$` shell, `project`, `serverUrl`
- All communication happens through the defined tool and hook interfaces

### 7. Tool result types

TypeScript tools can return structured results (`{ title, output, metadata, attachments }`). Go tools return `(string, error)` -- just the output text. If you need structured metadata, encode it in the string output (e.g., as JSON).

### 8. Plugin name resolution

TypeScript plugins are resolved via npm package names. Go plugins are resolved by binary name:

- Config dir: `~/.config/tinycode/plugins/<name>`
- PATH: `tinycode-plugin-<name>`

Make sure your binary name matches the plugin ID you use in config.

### 9. No schema validation

TypeScript plugins can export a Zod `schema` to validate options before loading. Go plugins receive options in the `InitializeParams.Options` field and must validate them manually in the initialize handler. The SDK's `Run()` function handles the initialize handshake automatically, so custom option validation would need to happen in hook or tool handlers.

### 10. Hooks not available in Go

The following TypeScript hooks do not have Go equivalents:

- `chat.message` -- message interception
- `chat.params` -- LLM parameter modification
- `chat.headers` -- HTTP header injection
- `tool.definition` -- tool definition modification
- `auth` -- custom authentication flows
- `provider` -- custom LLM provider registration
- `config` -- config modification at load time
- `event` -- server event firehose
- `command.execute.before` -- slash command interception
- All `experimental.*` hooks

If your TypeScript plugin relies on these hooks, the functionality must be implemented differently (e.g., as a tool, via config, or as a code change to tinycode itself).

---

## Plugin Refactor Migration

The Go plugin registry was consolidated from 42 plugins (~120 tools) to 30 external plugins (~90 tools) plus 4 core builtins. This section covers what changed and what to update in your config.

### Removed plugins

| Plugin | Replacement |
|--------|-------------|
| `cluster-ops` | Absorbed into `ocp-context-injection` (`oc_login` tool + `ShellEnv` hook) |
| `ocp-oauth` | Absorbed into `ocp-context-injection` (`oc_login` tool + `ShellEnv` hook) |
| `rhoai-mcp-bridge` | Removed — native MCP support in `internal/mcp/` replaces it |
| `snippets` | Removed — LLMs generate better YAML than static templates |
| `web-search` | Removed — generic DuckDuckGo scraping with limited value |
| `command-inject` | Removed — arbitrary script execution was a security risk |
| `eda-events` | Removed — niche hook-only event bridge |

### Promoted to core builtins

These are always available without any config. Remove them from your `"plugins"` array if present.

| Former plugin | Builtin behavior |
|---------------|-----------------|
| `context-pruning` | Hook-only (ToolExecAfter) — deduplicates repeated tool outputs |
| `notify` | 1 tool — desktop notifications (macOS/Linux) |
| `code-review` | 1 tool — git diff formatted for AI review |
| `handoff` | 1 tool + 3 hooks — session context save/restore |

### Merged plugins

| Old plugins | New plugin |
|-------------|------------|
| `rhoai-mlflow-tools` + `rhoai-experiment-tracker` | `rhoai-mlflow` (10 tools + 4 hooks) |
| `rhoai-eval-trustyai` + `rhoai-model-serving` | `rhoai-serving` (14 tools) |

### Must-gather tool changes

The `mg_etcd` and `mg_haproxy` tools were removed from `ocp-must-gather` to avoid overlap with specialist plugins. Use `etcd-diag` and `ingress-inspect` instead — they provide deeper, dedicated analysis of the same must-gather data.

### Config compatibility

If your `"plugins"` config array contains any removed or promoted plugin name, it is silently skipped at startup (debug-level log, no crash). No config change is required, but cleaning up stale entries is recommended.

### New CLI commands

- `tinycode init` — optional Red Hat plugin/role setup: prompts for username, picks plugins by role (models via `/connect`, `OPENROUTER_API_KEY`, or Ollama)
- `tinycode plugin list --category <slug>` — filter plugins by category (`sre`, `security`, `ai-ml`, `platform`, `developer`, `essential`)

### Config migration example

Before:
```json
{
  "plugins": [
    "ocp-oauth", "ocp-context-injection", "cluster-ops",
    "context-pruning", "notify", "code-review", "handoff",
    "rhoai-mlflow-tools", "rhoai-experiment-tracker",
    "rhoai-eval-trustyai", "rhoai-model-serving",
    "rhoai-pipelines", "safety-net"
  ]
}
```

After:
```json
{
  "plugins": [
    "ocp-context-injection",
    "rhoai-mlflow", "rhoai-serving", "rhoai-pipelines",
    "safety-net"
  ]
}
```

Builtins (`context-pruning`, `notify`, `code-review`, `handoff`) are automatic. `ocp-oauth` and `cluster-ops` are absorbed into `ocp-context-injection`. RHOAI plugins are merged.
