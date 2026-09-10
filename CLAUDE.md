# CLAUDE.md

This file provides guidance to Claude Code when working with code in this repository.

tinycode-go is a Go rewrite of [tinycode](https://github.com/bobbyjohnstx/tinycode) (TypeScript). It is a standalone Go binary — a single process embeds the HTTP server (ephemeral port), TUI (bubbletea), session management, LLM client, and tool execution.

## Commands

```bash
# Build
make build                      # builds dist/tinycode

# Run
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
- **`config/`** — Config file parsing. On macOS, loads from both `~/Library/Application Support/tinycode/` and `~/.config/tinycode/` (for TS tinycode compatibility). JSONC support. `LSPConfig` accepts both boolean and struct JSON.
- **`storage/`** — SQLite via modernc.org/sqlite. Migrations in `migrations/`.
- **`bus/`** — Event bus for inter-component communication.
- **`lsp/`** — LSP client for code intelligence. Lazy-connects to language servers (gopls, pyright, rust-analyzer, typescript-language-server) and exposes tools: `lsp_diagnostics`, `lsp_hover`, `lsp_definition`, `lsp_references`, `lsp_symbols`. Auto-detects project language from marker files.
- **`mcp/`** — Model Context Protocol client.
- **`acp/`** — Agent Client Protocol (stdio transport for IDE integration).
- **`plugin/`** — Plugin lifecycle management.
- **`permission/`** — Tool permission prompting and rules.
- **`skill/`** — Skill discovery and loading.
- **`vcs/`** — Git operations.
- **`command/`** — Slash command discovery. Merges built-in commands, agent names, user skills, and project skills into a unified command list.
- **`earlyinit/`** — Package-init side effects that must run before other imports (e.g., lipgloss dark-background default).
- **`frontmatter/`** — Simple YAML-like frontmatter parser for markdown files. Used by skill and agent loaders.
- **`id/`** — Sortable ID generation with typed prefixes (`ses_`, `msg_`, `evt_`, etc.). Supports ascending and descending time ordering.
- **`project/`** — Project metadata: directory-based ID generation, VCS detection, worktree paths.
- **`redhat/`** — Red Hat shared library: OcClient (oc CLI wrapper), APIClient (HTTP with retry/auth), ConsoleAuthClient (SSO token exchange), PromQLClient (Prometheus/AlertManager), ContainerfileParser, MLflow client, HTML stripping.
- **`static/`** — Embedded web app file server with SPA fallback. Serves `dist/` assets via `embed.FS` or a dev directory override.

### Other directories

- **`cmd/tinycode/`** — Main binary entry point.
- **`cmd/plugin-*/`** — Plugin binaries (cluster-ops, code-review, command-inject, context-pruning, handoff, log-sanitizer, notify, pilot, safety-net, snippets, telemetry, web-search, ocp-context-injection, ocp-oauth, ocp-obs-logging, ocp-obs-metrics, aap-bridge, eda-events, rhoai-eval-trustyai, rhoai-experiment-tracker, rhoai-mcp-bridge, rhoai-mlflow-tools, rhoai-model-serving, rhoai-pipelines, satellite-lightspeed, quay, rhdh, tekton, rhacm, rhacs, rh-api-catalog, rh-dev-content, rh-ecosystem-catalog, rhdp-provisioner, container-linter, lightwell).
- **`pkg/plugin/`** — Public plugin SDK (protocol, hooks, tools).
- **`packages/`** — Legacy TypeScript packages (app, desktop, etc.) from the original repo. The web app (`packages/app`) can be embedded into the Go binary via `make embed-webapp`.

## Key Patterns

- **Bubbletea Elm architecture**: All TUI state is immutable — `Update()` returns a new model. `tea.Cmd` for async work (API calls, SSE). `tea.Msg` for event dispatch.
- **connectedApp wrapper**: `run.go` wraps `App` with an API client. Handles prompt submission, session creation, SSE event mapping, permission replies. The inner `App` is pure UI state.
- **SSE event flow**: `api.Client.Subscribe()` returns a channel of `ServerEvent`. `waitForSSE()` converts channel reads to `tea.Cmd`. Events are mapped to TUI messages via `mapSSEToMsg()`.
- **`/ask <agent> <message>`**: Parsed by `parseAskCommand()`, validated against loaded agent list via `isKnownAgent()`, sent as `agent` field on `PromptInput`. Prompt autocomplete filters `/ask` suggestions to non-primary agents.
- **`/connect` dialog**: Two-step flow — select provider, then select model. `handleClientCommand("connect")` sets `PendingModelDialog=true` and triggers `ProvidersRefreshMsg` for an async re-fetch. Model list is capped at 8 visible items with scroll and type-to-search filter.
- **Permission prompt**: Shows tool-specific context via `PermissionRequest.Permission` and `Metadata` fields. Three actions: Allow, Always Allow, Reject.
- **Thought blocks**: Reasoning parts render as expandable `+/- Thought` blocks. `T` key toggles all, per-part toggle via `ToggleThoughtMsg`.
- **Spinner tick chain**: `SetWorking(true)` returns a `tea.Cmd` that must be propagated to maintain the spinner animation tick chain.
- **Startup guard**: OSC terminal escape responses leak as printable characters on startup. `prompt.go` has a guard that discards rune input for 2 seconds after first render, plus `isTerminalEscape()` regex filtering.
- **Leader key**: Vim-style `<leader>` key sequences for sidebar toggle, agent/model/session lists. State machine in `leader.go`.
- **Toast overlay**: Non-blocking notification rendered atop the main view. Used for errors and status messages.
- **Plugin system**: Plugins are standalone Go binaries in `cmd/plugin-*/` using `pkg/plugin/` SDK. Communication is JSON-RPC over stdin/stdout. The plugin manager (`internal/plugin/`) spawns processes, performs initialize handshake, and dispatches hooks (session lifecycle, permission, tool execution) and tool calls.

## Agent Delegation

Use specialized agents instead of doing everything inline:

- **`debugger`** — Finding and diagnosing bugs. Root-cause analysis, race conditions, stack traces.
- **`executor`** — Writing and editing code. Implementation work, refactors, applying fixes.
- **`architect`** — Designing solutions. Architecture decisions, API design, system-level trade-offs.

## Known Issues

See `docs/architecture-review.md` for the original audit. All five issues identified there (plugin wire protocol, path traversal, data races, SSE route inversion, dead TUI components) have been fixed.
