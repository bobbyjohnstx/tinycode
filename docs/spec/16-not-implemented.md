# 16. Not Implemented (TS Features)

Features present in the TypeScript tinycode that are not implemented in tinycode. This section summarizes the TS behavior for each, to serve as a reference for future porting.

---

## 16.1 Desktop Application

**TS behavior:** Electron-based desktop shell wrapping the web UI with platform-specific features.

- Content Security Policy headers on all windows
- Navigation origin validation, URL scheme validation (`http`, `https`, `mailto` only)
- Custom protocol (`oc://renderer`) with path traversal prevention
- Context isolation, no Node.js in renderer, sandbox enabled
- System tray with Show Window and Quit context menu
- Cross-platform application menus with Help links
- Window management: min size 960x600, persistent position/size, macOS hidden title bar, Windows frameless with custom overlay
- Persistent zoom level, pinch-zoom toggle (0.2x-10x)
- macOS dock icon restoration, OS theme sync
- Unresponsive detection with relaunch/export-logs/keep-waiting dialog
- Render process crash and load failure recovery
- Sidecar process: API server in utility process with system CA cert loading, proxy support, health check, migration progress reporting
- Auto-updater: GitHub Releases channel, background download, `quitAndInstall`
- Loading window: 640x480 splash during startup
- Settings migration from Tauri predecessor
- Desktop logging to disk
- `Document-Policy: include-js-call-stacks-in-crash-reports` header

**Go status:** Not in this repository, and not planned. The Electron source (`packages/desktop`) has been removed. The GUI is `tinycode web` (the SolidJS app embedded in the Go binary).

---

## 16.2 Web UI

**TS behavior:** React SPA connecting to the API server via REST + SSE.

- Prompt input with file references and slash commands
- Titlebar with session history and event timeline
- Model selection with favorites and capability tooltips
- Provider management and auth configuration
- Settings panels: General, Keybindings, Models, Providers
- MCP server management UI
- Session fork dialog
- File tree and terminal integration
- Context usage meter showing token consumption
- Update notification banner (non-blocking, i18n, ARIA)
- Debug bar for development

**Go status:** The Go binary can serve a pre-built web app from embedded static files (`internal/static/`), but the web UI itself is not developed in Go -- it is built from the `packages/app` TypeScript source and embedded via `make embed-webapp`.

---

## 16.3 Swarm Mode

**TS behavior:** Multi-agent coordination using tmux sessions via the `swarm` tool.

- Creates a tmux session with dashboard, supervisor, and worker panes
- Workers coordinate via shared filesystem: `board.md`, `inbox/`, `claims/`, status/heartbeat files
- Supervisor monitors worker health, detects stale workers (no heartbeat within `stale_seconds`, default 240s), sends nudge messages
- Parameters: `workers` (default 4, max 8), `stale_seconds` (240), `poll_seconds` (30)
- Custom worker commands, shared directory, switch tmux client

**Go status:** Implemented. `/swarm` dispatches parallel subagents as goroutines (not the legacy TypeScript tmux swarm).

---

## 16.4 Bundled Provider SDKs (TS-only)

**TS behavior:** ~25+ bundled AI SDK provider packages: Anthropic, OpenAI, Google (Gemini), Amazon Bedrock, Azure, Google Vertex, XAI, Mistral, Groq, DeepInfra, Cerebras, Cohere, Gateway, TogetherAI, Perplexity, Vercel, Alibaba, OpenRouter, GitLab, Venice, AI Gateway. Non-bundled providers installable dynamically from npm.

**Go status:** Go uses direct HTTP clients (OpenAI-compatible and Anthropic native). Any provider exposing an OpenAI-compatible `/v1/chat/completions` endpoint works automatically. No SDK bundling needed since the wire format is standardized.

---

## 16.5 Remote Model Catalog

