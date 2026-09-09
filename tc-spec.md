# tinycode Technical Specification

Version: 1.0
Date: 2026-09-04

---

## 1. Overview

tinycode is an AI coding assistant designed for local LLM inference as its primary use case, with support for cloud providers. It provides a multi-interface experience -- terminal UI, web UI, desktop application, headless API server, and IDE integration -- all backed by a single HTTP API server.

The system manages AI conversation sessions, coordinates tool execution, handles LLM provider discovery and connection, and provides an extensible plugin and skill framework. It is designed to work well with small local models (8B-14B parameters) while scaling gracefully to larger cloud models.

### 1.1 Target Audience

Developers who want an AI coding assistant that runs primarily against local LLM infrastructure (Ollama, vLLM, ramalama, LM Studio, or any OpenAI-compatible endpoint), with optional cloud provider support via OpenRouter and direct API providers.

### 1.2 Use Cases

- Interactive AI-assisted coding in a terminal, browser, or desktop app
- Headless API server for programmatic AI agent workflows
- IDE integration via the Agent Client Protocol (ACP) for editor extensions
- Multi-agent orchestration with subagent spawning and swarm mode
- Local-first AI coding with no cloud dependency required

### 1.3 Installation

The system is distributed as a standalone binary. Installation methods:
- Shell installer: `curl -fsSL <install-url> | sh`
- npx: `npx tinycode-ai`
- Default install directory: `$HOME/.local/bin` (overridable via `TINYCODE_INSTALL_DIR`)

---

## 2. Interfaces

### 2.1 Terminal UI (TUI)

A reactive terminal interface that functions as the primary interactive mode. The TUI either spawns the API server in a worker thread or connects to an existing running server.

**Key features:**
- Session management with parent-child hierarchy displayed as a toggleable ASCII tree sidebar
- Unified command palette searching across commands, agents, sessions, and skills, sorted by frecency (frequency + recency)
- Toast notifications for warnings and errors (e.g., tool-call failure warnings)
- Model picker, agent switcher, session forking, message revert/unrevert
- Subagent results rendered inline as collapsible blocks below the task header
- File references via `@filename` in the prompt input
- Slash commands (`/skill-name args`) for invoking skills
- Which-key panel for keybinding discovery
- External editor support for long-form prompt editing
- Diff viewer for reviewing file changes
- Code block concealment toggle
- Stash and tags for session organization

**Keybindings** (leader key is `Ctrl+X` by default):

| Binding | Action |
|---------|--------|
| `<leader>n` | New session |
| `<leader>o` | List sessions |
| `<leader>h` / `<leader>l` | Navigate sibling sessions |
| `<leader>j` / `<leader>k` | Navigate child/parent sessions |
| `<leader>b` | Toggle session tree sidebar |
| `<leader>1-9` | Quick-switch session slots |
| `Ctrl+F` | Pin session |
| `<leader>a` | List agents |
| `Tab` / `Shift+Tab` | Cycle through agents |
| `<leader>m` | Switch model |
| `F2` / `Shift+F2` | Cycle recent models |
| `<leader>c` | Compact context |
| `<leader>x` | Export session |
| `Ctrl+R` | Rename session |
| `Ctrl+D` | Delete session |
| `<leader>e` | Open external editor |
| `<leader>y` | Copy last response |
| `<leader>u` / `<leader>r` | Undo / Redo |
| `<leader>t` | Switch theme |
| `<leader>s` | Show status |
| `<leader>d` | Diff viewer |
| `<leader>;` | Toggle code block concealment |
| `Ctrl+Alt+K` | Which-key panel |
| `Ctrl+P` | Unified command palette |
| `F1` | Help |
| `<leader>g` | Session timeline |
| `<leader>i` | Toggle tips |
| `Ctrl+T` | Cycle model variant |
| `<leader>q` | Exit (alternative) |
| `PageUp` / `PageDown` | Page scroll |
| `Ctrl+Alt+B` / `Ctrl+Alt+F` | Page scroll (alternative) |
| `Ctrl+Alt+Y` / `Ctrl+Alt+E` | Line scroll |
| `Ctrl+Alt+U` / `Ctrl+Alt+D` | Half-page scroll |

**Frecency ranking:** Commands and files are ranked by a score combining usage frequency and recency. Scores are stored in a namespaced store, persisted per project.

### 2.2 Web UI

A reactive web application that connects to the tinycode API server via REST and SSE for real-time event streaming. Used by both the browser experience (launched via `tinycode web`) and the desktop application.

**Features:**
- Prompt input with file references and slash commands
- Titlebar with session history and event timeline
- Model selection with favorites and capability tooltips
- Provider management and auth configuration
- Settings panels: General, Keybindings, Models, Providers
- MCP server management
- Session fork dialog
- File tree and terminal integration
- Context usage meter showing token consumption
- Update notification banner (non-blocking)
- Debug bar for development

### 2.3 Desktop Application

A desktop shell wrapping the web UI with platform-specific features:

**Security:**
- Content Security Policy headers on all windows
- Navigation origin validation (prevents navigation away from the app)
- URL scheme validation: only `http`, `https`, and `mailto` for external links
- Controlled window opening (new window requests denied; valid URLs opened externally)
- Custom protocol (`oc://renderer`) with path traversal prevention
- Context isolation, no Node.js integration in renderer, sandbox enabled
- Permission handler: only clipboard-sanitized-write and notifications from trusted renderer

**System tray:** Cross-platform tray integration with Show Window and Quit context menu actions.

**Application menus:** Cross-platform menus for Windows/Linux/macOS with Help menu linking to GitHub (repo, discussions, issues).

**Window management:**
- Minimum size: 960x600
- Persistent window state (position, size) across restarts
- macOS: hidden title bar with traffic light controls
- Windows: frameless with custom title bar overlay
- Persistent zoom level, pinch-zoom toggle, zoom range 0.2x--10x

**Platform lifecycle:**
- macOS dock icon restoration
- OS theme sync (dark/light mode)
- Global exception handling (uncaught exceptions and unhandled rejections)
- Unresponsive detection with relaunch/export-logs/keep-waiting dialog
- Render process crash and load failure recovery

**Sidecar process:** The desktop app runs the API server in a utility process (worker thread) with:
- System CA certificate loading (merges default + system certificates for corporate proxy environments)
- Proxy support (ensures loopback addresses are excluded from proxy via `NO_PROXY`)
- Start/stop lifecycle via message passing to the parent process
- Health check on startup to verify sidecar is ready
- Database migration progress reporting to the parent process

**Auto-updater:**
- Checks GitHub Releases for new versions
- Channel: `latest` (no prereleases by default)
- Downloads updates in the background, then notifies the user via the web UI
- `quitAndInstall` restarts the app with the new version after killing the sidecar
- Coalesces concurrent update checks via a pending promise

**Loading window:** 640x480 non-resizable splash screen shown during startup while the sidecar initializes.

**Settings migration:** Migrates settings from previous application versions (e.g., Tauri-based predecessor) on first launch.

**Desktop logging:** Structured logging to disk for diagnostic purposes.

**Document-Policy header:** `include-js-call-stacks-in-crash-reports` for crash diagnostics.

**Update notifications:** Non-blocking slide-in banner checking GitHub Releases for available updates, with internationalization and ARIA accessibility.

### 2.4 CLI Commands

