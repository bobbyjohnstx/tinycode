# Adding a Tool

There are two ways to add a tool to tinycode. Choose based on whether the tool needs access to internal packages.

## Path 1: Plugin Tool (Recommended for New Tools)

Plugin tools are standalone Go binaries that communicate with tinycode via JSON-RPC over stdin/stdout. They use the `pkg/plugin/` SDK and do not need access to internal packages.

### Plugin SDK Types

A plugin defines its tools using `plugin.ToolDef` (`pkg/plugin/tool.go`):

```go
type ToolDef struct {
    Name        string
    Description string
    Parameters  map[string]any
    Execute     func(ctx context.Context, args json.RawMessage, tc ToolContext) (string, error)
}
```

A plugin is registered and started with `plugin.Plugin` and `plugin.Run()` (`pkg/plugin/plugin.go`):

```go
type Plugin struct {
    ID    string
    Tools []ToolDef
    Hooks HookHandlers
}

func Run(p Plugin)
```

`Run()` starts the JSON-RPC stdin/stdout loop, handles the `initialize` handshake, and dispatches tool calls and hook invocations.

### Minimal Example: plugin-notify

The simplest plugin (`cmd/plugin-notify/main.go`) defines one tool with two required parameters:

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "os/exec"
    "runtime"

    "github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

type notifyArgs struct {
    Title   string `json:"title"`
    Message string `json:"message"`
}

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
            Execute: func(ctx context.Context, raw json.RawMessage, _ plugin.ToolContext) (string, error) {
                var args notifyArgs
                if err := json.Unmarshal(raw, &args); err != nil {
                    return "", fmt.Errorf("invalid arguments: %w", err)
                }
                var cmd *exec.Cmd
                switch runtime.GOOS {
                case "darwin":
                    cmd = exec.CommandContext(ctx, "osascript", "-e",
                        fmt.Sprintf(`display notification "%s" with title "%s"`, args.Message, args.Title))
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
    }
}

func main() {
    plugin.Run(newPlugin())
}
```

### More Complete Example: plugin-code-review

A plugin with optional parameters and use of `ToolContext.Directory` (`cmd/plugin-code-review/main.go`):

```go
func newPlugin() plugin.Plugin {
    return plugin.Plugin{
        ID: "code-review",
        Tools: []plugin.ToolDef{{
            Name:        "code_review",
            Description: "Show git diff for code review, formatted as a markdown diff block",
            Parameters: map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "ref":           map[string]any{"type": "string", "description": "Git ref to diff against (default: HEAD)"},
                    "path":          map[string]any{"type": "string", "description": "Limit diff to a specific file or directory"},
                    "staged":        map[string]any{"type": "boolean", "description": "Show only staged changes"},
                    "context_lines": map[string]any{"type": "integer", "description": "Number of context lines in diff (default: 3)"},
                },
            },
            Execute: executeCodeReview,
        }},
    }
}