**TS behavior:** Fetches curated model catalog from `<TINYCODE_MODELS_URL>/api.json` for accurate pricing, capabilities, context limits, and release dates.

- 10-second timeout, 2 retries with exponential backoff
- Cached to disk with 5-minute TTL, cross-process file locking
- Background refresh every 60 minutes
- Fallback chain: disk cache -> bundled snapshot -> local catalog file -> hardcoded fallback
- Environment variables: `TINYCODE_MODELS_URL`, `TINYCODE_MODELS_PATH`, `TINYCODE_DISABLE_MODELS_FETCH`

**Go status:** Not implemented. Go discovers models directly from provider APIs (Ollama `/api/tags`, vLLM `/v1/models`, OpenRouter `/api/v1/models`). Model capabilities are inferred from provider responses rather than a curated catalog.

---

## 16.6 Interactive Run Mode

**TS behavior:** `tinycode run --interactive` provides a split-footer direct mode with an in-process server.

- Requires TTY stdout
- Session history replay (`--replay`, `--replay-limit N`)
- Agent/model selection, file attachments, thinking block display
- `--interactive --attach <url>` connects to a running server with basic auth

**Go status:** Not implemented. Go run mode supports non-interactive (single-prompt and multi-turn via stdin) but not the interactive split-footer TUI mode.

---

## 16.7 Additional CLI Subcommands

**TS behavior:** Several CLI subcommands not present in Go:

| Command | Description | Go status |
|---------|-------------|-----------|
| `tinycode setup` | Interactive setup wizard | **Superseded.** Use `tinycode doctor` (diagnose) + `/connect` in the TUI (configure models). No separate setup wizard is planned. |
| `tinycode mcp` | MCP server management (list, auth, add, debug, logout) | **Implemented** as CLI (`list`/`add`/`auth`/`logout`/`debug`). Interactive OAuth still parked (see §16.28) |
| `tinycode db` | Database management (query, path, migrate) | Not implemented |
| `tinycode import` | Import session data | Not implemented |
| `tinycode uninstall` | Remove tinycode | Not implemented |
| `tinycode generate` | Generate OpenAPI spec JSON | Not implemented |
| `tinycode plugin-init` | Scaffold a new plugin project | Not implemented |
| `tinycode plugin-search` | Search plugin marketplace | Not implemented |

**Go status:** Go has `models`, `providers`, `session`, `status`, `export`, `plugin`, `agent`, `doctor`, `mcp`, `debug`. Remaining rows above are still missing (`setup` is intentionally replaced by doctor + `/connect`).

---

## 16.8 oh-my-tiny Plugin

**TS behavior:** Native plugin providing 23 tools across five categories:

- **State Management (5 tools):** `omt_state_*` -- predefined modes (autopilot, autoresearch, team, etc.), max 64 KB, project-scoped in `.tinycode/`
- **Notepad (6 tools):** `omt_notepad_*` -- priority/working/manual sections, timestamped entries, 7-day auto-prune, stored at `.tinycode/notepad.md`
- **Project Memory (4 tools):** `omt_project_memory_*` -- techStack, build, conventions, structure, notes, directives; stored at `.tinycode/project-memory.json`
- **Wiki (6 tools):** `omt_wiki_*` -- categories (architecture, decision, pattern, etc.), max 512 KB pages, keyword/tag/category search, stored in `.tinycode/wiki/`
- **AST Grep (2 tools):** `omt_ast_grep_*` -- AST-based code pattern matching using meta-variables, supports 15+ languages, 1000-file search limit

**Go status:** Not implemented. May be revisited once the Go core stabilizes.

---

## 16.9 Provider-Specific Temperature Defaults

**TS behavior:** Default temperatures per model family:

| Model Pattern | Default |
|---------------|---------|
| Qwen | 0.55 |
| Claude | undefined (model default) |
| Gemini, GLM-4.6/4.7, MiniMax-M2, Kimi-K2-thinking/K2.5 | 1.0 |
| Kimi-K2 (non-thinking) | 0.6 |

