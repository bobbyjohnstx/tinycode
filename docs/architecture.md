# Architecture

tinycode is a local-LLM-first AI coding assistant. It ships as a single Go binary that embeds an HTTP server (ephemeral port), a bubbletea terminal UI, session management, LLM streaming, and tool execution. No runtime dependencies beyond the binary itself.

The primary inference targets are Ollama, vLLM, LM Studio, and OpenAI-compatible servers on localhost or LAN. Cloud providers (OpenRouter and any OpenAI-compatible endpoint) are available via API key and config.

---

## Directory Structure

Standard Go layout: `cmd/` for binaries, `internal/` for private packages, `pkg/` for public SDK.

```
cmd/
  tinycode/           Main binary entry point
  plugin-*/           Plugin binaries (30 plugins)
internal/
  acp/                Agent Client Protocol (stdio for IDE integration)
  agent/              Agent definitions and defaults
  bus/                Event bus for inter-component communication
  command/            Slash command discovery and merging
  config/             Config file parsing (JSONC)
  earlyinit/          Package-init side effects (e.g., lipgloss defaults)
  frecency/           Frequency + recency ranking for command palette
  frontmatter/        YAML-like frontmatter parser for markdown files
  id/                 Sortable ID generation with typed prefixes
  llm/                LLM client abstraction (OpenAI-compatible)
  mcp/                Model Context Protocol client
  permission/         Tool permission prompting and rules
  plugin/             Plugin lifecycle management (server side)
  project/            Project metadata and directory detection
  provider/           Provider discovery (Ollama, vLLM, LM Studio, OpenRouter)
  redhat/             Red Hat product integrations (shared library for RH plugins)
  server/             HTTP server (net/http, REST + SSE)
  session/            Session lifecycle, processor loop, LLM coordination
  skill/              Skill discovery and loading
  static/             Embedded web app file server with SPA fallback
  storage/            SQLite via modernc.org/sqlite
  safego/             Panic-recovery goroutine wrapper
  tool/               Tool implementations (18 tools: read, write, edit, shell, grep, glob, etc.)
  tui/                Terminal UI (bubbletea, Elm architecture)
  tui/api/            HTTP client for the embedded server API
  vcs/                Git operations
pkg/
  plugin/             Public plugin SDK (protocol, hooks, tools)
packages/             Legacy TypeScript packages (app, desktop, etc.)
```

---

## Key Packages

### `cmd/tinycode/` -- Entry Point

`main.go` parses CLI commands (`tui`, `serve`, `web`, `acp`, `run`, `models`, `providers`, `session`, `status`, `export`, `agent`, `debug`) and bootstraps dependencies. Each command initializes the bus, database, config, providers, agents, tools, and plugins, then starts the appropriate mode.

### `internal/tui/` -- Terminal UI

Bubbletea Elm-architecture UI. `app.go` defines the root `App` model with sub-components: prompt, chat, sidebar, statusbar, dialogs, palette, permission prompt, toast overlay. `run.go` wraps `App` with `connectedApp`, which manages the API client, SSE subscription, prompt submission, session creation, and permission replies. The inner `App` is pure UI state with no network calls.

### `internal/tui/api/` -- API Client

HTTP + SSE client used by the Go TUI (`connectedApp`) against `tinycode serve`. This is a **TUI subset** (~27 methods: sessions, prompts, abort, permissions, providers/agents/commands, MCP, plugins, fork/rewind/btw, config patch, SSE subscribe). It is not a full OpenAPI client.

The TypeScript `@tinycode/sdk` / `packages/sdk/openapi.json` surface is larger. Routes and clients **not** covered by the Go TUI API package include session share/unshare, PTY, TUI control (`/tui/*`), OAuth/account flows, and other OpenAPI-only paths — see [spec/02-api-routes.md](spec/02-api-routes.md) §2.23 and [spec/16-not-implemented.md](spec/16-not-implemented.md) §16.11–16.12, §16.26. Manual summarize/compact (`POST /session/{id}/summarize`) is implemented in Go and runs `Processor.Compact`.

The embedded Go web UI does **not** support PTY terminals or session share/publish — `config.share` defaults to `"disabled"`, and PTY is gated off in the SPA. Those features exist only in the TypeScript OpenAPI contract.

### `internal/server/` -- HTTP Server

Standard `net/http` server with middleware (CORS, error handling, logging). REST endpoints for sessions, messages, prompts, providers, models, agents, config, and permissions. SSE endpoints for real-time event streaming. Optional static file serving for the embedded web UI.

### `internal/session/` -- Session Store + Processor

Persistence lives in `session.go` (`Store`) and `message.go` (`MessageStore`) — there is no `store.go`. `processor.go` runs the main agent loop: send prompt to LLM, stream response, execute tool calls, manage context overflow via compaction, handle retries, track token usage. Each prompt turn gets its own processor instance.

