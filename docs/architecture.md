# Architecture

tinycode is a local-LLM-first AI coding assistant. It exposes an HTTP API server, a terminal UI (TUI), and a web UI, all wired to a session engine that orchestrates LLM calls and agent tools. The primary inference targets are Ollama, vLLM, and OpenAI-compatible servers on localhost or LAN. Cloud providers (Anthropic, OpenAI, Google, etc.) are available via API key as a secondary option.

---

## Monorepo Structure

Bun workspaces with Turborepo. All packages live under `packages/`.

| Package                 | Name                              | Purpose                                                                                  |
| ----------------------- | --------------------------------- | ---------------------------------------------------------------------------------------- |
| `tinycode`              | `tinycode`                        | Core server, HTTP API, CLI, TUI, session processor, provider abstraction, tools, plugins |
| `app`                   | `@tinycode/app`                   | SolidJS + TailwindCSS v4 web UI                                                          |
| `desktop`               | `@tinycode/desktop`               | Electron desktop app wrapping `packages/app`                                             |
| `llm`                   | `@tinycode/llm`                   | LLM protocol implementations (Anthropic Messages, OpenAI Chat, Bedrock Converse, Gemini) |
| `ui`                    | `@tinycode/ui`                    | Shared SolidJS component library (icons, themes, i18n, markdown, diffs)                  |
| `plugin`                | `@tinycode/plugin`                | Public plugin SDK                                                                        |
| `sdk/js`                | `@tinycode/sdk`                   | Auto-generated TypeScript SDK from OpenAPI spec                                          |
| `effect-drizzle-sqlite` | `@tinycode/effect-drizzle-sqlite` | Effect wrapper for Drizzle ORM + SQLite                                                  |
| `http-recorder`         | `@tinycode/http-recorder`         | HTTP/WebSocket recording for tests (VCR-style cassettes)                                 |
| `script`                | `@tinycode/script`                | Build scripts and release utilities                                                      |

There is no standalone `packages/core` package. Core utilities (logging, git, npm helpers, schema definitions) live at `packages/tinycode/src/core/`.

---

## packages/tinycode — Core Server & CLI

The heart of the project. Every directory under `src/`:

| Directory        | Purpose                                                                                                                  |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------ |
| `account/`       | User account management and schema                                                                                       |
| `agent/`         | Agent definitions (explore, scout, general, compaction, title, summary) and subagent permissions                         |
| `auth/`          | Authentication for providers and services                                                                                |
| `background/`    | Background job processing                                                                                                |
| `bus/`           | Event bus — Effect PubSub for typed event streaming between session processor, TUI, and web clients                      |
| `cli/`           | CLI entry point, command routing, TUI implementation                                                                     |
| `command/`       | Custom command configuration                                                                                             |
| `config/`        | Configuration loading — global, project-local, env overrides, plugin configs, managed configs                            |
| `control-plane/` | Workspace management and schema                                                                                          |
| `core/`          | Shared utilities (logging, git, npm, schema, process management)                                                         |
| `effect/`        | Effect framework utilities (bridges, instance state, runtime flags)                                                      |
| `env/`           | Environment variable management                                                                                          |
| `file/`          | File system utilities (ripgrep integration, glob patterns)                                                               |
| `format/`        | Code formatting integration                                                                                              |
| `git/`           | Git operations                                                                                                           |
| `id/`            | ID generation (ULID)                                                                                                     |
| `ide/`           | IDE integration                                                                                                          |
| `image/`         | Image processing                                                                                                         |
| `lsp/`           | Language Server Protocol client                                                                                          |
| `mcp/`           | Model Context Protocol client (stdio, SSE, HTTP transports; OAuth flow)                                                  |
| `omt/`           | oh-my-tiny plugin integration                                                                                            |
| `patch/`         | Diff/patch utilities                                                                                                     |
| `permission/`    | Tool access control                                                                                                      |
| `plugin/`        | Plugin system — loader, installer, built-in plugins (azure, cloudflare, digitalocean, github-copilot, xai, openai/codex) |
| `project/`       | Project context detection and schema                                                                                     |
| `provider/`      | LLM provider abstraction — wraps Vercel AI SDK, auto-discovers local LLMs, model warmup with tool-call probe              |
| `pty/`           | Pseudo-terminal — dual runtime (bun-pty / @lydell/node-pty)                                                              |
| `question/`      | Interactive question/prompt system                                                                                       |
| `reference/`     | Reference management (repos, directories)                                                                                |
| `server/`        | HTTP server — Effect HTTP with middleware, route groups, WebSocket, SSE                                                  |
| `session/`       | Session management — processor loop, LLM streaming, tool execution, compaction, overflow handling                        |
| `shell/`         | Shell command execution                                                                                                  |
| `skill/`         | Skills system (slash commands, remote skill indexes)                                                                     |
| `snapshot/`      | Filesystem snapshot tracking for undo/redo                                                                               |
| `storage/`       | SQLite via Drizzle ORM — dual runtime (bun:sqlite / node:sqlite)                                                         |
| `sync/`          | Synchronization events and schema                                                                                        |
| `tool/`          | Tool implementations — read, write, edit, glob, grep, shell, LSP, webfetch, websearch, MCP, task, plan, skill, etc.      |
| `util/`          | General utilities                                                                                                        |
| `worktree/`      | Git worktree management                                                                                                  |