**Go status:** Not implemented. Go does not set provider-specific temperature defaults.

---

## 16.10 Retry Logic (Advanced)

**TS behavior:** Sophisticated retry system with ~30 regex patterns:

- Initial delay 2,000ms, 2x exponential backoff, 25% jitter
- Max 5 retries, max 30s delay (without retry-after headers), max-int with headers
- Honors `retry-after` and `retry-after-ms` response headers
- Context overflow detection via ~15 provider-specific regex patterns (not retried)
- 5xx override: always retried even if SDK doesn't mark retryable

**Go status:** Near parity. Go has 28 retryable error patterns, 5 retryable HTTP status codes, 18 overflow detection patterns, and the same backoff formula (2s initial, 2x, 30s max, 25% jitter, 5 retries). Missing: `retry-after` / `retry-after-ms` header parsing for server-directed delay.

---

## 16.11 PTY Routes

**TS behavior:** HTTP API for pseudo-terminal sessions:

- List available shells, create/get/update/delete PTY sessions
- WebSocket connection with ticket-based auth
- Used by the web UI for embedded terminal

**Go status:** Not implemented. Go does not expose PTY management via the HTTP API. These OpenAPI paths (`/pty`, `/pty/{ptyID}`, `/pty/{ptyID}/connect`, …) exist only in the TypeScript contract.

---

## 16.12 TUI Control Routes

**TS behavior:** HTTP routes for programmatic TUI control:

- `POST /tui/append-prompt`, `/submit-prompt`, `/clear-prompt`
- `POST /tui/execute-command`, `/show-toast`, `/publish`
- `POST /tui/select-session`, `/open-help`, `/open-sessions`, `/open-themes`, `/open-models`
- `GET /tui/control/next`, `POST /tui/control/response`

**Go status:** Not implemented. Documented in OpenAPI for the TS server only; not part of the Go HTTP surface. Session share/unshare routes (`/session/{id}/share`) are similarly OpenAPI-only.

**Go TUI API client:** `internal/tui/api` covers only the Bubbletea TUI subset (~27 methods). Prefer `@tinycode/sdk` / OpenAPI for the full historical client surface; see [02-api-routes.md](02-api-routes.md) §2.23 and [architecture.md](../architecture.md) (API Client).

---

## 16.13 IDE Extension Auto-Installation

**TS behavior:** Detects running IDEs (Windsurf, VS Code, Cursor, VSCodium) via `TERM_PROGRAM` and `GIT_ASKPASS` env vars and installs the VS Code extension automatically.

**Go status:** Not implemented.

---

## 16.14 Kubernetes In-Cluster Discovery

**TS behavior:** Auto-discovers vLLM services in Kubernetes clusters:

- Detects `KUBERNETES_SERVICE_HOST` env var
- Reads service account token and namespace
- Three priority tiers: `tinycode.dev/discover=vllm` annotation, KServe label, port probing (8080, 8000, 80)

**Go status:** Not implemented. Go supports explicit `TINYCODE_VLLM_HOST` but not Kubernetes service discovery.

---

## 16.15 V2 API Routes

**TS behavior:** Next-generation API schema:

- `GET /api/model` -- list all models (V2 schema)
- `GET /api/provider` -- list all providers (V2 schema)
- `GET /api/provider/:providerID` -- get specific provider

**Go status:** Not implemented.

---

## 16.16 Workspace and Worktree Management

**TS behavior:**

- Workspace table in database (id, type, name, branch, directory, extra, project_id, time_used)
- `GET /experimental/worktree` -- list git worktrees
- `POST /experimental/worktree` -- create worktree
- `DELETE /experimental/worktree` -- remove worktree
- `POST /experimental/worktree/reset` -- reset worktree
- Fence middleware: tracks state changes on mutating requests, returns `X-Tinycode-Sync` header

