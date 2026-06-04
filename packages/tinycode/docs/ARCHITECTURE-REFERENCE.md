# tinycode Architecture Reference

> Verified reference document. For AI-generated drafts see `ARCHITECTURE.md`.

## Overview

tinycode is a local-LLM-first AI coding assistant delivered as a Terminal UI (TUI). It runs against local inference servers (Ollama, vLLM) or LAN MaaS endpoints with zero cloud dependencies required. Cloud providers (Anthropic, OpenAI, Google) are supported via API key as a secondary option.

**Runtime**: Bun  
**Language**: TypeScript (ESM-first)  
**Key frameworks**: Effect v4 (beta.66), SolidJS, opentui, Drizzle ORM  
**Server port**: 4096 (default, falls back to free port)

---

## Monorepo Structure

```
packages/
├── tinycode/          ← Core server, TUI, CLI, all business logic
├── core/              ← Shared utilities: models, permissions, git, npm config
│                        Imported as @opencode-ai/core/...
├── app/               ← SolidJS + TailwindCSS v4 web UI
├── desktop/           ← Electron shell wrapping packages/app
├── plugin/            ← Plugin SDK (@opencode-ai/plugin)
├── sdk/js/            ← Auto-generated JS SDK (from OpenAPI spec)
├── llm/               ← @opencode-ai/llm — native LLM client
├── script/            ← @opencode-ai/script — build/publish tooling
├── ui/                ← Shared UI component library
└── effect-drizzle-sqlite/ ← Effect-integrated SQLite/Drizzle layer
```

---

## packages/tinycode — Source Tree

All 38 source directories under `src/`:

