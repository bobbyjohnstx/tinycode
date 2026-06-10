# Architecture: Current State

> This document describes the architecture of the tinycode project as it exists today. It is intended for engineers who need to understand the system before undertaking a refactor. All file paths are relative to the repository root unless otherwise noted.

---

## Executive Summary

Tinycode is a fork of opencode — a Bun monorepo managed by Turborepo. Its core value is a local AI coding assistant that exposes an HTTP API server, a terminal UI (TUI), and a web UI, all wired to a session engine that orchestrates LLM calls and agent tools. The codebase carries significant upstream coupling to the tinycode.dev cloud platform: account auth, model catalog fetching, a paid-model gate, web UI proxying, and a private terminal widget dependency all reach out to external services at runtime. Understanding and isolating these dependencies is essential before any refactor aimed at self-contained or simplified deployment.

---

## Monorepo Structure

The repository uses Bun workspaces with Turborepo for task orchestration. All packages live under `packages/`.

| Package | Purpose |
|---|---|
| `tinycode` | Core server, CLI, TUI, session engine, provider abstraction, and agent tools. Renamed from `opencode` in this fork. |
| `core` | Shared utilities: account definitions, agent definitions, model catalog, provider plugins, git utilities, npm config, filesystem helpers. Imported as `@tinycode/core/...`. |
| `app` | SolidJS + TailwindCSS v4 web UI. Connects to the API server. Uses the private `ghostty-web` terminal widget from `github:bobbyjohnstx/ghostty-web#main`. |
| `desktop` | Electron wrapper around `packages/app`. |
| `llm` | Low-level LLM protocol implementations: Anthropic messages, OpenAI chat/responses, Bedrock converse, Gemini. |
| `ui` | Shared SolidJS UI component library. |
| `plugin` | Public plugin API SDK (`@tinycode/plugin`). |
| `effect-drizzle-sqlite` | Effect wrapper for Drizzle ORM + SQLite. |
| `enterprise` | Enterprise web app (Cloudflare Workers, Nitro SSR, org management). |
| `console` | Admin/console web app for tinycode.dev. |
| `stats` | Stats dashboard app. |
| `web` | Astro documentation site (tinycode.dev public docs). |
| `docs` | OpenAPI spec and docs content. |
| `sdk/js` | Auto-generated JavaScript SDK from the OpenAPI spec. Regenerate with `./packages/sdk/js/script/build.ts`. |
| `function` | Cloudflare Workers functions (GitHub app auth, webhooks). |
| `identity` | Identity/auth server components. |
| `containers` | Docker container definitions for cloud deployment. |
| `slack` | Slack bot integration. |
| `http-recorder` | HTTP request/response recording utilities for tests. |
| `script` | Build, release, and CI scripts. |
| `storybook` | Storybook for `packages/ui` components. |
| `extensions` | VS Code extension source. |

---

## Core Data Flow

The following describes what happens from CLI invocation through to an LLM response being rendered in the TUI or web client.

### 1. Entry Point

`packages/tinycode/src/index.ts` — Yargs-based CLI. Parses subcommands (`tui`, `serve`, `web`, `run`, etc.). The default command launches the TUI. During development, `bun dev` runs with `--conditions=browser`.

### 2. TUI