**Go status:** Not implemented at the API level. `internal/project/` has worktree path detection but no HTTP endpoints.

---

## 16.17 Advanced Export Features

**TS behavior:**

- HTML export: self-contained HTML file with `session-<id>-<timestamp>.html` naming
- `--sanitize` flag: redacts file contents, tool outputs, paths using `[redacted:<type>:<id>]` format
- Import: `tinycode import` to restore session data

**Go status:** Implemented. Go supports JSON and HTML export.

---

## 16.18 Background Job System

**TS behavior:** Typed job manager for async work:

- Job states: `running` -> `completed` | `error` | `cancelled`
- `start(input)`, `wait(id, timeout?)`, `cancel(id)`, `list()`, `get(id)`
- Deduplication: returns existing job if same ID already running
- Scoped to instance, cleaned up on disposal

**Go status:** Implemented in `internal/session/job.go`. `JobManager` provides `Start`, `Get`, `Wait`, `Cancel`, `List`, and `Shutdown`. States are `running`, `completed`, `failed`, and `cancelled`. Job IDs are allocated by the manager (`job_N`); there is no caller-supplied ID deduplication.

---

## 16.19 Frecency Ranking

**TS behavior:** Commands and files ranked by frequency + recency score, stored in namespaced store per project.

**Go status:** Implemented. Commands and files ranked by frecency score.

---

## 16.20 Additional TUI Features (TS-only)

| Feature | TS Behavior |
|---------|-------------|
| Session stash | Stash sessions for later (like git stash) |
| Session tags | Tag sessions for organization |
| Session slots (`<leader>1-9`) | Quick-switch to numbered session slots |
| Session pinning (`Ctrl+F`) | Pin sessions to prevent archiving |
| Session timeline (`<leader>g`) | Visual session timeline |
| Tips toggle (`<leader>i`) | Toggle inline tips |
| Which-key panel (`Ctrl+Alt+K`) | Keybinding discovery panel |
| External editor (`<leader>e`) | Open external editor for prompt editing |
| Copy last response (`<leader>y`) | Copy response to clipboard |
| Undo/redo (`<leader>u/r`) | Message undo/redo |
| Theme switcher (`<leader>t`) | Switch TUI themes |
| Status display (`<leader>s`) | Show session status overlay |
| Diff viewer (`<leader>d`) | Review file changes |
| Code block concealment (`<leader>;`) | Toggle code block visibility |
| Subagent inline rendering | Collapsible blocks below task header |
| Model variant cycling (`Ctrl+T`) | Cycle model variant (reasoning effort) |
| File attachments in prompt | `@filename` references with content injection |
| Session rename (`Ctrl+R`) | Rename session title |
| Session delete (`Ctrl+D`) | Delete session from TUI |

---

## 16.21 Question Tool

**TS behavior:** `question` tool for structured user interaction:

- Array of `{question, header?, custom?, options?}`
- Available when client is `app`, `cli`, or `desktop`, or `enableQuestionTool` flag is set
- Denied by default in run mode

**Go status:** Implemented. `question` tool available for structured user interaction.

---

## 16.22 Repository Tools

**TS behavior:**

- `repo_clone`: clone and cache repositories (GitHub shorthand, branch selection)
- `repo_overview`: analyze repository structure, detect ecosystems (Node.js, Python, Go, Rust, Ruby, Java, PHP), list entrypoints, 200-entry limit
- Both gated behind `experimentalScout` feature flag

**Go status:** Not implemented.

---

## 16.23 Apply Patch Tool

**TS behavior:** `apply_patch` tool for GPT models (model ID contains `gpt-`, excluding `gpt-4` and `oss` models):

- Mutually exclusive with `edit` and `write` (replaces them)
- Custom patch parser supporting add/update/delete/move operations
- Per-file formatting and LSP diagnostics

**Go status:** Implemented. `apply_patch` tool available for GPT models.

