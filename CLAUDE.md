# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

> **Note:** End users install tinycode via `curl -fsSL https://raw.githubusercontent.com/bobbyjohnstx/tinycode/dev/install.sh | sh` or `npx tinycode-ai`. The `bun install` path below is for **development** only.

```bash
# Install dependencies (from repo root, development only)
bun install

# Run in development (TUI mode)
bun dev
bun dev <directory>     # run against a different directory
bun dev .               # run against repo root

# Other dev modes
bun dev serve           # headless API server (port 4096)
bun dev web             # server + open web interface
bun dev acp             # Agent Client Protocol mode (IDE integration, stdio transport)
bun run --cwd packages/app dev        # web UI only (requires server running)
bun run --cwd packages/desktop dev    # Electron desktop app

# Lint
bun run lint            # oxlint across all packages

# Type check (run from a package directory, not root)
bun typecheck           # from e.g. packages/tinycode

# Tests (run from a package directory, never from root)
bun test                # from e.g. packages/tinycode
bun test --timeout 30000 path/to/file.test.ts  # single test file

# Build standalone binary
./packages/tinycode/script/build.ts --single

# Regenerate SDK after API changes (server.ts changes → run this)
./script/generate.ts

# Export session to JSON or HTML
tinycode export --format json <session-id>       # JSON format (default)
tinycode export --format html <session-id>       # Self-contained HTML file
```

> **Tests cannot run from repo root** (`do-not-run-tests-from-root` guard). Always `cd` into a package first.

## Testing

### Mock LLM Provider

For deterministic testing of session logic, use `MockLanguageModel` from `test/fake/mock-language-model.ts`. It accepts a sequence of scenarios that define responses for successive LLM calls:

```typescript
import { MockLanguageModel, type MockScenario } from "@/test/fake/mock-language-model"

const scenarios: MockScenario[] = [
  { type: "text", content: "First response" },
  { type: "text", content: "Second response" },
  {
    type: "tool-call",
    calls: [{ id: "call-1", name: "tool-name", args: { key: "value" } }],
  },
  { type: "error", error: new Error("Simulated failure") },
]

const model = new MockLanguageModel(scenarios)
```

Each call to `doGenerate` or `doStream` advances to the next scenario. Use `ProviderTest.fake({ scenarios: [...] })` to set up a provider with mock models for session processor tests.

### Slow Tests

Tests that take >30 seconds are marked `.skip` in the default suite. Run them explicitly before releases:

```bash
# Full suite including slow tests
bun test --timeout 120000

# Just the slow tests
bun test --timeout 120000 test/session/prompt.test.ts test/server/httpapi-listen.test.ts
```

## Architecture

This is a Bun monorepo with Turborepo. Key packages:

### `packages/tinycode` — Core server & CLI

The heart of the project. Contains the HTTP API server, all business logic, and the TUI.

