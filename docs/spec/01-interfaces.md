# 1. Interfaces

## 1.1 Terminal UI (TUI)

Package: `internal/tui/`

A reactive terminal interface built with [bubbletea](https://github.com/charmbracelet/bubbletea) using the Elm architecture (Model/Update/View). The TUI spawns an in-process HTTP server on an ephemeral port and communicates with it via an API client and SSE event stream.

### Architecture

- **`app.go`** — Root `App` model containing all TUI state. Pure UI state with no I/O.
- **`run.go`** — `connectedApp` wrapper that adds server communication (API client, SSE subscription, permission replies). This is the outermost bubbletea model.
- **`prompt.go`** — Text input area with `@file` reference support, slash command autocomplete, and a startup guard that discards rune input for 2 seconds after first render (filters leaked OSC terminal escape sequences via `isTerminalEscape()` regex).
- **`chat.go` / `chat_render.go`** — Conversation history rendering with markdown, code blocks, tool call results, and thought block expansion.
- **`sidebar.go`** — Session tree sidebar (toggled via `<leader>b`).
- **`palette.go`** — Unified command palette (agents, sessions, skills, commands).
- **`dialog_model.go`** — Model/provider selection dialog with two-step flow: select provider, then select model. Type-to-search filter, max 8 visible items with scroll.
- **`statusbar.go`** — Bottom status bar showing agent, model, working state.
- **`leader.go`** — Vim-style leader key state machine (default `Ctrl+X`, 500ms timeout).
- **`layout.go`** — Layout calculations for responsive terminal resizing.
- **`styles.go`** — Lip Gloss style definitions.
- **`toast.go`** — Non-blocking overlay notification system.
- **`overlay_test.go`** — Permission prompt overlay.
- **`msg.go`** — Message type definitions for the bubbletea message bus.
- **`cmd.go`** — Tea command constructors.
- **`state.go`** — State management helpers.
- **`welcome.go`** — Welcome screen / onboarding.

### Key Features

- Session management with parent-child hierarchy displayed as a toggleable ASCII tree sidebar
- Unified command palette searching across commands, agents, sessions, and skills
- Toast notifications for warnings and errors (e.g., tool-call failure warnings)
- Model picker with provider grouping, type-to-search filter
- Agent switcher via Tab/Shift+Tab cycling or `<leader>a` picker
- File references via `@filename` in the prompt input
- Slash commands (`/skill-name args`) for invoking skills
- Thinking/reasoning blocks rendered as expandable `+/- Thought` blocks (`T` key toggles all)
- Diff viewer for reviewing file changes
- Permission prompt overlay with Allow / Always Allow / Reject actions

### Keybindings

**Leader key** = `Ctrl+X` by default (500ms timeout for follow-up key, configurable via `keybinds.leader` in config)

| Binding | Action |
|---------|--------|
| `Ctrl+C` | Clear prompt / quit (if empty) |
| `Ctrl+D` | Quit tinycode |
| `Ctrl+P` | Unified command palette |
| `Enter` | Submit prompt |
| `Shift+Enter` / `Alt+Enter` | Insert newline in prompt |
| `Escape` | Interrupt current session |
| `Tab` / `Shift+Tab` | Cycle to next/previous agent |
| `F2` / `Shift+F2` | Cycle to next/previous recent model |
| `PgUp` / `PgDown` | Scroll chat history |
| `T` | Toggle all thought blocks |
| `<leader>b` | Toggle session tree sidebar |
| `<leader>o` | List all sessions |
| `<leader>n` | Create a new session |
| `<leader>m` | List available models |
| `<leader>a` | List available agents |
| `<leader>h` / `<leader>l` | Navigate sibling sessions |
| `<leader>j` / `<leader>k` | Navigate child/parent sessions |
| `<leader>c` | Compact context |
| `<leader>;` | Toggle code block concealment |

### Spinner Tick Chain

`SetWorking(true)` returns a `tea.Cmd` that must be propagated to maintain the spinner animation tick chain. Failure to propagate stops the spinner.

### SSE Event Flow

1. `api.Client.Subscribe()` returns a channel of `ServerEvent`
2. `waitForSSE()` converts channel reads to `tea.Cmd`
3. Events mapped to TUI messages via `mapSSEToMsg()`
4. TUI updates state and re-renders

### `/connect` Dialog

Two-step model selection flow:
1. `handleClientCommand("connect")` sets `PendingModelDialog=true` and triggers `ProvidersRefreshMsg`
2. Async provider re-fetch populates the model list
3. User selects provider, then model from filtered list (max 8 visible, scrollable, type-to-search)

---

## 1.2 Web UI

The Go binary can optionally serve a web UI. The TypeScript web app (`packages/app`) can be embedded into the binary via `make embed-webapp`. When `cfg.ServeWebUI` is true, the static file handler at `/` serves the SPA.

Package: `internal/static/`

The static server supports:
- Embedded `embed.FS` assets from the build
- Dev directory override for development
- SPA fallback (all non-file routes serve `index.html`)

> **Note:** The web UI is the original TypeScript app, not a Go-native implementation. It connects to the Go HTTP server's REST and SSE endpoints.

---

## 1.3 CLI Commands

| Command | Description |
|---------|-------------|
| `tinycode` | Launch TUI (default mode) |
| `tinycode <directory>` | Launch TUI against specified directory |
| `tinycode tui` | Explicit TUI launch |
| `tinycode serve` | Start headless API server |
| `tinycode web` | Start server and open web UI |
| `tinycode acp` | Agent Client Protocol mode (stdio) |
| `tinycode run [message]` | Run a prompt non-interactively |
| `tinycode models` | List available models |
| `tinycode providers` | List discovered providers |
| `tinycode session` | Session management (list, delete) |
| `tinycode status` | Show server health and status |
| `tinycode export [sessionID]` | Export session messages as JSON |
| `tinycode plugin` | Plugin management (list, install, uninstall) |
| `tinycode agent` | List available agents |
| `tinycode debug` | Debug info (config, paths) |
| `tinycode version` | Print version information |
| `tinycode help` | Show help |

Source: `cmd/tinycode/main.go`

### Global Flags

| Flag | Description |
|------|-------------|
| `-m`, `--model` | Model to use (`provider/model` format) |

---

## 1.4 `tinycode run` — Non-Interactive CLI

Source: `cmd/tinycode/run.go`, `cmd/tinycode/run_helpers.go`

A headless mode for scripting, CI/CD, and programmatic interaction. Three permission modes determine how tool permissions are handled.

### Flags

| Flag | Description |
|------|-------------|
| `--agent` | Agent to use (default: `build`) |
| `--format` | Output format: `default` (text) or `json` (NDJSON) |
| `-c`, `--continue` | Continue the most recently updated session |
| `-s`, `--session` | Continue a specific session by exact ID |
| `-r`, `--resume` | Resume by session ID or title/slug (TUI parity) |
| `--title` | Session title; with `-c/--continue`, updates the continued session title |
| `--dangerously-skip-permissions` | Auto-approve all tool permissions |
| `-i`, `--interactive` | Show terminal permission prompts |
| `--permissions` | Permission handling: `default` or `json` |
| `--max-iterations` | Maximum processor iterations per prompt (0 = default 200) |
| `--multi-turn` | Multi-turn mode: loop on stdin after initial prompt |
| `--fail-fast` | Multi-turn: exit immediately on the first turn error |
| `--append-system-prompt` | Append text to the system prompt |
| `--append-system-prompt-file` | Append file contents to the system prompt |
| `--max-tokens` | Cumulative token budget (input+output) |
| `--safe-mode` | Skip plugins, MCP servers, and user agents |

### Permission Modes

Default rules allow `read *`. Headless ask handling is separate: when a tool needs an Ask (e.g. shell/edit), the reply is auto-rejected unless one of the modes below applies.

| Mode | Behavior |
|------|----------|
| Default (no flags) | Auto-reject Ask requests (allowed rules like `read *` still pass without Ask) |
| `--interactive` / `-i` | Show terminal prompts for each Ask |
| `--dangerously-skip-permissions` | Auto-approve everything |
| `--permissions json` | Programmatic JSON protocol over stdin/stdout |
| Config `permission.allow` / `deny` | Pre-approve or deny patterns without prompting |

### NDJSON Event Output (`--format json`)

Each line is a JSON object with a `type` field:

| Type | Fields | Description |
|------|--------|-------------|
| `session` | `sessionID` | Emitted once after session resolve |
| `text` | `text` | LLM text output fragment |
| `tool_begin` | `toolName`, `toolCallID` | Tool call starting (LLM began the call) |
| `tool_call_end` | `toolName`, `toolCallID`, `toolArgs` | LLM finished streaming tool-call args |
| `tool_end` | `toolName`, `toolCallID`, `output`, `isError` | Tool execution completed with result |
| `reasoning` | `sessionID`, `text` | Model reasoning/thinking output |
| `step_start` | `stepID`, `iteration`, `model` | Processor loop iteration beginning |
| `step_finish` | `stepID`, `iteration`, `usage`, `error` | Processor loop iteration completed |
| `warning` | `message` | Warning from the processor (e.g., doom loop detection) |
| `compacted` | `compactionNum`, `preMessages`, `postMessages` | Context compaction occurred |
| `permission` | `id`, `permission`, `patterns` | Ask request (`--permissions json`) |
| `ready` | — | Multi-turn: ready for next prompt |
| `done` | `sessionID`, `ok` | Run finished successfully |
| `error` | `sessionID`, `message` | Run or turn failed |

Example stream:

```json
{"type":"session","sessionID":"ses_01HQXY"}
{"type":"step_start","stepID":"step_1","iteration":1,"model":"qwen3:8b"}
{"type":"text","text":"I'll read the file first.\n"}
{"type":"tool_begin","toolName":"read","toolCallID":"call_abc123"}
{"type":"tool_call_end","toolName":"read","toolCallID":"call_abc123","toolArgs":"{\"path\":\"main.go\"}"}
{"type":"tool_end","toolName":"read","toolCallID":"call_abc123","output":"file contents...","isError":false}
{"type":"text","text":"The file contains..."}
{"type":"step_finish","stepID":"step_1","iteration":1,"usage":{"input":1200,"output":350},"error":null}
{"type":"done","sessionID":"ses_01HQXY","ok":true}
```

In text format (`--format default`), tool progress is written to stderr (`tool <name> begin` / `tool <name> end (ok|error)`), and the session ID is written as `session: <id>` on stderr.

### Multi-Turn Protocol

In `--multi-turn` mode, the processor is called multiple times. Messages accumulate across prompts; the iteration counter resets for each new prompt. If any turn fails, the process exits non-zero after the loop (unless `--fail-fast` exits immediately). SIGINT exits with code 130.

**Text protocol** (when `--format default`):
- One line per prompt from stdin
- `exit` or `quit` to end the session
- Empty lines are ignored

**JSON protocol** (when `--format json`):
`readNextPrompt()` reads JSON objects from stdin and routes them by `type`:

| Type | Fields | Description |
|------|--------|-------------|
| `prompt` | `text` | Submit a new prompt |
| `exit` | — | End the session |
| `permission_reply` | `id`, `reply` | Reply to a pending permission request |

`permission_reply` messages are routed to a dedicated `permReplyCh` channel. Only `prompt` and `exit` reach the main loop.

### JSON Permission Protocol (`--permissions json`)

Permission requests are emitted as NDJSON objects on stdout:

```json
{"type":"permission","id":"perm_abc","permission":"bash","patterns":["git status"]}
```

Replies are read from stdin:

```json
{"type":"permission_reply","id":"perm_abc","reply":"once"}
```

Valid `reply` values: `"once"`, `"always"`, `"reject"`.

### Iteration Budget

`--max-iterations N` caps LLM round-trips per prompt.

| Constant | Value | Source |
|----------|-------|--------|
| `maxIterations` | 200 | `internal/session/processor.go` |
| `--max-iterations 0` | (uses default 200) | `maxIter()` helper |

When exceeded, the processor returns: `"processor exceeded N iterations"`. The counter resets at the start of each `Processor.Process()` call.

### Config Permission Rules

Permission rules can be pre-configured in `config.json` to avoid interactive prompts:

```json
{
  "permission": {
    "allow": ["read *", "bash git *"],
    "deny": ["edit /etc/*"]
  }
}
```

Loaded via `permission.FromConfig(allow, deny)` → `permSvc.SetBaseRules(rules)`.

Pattern format: `<permission-name> [<path-glob>]`. If no path is specified, `*` (match all) is assumed. `~/` and `$HOME/` are expanded. These rules sit between `DefaultRules` and agent/always-approved rules in the evaluation chain (see [12-permissions.md](12-permissions.md)).

### Piped Input

When stdin is not a TTY, stdin content is read and appended to the message argument.

---

## 1.5 Agent Client Protocol (ACP)

Package: `internal/acp/`

IDE integration via stdio-based agent communication using newline-delimited JSON.

Source: `cmd/tinycode/main.go` (`runACP`)

### Supported Operations

| Operation | Description |
|-----------|-------------|
| `initialize` | Handshake |
| `newSession` | Create new session |
| `loadSession` | Load existing session |
| `listSessions` | List all sessions |
| `prompt` | Send message |
| `cancel` | Abort current processing |

---

## 1.6 Headless Server (`tinycode serve`)

Starts the HTTP API server without the TUI. The server binds to the configured host and port (default `127.0.0.1:4096`) and serves the REST + SSE API documented in [02-api-routes.md](02-api-routes.md).

### Behavior

- Blocks until SIGINT or SIGTERM
- Graceful shutdown: disposes all sessions, drains connections, stops listener
- Optional web UI serving when `--web` flag is used or `cfg.ServeWebUI` is true
- Bearer token authentication when `TINYCODE_AUTH_TOKEN` is set (auto-generated and logged on startup if unset; disabled via `TINYCODE_NO_AUTH`)

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TINYCODE_PORT` | `4096` | Server port (`0` = ephemeral OS-assigned port) |
| `TINYCODE_HOST` | `127.0.0.1` | Bind address |
| `TINYCODE_AUTH_TOKEN` | (auto-generated) | Bearer token for API auth |
| `TINYCODE_NO_AUTH` | (unset) | Disable auth entirely |
| `TINYCODE_SERVER_PASSWORD` | (none) | Deprecated alias for `TINYCODE_AUTH_TOKEN` |
| `TINYCODE_WEB_DIR` | (none) | Serve web UI from directory (dev mode) |