| Command | Description |
|---------|-------------|
| `tinycode` | Launch TUI (default mode) |
| `tinycode <directory>` | Launch TUI against specified directory |
| `tinycode serve` | Start headless API server |
| `tinycode web` | Start server and open web UI |
| `tinycode acp` | Agent Client Protocol mode (stdio transport) |
| `tinycode export [sessionID]` | Export session to JSON or self-contained HTML |
| `tinycode models` | List available models |
| `tinycode run [message]` | Run with a message (non-interactive default, or `--interactive` for split-footer mode) |
| `tinycode agent` | Agent management (create, list) |
| `tinycode providers` | Provider management (login, logout, list) |
| `tinycode session` | Session management (list, delete) |
| `tinycode setup` | Interactive setup wizard |
| `tinycode status` | Show server/instance status |
| `tinycode mcp` | MCP server management (list, auth, add, debug, logout) |
| `tinycode plugin <name>` | Install plugin (from npm or file://) |
| `tinycode plugin-init [name]` | Scaffold a new plugin project |
| `tinycode plugin-search [query]` | Search the plugin marketplace |
| `tinycode debug` | Debugging and troubleshooting tools |
| `tinycode generate` | Generate OpenAPI spec JSON with code samples |
| `tinycode uninstall` | Remove tinycode |
| `tinycode db` | Database management (query, path, migrate) |
| `tinycode import` | Import session data |

**Export formats:**
- **JSON:** Full session data (info + messages with parts) written to stdout
- **HTML:** Self-contained HTML file written to `session-<id>-<timestamp>.html`
- Both formats support a `--sanitize` flag that redacts sensitive data (file contents, tool outputs, paths) while preserving structure. Redacted values use `[redacted:<type>:<id>]` format.

### 2.5 `tinycode run` — Programmatic and Interactive Execution

A separate interface mode for scripting, CI/CD pipelines, and lightweight interactive sessions. Three modes:

**Non-interactive (default):** Sends a single prompt, streams events to stdout, and exits when the session goes idle. In non-interactive mode, `question` tool calls, plan entry, and plan exit are auto-denied. Permission requests are auto-rejected (or auto-approved with `--dangerously-skip-permissions`).

**Interactive direct (`--interactive`):** Boots a split-footer direct mode with an in-process server (no external HTTP). Requires a TTY stdout. Supports session history replay (`--replay`, `--replay-limit N`), agent/model selection, file attachments, and thinking block display.

**Interactive attach (`--interactive --attach <url>`):** Connects to a running tinycode server and runs interactive mode against it. Useful for remote server interaction with basic auth (`--username`, `--password`).

**Options:**

| Flag | Description |
|------|-------------|
| `--command` | Execute a slash command instead of a prompt |
| `--continue` / `-c` | Continue the last session |
| `--session` / `-s` | Continue a specific session by ID |
| `--fork` | Fork the session before continuing (requires `--continue` or `--session`) |
| `--share` | Share the session |
| `--model` / `-m` | Model in `provider/model` format |
| `--agent` | Agent to use |
| `--format` | Output format: `default` (formatted) or `json` (raw JSON events per line) |
| `--file` / `-f` | File(s) to attach to the message (repeatable) |
| `--title` | Session title (uses truncated prompt if empty string) |
| `--variant` | Model variant (provider-specific reasoning effort, e.g., `high`, `max`) |
| `--thinking` | Show thinking/reasoning blocks |
| `--replay` | Replay visible session history on interactive resume |
| `--replay-limit N` | Cap visible interactive replay to newest N messages |
| `--interactive` / `-i` | Run in direct interactive split-footer mode |
| `--attach <url>` | Attach to a running tinycode server |
| `--dir` | Directory to run in (local path or remote path when attaching) |
| `--port` | Port for the local server (defaults to random port) |
| `--demo` | Enable demo slash commands (requires `--interactive`) |
| `--dangerously-skip-permissions` | Auto-approve all permission requests |

**JSON output mode (`--format json`):** Each event is a single JSON line with `{type, timestamp, sessionID, ...data}`. Event types include `tool_use`, `text`, `reasoning`, `step_start`, `step_finish`, `error`.

**Piped input:** When stdin is not a TTY, stdin content is read and appended to the message argument.

**Subcommand details for other CLI commands:**

**`tinycode agent`:**
- `agent create` — Generate an agent definition from a natural language description via LLM. Options: `--path`, `--description`, `--mode` (all/primary/subagent), `--permissions` (comma-separated), `--model`.
- `agent list` — List all available agents with mode, permissions, and compact variant info.

**`tinycode providers`:**
- `providers list` — List all discovered providers
- `providers login [url]` — Authenticate with a provider
- `providers logout` — Remove provider credentials

**`tinycode mcp`:**
- `mcp list` — List configured MCP servers
- `mcp auth [name]` — Authenticate with an MCP server
- `mcp add` — Add a new MCP server
- `mcp debug <name>` — Debug an MCP server connection
- `mcp logout [name]` — Remove MCP server credentials

**`tinycode session`:**
- `session list` — List all sessions
- `session delete <sessionID>` — Delete a session

**`tinycode db`:**
- `db [query]` — Run a SQL query against the database (default)
- `db path` — Print the database file path
- `db migrate` — Run database migrations

**`tinycode debug`:**

Debugging and troubleshooting subcommands:
- `debug config` — Show resolved configuration
- `debug file` — File system debugging
- `debug lsp` — LSP server diagnostics
- `debug ripgrep` — Ripgrep binary diagnostics
- `debug scrap` — Scratch debugging utility
- `debug skill` — Skill resolution debugging
- `debug snapshot` — Filesystem snapshot debugging
- `debug agent` — Agent prompt/tool inspection
- `debug startup` — Startup timing diagnostics
- `debug v2` — V2 event system debugging
- `debug info` — Show debug information
- `debug paths` — Show important file paths
- `debug wait` — Wait indefinitely (for debugging)

### 2.6 IDE Extension Auto-Installation

tinycode can detect running IDE instances and install its VS Code extension directly:
- **Supported IDEs:** Windsurf, Visual Studio Code (Insiders), Visual Studio Code, Cursor, VSCodium
- **Detection:** Uses `TERM_PROGRAM` and `GIT_ASKPASS` environment variables to identify the active IDE
- **Installation:** Runs `<ide-cmd> --install-extension sst-dev.tinycode`

### 2.7 Agent Client Protocol (ACP)

IDE integration via stdio-based agent communication using newline-delimited JSON. Protocol version: `0.1.0`.

**Supported operations:**

| Operation | Description |
|-----------|-------------|
| `initialize` | Handshake, declare capabilities (`supportsPermissions: true`) |
| `authenticate` | Provider authentication |
| `newSession` | Create new session |
| `loadSession` | Load existing session |
| `listSessions` | List all sessions |
| `resumeSession` | Resume a session |
| `closeSession` | Close a session |
| `unstable_forkSession` | Fork session at a point |
| `prompt` | Send message with optional embedded context and images |
| `cancel` | Abort current processing |
| `unstable_setSessionModel` | Change session model |
| `setSessionMode` | Change session mode/agent |
| `setSessionConfigOption` | Update session configuration |

**Session state:** Each ACP session maintains in-memory state: id, cwd, MCP servers, creation time, model, variant, mode ID, and known parts.

**Content bridging:** ACP content types are translated to/from internal message parts, enabling IDE-native content (file selections, diagnostics) to flow into the AI conversation.

### 2.6 VS Code Extension

Reference VS Code extension demonstrating ACP integration:

- **Commands:** `tinycode.start`, `tinycode.stop`
- **Transport:** Spawns `tinycode acp --cwd <workspace>` as child process, communicates via stdio NDJSON
- **Chat provider:** Registers a VS Code chat participant for inline AI interaction
- **Config:** `tinycode.path` setting for custom binary path
- **Auto-start:** Starts automatically when a workspace folder is open

### 2.7 HTTP API Server

REST + SSE server running on port 4096 by default (configurable; falls back to any free port if unavailable).

**Server defaults:**

| Setting | Default |
|---------|---------|
| Port | 4096 |
| Max instances | 32 |
| Max sessions | Unlimited |
| mDNS domain | tinycode.local |

**Authentication:** Password-based when binding to non-loopback addresses (via `TINYCODE_SERVER_PASSWORD`). Loopback addresses (127.0.0.1, localhost, ::1) run unsecured by default.

**CORS:** Configurable cross-origin support.

**mDNS:** Optional multicast DNS publishing for network service discovery (disabled on loopback addresses).

**Server middleware:**

| Middleware | Behavior |
|------------|----------|
| HTTP compression | gzip/deflate encoding for responses over 1,024 bytes. Skips SSE streams (`/event`, `/global/event`), POST streaming paths (`/session/*/message`, `/session/*/prompt_async`), HEAD requests, and responses with `no-transform` cache-control. |
| Security headers | `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: strict-origin-when-cross-origin`, `Permissions-Policy: camera=(), microphone=(), geolocation=()` on all responses. |
| Fence middleware | For workspace-aware deployments: tracks state changes on mutating requests (non-GET/HEAD/OPTIONS) and returns a `X-Tinycode-Sync` header with a JSON diff of changed workspace state. Active only when `TINYCODE_WORKSPACE_ID` is set. |

---

## 3. HTTP API Routes

### 3.1 Global Routes

| Method | Path | Description |
|--------|------|-------------|
| GET | `/global/health` | Health check (returns version + healthy flag) |
| GET | `/global/event` | Global SSE event stream (carries directory/project/workspace context) |
| GET | `/global/config` | Get global configuration |
| PATCH | `/global/config` | Update global configuration |
| POST | `/global/dispose` | Dispose all instances |
| POST | `/global/upgrade` | Upgrade tinycode to specified or latest version |

### 3.2 Auth Routes

| Method | Path | Description |
|--------|------|-------------|
| PUT | `/auth/:providerID` | Set auth credentials for a provider |
| DELETE | `/auth/:providerID` | Remove auth credentials |

### 3.3 Session Routes

| Method | Path | Description |
|--------|------|-------------|
| GET | `/session` | List sessions (filterable by scope, path, roots, search, with pagination) |
| GET | `/session/status` | Status map of all sessions |
| GET | `/session/:sessionID` | Get session details |
| GET | `/session/:sessionID/children` | List child sessions |
| GET | `/session/:sessionID/todo` | Get session todo list |
| GET | `/session/:sessionID/diff` | Get file changes diff for a message |
| GET | `/session/:sessionID/message` | List messages (paginated: 50 per page, cursor-based via `before` parameter) |
| GET | `/session/:sessionID/message/:messageID` | Get specific message |
| POST | `/session` | Create session |
| DELETE | `/session/:sessionID` | Delete session (cascades to children) |
| PATCH | `/session/:sessionID` | Update session (title, permissions, archive time) |
| POST | `/session/:sessionID/fork` | Fork session at a specific message |
| POST | `/session/:sessionID/abort` | Abort active processing |
| POST | `/session/:sessionID/share` | Create shareable link |
| DELETE | `/session/:sessionID/share` | Remove shareable link |
| POST | `/session/:sessionID/init` | Initialize project AGENTS.md |
| POST | `/session/:sessionID/summarize` | Trigger manual context compaction |
| POST | `/session/:sessionID/message` | Send prompt (synchronous, streams response) |
| POST | `/session/:sessionID/prompt_async` | Send prompt (async, returns immediately) |
| POST | `/session/:sessionID/command` | Send slash command |
| POST | `/session/:sessionID/shell` | Execute shell command in session context |
| POST | `/session/:sessionID/revert` | Revert message file changes |
| POST | `/session/:sessionID/unrevert` | Restore reverted messages |
| POST | `/session/:sessionID/permissions/:permissionID` | Respond to permission request |
| DELETE | `/session/:sessionID/message/:messageID` | Delete message |
| DELETE | `/session/:sessionID/message/:messageID/part/:partID` | Delete message part |
| PATCH | `/session/:sessionID/message/:messageID/part/:partID` | Update message part |

### 3.4 Provider Routes

| Method | Path | Description |
|--------|------|-------------|
| GET | `/provider` | List all providers with models and connection status |
| GET | `/provider/auth` | Get available authentication methods per provider |
| POST | `/provider/:providerID/oauth/authorize` | Start OAuth flow |
| POST | `/provider/:providerID/oauth/callback` | Handle OAuth callback |

### 3.5 Event Routes

| Method | Path | Description |
|--------|------|-------------|
| GET | `/event` | Subscribe to SSE event stream (instance-scoped) |

### 3.6 Config Routes

| Method | Path | Description |
|--------|------|-------------|
| GET | `/config` | Get configuration |
| PATCH | `/config` | Update configuration |
| GET | `/config/providers` | List configured providers |

### 3.7 File Routes

| Method | Path | Description |
|--------|------|-------------|
| GET | `/find` | Text search (via ripgrep) |
| GET | `/find/file` | File search by name/pattern |
| GET | `/find/symbol` | Symbol search (via LSP) |
| GET | `/file` | List files at a path |
| GET | `/file/content` | Read file contents |
| GET | `/file/status` | Git file status |

### 3.8 MCP Routes

| Method | Path | Description |
|--------|------|-------------|
| GET | `/mcp` | Status of all MCP servers |
| POST | `/mcp` | Add MCP server dynamically |
| POST | `/mcp/:name/auth` | Start MCP OAuth flow |
| POST | `/mcp/:name/auth/callback` | Complete MCP OAuth |
| POST | `/mcp/:name/auth/authenticate` | Start + wait for MCP OAuth (opens browser) |
| DELETE | `/mcp/:name/auth` | Remove MCP OAuth credentials |
| POST | `/mcp/:name/connect` | Connect to MCP server |
| POST | `/mcp/:name/disconnect` | Disconnect MCP server |

### 3.9 Permission Routes

| Method | Path | Description |
|--------|------|-------------|
| GET | `/permission` | List pending permission requests |
| POST | `/permission/:requestID/reply` | Reply (approve/deny) to permission request |

### 3.10 Question Routes

| Method | Path | Description |
|--------|------|-------------|
| GET | `/question` | List pending questions |
| POST | `/question/:requestID/reply` | Answer a question |
| POST | `/question/:requestID/reject` | Reject a question |

### 3.11 Instance Routes

| Method | Path | Description |
|--------|------|-------------|
| POST | `/instance/dispose` | Dispose instance |
| GET | `/path` | Get paths (home, state, config, worktree, directory) |
| GET | `/vcs` | Get VCS info (branch, etc.) |
| GET | `/vcs/status` | Get changed files |
| GET | `/vcs/diff` | Get VCS diff (structured) |
| GET | `/vcs/diff/raw` | Get raw patch |
| POST | `/vcs/apply` | Apply a raw patch |
| GET | `/command` | List all commands |
| GET | `/agent` | List all agents |
| GET | `/skill` | List all skills |
| GET | `/lsp` | LSP server status |
| GET | `/formatter` | Formatter status |

### 3.12 PTY Routes

| Method | Path | Description |
|--------|------|-------------|
| GET | `/pty/shells` | List available shells |
| GET | `/pty` | List PTY sessions |
| POST | `/pty` | Create PTY session |
| GET | `/pty/:ptyID` | Get PTY session |
| PUT | `/pty/:ptyID` | Update PTY session |
| DELETE | `/pty/:ptyID` | Remove PTY session |
| POST | `/pty/:ptyID/connect-token` | Create WebSocket auth token |
| GET | `/pty/:ptyID/connect` | WebSocket connect (HTTP upgrade) |

### 3.13 TUI Control Routes

| Method | Path | Description |
|--------|------|-------------|
| POST | `/tui/append-prompt` | Append text to TUI prompt |
| POST | `/tui/submit-prompt` | Submit TUI prompt |
| POST | `/tui/clear-prompt` | Clear TUI prompt |
| POST | `/tui/execute-command` | Execute TUI command |
| POST | `/tui/show-toast` | Show toast notification |
| POST | `/tui/publish` | Publish a TUI event |
| POST | `/tui/select-session` | Navigate to a session |
| POST | `/tui/open-help` | Open help dialog |
| POST | `/tui/open-sessions` | Open sessions dialog |
| POST | `/tui/open-themes` | Open themes dialog |
| POST | `/tui/open-models` | Open models dialog |
| GET | `/tui/control/next` | Get next TUI request |
| POST | `/tui/control/response` | Submit TUI response |

### 3.14 Experimental Routes

| Method | Path | Description |
|--------|------|-------------|
| GET | `/experimental/tool` | List tools with JSON schemas |
| GET | `/experimental/tool/ids` | List all tool IDs |
| GET | `/experimental/worktree` | List git worktrees |
| POST | `/experimental/worktree` | Create worktree |
| DELETE | `/experimental/worktree` | Remove worktree |
| POST | `/experimental/worktree/reset` | Reset worktree |
| GET | `/experimental/session` | List all sessions cross-project |
| GET | `/experimental/resource` | List MCP resources |
| GET | `/experimental/console` | Active Console provider metadata |
| GET | `/experimental/console/orgs` | List switchable Console orgs |
| POST | `/experimental/console/switch` | Switch active Console org |

### 3.15 Project Routes

| Method | Path | Description |
|--------|------|-------------|
| GET | `/project` | List all projects |
| GET | `/project/current` | Get current project |
| POST | `/project/git/init` | Initialize git repository for current project |
| PATCH | `/project/:projectID` | Update project (name, icon, commands) |

### 3.16 V2 API Routes

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/model` | List all models (V2 schema) |
| GET | `/api/provider` | List all providers (V2 schema) |
| GET | `/api/provider/:providerID` | Get a specific provider (V2 schema) |

### 3.17 Logging

| Method | Path | Description |
|--------|------|-------------|
| POST | `/log` | Write a log entry (service, level, message, extra) |

---

## 4. Event System

### 4.1 Architecture

Internal pub/sub event bus using a sliding-window buffer (capacity: 4096 events per channel). Events are published to both typed channels and a wildcard channel.

**Event flow:**
1. Components publish events via the bus service
2. Events propagate to both typed and wildcard subscribers
3. Global bus forwards events across instances (for multi-instance scenarios)
4. SSE endpoint streams wildcard events to connected clients

**Subscription modes:**
- **Typed:** Receive events of a specific type only
- **Wildcard:** Receive all events
- **Eager:** Subscription is acquired immediately at creation time (not lazily on first pull), preventing race conditions where events published between subscription and consumption are lost

### 4.2 SSE Protocol

- **Content-Type:** `text/event-stream`
- **Headers:** `Cache-Control: no-cache, no-transform`, `X-Accel-Buffering: no`, `X-Content-Type-Options: nosniff`
- **First event:** `server.connected`
- **Heartbeat:** Every 10 seconds, type `server.heartbeat`
- **Event format:** JSON-encoded payload with SSE `event: message` type
- **Termination:** Stream ends on `Bus.InstanceDisposed` event
- **Global SSE** (`/global/event`) carries directory + optional project/workspace context per event
- **Part delta batching:** Text and reasoning deltas are batched with a 16ms debounce for SSE efficiency

### 4.3 WebSocket Protocol

PTY connections use WebSocket at `/pty/:ptyID/connect` with ticket-based authentication (token obtained via `/pty/:ptyID/connect-token`).

### 4.4 Built-in Event Types

**Server lifecycle:**
- `server.connected` -- Connection established
- `server.heartbeat` -- Keepalive
- `server.instance.disposed` -- Instance shutdown
- `global.disposed` -- Global shutdown (triggers SSE client cleanup)

**Session events:**
- `AgentSwitched`, `ModelSwitched`, `Prompted`, `Synthetic`
- `Processor.Started`, `Processor.Ended`
- `LLM.Started`, `LLM.Ended`

**Streaming events:**
- `Text.Started` / `Text.Delta` / `Text.Ended`
- `Reasoning.Started` / `Reasoning.Delta` / `Reasoning.Ended`
- `ToolCall.Result.Started` / `ToolCall.Result.Delta` / `ToolCall.Result.Ended`

**Tool events:**
- `Tool.Called`, `Tool.Progress`, `Tool.Success`, `Tool.Failed`

**Infrastructure events:**
- `Retried` -- LLM retry occurred
- `file.edited`, `file.watcher.updated`
- `pty.created`, `pty.updated`, `pty.exited`, `pty.deleted`, `pty.open`
- `worktree.ready`, `ide.installed`
- `mcp.tools.changed`, `mcp.browser.open.failed`
- `Account.Added`, `Account.Removed`, `Account.Switched`
- `Catalog.ModelUpdated`, `Catalog.Refreshed`

---

## 5. Session Management

### 5.1 Session Lifecycle

A session represents a conversation with an AI model.

**Create:** Generates a unique session ID (descending order for newest-first sorting), a URL-friendly slug, timestamps, and ties to a project. Optional: parent ID, title, agent, model, permission ruleset, workspace ID.

**Fork:** Creates a new session branching from an existing one at a specific message. Copies all messages up to the fork point with ID mapping for reference integrity. Title gets a "(fork #N)" suffix.

**Archive:** Sessions can be archived by setting an archive timestamp. Archived sessions remain in the database but are filtered from default listings.

**Delete:** Recursively removes child sessions, cancels background jobs, removes from storage.

**Touch:** Updates the `time_updated` timestamp on any session activity.

### 5.2 Session Data Model

| Field | Type | Description |
|-------|------|-------------|
| id | string | Unique session identifier |
| project_id | string | Parent project reference |
| workspace_id | string | Workspace scope (optional) |
| parent_id | string | Parent session for subagents (optional) |
| slug | string | URL-friendly identifier |
| directory | string | Working directory |
| path | string | File path context (optional) |
| title | string | Session title |
| version | string | Protocol version |
| share_url | string | Shareable link (optional) |
| cost | number | Accumulated LLM cost (default 0) |
| tokens_input | integer | Total input tokens (default 0) |
| tokens_output | integer | Total output tokens (default 0) |
| tokens_reasoning | integer | Total reasoning tokens (default 0) |
| tokens_cache_read | integer | Cache read tokens (default 0) |
| tokens_cache_write | integer | Cache write tokens (default 0) |
| agent | string | Active agent (optional) |
| model | object | Active model: `{id, providerID, variant?}` (optional) |
| permission | object | Session-scoped permission ruleset (optional) |
| revert | object | Revert state: `{messageID, partID?, snapshot?, diff?}` (optional) |
| summary_additions | integer | Lines added (optional) |
| summary_deletions | integer | Lines deleted (optional) |
| summary_files | integer | Files modified (optional) |
| summary_diffs | array | Detailed file diffs (optional) |
| time_created | integer | Creation timestamp (ms) |
| time_updated | integer | Last update timestamp (ms) |
| time_compacting | integer | Compaction start timestamp (optional) |
| time_archived | integer | Archive timestamp (optional) |

### 5.3 Message Structure

Messages belong to sessions. Each message has an ID, session reference, timestamps, and a JSON data payload containing the message info (role, content, metadata).

**Message parts** compose the message content. Part types:

| Part Type | Description |
|-----------|-------------|
| text | Text content with optional timing and provider metadata |
| reasoning | Model reasoning/thinking content |
| tool | Tool call with status lifecycle (pending -> running -> completed/error) |
| file | File attachment (image, document) |
| subtask | Subagent task reference |
| patch | File system change record (hash + affected file list) |
| step-start | LLM inference step boundary (start) |
| step-finish | LLM inference step boundary (end), with snapshots and token usage |
| snapshot | Filesystem snapshot reference |
| agent | Agent change notification |

### 5.4 Todo Items

Per-session todo list. Each item has: content, status, priority, and position. Composite primary key: (session_id, position).

### 5.5 Session Processing Loop

The session processor manages the core conversation loop:

1. **Pre-capture snapshot** before the LLM stream starts (captures filesystem state for revert support)
2. **Stream LLM response** processing events as they arrive:
   - `reasoning-start/delta/end` -- Accumulate reasoning text into reasoning parts
   - `text-start/delta/end` -- Accumulate response text into text parts, with plugin text-complete hook
   - `tool-input-start/delta/end` -- Track tool call input streaming
   - `tool-call` -- Execute tool, with doom-loop detection
   - `tool-result` -- Complete tool call, track consecutive failures
   - `tool-error` -- Record tool call failure
   - `step-start/finish` -- Track inference steps, record token usage, check for context overflow
   - `provider-error` -- Throw (non-recoverable)
3. **Cleanup** -- Settle pending tool calls (250ms timeout), finalize incomplete parts
4. **Return result:** `compact` (needs compaction), `stop` (done), or `continue` (more steps needed)

**Doom-loop detection:** If the last N tool calls (configurable via `experimental.doom_loop_threshold`, default 3) are identical (same tool name and same input), a permission check is triggered asking the user to confirm continuation.

**Tool-call failure tracking:** After 3+ consecutive tool calls to the `invalid` tool (malformed tool calls the model generated), a warning toast suggests switching to a larger model.

**Overflow detection:** After each step finishes, checks if total token count >= (model input limit - reserved buffer). Buffer: minimum of 20,000 tokens and the model's max output tokens.

### 5.6 Background Job System

A typed job manager handles asynchronous work (primarily background subagents):

**Job lifecycle states:** `running` → `completed` | `error` | `cancelled`

**Operations:**
- `start(input)` — Create and fork a new background job. If a job with the same ID is already running, returns the existing job. Each job has: id, type, title, status, timestamps, optional metadata, output, and error.
- `wait(id, timeout?)` — Block until a job completes or times out. Returns `{info, timedOut}`.
- `cancel(id)` — Interrupt a running job's fiber, transition to `cancelled`.
- `list()` / `get(id)` — Query job state.

Jobs are scoped to the instance and cleaned up on disposal.

### 5.7 Subagent Behavior

- Controlled by `subagent_depth` config (default: 1)
- Root sessions can spawn subagents; subagents cannot spawn further subagents unless depth > 1
- Child sessions inherit a restricted permission set (see Section 14.5)
- Subagent results rendered inline as collapsible blocks in the TUI

---

## 6. Context Compaction

Automatic context summarization when the conversation approaches the model's context limit.

### 6.1 Algorithm

1. **Trigger:** Overflow detected (tokens used >= usable context), unless `compaction.auto` is false
2. **Lazy tail estimation:** Estimates token cost per turn, most-recent-first, stopping as soon as the preserve budget is exceeded. Avoids wasting estimation calls on turns that will not fit.
3. **Determine preserve window:** Recent turns kept verbatim. Budget = config value or `min(15000, max(2000, usable * 0.25))`.
4. **Select head vs. tail:** Find all user turns (excluding compaction markers). Working backwards, accumulate turns into the tail until budget is exceeded. If a turn exceeds remaining budget, attempt to split it (keeping some assistant messages). Result: `head` (messages to summarize) and `tail_start_id` (where preserved messages begin).
5. **Observation masking:** If `compaction.mask_observations` is true (default), replace old tool outputs with `[output masked -- toolname on filepath]` placeholders, preserving the 5 most recent tool outputs.
6. **Text serialization:** Convert the conversation to tagged text format (`[User]`, `[Assistant]`, `[Tool: name]`, `[Tool Result]`, `[Thinking]`). Text content is truncated to 2,000 characters, tool input JSON to 500 characters, and tool output to 2,000 characters per entry. An empty system prompt is used (no "summarization assistant" framing); continuation prevention comes from the structured summary template itself rather than a system prompt directive.
7. **Build prompt:** Wrap conversation in `<conversation>` tags with `<prior-summary>` for previous compaction summaries.
8. **Deterministic file tracking:** After summarization, scan tool calls for read/write/edit operations and append `<read-files>` and `<modified-files>` XML blocks to the summary. Merge with any prior summary's file operations. This is deterministic (not LLM-dependent).
9. **Summary structure template:**
   - Goal
   - Constraints & Preferences
   - Progress (Done / In Progress / Blocked)
   - Key Decisions
   - Next Steps
   - Critical Context
   - Relevant Files (from deterministic file tracking)

**Model selection for compaction:** compaction agent model override -> `small_model` config -> session model.

### 6.2 Auto-Continue After Compaction

- If overflow occurred and there was a prior user message, replay it as a new message (without media attachments)
- Otherwise, inject a synthetic "Continue if you have next steps" message
- After 3+ compactions (circuit breaker), inject a warning suggesting starting a new session or using a subagent approach

### 6.3 Pruning

Separate from compaction. Only runs if `compaction.prune` is true in config.

- Works backwards through tool call outputs
- Protects the most recent 40,000 tokens of tool outputs
- Marks older tool outputs as compacted (sets `time.compacted`) if prunable total exceeds 20,000 tokens
- Skill tool outputs are protected from pruning (`PRUNE_PROTECTED_TOOLS = ["skill"]`)
- Note: The 2,000-character limit (`TOOL_OUTPUT_MAX_CHARS`) applies during context serialization for summarization, not during pruning

### 6.4 Thresholds and Configuration

| Parameter | Default | Config Key |
|-----------|---------|------------|
| Auto compaction | true | `compaction.auto` |
| Prune old outputs | false | `compaction.prune` |
| Prune minimum (tokens) | 20,000 | -- |
| Prune protect (tokens) | 40,000 | -- |
| Tool output max chars (serialization) | 2,000 | -- |
| Min preserve recent tokens | 2,000 | -- |
| Max preserve recent tokens | 15,000 | `compaction.preserve_recent_tokens` |
| Tail turns to keep verbatim | unlimited (all user turns eligible) | `compaction.tail_turns` |
| Reserved buffer (tokens) | configurable | `compaction.reserved` |
| Mask observations | true | `compaction.mask_observations` |

### 6.5 Telemetry

Each compaction logs structured data: pre/post token counts, model used, timing, and compaction number within the session.

---

## 7. LLM Providers

### 7.1 Provider Abstraction

Each provider has:
- **ID** -- Unique identifier (e.g., `ollama`, `openrouter`, `anthropic`)
- **Name** -- Display name
- **Source** -- How configured: `env`, `config`, `custom`, `api`
- **Models** -- Map of model ID to model definition
- **Options** -- Provider-specific options (base URL, API key, timeouts, etc.)

### 7.2 Model Definition

| Field | Type | Description |
|-------|------|-------------|
| id | string | Unique model identifier |
| providerID | string | Parent provider |
| name | string | Display name |
| family | string | Model family (optional) |
| api | object | Wire identity: `{id, url, npm}` |
| status | string | `active`, `deprecated`, `preview`, `alpha`, `beta` |
| capabilities | object | See capability flags below |
| cost | object | Per-million-token pricing (see below) |
| limit | object | Token limits: `{context, input?, output}` |
| size | number | Model size in billions of parameters (optional) |
| options | object | Provider-specific options (optional) |
| headers | object | Additional HTTP headers (optional) |
| release_date | string | Release date (optional) |
| variants | object | Model variant configurations (optional) |

**Capability flags:**

| Flag | Type | Description |
|------|------|-------------|
| temperature | boolean | Supports temperature parameter |
| reasoning | boolean | Supports reasoning/thinking mode |
| attachment | boolean | Supports file/image attachments |
| toolcall | boolean | Supports tool calling |
| input | object | Input modalities: `{text, audio, image, video, pdf}` |
| output | object | Output modalities: `{text, audio, image, video, pdf}` |
| interleaved | mixed | Supports interleaved reasoning: boolean or `{field: "reasoning_content" | "reasoning_details"}` |

**Cost structure:**

| Field | Description |
|-------|-------------|
| input | Cost per million input tokens |
| output | Cost per million output tokens |
| cache.read | Cost per million cache-read tokens |
| cache.write | Cost per million cache-write tokens |
| tiers | Context-based pricing tiers (higher rates when context exceeds thresholds) |

### 7.3 Provider Discovery

#### Local Provider Polling

Providers are discovered automatically on a **30-second** polling interval.

**Ollama** (default: `http://localhost:11434`, override: `TINYCODE_OLLAMA_HOST`):
- Probes `/api/tags` for model list
- Reads model capabilities from Ollama API (tools, vision, thinking)
- Probe timeout: 2 seconds
- Effective context for non-profiled models: 80% of advertised context
- Default output limit: `min(4096, 20% of context)`
- Profile entries use their baked-in `num_ctx` directly

**vLLM** (default: `http://localhost:8000`, override: `TINYCODE_VLLM_HOST` or `TINYCODE_VLLM_URLS`):
- Probes `/v1/models` for model list
- Assumes tool-call capability
- Effective context: 80% of `max_model_len`
- Default output limit: `min(4096, 20% of context)`

**LM Studio** (default: `http://localhost:1234`, override: `TINYCODE_LMSTUDIO_HOST`):
- Probes `/v1/models`, OpenAI-compatible

**ramalama** (via `TINYCODE_RAMALAMA_HOST`):
- Same `/v1/models` probe as vLLM

**MaaS** (Model-as-a-Service, via `TINYCODE_MAAS_HOST` + `TINYCODE_MAAS_API_KEY`):
- LiteMaaS/LiteLLM or any OpenAI-compatible endpoint
- Probes `/v1/models`, filters out embedding models (IDs containing "embed")

**Kubernetes in-cluster discovery:**
- Detects `KUBERNETES_SERVICE_HOST` environment variable
- Reads service account token and namespace
- Discovers vLLM services via three priority tiers:
  1. Services with annotation `tinycode.dev/discover=vllm` (explicit opt-in)
  2. Services with label `serving.kserve.io/inferenceservice` (KServe/RHOAI)
  3. Probe all TCP services on known vLLM ports (8080, 8000, 80)
- Also supports explicit `TINYCODE_VLLM_URLS` (comma-separated) regardless of cluster
- Each discovered service becomes its own provider (keyed as `vllm-<service-name>`)

#### OpenRouter Discovery

When `OPENROUTER_API_KEY` is set:
- Probes `https://openrouter.ai/api/v1/models` (5-second timeout)
- Filters to tool-capable models (those with `tools` in `supported_parameters`)
- Excludes `:free` and `:beta` variants
- Maps pricing, context lengths, modality support, and reasoning capability
- Provider ID: `openrouter`
- Cost tracking via OpenRouter's generation cost API

#### Bundled Provider SDKs

~25+ bundled AI SDK provider packages: Anthropic, OpenAI, Google (Gemini), Amazon Bedrock, Azure, Google Vertex, XAI, Mistral, Groq, DeepInfra, Cerebras, Cohere, Gateway, TogetherAI, Perplexity, Vercel, Alibaba, OpenRouter, GitLab, Venice, AI Gateway. Non-bundled providers can be installed dynamically from npm.

#### Remote Model Catalog

For cloud and API providers, tinycode fetches a curated model catalog from a remote URL to get accurate pricing, capabilities, context limits, and release dates for models that aren't self-describing (unlike Ollama which reports its own metadata).

**Fetch behavior:**
- Fetches `<TINYCODE_MODELS_URL>/api.json` with 10-second timeout and 2 retries with exponential backoff
- Cached to disk with a 5-minute TTL; cross-process file locking prevents concurrent fetches
- Background refresh every 60 minutes
- Fallback chain: disk cache → bundled snapshot → local catalog file → hardcoded fallback catalog

**Environment variables:**

| Variable | Description |
|----------|-------------|
| `TINYCODE_MODELS_URL` | Remote catalog URL (enables remote fetching) |
| `TINYCODE_MODELS_PATH` | Local file override (bypasses remote fetch) |
| `TINYCODE_DISABLE_MODELS_FETCH` | Disable remote fetching (use bundled/local only) |

**Catalog schema per model:** id, name, family, release_date, capabilities (attachment, reasoning, temperature, tool_call, interleaved), cost (input/output/cache per million tokens with optional context-size tiers), limits (context, input, output), modalities (text/audio/image/video/pdf), status (alpha/beta/deprecated), and experimental modes.

#### Provider Filtering

Config supports `enabled_providers` (whitelist) and `disabled_providers` (blacklist) arrays. Filters apply during discovery, so disabled providers are completely hidden from the provider list. Applies to all provider types.

### 7.4 Default Model Selection

Priority order:
1. Config `model` value
2. Most recently used model from persisted state file (`model.json`)
3. First provider's first model, sorted by priority list: gpt-5, claude-sonnet-4, big-pickle, gemini-3-pro

**Small model selection** (for title generation and compaction):
1. Config `small_model` value
2. Search by priority list: claude-haiku-4.5, claude-haiku-4-5, 3-5-haiku, 3.5-haiku, gemini-3-flash, gemini-2.5-flash, gpt-5-nano
3. Catalog scoring fallback: If no priority-list match, uses a cost+age scoring algorithm with regex `/\b(nano|flash|lite|mini|haiku|small|fast)\b/` to pick the cheapest recent small model from available providers

### 7.5 Provider-Specific Temperature Defaults

| Model Pattern | Default Temperature |
|---------------|-------------------|
| Qwen | 0.55 |
| Claude | undefined (model default) |
| Gemini, GLM-4.6/4.7, MiniMax-M2, Kimi-K2-thinking/K2.5 | 1.0 |
| Kimi-K2 (non-thinking) | 0.6 |
| Others | undefined |

### 7.6 Ollama Auto-Profiling

On first use of a local Ollama model, tinycode creates a derived model profile with a GPU-aware `num_ctx` baked in. Ollama's `/v1/chat/completions` ignores per-request `num_ctx` but respects it when baked into a Modelfile.

**Profile naming:** `{model}-tc{N}k` (e.g., `qwen3.5:9b-tc32k`). Detected by regex `/-tc\d+k$/`.

**Localhost detection:** Auto-profiling only runs when Ollama is local. Recognized hostnames: `localhost`, `127.0.0.1`, `[::1]`, `host.docker.internal`, `host.containers.internal`. Ollama `/api/show` calls use a 5-second timeout.

**`num_ctx` calculation formula:**

```
budget = min(gpuMemoryBytes * 0.5, 32 GB)          // 50% of GPU memory, capped at 32 GB
modelWeightBytes = parameterSize * quantBytesPerParam(quantLevel)
kvBudgetBytes = max(budget - modelWeightBytes, 100 MB)   // floor: 100 MB

headDim = embeddingLength / headCount
kvBytesPerToken = 2 * blockCount * headDim * headCountKV * 2
numCtx = floor(kvBudgetBytes / kvBytesPerToken)
numCtx = floor(numCtx / 1024) * 1024               // round down to nearest 1024
numCtx = clamp(numCtx, 2048, min(advertisedContextLength, 131072))
```

**GPU memory detection priority:**
1. macOS unified memory (`sysctl hw.memsize`)
2. NVIDIA (`nvidia-smi`)
3. AMD (`/sys/class/drm/card0/device/mem_info_vram_total`)
4. Fallback: 8 GB

GPU detection timeout: 2 seconds. Result is cached for the process lifetime.

**Quantization bytes-per-parameter lookup:**

| Quantization | Bytes per Parameter |
|---|---|
| Q4_0 | 0.5 |
| Q4_K_S | 0.53 |
| Q4_K_M | 0.55 |
| Q5_0 | 0.625 |
| Q5_K_S | 0.63 |
| Q5_K_M | 0.65 |
| Q6_K | 0.75 |
| Q8_0 | 1.0 |
| FP16 / F16 / BF16 | 2.0 |
| Default | 0.6 |

**Profile creation:** POST to Ollama `/api/create` with `{model: profileName, from: baseName, parameters: {num_ctx}}`. Timeout: 30 seconds. Profiles persist across restarts and are reused automatically.

**Stale profile cleanup:** During discovery polls, profiles whose base model no longer exists are deleted via `/api/delete`.

**Configuration overrides** (in `provider.ollama.options.auto_profile`):

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| enabled | boolean | true | Toggle auto-profiling |
| default_num_ctx | integer | -- | Override calculated value globally |
| max_num_ctx | integer | -- | Cap the calculated value |
| models | map | -- | Per-model overrides: `{num_ctx?: number, skip?: boolean}` |

**Guards** (auto-profiling is skipped when):
- Config `enabled: false`
- Ollama host is remote (not localhost/127.0.0.1/[::1]/host.docker.internal)
- Model is already a profile name (matches `-tc\d+k$`)
- Per-model `skip: true`

### 7.7 Model Warmup

On first use of a local model, a warmup probe is sent to verify the model is loaded and tool-calling works.

**Warmup request:**
- POST to `/v1/chat/completions`
- Payload: single user message ("Call the ready tool to confirm you are ready.") with a `ready` tool definition
- `max_tokens: 64`, `stream: false`, `tool_choice: "auto"`
- Timeout: 15 seconds
- Returns: `{ready: boolean, toolcall: boolean, durationMs: number, model: string}`
- Applies to local providers: ollama, ramalama, vllm, maas, lmstudio

### 7.8 Provider Timeouts

| Timeout | Default | Description |
|---------|---------|-------------|
| Header timeout | 300,000 ms (5 min) | Time to receive initial response headers |
| Chunk timeout | 300,000 ms (5 min) | Time between data chunks during streaming |
| Ollama keep_alive | "30m" | How long Ollama keeps model loaded between requests |
| Output token max | 32,000 | Maximum output tokens per inference step |

### 7.9 TLS Handling

Providers can set `tlsRejectUnauthorized: false` to skip certificate verification. TLS errors (UNABLE_TO_VERIFY_LEAF_SIGNATURE, CERT_HAS_EXPIRED, SELF_SIGNED_CERT, DEPTH_ZERO_SELF_SIGNED_CERT) trigger a helpful recovery message suggesting configuration or CA installation.

---

## 8. Retry Logic

### 8.1 Configuration

| Parameter | Value |
|-----------|-------|
| Initial delay | 2,000 ms |
| Backoff factor | 2x exponential |
| Jitter factor | 25% (random 0-25% added to base delay) |
| Max delay (no retry-after headers) | 30 seconds |
| Max delay (with headers) | 2,147,483,647 ms (max 32-bit signed int) |
| Max retries | 5 |

### 8.2 Delay Calculation

```
If error has responseHeaders object:
  If retry-after-ms header present: use parsed value, cap at max-int
  Else if retry-after header present: parse as seconds or HTTP date, cap at max-int
  Else: use exponential backoff, cap at max-int
Else (no error object or no responseHeaders):
  base = INITIAL_DELAY * 2^(attempt - 1)
  delay = ceil(base + base * 0.25 * random())
  Cap at min(delay, 30 seconds)
```

Note: The 30-second cap only applies when the error has no `responseHeaders` object at all. When `responseHeaders` exists (even without retry-after keys), the delay cap is max-int (2,147,483,647 ms).

### 8.3 Retryable Error Patterns

Approximately 30 regex patterns matching:

| Category | Patterns |
|----------|----------|
| HTTP status codes | 429, 500, 502, 503, 504, 524 |
| Rate limiting | `rate limit`, `too many requests`, `rate increased too quickly` |
| Server errors | `overloaded`, `service unavailable`, `internal error`, `server error`, `provider returned error` |
| Network failures | `fetch failed`, `connection error`, `connection refused`, `socket hang up`, `econnreset`, `econnrefused`, `etimedout`, `enotfound`, `eai_again`, `getaddrinfo`, `upstream connect`, `connection lost` |
| Timeouts | `timeout`, `request timeout`, `connection timed out`, `stream timeout`, `read timed out` |
| Retry suggestions | `try your request again`, `retry your request`, `resource exhausted` |
| Capacity | `try again later`, `at capacity`, `temporarily at capacity` |

**Not retried:** Context overflow errors (detected via ~15 provider-specific regex patterns for Anthropic, OpenAI, Bedrock, Google, xAI, Groq, OpenRouter, DeepSeek, vLLM, llama.cpp, LM Studio, MiniMax, Moonshot, Mistral, and generic fallbacks). Also matches `400 (no body)` and `413 (no body)`.

**5xx override:** 5xx errors are always retried, even when the provider SDK does not mark them as retryable.

---

## 9. Agents

### 9.1 Agent System

Agents define the AI's persona, permissions, and behavior for a session. Each agent has:

| Property | Description |
|----------|-------------|
| name | Agent identifier |
| description | Brief description |
| mode | `primary` (user-facing), `subagent` (spawned by other agents), `all` (both) |
| native | Whether it is a built-in agent |
| hidden | Whether it appears in the agent picker |
| permission | Permission ruleset scoping which tools are available |
| model | Optional model override |
| variant | Optional model variant |
| compact | Whether this is a compact variant (for small models) |
| prompt | System prompt content |
| temperature | LLM temperature override |
| topP | LLM top-p override |
| color | Display color |
| options | Additional options |
| steps | Max LLM steps |

### 9.2 Built-in Agents

**Primary agents** (user-facing, full session context):

| Agent | Description |
|-------|-------------|
| build | Default agent. Full tool permissions. Concise style, smallest correct change. |
| plan | Read-only agent. Hard permission enforcement -- can only write to `.tinycode/plans/*.md`. Prompts user to switch to build when implementation is ready. |

**Subagent agents** (spawned by primary agents via the task tool):

| Agent | Description |
|-------|-------------|
| general | General-purpose subagent |
| explore | Read-only codebase search (first agent to use per-agent tool permissions) |
| scout | Read-only with repository clone capability |

**Hidden agents** (system-internal, never shown in picker):

| Agent | Description |
|-------|-------------|
| compaction | Context summarization |
| title | Session title generation (uses small_model) |
| summary | Message summary generation |

**Specialized agents** (available as both primary and subagent modes):

agent-reviewer, analyst, architect, cluster-admin, code-reviewer, code-simplifier, critic, debugger, deep-explore, designer, document-specialist, executor, explore, git-master, planner, qa-tester, rules-reviewer, scientist, security-reviewer, skills-reviewer, test-engineer, tracer, verifier, workspace, writer

### 9.3 Compact Variants

For models with <= 8B parameters, compact agent variants are auto-selected. These have simplified prompts optimized for smaller context windows and less capable models. File naming convention: `agent-name.compact.md`.

Note: Some agents only exist as compact variants (e.g., `deep-explore` has only a `.compact.md` file and is therefore only available to models with 8B parameters or fewer).

### 9.4 Per-Agent Tool Permissions

Each agent's definition declares a `permission:` block that scopes which tools are injected into LLM calls:

| Tier | Tools | Approximate Token Cost |
|------|-------|----------------------|
| Read-only | read, glob, grep, bash | ~1,800 tokens |
| Write | Above + edit, write | ~2,700 tokens |
| Full | All tools | Higher |

This reduces prompt processing time significantly on local models (from ~38s to ~4-8s on 9B models).

### 9.5 Tool Tiering by Model Size

For local models, tools are selectively injected based on parameter count:

| Model Size | Available Tools |
|------------|----------------|
| <= 8B | Essential: invalid, question, bash, read, glob, grep, edit, write |
| <= 24B | Standard: above + task, skill, webfetch, todowrite |
| > 24B | All tools |

### 9.6 Custom Agents

Users can create custom agents via:

1. **Config:** Define in `agent` config map with prompt, mode, permissions, etc.
2. **Generation:** Use LLM to generate agent definition files from natural language descriptions (structured output via `generateObject`)
3. **Agent files:** Place `.md` files with frontmatter in designated directories

```json
{
  "agent": {
    "my-agent": {
      "prompt": "You are a specialized agent for...",
      "mode": "subagent",
      "temperature": 0.7,
      "permission": { "bash": "deny" }
    }
  }
}
```

Agents can be disabled via `"disable": true` in the agent config.

---

## 10. Tools

### 10.1 Tool Framework

Tools are defined with:
- **id** -- Unique tool name
- **description** -- Sent to the LLM as part of the system prompt
- **parameters** -- JSON Schema for arguments
- **execute** -- Async function receiving validated arguments and a context object

**Tool context provides:** `sessionID`, `messageID`, `agent` (current agent), `abort` (cancellation signal), `ask` (permission request function), `metadata` (callback to update tool title and metadata during execution).

### 10.2 Built-in Tools

#### File Operations

**read** -- Read files and directories

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| filePath | string | required | Absolute path |
| offset | integer | 1 | Line offset (1-indexed) |
| limit | integer | 2000 | Max lines to read |

Behaviors:
- Max bytes per read: 50 KB
- Max line length: 2,000 characters (truncated with indicator)
- Binary detection: extension-based check + byte analysis (30% non-text threshold)
- Image support: JPEG, PNG, GIF, WebP returned as base64 attachments
- PDF support: text extraction with fallback to base64
- Directory listing: returns file listing when path is a directory

**write** -- Create or overwrite files

| Parameter | Type | Description |
|-----------|------|-------------|
| filePath | string | Absolute path |
| content | string | File content |

Behaviors:
- Creates parent directories automatically
- Runs configured formatter after write
- Collects LSP diagnostics (up to 5 project files)

**edit** -- Search-and-replace file editing

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| filePath | string | required | Absolute path |
| oldString | string | required | Text to find |
| newString | string | required | Replacement text |
| replaceAll | boolean | false | Replace all occurrences |

Nine cascading fuzzy replacement strategies (tried in order until one succeeds):

| # | Strategy | Behavior |
|---|----------|----------|
| 1 | SimpleReplacer | Exact string match |
| 2 | LineTrimmedReplacer | Trim leading/trailing whitespace per line |
| 3 | BlockAnchorReplacer | Levenshtein similarity (threshold: 0.0 single match, 0.3 multiple) |
| 4 | WhitespaceNormalizedReplacer | Normalize all whitespace |
| 5 | IndentationFlexibleReplacer | Flexible indentation matching |
| 6 | EscapeNormalizedReplacer | Normalize escape sequences |
| 7 | TrimmedBoundaryReplacer | Trim boundary whitespace |
| 8 | ContextAwareReplacer | Context-based matching |
| 9 | MultiOccurrenceReplacer | Handle multiple occurrence disambiguation |

Additional behaviors: auto-formatting after write, LSP diagnostics collection, BOM handling, line ending detection/preservation (CRLF vs LF), per-file semaphore locking for concurrent edit safety.

**apply_patch** -- Unified patch application (mutually exclusive with `edit` and `write`)

| Parameter | Type | Description |
|-----------|------|-------------|
| patchText | string | Patch content |

Injected for GPT models (model ID contains `gpt-`, excluding `gpt-4` and models with `oss` in the ID). When `apply_patch` is active, `edit` and `write` tools are **removed** (not supplemented). Supports add/update/delete/move operations. Custom patch parser. Per-file formatting and LSP diagnostics.

#### Search

**glob** -- File pattern matching (via ripgrep)

| Parameter | Type | Description |
|-----------|------|-------------|
| pattern | string | Glob pattern |
| path | string | Search root (optional) |

Limit: 100 results. Sorted by modification time (newest first).

**grep** -- Regex content search (via ripgrep)

| Parameter | Type | Description |
|-----------|------|-------------|
| pattern | string | Regex pattern |
| path | string | Search root (optional) |
| include | string | File filter, e.g. `*.ts` (optional) |

Limit: 200 results (100 displayed). Sorted by modification time (newest first). Max line length: 2,000 characters.

#### Shell

**bash** -- Command execution

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| command | string | required | Shell command |
| timeout | integer | -- | Timeout in milliseconds (optional) |
| workdir | string | session dir | Working directory (optional) |
| description | string | -- | Human-readable description of command purpose (optional) |

Behaviors:
- Default timeout: 2 minutes
- Uses syntax analysis (AST parsing with bash + PowerShell grammars) for command safety checking
- Detects destructive commands: `rm -rf`, `git reset --hard`, `git push --force`, `git checkout --`, `git clean -f`, `mkfs`, `dd`, format commands
- Detects secrets file access patterns (`.env`, credentials files)
- Output streaming with chunked capture
- Truncation at configured limits (full output saved to truncation directory)
- Cross-platform: bash (Unix), PowerShell/cmd (Windows)

#### Subagent

**task** -- Spawn subagent sessions

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| description | string | -- | Short (3-5 word) task description (optional) |
| prompt | string | required | Task prompt |
| subagent_type | string | required | Agent type for subagent (no default) |
| task_id | string | -- | Resume existing task (optional) |
| command | string | -- | Slash command to run (optional) |
| background | boolean | false | Run in background (experimental, requires feature flag) |

Behaviors:
- Enforces `subagent_depth` limit (default 1)
- Creates child sessions with derived permissions
- **Foreground mode:** Blocks parent until complete, returns result inline
- **Background mode:** Returns immediately, injects result notification into parent session when complete
- Inherits model from parent session unless agent definition overrides

#### Web

**webfetch** -- Fetch URL content

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| url | string | required | URL to fetch |
| format | string | "text" | `text`, `markdown`, or `html` |
| timeout | integer | 30s | Request timeout |

Max response size: 5 MB. Max timeout: 120 seconds. HTML-to-markdown conversion. PDF text extraction. Image base64 encoding. Cloudflare bot detection retry.

**websearch** -- Web search

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| query | string | required | Search query |
| numResults | integer | 8 | Number of results |
| livecrawl | enum | "fallback" | `"fallback"` or `"preferred"` — live crawling mode (optional) |
| type | string | "auto" | `auto`, `fast`, or `deep` |
| contextMaxCharacters | integer | -- | Max characters per result (optional) |

Two search providers (Exa and Parallel), selected based on session hash for load distribution.

#### LSP Operations

**lsp** -- Language Server Protocol operations

| Parameter | Type | Description |
|-----------|------|-------------|
| operation | enum | See operations below |
| filePath | string | Target file |
| line | integer | Line number (optional) |
| character | integer | Column number (optional) |
| query | string | Symbol search query (optional) |

Operations: `goToDefinition`, `findReferences`, `hover`, `documentSymbol`, `workspaceSymbol`, `goToImplementation`, `prepareCallHierarchy`, `incomingCalls`, `outgoingCalls`.

#### Repository Operations

**repo_clone** -- Clone and cache repositories

| Parameter | Type | Description |
|-----------|------|-------------|
| repository | string | Repository URL or GitHub shorthand |
| refresh | boolean | Force re-clone (optional) |
| branch | string | Specific branch (optional) |

**repo_overview** -- Analyze repository structure

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| repository | string | required | Repository |
| path | string | -- | Subdirectory (optional) |
| depth | integer | 3 (max 6) | Directory tree depth |

Detects ecosystems (Node.js, Python, Go, Rust, Ruby, Java, PHP), package managers, entrypoints. Structure limit: 200 entries.

#### User Interaction

**question** -- Ask user questions (only available when client is `app`, `cli`, or `desktop`, or when `enableQuestionTool` flag is set)

| Parameter | Type | Description |
|-----------|------|-------------|
| questions | array | Array of `{question, header?, custom?, options?}` |

**todowrite** -- Manage todo list

| Parameter | Type | Description |
|-----------|------|-------------|
| todos | array | Array of `{content, status, priority}` |

#### Other Tools

| Tool | Description |
|------|-------------|
| skill | Load and execute skills (see Section 12) |
| swarm | Multi-agent coordination (see Section 10.3) |
| plan_exit | Exit plan mode (prompts user to switch to build agent). **Experimental:** gated behind `experimentalPlanMode` flag, CLI-only. |
| invalid | Fallback tool for malformed tool calls |

**Experimental tools:** `repo_clone` and `repo_overview` are gated behind the `experimentalScout` feature flag, `lsp` behind `experimentalLspTool`, and `plan_exit` behind `experimentalPlanMode` (CLI only). These are not available by default.

### 10.3 Swarm Mode

Multi-agent coordination using tmux sessions.

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| task | string | required | Task description |
| workers | integer | 4 (max 8) | Number of worker agents |
| model | string | configurable | Model for workers |
| variant | string | -- | Model variant (optional) |
| session_name | string | -- | tmux session name (optional) |
| shared_dir | string | -- | Shared filesystem directory (optional) |
| stale_seconds | integer | 240 | Worker stale timeout |
| poll_seconds | integer | 30 | Supervisor poll interval |
| worker_command | string | -- | Custom worker command (optional) |
| switch_client | boolean | -- | Switch tmux client to session (optional) |
| permissive | boolean | -- | Permissive mode (optional) |

**Architecture:**
- Creates a tmux session with a dashboard pane, supervisor pane, and worker panes
- Workers coordinate via shared filesystem:
  - `board.md` -- Task board
  - `inbox/` -- Incoming tasks
  - `claims/` -- Task claims
  - Status files and heartbeat files per worker
- Supervisor monitors worker health, detects stale workers (no heartbeat within `stale_seconds`), sends nudge messages

### 10.4 Output Truncation

Long tool outputs are automatically truncated:
- Max lines: 2,000 (configurable via `tool_output.max_lines`)
- Max bytes: 51,200 / 50 KB (configurable via `tool_output.max_bytes`)
- Full output is written to a truncation directory with a 7-day retention policy (hourly cleanup)
- Truncated output includes a preview and a hint pointing to the full output file

### 10.5 Tool Filtering

Tools are selectively injected based on:
- **Model capabilities:** Models with `toolcall=false` get no tools
- **Model size:** Essential/standard/full tiers (see Section 9.5)
- **Provider/model specific:** `apply_patch` only for GPT models; `websearch` only when search providers are configured
- **Agent permissions:** Per-agent tool scoping via permission rulesets
- **Config:** Individual tools can be enabled/disabled via `tools` config (deprecated, use `permission`)

### 10.6 Malformed Tool-Call Repair

When the LLM generates invalid JSON for tool call arguments:
1. Strip markdown fences (`` ```json ... ``` ``)
2. Remove trailing commas
3. Route to `invalid` tool if repair fails

---

## 11. Plugins

### 11.1 Plugin SDK

Plugins extend tinycode with custom tools, providers, authentication flows, and lifecycle hooks. A plugin is a module that exports a `PluginModule` object.

**Plugin module shape:**
- `id` -- Optional unique identifier
- `server` -- Plugin entry point (async function)
- `schema` -- Optional config validation schema

API version: 1 (declared in `package.json` under `engines.tinycode-plugin`).

**Plugin entry point receives:**
- `client` -- SDK client connected to the running server
- `project` -- Current project info (id, worktree, time)
- `directory` -- Current working directory
- `worktree` -- Project worktree root
- `serverUrl` -- URL of the running tinycode server
- `$` -- Shell helper for running commands

**Returns a `Hooks` object** with optional hook implementations.

### 11.2 Plugin Hooks

**Session lifecycle (observe-only):**

| Hook | Event |
|------|-------|
| `session.start` | Session created (receives sessionID, parentID?, agent?) |
| `session.end` | Session deleted |
| `session.switch` | User switched sessions |
| `session.model.change` | Model changed for a session |

**Tool hooks:**
- `tool` -- Register custom tools (keyed by tool name)
- `tool.execute.before` / `tool.execute.after` -- Intercept tool execution
- `tool.definition` -- Modify tool definitions

**Message hooks:**
- `chat.message` -- Intercept/modify chat messages before they reach the LLM
- `chat.params` -- Modify LLM parameters
- `chat.headers` -- Modify HTTP headers
- `experimental.chat.messages.transform` -- Transform the message array
- `experimental.chat.system.transform` -- Transform the system prompt
- `experimental.text.complete` -- Modify completed text output

**Provider hooks:**
- `provider` -- Register custom LLM providers with model discovery
- `aisdk.language` / `aisdk.sdk` -- AI SDK integration hooks
- `catalog.transform` -- Modify the provider/model catalog

**Other hooks:**
- `shell.env` -- Inject environment variables into shell commands
- `permission.ask` -- Handle permission requests
- `auth` -- Custom authentication flows (OAuth, API key)
- `event` -- Server event streaming
- `config` -- Config modification at load time
- `command.execute.before` -- Intercept command execution
- `agent.update` / `agent.remove` / `agent.default` -- Agent lifecycle
- `account.switched` -- Account switch notification
- `dispose` -- Cleanup on shutdown
- `experimental.session.compacting` -- Custom compaction behavior
- `experimental.compaction.autocontinue` -- Custom auto-continue after compaction

### 11.3 TUI Plugin API

Plugins can extend the TUI with:
- Custom commands and routes
- Dialogs and prompt modifications
- Theme registration
- Keymap extensions
- Custom UI slots
- Toast notifications and attention sounds
- KV store access
- Event bus integration

### 11.4 Plugin Installation

```
tinycode plugin <package-name>           # From npm
tinycode plugin file:///path/to/plugin   # Local file
```

Or via config:
```json
{
  "plugin": ["package-name", "file:///local/path"]
}
```

**Loading order:** Parallel resolution and install, compatibility check, import, then sequential hook registration.

### 11.5 Plugin Marketplace

Curated plugin registry with search via `tinycode plugin-search [query]`. Registry maps names to npm packages. Search is case-insensitive across name, description, and tags.

**Built-in plugins** (shipped with tinycode, not installed from npm):
- Codex (OpenAI)
- Copilot (GitHub)
- GitLab Auth
- Poe Auth
- Cloudflare Workers/AI Gateway
- Azure
- DigitalOcean
- xAI

### 11.6 Tool Definition API

Custom tools use the `tool()` helper:
- `description` -- Tool description string
- `args` -- Argument schemas (using validation library)
- `execute(args, context)` -- Returns string or `{title?, output, metadata?, attachments?}`

**Tool context provides:** sessionID, messageID, agent, directory, worktree, abort signal, metadata callback, ask (permission request), progress (emit progress messages), messages() (read conversation history), sessionInfo() (read session metadata).

### 11.7 oh-my-tiny (Built-in Plugin)

Native plugin providing 23 tools across five categories:

**State Management (5 tools):**
- `omt_state_read` / `omt_state_write` / `omt_state_clear` / `omt_state_list_active` / `omt_state_get_status`
- Predefined modes: autopilot, autoresearch, team, ralph, ultrawork, ultraqa, deep-interview, self-improve, ralplan, omc-teams, skill-active
- Max state size: 64 KB
- State stored in `.tinycode/` directory, scoped to project

**Notepad (6 tools):**
- `omt_notepad_read` / `omt_notepad_write_priority` / `omt_notepad_write_working` / `omt_notepad_write_manual` / `omt_notepad_prune` / `omt_notepad_stats`
- Three sections:
  - Priority Context: replaced on write, recommended < 500 characters
  - Working Memory: timestamped entries, auto-pruned after 7 days
  - Manual: never auto-pruned
- Stored at `.tinycode/notepad.md`

**Project Memory (4 tools):**
- `omt_project_memory_read` / `omt_project_memory_write` / `omt_project_memory_add_note` / `omt_project_memory_add_directive`
- Sections: techStack, build, conventions, structure, notes, directives
- Notes categorized by type; directives have priority (high/normal) and context
- Stored at `.tinycode/project-memory.json`

**Wiki (6 tools):**
- `omt_wiki_list` / `omt_wiki_read` / `omt_wiki_query` / `omt_wiki_add` / `omt_wiki_ingest` / `omt_wiki_delete`
- Categories: architecture, decision, pattern, debugging, environment, session-log, reference, convention
- Max page size: 512 KB
- Search by keywords, tags, and category with relevance snippets
- Stored in `.tinycode/wiki/` directory

**AST Grep (2 tools):**
- `omt_ast_grep_search` / `omt_ast_grep_replace`
- AST-based code pattern matching using meta-variables (`$NAME` for single node, `$$$ARGS` for multiple)
- Supported languages: JavaScript, TypeScript, Python, Go, Rust, Ruby, Java, C, C++, C#, Kotlin, Swift, Lua, HTML, CSS, and more
- Replace supports dry-run mode (default true)
- Search limited to first 1,000 files

---

## 12. Skills

### 12.1 Skill System

Skills are packaged instructions that guide agent behavior for specific tasks. Each skill is defined by a `SKILL.md` file with YAML frontmatter.

**SKILL.md frontmatter:**
```yaml
name: skill-name
description: One-line description
params: [param1, param2]    # Optional parameter list
```

**Parameter substitution:** Parameters declared in frontmatter map to `$1`, `$2`, ... `$N` placeholders in the skill content, substituted at invocation time. The last `$N` placeholder receives all remaining arguments joined with spaces. A special `$ARGUMENTS` placeholder receives the full raw argument string. If no placeholders exist and no `$ARGUMENTS`, arguments are appended to the template.

### 12.2 Skill Discovery

Skills are discovered from multiple sources (in priority order):
1. `.agents/` external directory
2. `.tinycode/skill/` and `.tinycode/skills/` project directories
3. Config `skills.paths` -- additional skill folder paths
4. Config `skills.urls` -- remote skill URLs
5. Bundled defaults

### 12.3 Built-in Skills

| Skill | Description |
|-------|-------------|
| ai-slop-cleaner | Clean AI-generated bloat with regression-safe deletion workflow |
| configure-notifications | Set up Telegram/Discord/Slack alerts |
| debug | Isolate single most-likely root cause; gather evidence, recommend smallest fix |
| deepinit | Generate per-directory AGENTS.md files across entire codebase |
| mcp-setup | Configure MCP servers via guided menu (curated bundles or custom) |
| remember | Triage session findings across memory surfaces |
| tc-doctor | 14+ self-diagnostic checks (Ollama, model health, tool-call probe, RAM, Metal, Rosetta, disk) using pure bash |
| trace | Evidence-driven causal tracing with competing hypotheses |
| verify | Confirm changes work before claiming completion via tiered evidence ladder |
| wiki | Guide agents on using project wiki for knowledge persistence |
| customize-tinycode | Interactive customization wizard (loaded when model touches tinycode config files) |

### 12.4 Skill Permissions

Skills are filtered based on agent permissions -- not all skills are available to all agents.

---

## 13. MCP (Model Context Protocol) Client

### 13.1 Client Integration

tinycode acts as an MCP client, connecting to external MCP servers to extend tool availability.

**Supported transports:**
- **stdio** -- Standard input/output with child processes
- **StreamableHTTP** -- HTTP-based streaming (primary)
- **SSE** -- Server-Sent Events (fallback from StreamableHTTP)

**Connection lifecycle:**
- Servers configured via `mcp` config key
- Each server has a status: `connected`, `disabled`, `failed`, `needs_auth`, `needs_client_registration`
- Parallel initialization of all MCP servers
- Default operation timeout: 30 seconds (configurable via `experimental.mcp_timeout`)

**Tool integration:**
- Tools from MCP servers are discovered via `tools/list`
- Tool naming: `sanitize(clientName) + "_" + sanitize(toolName)`
- MCP tools are converted to internal tool format and injected into LLM calls
- Tool list change notifications: watches for `ToolListChangedNotification`, refreshes tools live

**Resource and prompt discovery:**
- Resources fetched from MCP servers
- MCP prompts are mapped to slash commands

**OAuth authentication:**
- Full OAuth flow support for MCP servers requiring authentication
- Dynamic client registration (RFC 7591)
- Configurable callback port
- Browser-based authorization flow

**Cleanup:** On shutdown, kills child processes including descendant PIDs.

**Reconnect fix:** Patched MCP SDK to recognize JSON-RPC error responses, preventing infinite SSE reconnection loops when MCP servers return errors instead of valid JSON-RPC responses.

### 13.2 Configuration

**Local MCP server:**
```json
{
  "mcp": {
    "server-name": {
      "type": "local",
      "command": ["npx", "-y", "@example/mcp-server"],
      "environment": { "API_KEY": "..." },
      "enabled": true,
      "timeout": 30000
    }
  }
}
```

**Remote MCP server:**
```json
{
  "mcp": {
    "remote-server": {
      "type": "remote",
      "url": "https://example.com/mcp",
      "headers": { "Authorization": "Bearer ..." },
      "oauth": {
        "clientId": "...",
        "clientSecret": "...",
        "scope": "...",
        "callbackPort": 3000,
        "redirectUri": "http://localhost:3000/callback"
      },
      "enabled": true,
      "timeout": 30000
    }
  }
}
```

---

## 14. Permission System

### 14.1 Permission Rules

Each rule has three fields:
- **permission** -- Category (tool name, `doom_loop`, `guardrail`, `external_directory`)
- **pattern** -- Glob pattern matching the target (file path, command, etc.)
- **action** -- `allow`, `ask`, or `deny`

Evaluation: wildcard matching with longest-match-wins semantics.

### 14.2 Permission Flow

1. Agent or tool requests permission via `ask()`
2. System evaluates rules: first matching rule wins
3. If action is `ask`, user is prompted with options:
   - **once** -- Allow this specific request only
   - **always** -- Add to approved list for this project
   - **reject** -- Fail the tool call
4. Rejecting one permission rejects all pending permissions for that session

### 14.3 Default Permission Rules

| Permission | Pattern | Action |
|------------|---------|--------|
| * | * | allow |
| doom_loop | * | ask |
| guardrail | * | ask |
| external_directory | * | ask |
| question | * | deny |
| plan_enter | * | deny |
| plan_exit | * | deny |
| repo_clone | * | deny |
| repo_overview | * | deny |
| swarm | * | ask |
| read | * | allow |
| read | *.env | ask |
| read | *.env.* | ask |
| read | *.env.example | allow |

### 14.4 Permission Layering

Permissions are merged from multiple sources (in order):
1. Agent definition (`permission:` in agent frontmatter)
2. Session-level overrides
3. User config (`permission` in config file)
4. Persisted approved rules (stored in database per project)

### 14.5 Subagent Permission Derivation

Child sessions inherit a restricted permission set combining:
- Parent agent's edit deny rules
- Parent session's deny and external_directory rules
- Default denies for todowrite and task (preventing recursive spawning)

---

## 15. Configuration

### 15.1 Config File Locations

Config is loaded from multiple sources and merged (later sources override earlier):
1. Global config directory: `~/.config/tinycode/` -- files: `config.json`, `tinycode.json`, `tinycode.jsonc`
2. Custom config file: via `TINYCODE_CONFIG` env var
3. Additional config directory: via `TINYCODE_CONFIG_DIR` env var
4. Project config: `.tinycode/` directory
5. Inline JSON: via `TINYCODE_CONFIG_CONTENT` env var
6. Remote well-known configs (URL-based, fetched at startup)
7. Managed preferences (MDM/enterprise)

**JSONC support:** Config files support JSON with Comments format.

**Forward compatibility:** Config parsing silently ignores unknown fields, enabling newer config files to work with older versions and shared configs across teams.

### 15.2 Config Schema

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| shell | string | -- | Default shell |
| logLevel | enum | -- | `DEBUG`, `INFO`, `WARN`, `ERROR` |
| server.port | integer | 4096 | Server port |
| server.hostname | string | -- | Bind address |
| server.mdns | boolean | -- | Enable mDNS |
| server.cors | boolean | -- | Enable CORS |
| server.max_instances | integer | 32 | Max server instances |
| server.max_sessions | integer | unlimited | Max sessions |
| command | map | -- | Named command configurations |
| skills.paths | string[] | -- | Additional skill folder paths |
| skills.urls | string[] | -- | Remote skill URLs |
| reference | map | -- | Named directory references (`@alias`) |
| watcher.ignore | string[] | -- | File watcher ignore patterns |
| snapshot | boolean | true | Enable filesystem snapshot tracking |
| plugin | array | -- | Plugin specifications |
| share | enum | -- | `manual`, `auto`, `disabled` |
| autoupdate | boolean or "notify" | -- | Auto-update behavior |
| disabled_providers | string[] | -- | Provider blacklist |
| enabled_providers | string[] | -- | Provider whitelist |
| model | string | -- | Default model (`provider/model` format) |
| small_model | string | -- | Small model for title gen / compaction |
| default_agent | string | "build" | Default primary agent |
| subagent_depth | integer | 1 | Max subagent nesting |
| username | string | -- | Custom display username |
| agent | map | -- | Agent overrides and custom agents |
| provider | map | -- | Custom provider configurations |
| mcp | map | -- | MCP server configurations |
| formatter | boolean or object | -- | Formatter configuration |
| lsp | boolean or object | -- | LSP server configuration |
| instructions | string[] | -- | Additional instruction file paths/patterns |
| permission | object | -- | Permission rules |
| tools | map | -- | Tool enable/disable (deprecated, use permission) |
| attachment.image | object | -- | Image processing config |
| enterprise | object | -- | Enterprise URL |
| tool_output.max_lines | integer | 2000 | Tool output truncation lines |
| tool_output.max_bytes | integer | 51200 | Tool output truncation bytes |
| compaction | object | -- | Compaction tuning (see Section 6.4) |
| experimental | object | -- | Feature flags (see below) |

**Image attachment config:**

| Field | Type | Description |
|-------|------|-------------|
| attachment.image.auto_resize | boolean | Auto-resize images |
| attachment.image.max_width | integer | Max image width |
| attachment.image.max_height | integer | Max image height |
| attachment.image.max_base64_bytes | integer | Max base64 size |

**Agent config (per-agent):**

| Field | Type | Description |
|-------|------|-------------|
| model | string | Model override |
| variant | string | Model variant |
| temperature | number | LLM temperature |
| top_p | number | LLM top-p |
| prompt | string | Custom system prompt |
| description | string | Agent description |
| mode | enum | `subagent`, `primary`, `all` |
| hidden | boolean | Hide from agent picker |
| options | object | Additional options |
| color | string | Display color |
| steps | integer | Max LLM steps |
| permission | object | Permission ruleset |
| disable | boolean | Disable agent |

**Provider config (per-provider):**

| Field | Type | Description |
|-------|------|-------------|
| api | string | API identifier |
| name | string | Display name |
| env | string | Environment variable for API key |
| npm | string | npm package for SDK |
| whitelist | string[] | Model whitelist |
| blacklist | string[] | Model blacklist |
| options.apiKey | string | API key |
| options.baseURL | string | Base URL |
| options.timeout | integer | Request timeout |
| options.headerTimeout | integer | Header timeout (default 300,000 ms) |
| options.chunkTimeout | integer | Chunk timeout (default 300,000 ms) |
| options.tlsRejectUnauthorized | boolean | TLS cert verification |
| options.keepAlive | string | Model keep-alive duration |
| options.auto_profile | object | Ollama auto-profiling config |
| models | map | Per-model configuration overrides |

**Experimental flags:**

| Flag | Default | Description |
|------|---------|-------------|
| disable_paste_summary | false | Disable paste content summarization |
| batch_tool | false | Enable batch tool |
| openTelemetry | false | Enable OpenTelemetry spans |
| primary_tools | -- | Tools restricted to primary agents only |
| continue_loop_on_deny | false | Continue agent loop when tool call is denied |
| doom_loop_threshold | 3 | Identical tool call threshold |
| mcp_timeout | 30000 | MCP request timeout in ms |
| auto_continue | 3 | Max auto-continue nudges for small models (0 to disable) |
| wiki.auto_query | true | Inject wiki page hints into system prompt |
| wiki.triage | true | Enable wiki routing in `/remember` skill |

---

## 16. Storage

### 16.1 Database

Local relational database for persistence. Supports dual runtime via conditional imports for different JavaScript runtimes.

### 16.2 Schema

| Table | Purpose | Key Fields |
|-------|---------|------------|
| project | Project definitions | id (PK), worktree, vcs, name, icon_url, icon_url_override, icon_color, sandboxes (JSON string[]), commands (JSON), time_initialized, timestamps |
| session | Conversation sessions | id (PK), project_id (FK), workspace_id, parent_id, slug, directory, path, title, version, share_url, cost, tokens (5 fields), agent, model (JSON), permission (JSON), revert (JSON), summary fields, timestamps |
| message | Session messages | id (PK), session_id (FK), data (JSON), timestamps |
| part | Message parts | id (PK), message_id (FK), session_id, data (JSON), timestamps |
| todo | Session todo items | session_id (FK) + position (composite PK), content, status, priority, timestamps |
| session_message | Extended session messages | id (PK), session_id (FK), type, data (JSON), timestamps |
| permission | Project permissions | project_id (PK, FK), data (JSON ruleset), timestamps |
| workspace | Workspace definitions | id (PK), type, name, branch, directory, extra (JSON), project_id (FK), time_used |
| event_sequence | Event sequence tracking | id (PK), timestamps |
| event | Sync events | id (PK), sequence, data (JSON), timestamps |
| data_migration | Named migration tracking | name (PK), time_created |
| account | OAuth credentials | id (PK), email, url, access_token, refresh_token, token_expiry, timestamps |
| account_state | Active account and org | id (PK), active_account_id, active_org_id |

**Indexes:**
- session: project_id, workspace_id, parent_id
- message: (session_id, time_created, id) composite
- part: (message_id, id), session_id
- todo: session_id
- session_message: session_id, (session_id, type), time_created

**Timestamps:** All tables use `time_created` (auto-set on insert) and `time_updated` (auto-set on update), stored as integer milliseconds.

### 16.3 Data Migrations

Named data migrations run on startup in a background fiber. Each migration is tracked in the `data_migration` table by name — once a migration's completion row is written, it is never re-run. Migrations are resumable: if the process crashes mid-migration, it will re-run on next startup (idempotent). Migrations process data in paginated batches (e.g., 100 sessions per page) with short sleeps between pages to avoid blocking the main thread.

---

## 17. Graceful Shutdown

### 17.1 Server Shutdown (`tinycode serve`)

1. Catch SIGTERM and SIGINT signals
2. Dispose all instances (sessions, bus subscribers, background tasks)
3. Emit `global.disposed` event (notifies SSE clients to disconnect)
4. Drain HTTP connections
5. Stop the HTTP server

### 17.2 Instance Disposal

1. Publish `server.instance.disposed` event (so subscribers see it before shutdown)
2. Shut down the wildcard PubSub (delivers final event, then closes)
3. Shut down all typed PubSub channels
4. Close all scoped resources

### 17.3 Listener Stop

1. Unpublish mDNS (if active)
2. Optionally force-close active HTTP and WebSocket connections
3. Close the listener scope (releases all resources)

---

## 18. Security

### 18.1 Server Security

- **Password authentication:** Required for non-loopback bindings via `TINYCODE_SERVER_PASSWORD`
- **Authorization middleware:** Applied to all API routes
- **CORS:** Configurable cross-origin headers

### 18.2 Shell Command Safety

The shell tool uses AST-based command analysis to detect:
- **Destructive commands:** `rm -rf`, `git reset --hard`, `git push --force`, `git checkout --`, `git clean -f`, `mkfs`, `dd`, format commands
- **Secrets file access:** `.env`, credentials files, key files
- **Dangerous patterns:** Commands that could cause data loss or expose sensitive information

Detected dangerous commands trigger permission prompts before execution.

### 18.3 Desktop Security

- Content Security Policy headers: `default-src 'self' oc://renderer; script-src 'self' oc://renderer; style-src 'self' 'unsafe-inline' oc://renderer; connect-src 'self' oc://renderer http://localhost:* http://127.0.0.1:* ws://localhost:* ws://127.0.0.1:*; img-src 'self' oc://renderer data: blob:; font-src 'self' oc://renderer data:`
- Context isolation enabled, no Node.js integration in renderer, sandbox enabled
- Navigation restricted to renderer URLs only (`will-navigate` prevention)
- Window open handler: all new window requests denied; valid http/https URLs opened externally
- Custom protocol (`oc://renderer`) with path traversal prevention (relative path check)
- Permission request handler: only clipboard-sanitized-write and notifications from trusted renderer
- External URL scheme validation: only `http`, `https`, and `mailto`

### 18.4 Export Sanitization

The `--sanitize` flag redacts: file contents and paths, tool inputs and outputs, session titles and directories, snapshot data and diffs, system prompts and summaries. Each redacted value uses `[redacted:<type>:<id>]` format, preserving structure.

### 18.5 Protected File System

The system identifies OS-protected directories that should never be casually scanned, watched, or stated:

**macOS:**
- Home-level TCC-protected directories: Music, Pictures, Movies, Downloads, Desktop, Documents, Public, Applications, Library
- Library subdirectories: AddressBook, Calendars, Mail, Messages, Safari, Cookies, TCC database, CoreSpotlight, Suggestions, PersonalizationPortrait
- Root-level system directories: `.DocumentRevisions-V100`, `.Spotlight-V100`, `.Trashes`, `.fseventsd`

**Windows:**
- Home-level directories: AppData, Downloads, Desktop, Documents, Pictures, Music, Videos, OneDrive

Protected paths are excluded from file watchers and scanning operations to prevent OS permission prompts and unnecessary access.

### 18.6 Plugin Security

- Working directory validation: omt plugin tools validate that paths are within the user's home directory
- State and wiki size limits prevent unbounded disk usage (64 KB state, 512 KB wiki pages)
- Plugin tools go through the same permission system as built-in tools

---

## 19. System Prompt Assembly

### 19.1 Prompt Components

The system prompt sent to the LLM is assembled from:
1. **Agent prompt** -- The active agent's system prompt
2. **Instructions** -- From config `instructions` paths, project instruction files (CLAUDE.md, AGENTS.md)
3. **Tool descriptions** -- Descriptions for all available tools (filtered by agent permissions and model capabilities)
4. **Wiki hints** -- If `experimental.wiki.auto_query` is true, relevant wiki page hints are injected

### 19.2 Instruction Sources

- Global: `~/.config/tinycode/` instruction files
- Project: `.tinycode/` directory, CLAUDE.md, AGENTS.md
- Config: Additional paths from `instructions` config field

### 19.3 Auto-Continue

For small models that stop prematurely after tool calls (finish reason indicates stop rather than tool-use continuation), the system automatically sends a nudge to continue. Max nudges: 3 (configurable via `experimental.auto_continue`, 0 to disable).

---

## 20. Dual Model Architecture

### 20.1 Primary Model

Used for conversation, tool calls, and all main interactions. Set via `model` config or model picker.

### 20.2 Small Model

Used for lightweight tasks:
- Session title generation
- Context compaction summarization

Set via `small_model` config. Falls back to the primary model if not set.

---

## 21. OpenAPI Specification

The server generates an OpenAPI specification from its route definitions. The spec is available programmatically and can be used to auto-generate client SDKs.

**Auto-generated SDK:** A JavaScript/TypeScript SDK is auto-generated from the OpenAPI spec. Regenerate after API changes with the `generate` script.

---

## 22. References and Named Directories

Config supports named references that can be mentioned as `@alias` or `@alias/path` in conversations:

```json
{
  "reference": {
    "docs": "/path/to/documentation",
    "shared-lib": "git://github.com/org/shared-lib"
  }
}
```

References can be git repositories (cloned and cached) or local directories.

---

## 23. Formatter and LSP Integration

### 23.1 Formatters

Configurable code formatters that run automatically after file write/edit operations. Can be enabled globally, disabled, or configured per-language/pattern.

### 23.2 LSP Servers

Language Server Protocol servers provide:
- Code navigation (go-to-definition, find-references, hover)
- Diagnostics collection after file edits (up to 5 project files)
- Symbol search (document and workspace)
- Call hierarchy analysis

LSP servers are configured via the `lsp` config key and can be enabled with built-in defaults or custom configurations.

---

## 24. Image Processing

Attachments with image MIME types are processed:
- Normalized (resized if needed) to fit within model-specific image size limits
- Returned as base64 data URLs in file parts
- Configurable via `attachment.image` config (auto_resize, max_width, max_height, max_base64_bytes)
- If the image resizer is unavailable, images are passed through unchanged

---

## 25. Session Revert

### 25.1 Snapshot Tracking

Before each LLM inference step, a filesystem snapshot is captured (when `snapshot` config is true, the default). After the step completes, a patch is computed representing all file changes.

### 25.2 Revert Operation

Reverting a message:
1. Identifies the snapshot taken before the message's changes
2. Restores files to their pre-message state
3. Records the revert state on the session for potential unrevert

### 25.3 Unrevert

Restoring previously reverted messages, re-applying their file changes.

---

## 26. Custom Tools

Users can add custom tools by placing JavaScript/TypeScript files in:
- `tool/*.{js,ts}` or `tools/*.{js,ts}` directories relative to the project

Custom tools use the same `tool()` API as plugin tools (see Section 11.6) and are automatically discovered and registered.

---

## 27. Environment Variables

| Variable | Description |
|----------|-------------|
| `TINYCODE_OLLAMA_HOST` | Ollama base URL (default `http://localhost:11434`) |
| `TINYCODE_VLLM_HOST` | vLLM base URL (default `http://localhost:8000`) |
| `TINYCODE_LMSTUDIO_HOST` | LM Studio base URL (default `http://localhost:1234`) |
| `TINYCODE_RAMALAMA_HOST` | ramalama base URL |
| `TINYCODE_MAAS_HOST` | MaaS/LiteLLM base URL |
| `TINYCODE_MAAS_API_KEY` | MaaS API key |
| `TINYCODE_VLLM_URLS` | Comma-separated vLLM endpoint URLs |
| `OPENROUTER_API_KEY` | OpenRouter API key |
| `TINYCODE_SERVER_PASSWORD` | Server password for non-loopback bindings |
| `TINYCODE_SERVER_USERNAME` | Server username (default `tinycode`) |
| `TINYCODE_INSTALL_DIR` | Installation directory (default `$HOME/.local/bin`) |
| `TINYCODE_CONFIG` | Config file path |
| `TINYCODE_CONFIG_DIR` | Additional config directory |
| `TINYCODE_CONFIG_CONTENT` | Inline JSON config |
| `TINYCODE_DB` | Custom database file path |
| `TINYCODE_CLIENT` | Client identifier: `cli` (default), `app`, `desktop` |
| `TINYCODE_PURE` | Disable external process spawning (for sandboxed environments) |
| `TINYCODE_PERMISSION` | Default permission mode |
| `TINYCODE_WORKSPACE_ID` | Workspace scope identifier |
| `TINYCODE_DISABLE_AUTOUPDATE` | Disable automatic update checks |
| `TINYCODE_ALWAYS_NOTIFY_UPDATE` | Always show update notifications |
| `TINYCODE_DISABLE_MOUSE` | Disable mouse input in TUI |
| `TINYCODE_DISABLE_TERMINAL_TITLE` | Don't set the terminal title |
| `TINYCODE_DISABLE_PRUNE` | Disable conversation pruning |
| `TINYCODE_DISABLE_AUTOCOMPACT` | Disable automatic context compaction |
| `TINYCODE_DISABLE_MODELS_FETCH` | Disable remote model catalog fetching |
| `TINYCODE_DISABLE_PROJECT_CONFIG` | Ignore project-level configuration |
| `TINYCODE_MODELS_URL` | Remote model catalog URL |
| `TINYCODE_MODELS_PATH` | Local model catalog file path |
| `TINYCODE_GIT_BASH_PATH` | Custom path to Git Bash (Windows) |
| `TINYCODE_TUI_CONFIG` | Custom TUI configuration path |
| `TINYCODE_PLUGIN_META_FILE` | Plugin metadata file path |
| `TINYCODE_FAKE_VCS` | Fake VCS directory for testing |
| `TINYCODE_EXPERIMENTAL` | Enable all experimental features |
| `TINYCODE_EXPERIMENTAL_WORKSPACES` | Enable workspace system |
| `TINYCODE_EXPERIMENTAL_SESSION_SWITCHER` | Enable experimental session switcher |
| `TINYCODE_EXPERIMENTAL_FILEWATCHER` | Enable file watcher |
| `TINYCODE_EXPERIMENTAL_DISABLE_FILEWATCHER` | Force-disable file watcher |
| `TINYCODE_EXPERIMENTAL_DISABLE_COPY_ON_SELECT` | Disable copy-on-select (default true on Windows) |
| `TINYCODE_SHOW_TTFD` | Show time-to-first-display metrics |
| `TINYCODE_AUTO_HEAP_SNAPSHOT` | Enable automatic heap snapshots for debugging |
| `KUBERNETES_SERVICE_HOST` | Detected for in-cluster discovery (not user-set) |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OpenTelemetry OTLP endpoint URL |
| `OTEL_EXPORTER_OTLP_HEADERS` | OTLP headers (comma-separated `key=value` pairs) |
| `OTEL_RESOURCE_ATTRIBUTES` | OTLP resource attributes (comma-separated `key=value` pairs) |
| `http_proxy` / `HTTP_PROXY` | HTTP proxy URL |
| `https_proxy` / `HTTPS_PROXY` | HTTPS proxy URL |
| `no_proxy` / `NO_PROXY` | Comma-separated hosts to bypass proxy |

---

## 28. LLM Protocol Translation Layer

A protocol translation layer converts between the internal message format and each LLM provider's wire protocol. This enables a single internal representation while supporting providers with different API contracts.

**Supported protocols:**

| Protocol | Description |
|----------|-------------|
| Anthropic Messages | Anthropic's Messages API (`/v1/messages`) |
| Bedrock Converse | AWS Bedrock Converse API |
| Bedrock Event Stream | AWS Bedrock streaming event format |
| Gemini | Google Gemini API |
| OpenAI Chat | OpenAI Chat Completions API (`/v1/chat/completions`) |
| OpenAI Compatible Chat | Generic OpenAI-compatible endpoints (Ollama, vLLM, LM Studio, etc.) |
| OpenAI Responses | OpenAI Responses API |

Each protocol implementation handles: message format translation, tool definition mapping, streaming event parsing, error normalization, and capability negotiation. Shared utilities handle common patterns like token counting, content part mapping, and response normalization.

---

## 29. Slash Command System

Built-in slash commands provide quick access to common workflows:

| Command | Description |
|---------|-------------|
| `/init` | Guided AGENTS.md setup (substitutes project path into template) |
| `/review` | Review changes — accepts commit, branch, or PR; defaults to uncommitted changes. Runs as a subtask. |
| `/ask <agent> <prompt>` | Delegate a prompt to a specific agent as a subtask |
| `/swarm` | Launch supervised tmux swarm |

**Command sources:** Commands are merged from three sources at startup:
1. **Built-in commands** — The four defaults above
2. **User-defined commands** — Via `command` config map with template, agent, model, description, and subtask flag
3. **MCP prompts** — Each MCP server's declared prompts are exposed as commands, with arguments mapped to `$1`, `$2`, etc.
4. **Skills** — All registered skills are available as `/skill-name` commands

**Template substitution:** Command templates use `$1`, `$2` positional placeholders and `$ARGUMENTS` for the full argument string. Hints are derived from the template at registration time.

---

## 30. Internationalization

The web UI and shared UI components support 19 languages:

Arabic, Brazilian Portuguese, Bosnian, Danish, German, English, Spanish, French, Japanese, Korean, Norwegian, Polish, Russian, Thai, Turkish, Ukrainian, Chinese (Simplified), Chinese (Traditional)

**Locale detection:** Uses the browser's language preference to select the translation bundle. English is the default fallback.

**Scope:** Internationalization covers the web UI and desktop app. The TUI uses English only with locale-aware formatting for dates, times, and numbers.

---

## 31. File Watcher

An optional file watcher monitors the project directory for changes using platform-native backends:

| Platform | Backend |
|----------|---------|
| macOS | FSEvents |
| Linux | inotify |
| Windows | Windows API |

**Behavior:**
- Publishes `file.watcher.updated` events with `{file, event}` where event is `add`, `change`, or `unlink`
- Subscribe timeout: 10 seconds
- Respects `.gitignore`-style ignore patterns plus configurable `watcher.ignore` patterns from config
- Excludes OS-protected directories (see Section 18.5)
- Optionally watches the `.git` directory for HEAD changes (excludes all git internals except HEAD)
- Gated behind `TINYCODE_EXPERIMENTAL_FILEWATCHER` flag

---

## 32. Proxy Support

HTTP/HTTPS proxy support for outbound requests:

- Configured via standard environment variables: `http_proxy`/`HTTP_PROXY`, `https_proxy`/`HTTPS_PROXY`, `no_proxy`/`NO_PROXY`
- `no_proxy` supports hostname matching, wildcard prefixes (`*.example.com`), and port-specific exclusions
- `all_proxy` fallback when protocol-specific proxy is not set
- The desktop sidecar automatically adds loopback addresses (127.0.0.1, localhost, ::1) to `NO_PROXY` to prevent proxying local server connections

---

## 33. Observability (OpenTelemetry)

Full OTLP log export for production diagnostics:

- **Endpoint:** Configured via `OTEL_EXPORTER_OTLP_ENDPOINT`
- **Headers:** Additional headers via `OTEL_EXPORTER_OTLP_HEADERS` (comma-separated `key=value`)
- **Resource attributes:** Via `OTEL_RESOURCE_ATTRIBUTES` (comma-separated `key=value`, URL-decoded)
- **Default resource attributes:** `deployment.environment.name` (installation channel), `tinycode.client`, `tinycode.process_role`, `tinycode.run_id`, `service.instance.id` (random UUID per process)
- **Service name:** `tinycode`, versioned with the installation version
- Also available as an experimental config flag (`experimental.openTelemetry`) for span-level tracing

---

## 34. Workspaces (Experimental)

An experimental workspace system for organizing sessions by context:

- **Gated behind:** `TINYCODE_EXPERIMENTAL_WORKSPACES` or `TINYCODE_EXPERIMENTAL=true`
- **Workspace data:** id, type, name, branch, directory, extra metadata (JSON), project reference, last-used timestamp
- **Operations:** CRUD via API routes, TUI dialogs for workspace creation and selection
- **Session scoping:** Sessions can be scoped to a workspace via `workspace_id`
- **Routing:** Server middleware routes requests to the appropriate workspace context

---

## 35. V2 Event and Schema System

A versioned event/schema layer coexists alongside the primary event bus:

- **Typed events:** `EventV2` definitions with schema validation for type-safe event publishing and subscription
- **Schema types:** `ModelV2`, `ProviderV2`, `CatalogV2` provide versioned data representations for the V2 API routes
- **Projectors:** Transform internal state into V2 schema representations
- **Migration path:** V2 routes (`/api/model`, `/api/provider`) use V2 schemas while V1 routes continue unchanged

---

## 36. Package Structure

The project is organized as a monorepo with the following packages:

| Package | Purpose |
|---------|---------|
| Core | HTTP API server, business logic, TUI, CLI, all agent/tool/session logic |
| Web UI | Reactive web application for browser and desktop |
| Desktop | Desktop application shell wrapping the web UI |
| VS Code Extension | Reference IDE extension demonstrating ACP integration |
| Plugin SDK | Public plugin API |
| JavaScript SDK | Auto-generated client SDK from OpenAPI spec |
| UI Library | Shared component library for web and desktop |
| LLM Protocol | LLM protocol implementations and wire-format translation (see Section 28) |
| Database Wrapper | ORM wrapper for relational database |
| HTTP Recorder | HTTP request/response recorder for test fixtures |
| Scripts | Build and release scripts |