`packages/tinycode/src/cli/cmd/tui/app.tsx` — SolidJS component tree running on [opentui](https://github.com/sst/opentui). On startup it:

- Configures the SDK client pointed at the local API server.
- Syncs providers and loads the project context.
- Either spawns the HTTP server in a worker thread (standalone mode) or attaches to an already-running server instance.

### 3. HTTP Server

`packages/tinycode/src/server/server.ts` — Effect-based HTTP server using Hono-style routing via `effect/unstable/http`. Default port is `4096` with automatic fallback to another available port. Features:

- REST endpoints and SSE/WebSocket for real-time event streaming.
- Optional mDNS advertisement via `bonjour-service` (see `src/server/mdns.ts`).
- Optional basic auth via `OPENCODE_SERVER_PASSWORD` / `OPENCODE_SERVER_USERNAME` environment variables.

### 4. Session

`packages/tinycode/src/session/session.ts` — A session represents one conversation. Persisted in SQLite via Drizzle ORM. Tracks: project, workspace, messages, message parts, model info, and token usage/cost.

### 5. Processor

`packages/tinycode/src/session/processor.ts` — Orchestrates the prompt/response loop for a session:

1. Resolves the agent for the current turn.
2. Gathers system prompts from agent, skills, and config.
3. Invokes the LLM service.
4. Dispatches tool calls to the tool registry.
5. Manages context compaction when the context window approaches its limit.
6. Emits events onto the bus for streaming to clients.

### 6. LLM Service

`packages/tinycode/src/session/llm.ts` — Wraps the Vercel AI SDK `streamText`. Responsibilities:

- Resolves the model via `Provider.Service`.
- Applies model-specific transformations via `ProviderTransform`.
- Enforces tool permission checks.
- Returns a `Stream<LLMEvent>` consumed by the processor.

### 7. Provider Resolution

`packages/tinycode/src/provider/provider.ts` — At startup and every 5 minutes, fetches the model catalog from `https://models.dev/api.json`. Loads the bundled provider plugins from `packages/core/src/plugin/provider/index.ts` (32 providers). API keys are resolved in this precedence order:

1. Environment variables
2. `auth.json` (XDG data home)
3. User config file
4. OAuth tokens obtained via account login

### 8. Provider Transform

`packages/tinycode/src/provider/transform.ts` — A 47 KB file of model-specific request/response transformation. Handles:

- Per-model output token limits
- Prompt caching header injection
- Reasoning/thinking mode configuration
- Tool call formatting differences across providers

### 9. Tools

`packages/tinycode/src/tool/` — 44 tool files registered in `src/tool/registry.ts`. Core tools:

| Tool | File |
|---|---|
| File read | `read.ts` |
| File write | `write.ts` |
| File edit | `edit.ts` |
| Shell execution | `shell.ts` |
| Grep search | `grep.ts` |
| Glob search | `glob.ts` |
| LSP integration | `lsp.ts` |
| Subagent task | `task.ts` |
| Planning | `plan.ts` |
| User question | `question.ts` |
| Web fetch | `webfetch.ts` |
| Web search | `websearch.ts` |
| MCP web search | `mcp-websearch.ts` |
| Patch application | `apply_patch.ts` |
| Repo overview | `repo_overview.ts` |
| Repo clone | `repo_clone.ts` |
| Todo tracking | `todo.ts` |
| Skill invocation | `skill.ts` |

### 10. Event Bus

`packages/tinycode/src/bus/` — Internal pub/sub system. Session processor emits events (message parts, tool results, completion signals) onto the bus. The server subscribes and forwards events to connected TUI workers and web clients over SSE or WebSocket.

---

## Directory Layout of `packages/tinycode/src/`

```
src/
├── index.ts              # CLI entry point
├── account/              # Account model and OAuth logic
├── acp/                  # Agent Communication Protocol (current)
├── acp-next/             # ACP next-gen (in progress)
├── agent/                # Agent definitions (build, plan, general)
├── auth/                 # auth.json credential store
├── background/           # Background task management
├── bus/                  # Internal pub/sub event bus
├── cli/                  # CLI command implementations
├── command/              # Command registry
├── config/               # Config loading and schema
├── control-plane/        # Control plane client
├── effect/               # Effect utility helpers
├── env/                  # Environment variable handling
├── file/                 # File utilities
├── format/               # Output formatting
├── git/                  # Git utilities
├── id/                   # ID generation
├── ide/                  # IDE integration (VS Code, etc.)
├── image/                # Image handling
├── installation/         # Self-upgrade logic
├── lsp/                  # LSP client
├── mcp/                  # MCP client integration
├── patch/                # Patch utilities
├── permission/           # Tool permission system
├── plugin/               # Plugin runtime
├── project/              # Project context detection
├── provider/             # LLM provider abstraction + transforms
├── pty/                  # PTY (pseudo-terminal) handling
├── question/             # Interactive question prompts
├── reference/            # Reference/context management
├── server/               # HTTP server and route handlers
├── session/              # Session engine (processor, llm, storage)
├── share/                # Session sharing system
├── shell/                # Shell utilities
├── skill/                # Skill loading and invocation
├── snapshot/             # Snapshot system
├── storage/              # SQLite/Drizzle DB layer
├── sync/                 # Multi-device sync
├── tool/                 # Agent tool implementations (44 files)
├── util/                 # General utilities
├── v2/                   # V2 protocol compatibility
└── worktree/             # Git worktree management
```

---

## External Service Dependencies

All network calls made at runtime to services outside the local machine:

| Service | URL | Source Location | Purpose |
|---|---|---|---|
| models.dev | `https://models.dev/api.json` | `core/src/models-dev.ts:140` | Remote model catalog; fetched every 5 minutes |
| tinycode.dev console | `https://console.tinycode.dev` | `tinycode/src/cli/cmd/account.ts:18` | Account login, org management |
| tinycode.dev API | `https://api.tinycode.dev` | `tinycode/src/cli/cmd/github.ts:363,745` | GitHub app auth, API gateway |
| tinycode.dev app | `https://app.tinycode.dev` | `tinycode/src/server/shared/ui.ts:9` | Upstream web UI proxy fallback |
| tinycode.dev install | `https://tinycode.dev/install` | `tinycode/src/installation/index.ts:154` | Self-upgrade binary download |
| tinycode.dev Go | `https://tinycode.dev/go` | `tinycode/src/session/retry.ts:10` | Paid tier upsell on rate-limit errors |
| tinycode.dev auth | `https://tinycode.dev/auth` | `tinycode/src/cli/cmd/providers.ts:464` | API key creation flow |
| tinycode.dev config schema | `https://tinycode.dev/config.json`, `https://tinycode.dev/tui.json` | `tinycode/src/config/config.ts:429-579` | JSON schema references for config validation |
| bobbyjohnstx GitHub releases | `bobbyjohnstx/tinycode`, `bobbyjohnstx/homebrew-tap` | `tinycode/src/installation/index.ts:264,290` | Homebrew tap, GitHub releases for self-upgrade |
| OpenTelemetry collector | OTLP HTTP (configurable) | `tinycode/src/cli/cmd/run/otel.ts` | Optional distributed tracing |
| Sentry | Sentry DSN (via `@sentry/solid`) | `packages/app/src/app.tsx` | Error reporting from the web UI |
| ghostty-web | `github:bobbyjohnstx/ghostty-web#main` | `packages/app/package.json` | Terminal emulator widget (private repo) |

---

## Account and Auth System

### OAuth Device Code Flow

Implemented in `packages/tinycode/src/account/account.ts`:

1. `POST ${server}/auth/device/code` with `client_id: "opencode-cli"` — initiates device code flow.
2. Poll `POST ${server}/auth/device/token` until token is granted.
3. Fetch user info, organization memberships, and remote config from the account server.
4. Persist all results in SQLite via `AccountRepo`.

### Credential Store

`packages/tinycode/src/auth/index.ts` — Stores credentials in `$XDG_DATA_HOME/auth.json`. Three credential types are supported:

| Type | Description |
|---|---|
| `OAuth` | Access/refresh token pair from the device code flow |
| `APIKey` | Manually configured API key for a provider |
| `WellKnown` | Provider-specific well-known credential (e.g., env-based) |

---

## Configuration System

Config is loaded and merged from multiple sources in precedence order:

1. `~/.config/opencode/config.json` — global user config (XDG-based path)
2. `.opencode/config.json` or `opencode.json` — project-level config in the working directory
3. Remote config fetched via `GET ${server}/api/config` — requires account login
4. Well-known config per provider (injected at provider resolution time)

### Key Config Fields

| Field | Type | Description |
|---|---|---|
| `shell` | string | Shell to use for command execution |
| `logLevel` | string | Logging verbosity |
| `server` | object | Server host/port overrides |
| `model` | string | Default model in `provider/model` format |
| `small_model` | string | Model for lighter tasks |
| `provider` | record | Per-provider config: `api`, `options.apiKey`, `options.baseURL` |
| `mcp` | object | MCP server configuration |
| `agent` | object | Agent behavior overrides |
| `skills` | array | Skill definitions |
| `permission` | object | Tool permission policy |
| `tools` | object | Tool enable/disable overrides |
| `share` | object | Session sharing configuration |
| `enterprise` | object | Enterprise feature flags |

Config is implemented in `packages/tinycode/src/config/config.ts`. Each config module self-exports (e.g., `export * as ConfigAgent from "./agent"`).

---

## Cloud Infrastructure

`sst.config.ts` at the repo root defines the full cloud stack via SST. This is not required for local development but represents the full production deployment:

- **Cloudflare** — edge workers, DNS, CDN
- **AWS us-east-1** — Lambda, S3, DynamoDB, and related services
- **Stripe** — billing integration
- **PlanetScale** — managed MySQL
- **Honeycomb** — observability and tracing

Infrastructure modules: `infra/app.ts`, `infra/lake.ts`, `infra/stats.ts`, `infra/console.ts`, `infra/enterprise.ts`, `infra/monitoring.ts`.

---

## Barriers to Simple Deployment

The following ten issues make it non-trivial to run tinycode in a fully self-contained or simplified environment. Each represents a coupling point to the upstream tinycode.dev platform or an external service.

### 1. SST Cloud Infrastructure

`sst.config.ts` ties the project to a multi-cloud stack (Cloudflare, AWS, Stripe, PlanetScale, Honeycomb). Standing this up requires accounts, credentials, and configuration across all five services. There is no official single-host or Docker Compose deployment path.

### 2. Account System Dependency

Multiple code paths in `packages/tinycode/src/account/` and the CLI commands check for an active account and organization. These checks gate model availability, surface upsell messages, and pull remote config. Removing this coupling requires auditing every call site that reads `AccountRepo` or calls the account server.

### 3. models.dev Remote Catalog

`packages/core/src/models-dev.ts` fetches `https://models.dev/api.json` every 5 minutes. If this request fails or is unavailable, no models are discoverable by default. Two environment variables can override this behavior: `OPENCODE_MODELS_URL` (alternative URL) and `OPENCODE_MODELS_PATH` (local file path). These escape hatches exist but are not prominently documented.

### 4. opencode Provider Plugin (Paid Model Gate)

`packages/core/src/plugin/provider/opencode.ts` implements a provider plugin that gates access to paid/managed models behind account authentication. Without a valid account token, these models are unavailable regardless of other configuration.

### 5. Web UI Upstream Proxy

When the embedded web UI build is absent, `packages/tinycode/src/server/shared/ui.ts` proxies requests to `https://app.tinycode.dev`. This means a server started without a built `packages/app` will silently serve the upstream production UI rather than a local build, which can cause version mismatches.

### 6. ghostty-web Private Dependency

`packages/app/package.json` declares `"ghostty-web": "github:bobbyjohnstx/ghostty-web#main"`. This is a private repository under the `bobbyjohnstx` GitHub organization. Installing dependencies without access to this repo will fail. Any fork or CI environment needs either access credentials or a replacement for this terminal widget.

### 7. 32 Bundled Provider Plugins

`packages/core/src/plugin/provider/index.ts` bundles 32 provider plugins. Each plugin may carry its own initialization logic, API surface assumptions, and dependencies. This makes the provider layer large and difficult to audit for what is strictly required for a minimal deployment.

### 8. Sentry Error Reporting

`packages/app/src/app.tsx` initializes Sentry via `@sentry/solid`. In production builds, crash reports are sent to Anthropic/opencode's Sentry project. This is a data exfiltration concern for forks and must be explicitly disabled or reconfigured.

### 9. Share System

`packages/tinycode/src/share/share-next.ts` uploads session data to the tinycode.dev remote server using an account access token. This is an opt-in feature per config, but the code path is present and active in the codebase.

### 10. Multi-Device Sync System

`packages/tinycode/src/sync/` implements event replay and synchronization across devices via the tinycode.dev backend. Like the share system, this creates a runtime dependency on the cloud platform that must be stubbed or removed for a fully isolated deployment.