func executeCodeReview(ctx context.Context, raw json.RawMessage, tc plugin.ToolContext) (string, error) {
    var args codeReviewArgs
    if err := json.Unmarshal(raw, &args); err != nil {
        return "", fmt.Errorf("invalid arguments: %w", err)
    }
    cmd := exec.CommandContext(ctx, "git", "diff", args.Ref, "--")
    if tc.Directory != "" {
        cmd.Dir = tc.Directory
    }
    output, err := cmd.Output()
    if err != nil {
        return "", fmt.Errorf("git diff failed: %w", err)
    }
    return string(output), nil
}
```

### Step by Step

1. **Create the plugin directory:**

   ```bash
   mkdir cmd/plugin-my-tool
   ```

2. **Write `cmd/plugin-my-tool/main.go`:**

   - Define a struct for your tool's arguments
   - Create a `plugin.Plugin` with your `plugin.ToolDef`
   - Parameters use JSON Schema (`map[string]any`) -- `type`, `properties`, `required`, `description` fields
   - The `Execute` function receives `json.RawMessage` args, a `plugin.ToolContext` (with `SessionID`, `Directory`), and returns `(string, error)`
   - Call `plugin.Run()` in `main()`

3. **Build the binary:**

   ```bash
   go build -o dist/plugin-my-tool ./cmd/plugin-my-tool
   ```

4. **Install the plugin:** Place the binary in your `$PATH` or configure it in `~/.config/tinycode/config.json`:

   ```json
   {
     "plugin": ["my-tool"]
   }
   ```

   The plugin manager looks for a binary named `plugin-<name>` when loading.

5. **Add tests:** Create `cmd/plugin-my-tool/main_test.go` next to the source. Test the execute function directly without the JSON-RPC transport.

### Available Hooks

Plugins can also register hooks via `plugin.HookHandlers`:

| Hook | When It Fires |
|---|---|
| `SessionStart` | A new session begins |
| `SessionEnd` | A session ends |
| `PermissionAsk` | A tool requests permission (can override) |
| `ShellEnv` | Shell tool is about to execute (inject env vars) |
| `ToolExecBefore` | Before any tool executes |
| `ToolExecAfter` | After any tool executes (can modify output) |
| `Dispose` | Plugin is shutting down |

---

## Path 2: Core Tool (For Tools Needing Internal Access)

Core tools live in `internal/tool/` and have access to all internal packages (database, bus, permissions, etc.).

### Core Tool Structure

A core tool is defined using `tool.Def` (`internal/tool/tool.go`):

```go
type Def struct {
    ID          string
    Description string
    Parameters  map[string]any
    Permission  string
    Execute     func(ctx context.Context, tc *Context, args json.RawMessage) (*ExecuteResult, error)
}
```

- `ID` -- the tool name exposed to the LLM
- `Description` -- LLM-facing description of what the tool does
- `Parameters` -- JSON Schema for arguments
- `Permission` -- permission category (e.g., `"read"`, `"write"`, `"shell"`)
- `Execute` -- the implementation; receives a `*Context` with `SessionID`, `Directory`, `Perms`, `Bus`

The execute function returns `*ExecuteResult`:

```go
type ExecuteResult struct {
    Output  string
    IsError bool
}
```

### Example: Read Tool

The read tool (`internal/tool/read.go`) shows the standard pattern:

```go
func ReadTool() *Def {
    return &Def{
        ID:          "read",
        Description: "Read a file from the filesystem. Returns the file contents with line numbers.",
        Permission:  "read",
        Parameters: map[string]any{
            "type": "object",
            "properties": map[string]any{
                "file_path": map[string]any{
                    "type":        "string",
                    "description": "The absolute path to the file to read",
                },
                "offset": map[string]any{
                    "type":        "integer",
                    "description": "The line number to start reading from (0-indexed)",
                },
                "limit": map[string]any{
                    "type":        "integer",
                    "description": "The number of lines to read",
                },
            },
            "required": []string{"file_path"},
        },
        Execute: executeRead,
    }
}

func executeRead(ctx context.Context, tc *Context, rawArgs json.RawMessage) (*ExecuteResult, error) {
    var args readArgs
    if err := json.Unmarshal(rawArgs, &args); err != nil {
        return &ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
    }
    // ... implementation ...
    return &ExecuteResult{Output: output}, nil
}
```

### Step by Step

1. **Create the tool file:** Add `internal/tool/my_tool.go` with a constructor function (e.g., `MyToolTool()`) that returns `*Def`.

2. **Implement the execute function:** Parse `json.RawMessage` args into a typed struct. Use `tc.Directory` for path resolution. Return `*ExecuteResult` with output string and error flag. Return tool-level errors as `IsError: true` results, not Go errors (Go errors indicate system failures).

3. **Set the permission level:** Use an existing permission category (`"read"`, `"write"`, `"shell"`) or define a new one. Tools with an empty `Permission` field skip the permission check.

4. **Register the tool:** Add it to `RegisterBuiltins()` in `internal/tool/builtin.go`:

   ```go
   func RegisterBuiltins(r *Registry) {
       // ... existing tools ...
       r.Register(MyToolTool())
   }
   ```

   If the tool depends on external configuration or API keys, add it to `RegisterConditional()` instead.

5. **Add tests:** Create `internal/tool/my_tool_test.go`. Test the execute function by constructing a `Context` and calling it directly.

### Currently Registered Core Tools

| Tool | Permission | Description |
|---|---|---|
| `read` | `read` | Read file contents with line numbers |
| `write` | `write` | Write content to a file |
| `edit` | `write` | Replace text in a file |
| `shell` | `shell` | Execute shell commands |
| `grep` | `read` | Search file contents with regex |
| `glob` | `read` | Find files by glob pattern |
| `question` | (none) | Ask the user a question |
| `webfetch` | `webfetch` | Fetch content from a URL |
| `task` | `task` | Create and manage subagent tasks |
| `todowrite` | `write` | Write a structured TODO list |
| `skill` | (none) | Execute a skill (conditional) |
| `websearch` | `websearch` | Search the web via Exa API (conditional) |

---

## Which Path Should I Choose?

| | Plugin Tool | Core Tool |
|---|---|---|
| Difficulty | Low | Moderate |
| Prerequisites | Go, `pkg/plugin/` SDK | Go, familiarity with internal packages |
| Access to internals | No (process-isolated) | Yes (database, bus, permissions) |
| Deployment | Standalone binary | Part of the main tinycode binary |
| Testing | Unit test the execute function | Unit test with mock Context |
| Best for | Self-contained utilities, external commands | Tools needing session state or internal APIs |