Runtime orchestration (busy state, prompt dispatch, abort, revert stash) is `SessionManager` in `internal/server/`, not in `internal/session/`. Subagents are synthetic child runs (`sessionID:label`) via the task tool; `ProcessorConfig.MaxSubagents` is currently unused. Session share/unshare and PTY HTTP routes are deferred — see [spec/16-not-implemented.md](spec/16-not-implemented.md) §16.11–16.12.

### `internal/llm/` -- LLM Client

Dual HTTP clients: OpenAI-compatible Chat Completions (`openai.go`) and Anthropic Messages (`anthropic.go`). Selection prefers `Model.API.NPM` / provider ID, with hostname fallback. Both stream SSE, reassemble tool-call arguments across chunks, and redirect unrepairable tool JSON to the `invalid` tool.

### `internal/provider/` -- Provider Discovery

`discovery.go` polls local LLM servers (Ollama, vLLM, LM Studio) at regular intervals, probing for available models. `registry.go` tracks discovered providers and their models. OpenRouter discovery fetches the model list via API when `OPENROUTER_API_KEY` is set. Config-based providers are registered from `config.json`.

### `internal/agent/` -- Agents

`registry.go` loads agent definitions from embedded `.md` files in `defaults/`, user agents from `~/.config/tinycode/agent/`, and project agents from `.tinycode/agent/`. Each agent has a name, mode, description, system prompt, and tool permission rules. Config overrides can adjust model, instructions, and permissions per agent.

### `internal/tool/` -- Tools

`tool.go` defines `Def` (tool definition with ID, description, parameters, permission level, and execute function) and `Registry` (thread-safe tool registration and execution). `builtin.go` registers the core set: read, write, edit, shell, grep, glob, question, webfetch, task, todowrite. Conditional tools (skill, websearch) are registered when their dependencies are available. Execution includes permission checks, bus events, plugin after-hooks, and output truncation.

### `internal/config/` -- Configuration

Parses config files with a 3-name fallback per directory: `tinycode.jsonc` → `tinycode.json` → `config.json`. Global config from `~/.config/tinycode/`. Project config from `.tinycode/` (walks up the directory tree, innermost wins). Supports environment variable overrides (`TINYCODE_PORT`, `TINYCODE_HOST`, `TINYCODE_DB`, `TINYCODE_LOG_LEVEL`). Configs are merged from global, project, and environment sources.

### `internal/storage/` -- Database

SQLite via `modernc.org/sqlite` (pure Go, no CGO). Migrations run automatically on startup. Stores sessions, messages, and project metadata at `~/.local/share/tinycode/tinycode.db`.

### `internal/bus/` -- Event Bus

In-process publish-subscribe event bus. Components publish typed events (session state changes, tool execution, permission requests, provider updates) and subscribe by topic. Used for decoupling the session processor, TUI, permission service, and plugin system.

### `internal/plugin/` -- Plugin Manager (Server Side)

`manager.go` spawns plugin binaries as child processes, performs the JSON-RPC `initialize` handshake, registers plugin-provided tools into the tool registry as `plugin__{pluginName}__{toolName}`, and dispatches hook invocations (session lifecycle, permission, shell env, tool execution) to plugins. Load works for any resolvable binary (`~/.config/tinycode/plugins/<name>` or `tinycode-plugin-<name>` on PATH), not only curated registry names.

### `internal/redhat/` -- Red Hat Product Integrations

Shared library for the in-tree Red Hat and OpenShift plugins. Provides common helpers for OpenShift API access, Red Hat SSO/OAuth token management, UBI container conventions, and product-specific client wrappers (RHACM, RHACS, Quay, RHOAI, Satellite, etc.). Plugins in `cmd/plugin-rh*`, `cmd/plugin-ocp-*`, and other Red Hat-specific plugin directories import this package instead of duplicating integration logic.

### `internal/mcp/` -- Model Context Protocol

Client for MCP servers (stdio transport). Configured via `config.json`. Discovers and registers MCP-provided tools into the tool registry at startup.

### `internal/acp/` -- Agent Client Protocol

Stdio-based JSON-RPC transport for IDE integration. Enables IDEs (VS Code, etc.) to communicate with tinycode sessions over stdin/stdout.

### `internal/permission/` -- Permissions

`service.go` manages tool permission prompts. When a tool requires permission, the service publishes a request via the bus, blocks until a reply arrives (from TUI or CLI), and returns the decision. Supports allow-once, allow-always, and reject.

### `internal/skill/` -- Skills

Discovers skill definitions from `~/.config/tinycode/skill/`, project `.tinycode/skill/`, and built-in defaults. Each skill is a directory with a `SKILL.md` file containing YAML frontmatter. Skills surface as slash commands in the TUI.

### `internal/command/` -- Slash Commands

Merges built-in commands, agent names, user skills, and project skills into a unified command list for TUI autocomplete.

### `pkg/plugin/` -- Plugin SDK