```
src/
├── account/           ← Account management
├── agent/             ← Agent definitions (build, plan, explore, scout, general...)
├── auth/              ← API key + OAuth credential storage
├── background/        ← Background job queue (Effect ScopedCache)
├── bus/               ← Internal pub/sub event bus (Effect PubSub)
│   ├── bus-event.ts   ← Event type definitions
│   ├── global.ts      ← Global bus instance
│   └── index.ts
├── cli/               ← CLI entry points and TUI
│   ├── cmd/           ← Yargs command definitions
│   │   ├── tui/       ← SolidJS terminal UI (opentui)
│   │   ├── setup.ts   ← tinycode setup command
│   │   ├── run.ts     ← tinycode run command
│   │   ├── serve.ts   ← headless server mode
│   │   └── ...
│   ├── ui.ts          ← CLI UI helpers
│   └── logo.ts        ← ASCII logo
├── command/           ← Slash-command registry and loading
├── config/            ← Config loading, merging, migration, schema
│   ├── config.ts      ← Main config loader (merges JSON/JSONC files)
│   ├── agent.ts       ← Agent config schema + .md file loader
│   ├── permission.ts  ← Permission config schema
│   ├── mcp.ts         ← MCP server config schema
│   ├── migration.ts   ← ~/.config/opencode → ~/.config/tinycode migration
│   └── ...
├── control-plane/     ← Workspace context management
├── effect/            ← Effect runtime extensions
│   ├── instance-state.ts  ← Per-directory lazy state cache (ScopedCache)
│   ├── instance-registry.ts
│   ├── runtime-flags.ts   ← Feature flags via env vars
│   └── bridge.ts      ← Effect↔Promise bridge for non-Effect consumers
├── env/               ← Environment variable loading
├── file/              ← File protection and managed file tracking
├── format/            ← Code formatter integration
├── git/               ← Git utilities (branch, blame, diff)
├── id/                ← ID generation utilities
├── ide/               ← IDE integration (VS Code, JetBrains)
├── image/             ← Image/attachment handling
├── lsp/               ← Language Server Protocol client
│   ├── lsp.ts         ← LSP service (Effect layer)
│   └── server.ts      ← Per-language server definitions (30+ servers)
├── mcp/               ← Model Context Protocol client
├── patch/             ← Patch/diff application utilities
├── permission/        ← Tool permission evaluation (allow/ask/deny)
│   ├── evaluate.ts
│   ├── schema.ts
│   └── index.ts
├── plugin/            ← Plugin loading and lifecycle
│   ├── index.ts       ← Plugin service (Effect layer)
│   └── loader.ts      ← External plugin loader
├── project/           ← Project bootstrap and instance management
├── provider/          ← LLM provider abstraction
│   ├── provider.ts    ← Provider registry, model resolution, defaultModel()
│   ├── local-discovery.ts ← Ollama/vLLM/MaaS auto-discovery
│   ├── transform.ts   ← Provider-specific message/param transforms
│   ├── auth.ts        ← Provider auth methods (API key, OAuth)
│   └── schema.ts      ← ProviderID, ModelID types
├── pty/               ← PTY (pseudo-terminal) for shell tool
│   ├── pty.bun.ts     ← Bun implementation
│   └── pty.node.ts    ← Node fallback
├── question/          ← Interactive question/prompt tool
├── reference/         ← Repository clone cache for @scout agent
├── server/            ← HTTP API server
│   ├── server.ts      ← Effect HTTP server (port 4096)
│   └── routes/        ← REST + SSE/WebSocket route handlers
│       └── instance/httpapi/groups/
│           ├── session.ts, provider.ts, agent.ts, config.ts, mcp.ts...
├── session/           ← AI conversation sessions
│   ├── processor.ts   ← Session processor loop (LLM calls + tool dispatch)
│   ├── session.ts     ← Session CRUD and state
│   ├── message-v2.ts  ← Message/part schema and assembly
│   ├── compaction.ts  ← Context window compaction
│   ├── system.ts      ← System prompt assembly
│   ├── instruction.ts ← Instruction file loading (AGENTS.md, CLAUDE.md)
│   ├── llm.ts         ← LLM dispatch (AI SDK path + native path)
│   └── llm/
│       ├── ai-sdk.ts  ← AI SDK stream event adapter
│       └── native-runtime.ts ← Native LLM client path
├── shell/             ← Shell command parsing utilities
├── skill/             ← Skill discovery, loading, system prompt injection
├── snapshot/          ← Git-based file snapshot for session revert
├── storage/           ← SQLite persistence (THE persistence layer)
│   ├── db.ts          ← Database service (Effect layer)
│   ├── db.bun.ts      ← Bun SQLite implementation
│   ├── db.node.ts     ← Node better-sqlite3 fallback
│   ├── schema.sql.ts  ← Drizzle schema (sessions, messages, parts, auth...)
│   └── storage.ts     ← Storage service operations
├── sync/              ← Legacy event sync system (NOT the persistence layer)
├── tool/              ← Agent tool implementations
│   ├── registry.ts    ← Central tool registry
│   ├── shell.ts       ← Bash tool (with guardrails)
│   ├── read.ts, write.ts, edit.ts, glob.ts, grep.ts
│   ├── lsp.ts, webfetch.ts, websearch.ts
│   ├── task.ts        ← Subagent spawning tool
│   ├── plan.ts, skill.ts, truncate.ts
│   └── shell/         ← Shell command tree-sitter parsing
├── util/              ← Shared utilities (error, record, media, etc.)
├── v2/                ← V2 API surface
├── worktree/          ← Worktree/workspace detection and management
└── index.ts           ← CLI entry point (Yargs command registration)
```

---

## Key Design Patterns

### Effect Framework

All server-side code uses Effect v4 for dependency injection, typed errors, and concurrency.

```typescript
// Service definition
export class Service extends Context.Service<Service, Interface>()("@opencode/Agent") {}

// Implementation via Layer
export const layer = Layer.effect(Service, Effect.gen(function* () {
  const config = yield* Config.Service
  // ...
  return Service.of({ /* interface impl */ })
}))
```

> **Important**: All service Context tags use `@opencode/` prefix (not `@tinycode/`). This is legacy naming from the upstream fork — it is not a bug.

### Per-directory Instance State

