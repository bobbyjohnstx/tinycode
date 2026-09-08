# CLAUDE.md

This file provides guidance to Claude Code when working with code in this repository.

tinycode-go is a Go rewrite of [tinycode](https://github.com/bobbyjohnstx/tinycode) (TypeScript). It is a standalone Go binary — the TUI connects to a tinycode server (TypeScript, port 4096) for session management, LLM calls, and tool execution.

## Commands

```bash
# Build
make build                      # builds dist/tinycode

# Run (requires tinycode server on port 4096)
./dist/tinycode                 # TUI mode
./dist/tinycode <directory>     # TUI against a different directory
./dist/tinycode serve           # headless API proxy
./dist/tinycode acp             # Agent Client Protocol (IDE integration, stdio)

# Tests
go test ./... -count=1          # all tests (or: make test)
go test ./internal/tui/... -count=1   # single package

# Lint
go vet ./...                    # or: make lint

# Embed web app into binary (requires packages/app built first)
make embed-webapp
```

## Architecture

Standard Go layout: `cmd/` for binaries, `internal/` for private packages, `pkg/` for public SDK.

### Core packages (`internal/`)

- **`tui/`** — Terminal UI using [bubbletea](https://github.com/charmbracelet/bubbletea) (Elm architecture: Model/Update/View). `app.go` is the root model; `run.go` wraps it with `connectedApp` for server communication via SSE. Sub-components: prompt, chat, sidebar, statusbar, dialogs, palette, permission prompt, toast.
- **`tui/api/`** — HTTP client for the tinycode server API. Types in `types.go`.
- **`server/`** — HTTP server (net/http + chi router). REST + SSE endpoints, middleware.
- **`session/`** — Session lifecycle, processor loop, LLM coordination.
- **`llm/`** — LLM client abstraction. OpenAI-compatible API client with streaming, tool-call JSON repair.
- **`provider/`** — Provider discovery (Ollama, OpenAI-compatible, OpenRouter).
- **`agent/`** — Agent definitions and defaults (`defaults/` has `.md` prompt files).
- **`tool/`** — Tool implementations (file ops, shell, grep, glob).
- **`config/`** — Config file parsing (`~/.config/tinycode/config.json`). JSONC support.
- **`storage/`** — SQLite via modernc.org/sqlite. Migrations in `migrations/`.
- **`bus/`** — Event bus for inter-component communication.
- **`mcp/`** — Model Context Protocol client.
- **`acp/`** — Agent Client Protocol (stdio transport for IDE integration).
- **`plugin/`** — Plugin lifecycle management.
- **`permission/`** — Tool permission prompting and rules.
- **`skill/`** — Skill discovery and loading.
- **`vcs/`** — Git operations.

### Other directories

- **`cmd/tinycode/`** — Main binary entry point.
- **`cmd/plugin-*/`** — Plugin binaries (notify, cluster-ops, safety-net).
- **`pkg/plugin/`** — Public plugin SDK (protocol, hooks, tools).
- **`packages/`** — Legacy TypeScript packages (app, desktop, etc.) from the original repo. The web app (`packages/app`) can be embedded into the Go binary via `make embed-webapp`.

## Key Patterns

- **Bubbletea Elm architecture**: All TUI state is immutable — `Update()` returns a new model. `tea.Cmd` for async work (API calls, SSE). `tea.Msg` for event dispatch.
- **connectedApp wrapper**: `run.go` wraps `App` with an API client. Handles prompt submission, session creation, SSE event mapping, permission replies. The inner `App` is pure UI state.
- **SSE event flow**: `api.Client.Subscribe()` returns a channel of `ServerEvent`. `waitForSSE()` converts channel reads to `tea.Cmd`. Events are mapped to TUI messages via `mapSSEToMsg()`.
- **`/ask <agent> <message>`**: Parsed by `parseAskCommand()`, validated against loaded agent list via `isKnownAgent()`, sent as `agent` field on `PromptInput`.
- **Startup guard**: OSC terminal escape responses leak as printable characters on startup. `prompt.go` has a guard that discards rune input for 2 seconds after first render, plus `isTerminalEscape()` regex filtering.
- **Leader key**: Vim-style `<leader>` key sequences for sidebar toggle, agent/model/session lists. State machine in `leader.go`.
- **Toast overlay**: Non-blocking notification rendered atop the main view. Used for errors and status messages.

## Agent Delegation

Use specialized agents instead of doing everything inline:

- **`debugger`** — Finding and diagnosing bugs. Root-cause analysis, race conditions, stack traces.
- **`executor`** — Writing and editing code. Implementation work, refactors, applying fixes.
- **`architect`** — Designing solutions. Architecture decisions, API design, system-level trade-offs.

## Known Issues

See `docs/architecture-review.md` for the full audit. Key hazards when working in the codebase:

- **Plugin subsystem is broken.** `internal/plugin/` (server-side) and `pkg/plugin/` (SDK) use incompatible wire protocols — different JSON-RPC method names, mismatched initialize field names, and the server drops plugin-provided tools. Do not assume plugins work without fixing protocol alignment first.
- **Path traversal in file handlers.** `handler_file.go` passes raw `path` query params to `os.ReadFile`/`os.ReadDir` — no validation against the working directory.
- **Data races.** `session_manager.Abort()` reads `processor` without the lock; `maybeWarmup` mutates `Model.Capabilities` from a goroutine; MCP `Configure` has an unlock/relock gap during iteration.
- **SSE route/handler inversion.** `/event` calls `StreamGlobalEvents` (wrapped) and `/global/event` calls `StreamEvents` (flat) — names are swapped from what the route names suggest.
- **Dead TUI components.** `DiffView`, `Workspace`, `Theme` (except Toast), escape-to-abort, and `SendPluginEvent` are implemented but never wired into the app.
