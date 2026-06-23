# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Install dependencies (from repo root)
bun install

# Run in development (TUI mode)
bun dev
bun dev <directory>     # run against a different directory
bun dev .               # run against repo root

# Other dev modes
bun dev serve           # headless API server (port 4096)
bun dev web             # server + open web interface
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
```

> **Tests cannot run from repo root** (`do-not-run-tests-from-root` guard). Always `cd` into a package first.

## Architecture

This is a Bun monorepo with Turborepo. Key packages:

### `packages/tinycode` — Core server & CLI

The heart of the project. Contains the HTTP API server, all business logic, and the TUI.

- **Server** (`src/server/`): Effect-based HTTP server using Hono-style routing via `effect/unstable/http`. Runs on port 4096. Exposes REST + SSE/WebSocket for real-time events.
- **TUI** (`src/cli/cmd/tui/`): Terminal UI written in SolidJS on top of [opentui](https://github.com/sst/opentui). The TUI either spawns the server in a worker thread or attaches to an existing one.
- **Session** (`src/session/`): Manages AI conversation sessions. Each session runs a processor loop that calls LLMs and coordinates tools.
- **Agent** (`src/agent/`): Agent definitions (build, plan, general subagent). Agents configure which tools are available and system prompts.
- **Tools** (`src/tool/`): Individual agent tools — file read/write/edit, shell, grep, glob, LSP, MCP websearch, etc.
- **Provider** (`src/provider/`): LLM provider abstraction (wraps Vercel AI SDK). Local LLM support via Ollama and OpenAI-compatible endpoints is the primary use case.
- **Config** (`src/config/`): User config parsing. Each config module self-exports (e.g., `export * as ConfigAgent from "./agent"`).
- **Storage** (`src/storage/`): SQLite via Drizzle ORM. DB schema in `schema.sql.ts`.
- **MCP** (`src/mcp/`): Model Context Protocol client integration.

### `packages/app` — Web UI

SolidJS + TailwindCSS v4 web app. Connects to the tinycode API server. Used by both the browser experience and the desktop app.

### `packages/desktop` — Electron desktop app

Electron shell wrapping `packages/app`. Run with `bun run --cwd packages/desktop dev`.

### `packages/plugin` — Plugin SDK

Source for `@tinycode/plugin`. Provides the public plugin API.

### `packages/sdk/js` — JavaScript SDK

Auto-generated from the OpenAPI spec. Regenerate with `./packages/sdk/js/script/build.ts`.

## Key Patterns

- **Effect framework**: Server-side code uses the [Effect](https://effect.website) library extensively for typed errors, dependency injection via `Context.Service`, and resource management via `Layer`/`Scope`.
- **Event bus** (`src/bus/`): Internal pub/sub (Effect PubSub) used to stream events from the session processor to the TUI and web clients via SSE.
- **Dual runtime**: `src/storage/db.ts` and `src/pty/` use conditional imports (`#db`, `#pty`) to swap Bun vs Node implementations.
- **`bun dev` = `tinycode`**: During development, `bun dev` from the repo root is equivalent to the `tinycode` CLI.
- **Local LLMs first**: The primary use case is local LLM inference via Ollama or any OpenAI-compatible endpoint. Configure via `~/.config/tinycode/config.json`.
- **oh-my-tiny**: Companion tool at `../oh-my-tiny/` for extended agent orchestration capabilities.
- **Style guide**: See [AGENTS.md](./AGENTS.md) for coding style rules (destructuring, control flow, Drizzle schema conventions, etc.).
- **Pass model on Task calls**: Use the model configured for the session, not hardcoded model names.