- **Server** (`src/server/`): Effect-based HTTP server using Hono-style routing via `effect/unstable/http`. Runs on port 4096. Exposes REST + SSE/WebSocket for real-time events.
- **TUI** (`src/cli/cmd/tui/`): Terminal UI written in SolidJS on top of [opentui](https://github.com/sst/opentui). The TUI either spawns the server in a worker thread or attaches to an existing one.
- **Session** (`src/session/`): Manages AI conversation sessions. Each session runs a processor loop that calls LLMs and coordinates tools. **Context compaction** automatically summarizes old turns when context usage approaches the model limit. Advanced compaction features:
  - **Deterministic file tracking**: Scans tool calls for read/write/edit operations, appends `<read-files>` and `<modified-files>` XML blocks to summaries (not LLM-dependent)
  - **Observation masking**: Replaces old tool outputs with placeholders before summarization (config: `compaction.mask_observations`, default true)
  - **Text serialization**: Conversation serialized to tagged text format with "Do NOT continue" system prompt, preventing model continuation
  - **Circuit breaker**: Warning issued after 3+ compactions, suggesting new session or subagent approach
  - **Structured telemetry**: Logs pre/post token counts, model, timing, and compaction number for diagnostics
  - **Summary size cap**: Limited to `min(4096, 10% of usable context)` to avoid disproportionate overhead
  - **Increased preserve budget**: MIN 4K tokens, MAX 20K tokens (up from 2K-8K) to maintain recent context
- **Agent** (`src/agent/`): Built-in agent definitions. Two modes have distinct behavior: **build** (default, full tool access) and **plan** (read-only, hard permission enforcement — can only write to `.tinycode/plans/*.md`). All other agents (architect, debugger, executor, etc.) are personas that share build's permissions but have specialized system prompts. Agent defaults live in `src/agent/defaults/` with `.compact.md` variants for small models.
- **Tools** (`src/tool/`): Individual agent tools — file read/write/edit, shell, grep, glob, LSP, MCP websearch, etc.
- **Provider** (`src/provider/`): LLM provider abstraction (wraps Vercel AI SDK). Local LLM support via Ollama and OpenAI-compatible endpoints is the primary use case. Includes **model warmup** (`src/provider/warmup.ts`) that sends a tool-call probe to Ollama on startup to pre-load the model into GPU memory and verify tool-calling capability.
- **Config** (`src/config/`): User config parsing. Each config module self-exports (e.g., `export * as ConfigAgent from "./agent"`).
- **Storage** (`src/storage/`): SQLite via Drizzle ORM. DB schema in `schema.sql.ts`.
- **MCP** (`src/mcp/`): Model Context Protocol client integration.
- **ACP** (`src/acp/`): Agent Client Protocol implementation for IDE integration. Implements stdio-based agent communication for editor extensions and language servers.

### `packages/vscode-extension` — VS Code Extension

Reference VS Code extension demonstrating ACP integration. Provides editor context injection and agent command routing to tinycode running in ACP mode.

### `packages/app` — Web UI

SolidJS + TailwindCSS v4 web app. Connects to the tinycode API server. Used by both the browser experience and the desktop app.

### `packages/desktop` — Electron desktop app

Electron shell wrapping `packages/app`. Run with `bun run --cwd packages/desktop dev`.

**Key implementation files:**
- **Security** (`src/main/window.ts`): Content Security Policy headers, navigation origin validation, `setWindowOpenHandler` preventing uncontrolled new windows, and `shell.openExternal` URL scheme validation (http/https/mailto only)
- **System tray** (`src/main/tray.ts`): Cross-platform tray integration with Show Window and Quit context menu actions
- **Application menus** (`src/main/menu.ts`): Cross-platform menus for Windows/Linux/macOS with Help menu linking to GitHub (repo, discussions, issues). No longer macOS-only
- **Update notification** (`packages/app/src/components/update-notification-banner.tsx`): Non-blocking slide-in banner notifying about available updates from GitHub Releases. Includes i18n and ARIA accessibility
- **Global exception handling** (`src/main/index.ts`): Captures `uncaughtException` and `unhandledRejection` globally
- **Platform lifecycle**: Minimum window size 960x600, macOS dock icon restoration, theme change listener for OS dark/light mode sync, persistent zoom level and window state

### `packages/plugin` — Plugin SDK

Source for `@tinycode/plugin`. Provides the public plugin API.

### `packages/sdk/js` — JavaScript SDK

Auto-generated from the OpenAPI spec. Regenerate with `./packages/sdk/js/script/build.ts`.

## Key Patterns

- **Effect framework**: Server-side code uses the [Effect](https://effect.website) library extensively for typed errors, dependency injection via `Context.Service`, and resource management via `Layer`/`Scope`.
- **Event bus** (`src/bus/`): Internal pub/sub (Effect PubSub) used to stream events from the session processor to the TUI and web clients via SSE.
- **Dual runtime**: `src/storage/db.ts` and `src/pty/` use conditional imports (`#db`, `#pty`) to swap Bun vs Node implementations.
- **`bun dev` = `tinycode`**: During development, `bun dev` from the repo root is equivalent to the `tinycode` CLI.
- **Local LLMs first**: The primary use case is local LLM inference via Ollama, ramalama, or any OpenAI-compatible endpoint. Configure via `~/.config/tinycode/config.json`. ramalama support is enabled via `TINYCODE_RAMALAMA_HOST` env var (e.g., `http://localhost:8080`).
- **OpenRouter provider**: Full-fledged cloud provider with auto-discovery. Set `OPENROUTER_API_KEY` and tinycode discovers available models via the OpenRouter API, filtering to tool-capable models with pricing/capability mapping. Discovery runs in `src/provider/local-discovery.ts` alongside local provider polling. Uses `@openrouter/ai-sdk-provider` SDK. Cost tracking via OpenRouter's generation cost API (`src/session/llm.ts`).
- **Provider filtering**: `enabled_providers` and `disabled_providers` in config apply to **all** providers — both custom API-configured providers and locally-discovered ones (Ollama, vLLM, ramalama, MaaS, OpenRouter). Filters apply during discovery, so disabled providers are completely hidden from the provider list.
- **oh-my-tiny**: Built-in plugin at `src/omt/` providing extended orchestration tools (notepad, wiki, state management, AST grep).
- **ACP mode**: Run with `bun dev acp` or `tinycode acp` to enable IDE integration via the Agent Client Protocol. Communicates via stdio with editor extensions. See `docs/acp-integration.md` for developer guide.
- **Session tree sidebar** (`src/cli/cmd/tui/component/session-tree.tsx`): Toggleable ASCII tree showing session hierarchy. Press `<leader>b` in the TUI to toggle visibility. Sessions are organized by parent-child relationships.
- **Plugin lifecycle hooks** (`@tinycode/plugin`): Four observe-only hooks fire during session lifecycle:
  - `session.start` — when a session is created
  - `session.end` — when a session is deleted
  - `session.switch` — when user switches to a different session
  - `session.model.change` — when the model is changed for a session
- **Style guide**: See [AGENTS.md](./AGENTS.md) for coding style rules (destructuring, control flow, Drizzle schema conventions, etc.).
- **Pass model on Task calls**: Use the model configured for the session, not hardcoded model names.
- **Tool-call failure handling**: Models with `capabilities.toolcall=false` get no tools injected (`src/session/llm/request.ts`). Malformed tool-call JSON is auto-repaired by stripping markdown fences and trailing commas (`src/session/llm.ts`). After 3+ consecutive tool-call failures, a warning toast suggests switching to a larger model (`src/session/processor.ts`).
- **Model warmup on startup**: For local providers (Ollama, ramalama, vLLM), tinycode sends a tool-call probe on startup (`src/provider/warmup.ts`) via `/v1/chat/completions` to pre-load the model into GPU memory and verify tool-calling support. Uses `keep_alive: "30m"`.
- **Per-agent tool permissions**: Each agent's `.md` frontmatter declares a `permission:` block that scopes which tools are injected into LLM calls. Read-only agents (architect, critic, etc.) get only read/glob/grep/bash (~1,800 tokens), write agents add edit (~2,700 tokens). This reduces prompt processing from ~38s to ~4-8s on local 9B models. The `explore` agent in `agent.ts` was the first to use this pattern.
- **Skill parameter substitution**: Skills can declare `params:` in SKILL.md frontmatter (e.g., `params: [name, language]`). Parameters map to `$1`, `$2` placeholders in the skill content, substituted by `SessionPrompt.command()`.
- **Plugin marketplace**: Curated plugin registry at `src/plugin/registry.json` with search via `tinycode plugin-search [query]`. Plugin install resolves registry names to npm packages automatically.
- **Graceful shutdown**: `tinycode serve` handles SIGTERM/SIGINT — disposes all instances, emits global disposed event for SSE clients, then drains HTTP connections with 25s timeout.
- **Frecency ranking**: Command palette sorts by usage frequency via namespaced frecency store (`src/cli/cmd/tui/component/prompt/frecency.tsx`). File autocomplete also uses frecency. The unified palette (`ctrl+p`) searches across commands, agents, sessions, and skills — all categories use frecency.
- **Subagent output inline**: The `Task` TUI component renders subagent results as a collapsible block below the task header. Extracted from `<task_result>` XML in `props.output` using the `BlockTool`/`collapseToolOutput` pattern.
- **tc-doctor skill** (`src/skill/defaults/tc-doctor/`): Self-diagnostic skill with 14+ checks covering Ollama install/health, model availability, tool-call capability probe, ramalama provider, Mac-specific checks (Metal, Rosetta, RAM pressure), tinycode↔Ollama integration, vLLM provider health, and disk space. All checks use pure bash — no python3 dependency. Run with `/tc-doctor`.