---

## 16.24 Web Search and Fetch (Provider Integration)

**TS behavior:**

- `websearch`: two search providers (Exa and Parallel), session-hash-based selection
- `webfetch`: 5 MB max, HTML-to-markdown, PDF text extraction, Cloudflare bot detection retry

**Go status:** Implemented. Both `webfetch` and `websearch` tools available.

---

## 16.25 Observation Masking

**TS behavior:** During compaction, if `compaction.mask_observations` is true (default), old tool outputs are replaced with `[output masked -- toolname on filepath]` placeholders, preserving the 5 most recent.

**Go status:** Implemented. `MaskObservations` defaults to true (`DefaultCompactionConfig`). `maskObservations` replaces older tool results with `[output masked for compaction]` and keeps the 5 most recent.

---

## 16.26 Account and OAuth System

**TS behavior:**

- `account` and `account_state` database tables
- OAuth flows: `POST /provider/:providerID/oauth/authorize`, `POST /provider/:providerID/oauth/callback`
- Auth routes: `PUT /auth/:providerID`, `DELETE /auth/:providerID`
- Account lifecycle events: `Account.Added`, `Account.Removed`, `Account.Switched`
- Console provider: `GET /experimental/console`, org switching

**Go status:** Not implemented. Go providers use environment-variable-based API keys only.

---

## 16.27 Formatter Integration

**TS behavior:** Configurable code formatter runs after file write/edit. Formatter status available via `GET /formatter`.

**Go status:** Not implemented.

---

## 16.28 MCP Auth HTTP Routes & CLI

**TS / OpenAPI behavior:**

- `GET/POST /mcp/{name}/auth`, `/mcp/{name}/auth/callback`, `/mcp/{name}/auth/authenticate`
- `POST /mcp/{name}/connect`, `POST /mcp/{name}/disconnect`
- CLI: `tinycode mcp` (list, auth, add, debug, logout) — also listed in §16.7

**Go status:** HTTP auth/connect/disconnect routes are **not** implemented (404). The `tinycode mcp` CLI **is** implemented (`list`, `add`, `auth`, `logout`, `debug`) and writes Bearer / `{env:VAR}` headers into config. Interactive MCP OAuth is **parked / unsupported** as a product surface.

Supported auth for MCP:

- `tinycode mcp auth` / `logout` (Bearer token or `{env:VAR}`)
- Static Bearer tokens in config `headers` (e.g. `"Authorization": "Bearer {env:TOKEN}"`)
- `{env:VAR}` substitution for secrets
- Optional `oauth.access_token` in config or tokens from `~/.local/share/tinycode/mcp-auth.json`

`internal/mcp/oauth.go` contains PKCE/`OAuthFlow` library helpers only — they are **not** exposed via `tinycode serve`, CLI, or TUI. Do not treat interactive browser OAuth as available.

Also implemented:

- Config-driven MCP connections
- Bearer injection on SSE / streamable-http transports via `createTransport`
- Status / reconnect: `GET /mcp`, `GET /mcp/status`, `POST /mcp/{name}/reconnect`
- CLI: `tinycode mcp list|add|auth|logout|debug`

---

## 16.29 OpenAPI `GET /skill` Shape vs Go

**TS / OpenAPI behavior:** Each skill object requires `name`, `location`, and `content` (full skill markdown inline).

**Go status:** `GET /skill` is implemented (`handleSkillList`) but returns `[]Skill` with `id`, `name`, `description`, `params`, `source`, and optional `dir`. It does **not** embed `content` or use `location`. Authoritative docs: [10-skills.md](10-skills.md) §10.9 and [02-api-routes.md](02-api-routes.md) §2.23.

Also: config `skills.urls` is parsed but remote skill fetch is not implemented; only `skills.paths` directory scanning is wired.

---

*Prev: [15-security.md](15-security.md) | [Back to overview](00-overview.md)*