### Server

Effect-based HTTP server using `effect/unstable/http`. Default port 4096. Route groups under `src/server/routes/instance/httpapi/groups/`: config, control, event, experimental, file, global, instance, mcp, metadata, permission, project, provider, pty, query, question, session, tui, v2.

Middleware: authorization, CORS, compression, error handling, workspace routing, schema validation.

Exposes REST endpoints, SSE at `/global/event` for real-time updates, and WebSocket at `/pty/:id/ws` for terminal I/O.

### Session Processor

`src/session/processor.ts` runs the main agent loop: stream LLM response, execute tool calls, manage context overflow (compaction), handle retries, track run state. Each session is independent with its own processor instance.

### Tools

Registered in `src/tool/registry.ts`. Each tool is an Effect service. Current tools: read, write, edit, glob, grep, shell, LSP, webfetch, websearch, question, task, todo, skill, plan, swarm, apply_patch, repo_clone, repo_overview. Some tools are conditionally enabled (LSP, repo_clone, repo_overview, plan).

---

## Provider Landscape

### Auto-Discovered (no config needed)

| Provider | Default URL                 | Env Override                                  |
| -------- | --------------------------- | --------------------------------------------- |
| ollama   | `http://localhost:11434/v1` | `TINYCODE_OLLAMA_HOST`                        |
| vllm     | `http://localhost:8000/v1`  | `TINYCODE_VLLM_HOST`                          |
| maas     | (none — set via env)        | `TINYCODE_MAAS_HOST`, `TINYCODE_MAAS_API_KEY` |

Auto-discovery polls every 30 seconds with a 2-second probe timeout (`src/provider/local-discovery.ts`).

On startup, tinycode sends a warmup probe to the configured Ollama model (`src/provider/warmup.ts`). The probe sends a tool-call request to `/api/chat` with `keep_alive: "10m"`, pre-loading the model into GPU memory and verifying tool-calling capability. Results are shown as a toast (TUI) or footer message (direct mode).

### Bundled Cloud Providers (via Vercel AI SDK)

anthropic, openai, google, amazon-bedrock, azure, google-vertex, xai, mistral, groq, deepinfra, cerebras, cohere, openrouter, togetherai, perplexity, vercel, alibaba, gateway, gitlab, venice, and any generic openai-compatible endpoint.

### Plugin Providers

github-copilot (`src/plugin/github-copilot/`), cloudflare (`src/plugin/cloudflare.ts`), digitalocean (`src/plugin/digitalocean.ts`), azure (`src/plugin/azure.ts`), xai (`src/plugin/xai.ts`).

---

## Key Patterns

### Effect Framework

All major subsystems are Effect `Context.Service` classes composed via `Layer`:

```ts
export class Service extends Context.Service<Service, Interface>()("@tinycode/Session") {}
export const layer = Layer.effect(Service, Effect.gen(function* () { ... }))
```

Dependencies are injected via Context. Resources are cleaned up via `Scope` and finalizers. Error handling uses typed Effect errors (`Schema.TaggedErrorClass`).

### Event Bus (`src/bus/`)

Built on Effect `PubSub`. Typed event definitions via `BusEvent.define()`. Per-instance buses (scoped to project directory) plus a global bus. Events flow from session processor through the bus to TUI and web clients via SSE.

### Dual Runtime

Conditional imports via `package.json` `imports` field:

- `#db`: `db.bun.ts` (bun:sqlite) vs `db.node.ts` (node:sqlite DatabaseSync)
- `#pty`: `pty.bun.ts` (bun-pty) vs `pty.node.ts` (@lydell/node-pty)