Public SDK for building plugin binaries. Defines `Plugin` (ID, tools, hooks), `ToolDef` (name, description, parameters, execute function), and `HookHandlers` (session start/end, permission, shell env, tool before/after, dispose). `Run()` starts the JSON-RPC stdin/stdout loop.

---

## Data Flows

### TUI Startup

```
cmd/tinycode/main.go
  -> setupLogger()
  -> initDependencies()          # bus, database, config
  -> startDiscovery()            # provider polling goroutines
  -> initAgentRegistry()         # load agent definitions
  -> initTooling()               # tool registry + permission service
  -> plugin.NewManager()         # spawn plugin processes
  -> server.New() + Listen()     # embedded HTTP server on port 0
  -> tui.Run()                   # launch bubbletea program
```

The embedded server binds to an ephemeral port (port 0). The TUI connects to it via the returned URL.

### Request Flow

```
User types prompt in TUI
  -> connectedApp.submitPrompt()     # HTTP POST to /session/:id/prompt
  -> server handler creates prompt
  -> session.Processor.Process()     # main agent loop
    -> llm.Client.ChatCompletionStream()  # SSE to LLM provider
    -> stream text deltas             # bus publish "session.text.delta"
    -> parse tool calls               # bus publish "session.tool.begin"
    -> tool.Registry.Execute()        # permission check + execute
    -> bus publish "session.tool.end"
    -> loop until LLM stops calling tools
  -> SSE events flow to TUI
    -> api.Client.Subscribe()         # SSE channel
    -> mapSSEToMsg()                  # convert to tea.Msg
    -> App.Update()                   # render new state
```

### Plugin Lifecycle

```
plugin.Manager.Load(name)
  -> resolve binary (config dir / PATH)
  -> exec.Command(binPath)           # spawn binary
  -> JSON-RPC "initialize" request
  -> plugin returns manifest (tools, hooks)
  -> register tools as plugin__{name}__{tool}

On tool call:
  -> JSON-RPC "tool/call" (params.args)
  -> plugin executes, returns result

On hook event:
  -> JSON-RPC "hook/invoke" request
  -> plugin processes event

On shutdown:
  -> JSON-RPC "hook/invoke" (dispose)
  -> kill process
```

### Tool Execution

```
Registry.Execute(ctx, name, args, sessionID)
  -> lookup Def by name
  -> check disabled map
  -> permission.Service.Ask()        # blocks for user reply
  -> bus publish "tool.execute.before"
  -> def.Execute(ctx, toolCtx, args)
  -> AfterHook (plugin dispatch)
  -> bus publish "tool.execute.after"
  -> Truncate output if needed
  -> return (output, isError)
```

---

## Dependencies

Key Go dependencies (see `go.mod` for exact versions):

| Dependency | Purpose |
|---|---|
| `github.com/charmbracelet/bubbletea` | Terminal UI framework (Elm architecture) |
| `github.com/charmbracelet/bubbles` | TUI components (textarea, viewport, etc.) |
| `github.com/charmbracelet/lipgloss` | Terminal styling and layout |
| `github.com/charmbracelet/glamour` | Markdown rendering in terminal |
| `github.com/muesli/reflow` | Text wrapping and word-break |
| `modernc.org/sqlite` | Pure-Go SQLite (no CGO) |
| `golang.org/x/term` | Terminal detection and raw mode |

No external router library -- the HTTP server uses standard `net/http` with a custom mux.

---

## Design Decisions

**Why Go.** Single statically-linked binary with no runtime dependencies. Cross-compiles to 5 platforms from a single machine. Fast startup, low memory footprint. The previous TypeScript version required Bun or Node.js at runtime.

**Why bubbletea.** Elm architecture (Model/Update/View) gives deterministic UI state management. All state transitions go through `Update()`, making the TUI testable without a terminal. `tea.Cmd` for async work prevents callback spaghetti. `tea.Msg` for event dispatch replaces ad-hoc event emitters.

**Why JSON-RPC for plugins.** Process isolation -- a crashing plugin cannot take down the host. Language-agnostic wire protocol -- plugins could be written in any language, though the SDK is Go. Stdin/stdout transport avoids network port allocation.

**Why modernc.org/sqlite.** Pure Go, no CGO dependency. Simplifies cross-compilation (CGO + SQLite requires platform-specific C toolchains). Trades some performance for build simplicity -- acceptable for the session/message storage workload.

**Why embedded server.** The TUI communicates with the session engine via HTTP/SSE, the same protocol the web UI and ACP use. This means one code path serves all three interfaces. The server binds to an ephemeral port (port 0) in TUI mode, avoiding conflicts.

---

## Historical Audit

An initial architecture review (`docs/architecture-review.md`) identified five issues: plugin wire protocol mismatch, path traversal in file tools, data races in provider registry, SSE route handler inversion, and dead TUI components. All five have been fixed.