`InstanceState` (backed by Effect's `ScopedCache`) ensures each service initializes once per project directory and is disposed when the project closes:

```typescript
const state = yield* InstanceState.make<State>(
  Effect.fn("Agent.state")(function* (ctx) {
    // ctx.worktree = absolute path to project
    // runs once per directory, cached thereafter
  })
)
```

### Self-Reexport Namespace Pattern

Modules use `export * as Foo from "./foo"` for namespaced access without TypeScript `namespace` keyword:

```typescript
// src/config/agent.ts
export * as ConfigAgent from "./agent"
```

One exception: `src/plugin/loader.ts` uses `export namespace PluginLoader` for its type grouping.

### Provider Model Resolution

`Provider.defaultModel()` resolves the active model in priority order:
1. CLI `--model` flag
2. Agent-level model config
3. Session-level model (SQLite per-session)
4. `cfg.model` in `~/.config/tinycode/config.json`
5. `model.json` recent selections list
6. First available configured provider

### Local Provider Auto-Discovery

`LocalDiscovery` probes at startup and every 30 seconds:
- **Ollama**: `http://localhost:11434/api/tags` (or `TINYCODE_OLLAMA_HOST`)
- **vLLM**: `http://localhost:8000/v1/models` (or `TINYCODE_VLLM_HOST`)
- **MaaS**: `TINYCODE_MAAS_HOST/v1/models` + `TINYCODE_MAAS_API_KEY`

### Permission System

Every tool call goes through `Permission.evaluate()` before execution:

```
permission.{tool}: "allow" | "ask" | "deny"
```

Special keys: `doom_loop` (repeated identical calls), `guardrail` (destructive commands), `external_directory`, `read` (with glob patterns).

---

## Config Loading

Config merges in order (later overrides earlier):

```
~/.config/tinycode/config.json
~/.config/tinycode/opencode.json  (compat)
~/.config/tinycode/opencode.jsonc (compat)
~/.config/tinycode/tinycode.json
~/.config/tinycode/tinycode.jsonc
{project}/.opencode/tinycode.json
{project}/.opencode/tinycode.jsonc
{project}/.opencode/opencode.json
{project}/.opencode/opencode.jsonc
```

Global agents: `~/.config/tinycode/agent/*.md`  
Global skills: `~/.config/tinycode/skills/` (via `skills.paths` config)

---

## Data Persistence

SQLite database at `~/.local/share/tinycode/opencode-{mode}.db` (legacy `opencode` name from upstream).

Schema defined in `src/storage/schema.sql.ts` via Drizzle ORM. Migrations in `migration/` (managed by Drizzle Kit).

Dual runtime:
- **Bun**: `db.bun.ts` (uses Bun's native SQLite)
- **Node**: `db.node.ts` (uses better-sqlite3)

Selected via `#db` conditional import alias in `bunfig.toml`.

---

## Session Processor Loop

```
User prompt
  → SessionPrompt (permission check, message assembly)
  → LLM.stream() → AI SDK or native runtime
  → Stream events: text-delta, reasoning-delta, tool-input-start/delta/end
  → Tool execution (permission gate → run → result)
  → Next LLM turn (with tool results)
  → Repeat until: finish | doom_loop | abort | compaction needed
  → Bus publishes events → TUI/web clients via SSE
```

---

## Agent System

Native agents (hardcoded in `src/agent/agent.ts`):
- `build` — default, full tool access
- `plan` — edit tools disabled
- `explore` — read-only (grep/glob/read/bash)
- `scout` — external research (repo clone allowed)
- `general` — parallel multi-task subagent
- `compaction`, `title`, `summary` — hidden system agents

Custom agents loaded from:
- `~/.config/tinycode/agent/*.md` (global)
- `{project}/.opencode/agent/*.md` (project-local)

Frontmatter fields: `mode`, `steps`, `description`, `permission`, `model`, `temperature`, `color`

---

## oh-my-tiny Integration

oh-my-tiny is a companion MCP server providing extended tools:

| Tool category | Tools |
|---|---|
| State | `state_read/write/clear/list/status` |
| Notepad | `notepad_read/write_priority/write_working/write_manual/prune/stats` |
| Project memory | `project_memory_read/write/add_note/add_directive` |
| Wiki | `wiki_list/read/query/add/ingest/delete` |
| LSP | `lsp_goto_definition/find_references/hover/diagnostics/document_symbols/workspace_symbols` |
| AST | `ast_grep_search/ast_grep_replace` |

Configured in `~/.config/tinycode/config.json`:
```json
{
  "mcp": {
    "oh-my-tiny": {
      "type": "local",
      "command": ["node", "~/.config/tinycode/mcp/node_modules/oh-my-tiny/dist/mcp/server.js"]
    }
  }
}
```

Run `tinycode setup` to install.