### TUI (SolidJS + opentui)

`src/cli/cmd/tui/` — Terminal UI built with SolidJS for reactive state and opentui for terminal rendering. Component-based architecture with contexts, routes, themes, and a keymap system with chord/leader key support. The TUI either spawns the server in a worker thread or attaches to an existing one.

---

## Web UI (`packages/app`)

SolidJS + TailwindCSS v4 + SolidJS Router. Connects to the tinycode API server.

**Routes**: `/` (home), `/:dir` (directory layout), `/:dir/session/:id` (session view).

**Connection**: REST for commands, SSE at `/global/event` for real-time updates (16ms event coalescing, auto-reconnect), WebSocket for terminal I/O.

**Dev**: `bun run --cwd packages/app dev` (Vite dev server, requires tinycode server running).

---

## Desktop (`packages/desktop`)

Electron app. Main process spawns the tinycode server as a utility process sidecar (`utilityProcess.fork()`), selects a random port, generates a Basic auth password, and waits for health check. Renderer loads the web UI via `@tinycode/app` with a MemoryRouter.

**Dev**: `bun run --cwd packages/desktop dev` (electron-vite).

---

## SDK (`packages/sdk/js`)

Auto-generated from the OpenAPI spec using `@hey-api/openapi-ts`.

**Regenerate**: `./script/generate.ts` (runs `bun dev generate` to produce `openapi.json`, then generates TypeScript client).

**Key exports**: `TinycodeClient` (class), `createTinycodeClient()` (factory), `createTinycodeServer()` (spawns server subprocess), `createTinycodeTui()` (launches TUI).

---

## Configuration

### Paths

| Scope        | Location                                                                                                  |
| ------------ | --------------------------------------------------------------------------------------------------------- |
| Global       | `~/.config/tinycode/config.json` (also checks `tinycode.json`, `tinycode.jsonc`)                          |
| Project      | `.tinycode/tinycode.json` (walks up to worktree root)                                                     |
| Env override | `TINYCODE_CONFIG` (file path), `TINYCODE_CONFIG_DIR` (directory), `TINYCODE_CONFIG_CONTENT` (inline JSON) |

Configs from all sources are deep-merged. Plugin configs are deduplicated by identity.

### Database

SQLite via Drizzle ORM at `~/.local/share/tinycode/tinycode.db` (XDG_DATA_HOME). Schema files: `storage/schema.sql.ts`, `session/session.sql.ts`, `project/project.sql.ts`, `account/account.sql.ts`, `control-plane/workspace.sql.ts`, `sync/event.sql.ts`. Migrations run automatically on startup.

---

## CLI Commands

| Command                       | Purpose                                                                       |
| ----------------------------- | ----------------------------------------------------------------------------- |
| `tinycode` (or `bun dev`)     | Interactive TUI (default)                                                     |
| `tinycode run [message..]`    | Non-interactive prompt execution                                              |
| `tinycode serve`              | Headless API server on port 4096                                              |
| `tinycode web`                | Server + open web UI in browser                                               |
| `tinycode models [provider]`  | List available models                                                         |
| `tinycode providers`          | List configured providers                                                     |
| `tinycode mcp`                | MCP server management                                                         |
| `tinycode plugin <module>`    | Plugin management                                                             |
| `tinycode session`            | Session management (list, delete, create)                                     |
| `tinycode export [sessionID]` | Export session                                                                |
| `tinycode import <file>`      | Import session                                                                |
| `tinycode setup`              | Initial setup wizard                                                          |
| `tinycode status`             | System status                                                                 |
| `tinycode uninstall`          | Uninstall tinycode                                                            |
| `tinycode db`                 | Database operations                                                           |
| `tinycode debug <name>`       | Debug utilities (agent, config, file, lsp, ripgrep, skill, snapshot, startup) |

---

## Plugin System

Plugins extend tinycode with custom tools, providers, and TUI features.

**Sources**: npm packages (installed to `.tinycode/node_modules/`), local `.ts` files, or directory-based plugins in `.tinycode/plugin/`.

**Hook categories**: auth, provider, chat (message/params/headers), tool (definition/execute), permission, command, shell (env injection), session (compaction), text (completion).

**Config**:

```json
{
  "plugin": ["npm-package-name", { "npm": "package-name", "options": {} }, "./local-plugin.ts"]
}
```

**Built-in plugins**: azure, cloudflare, digitalocean, github-copilot, xai, openai/codex, omt (oh-my-tiny).
