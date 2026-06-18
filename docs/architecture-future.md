# tinycode: Future Architecture

## Executive Summary

tinycode is a refactored fork of tinycode focused on one goal: a local-LLM-first AI coding assistant that runs entirely without cloud dependencies. The refactor strips every piece of tinycode's cloud infrastructure — account systems, telemetry, enterprise auth, cloud sync, remote model discovery — and replaces it with a lean, self-contained tool that works air-gapped on a developer's machine or LAN.

The primary inference target is Ollama and vLLM running on localhost or LAN. OpenAI, Anthropic, and Google remain available as cloud fallbacks, accessed via API key only — no proprietary auth flows, no OAuth, no traffic to `tinycode.dev`, `bobbyjohnstx`, or any external service by default.

The result is a single binary (or `bun run`) that a developer can drop onto a machine, point at a running Ollama instance, and use immediately.

**Companion tool:** `oh-my-tiny` (at `../oh-my-tiny/`) handles extended agent orchestration and replaces the cloud-based ACP/ACP-next system.

---

## What Changes

At a high level, three categories of change drive this refactor:

| Category | What happens |
|----------|-------------|
| **Cloud modules removed** | `enterprise`, `console`, `stats`, `function`, `identity`, `containers`, `slack`, `web`, `docs`, VS Code extension, SST infra, Nix environment |
| **Cloud code gutted from kept modules** | Account system, control plane, session sharing, auto-update, GitHub integrations, remote schema/config fetching, cloud provider auth, Sentry telemetry |
| **Local-first features added** | Auto-discovery of local Ollama/vLLM instances, bundled model catalog (no network on startup), simplified API-key-only auth |

The Effect framework, SQLite storage, TUI, HTTP server, web UI (`packages/app`), plugin system, and all core agent/tool logic are preserved unchanged.

---

## Package Decisions

| Package | Decision | Rationale |
|---------|----------|-----------|
| `packages/tinycode` | KEEP — gut cloud modules | Core server, TUI, session engine, tools |
| `packages/core` | KEEP — gut cloud modules | Shared utilities, simplified catalog |
| `packages/app` | KEEP — replace ghostty-web, remove Sentry | Web UI |
| `packages/desktop` | KEEP (optional) | Electron shell |
| `packages/llm` | KEEP | LLM protocol layer |
| `packages/ui` | KEEP | Shared UI components |
| `packages/plugin` | KEEP | Plugin API |
| `packages/effect-drizzle-sqlite` | KEEP | SQLite integration |
| `packages/sdk/js` | KEEP | Client SDK for web UI |
| `packages/http-recorder` | KEEP (dev only) | HTTP testing |
| `packages/script` | GUT — keep build scripts, remove deploy/release | CI tooling |
| `packages/enterprise` | REMOVE | Cloud enterprise features |
| `packages/console` | REMOVE | Cloud admin dashboard |
| `packages/stats` | REMOVE | Cloud analytics |
| `packages/web` | REMOVE | Documentation website |
| `packages/docs` | REMOVE | API docs |
| `packages/function` | REMOVE | Cloudflare Workers |
| `packages/identity` | REMOVE | Cloud identity service |
| `packages/containers` | REMOVE | Cloud container infrastructure |
| `packages/slack` | REMOVE | Slack bot |
| `packages/storybook` | REMOVE | UI dev tooling |
| `packages/extensions` | REMOVE | VS Code extension |

---

## `packages/tinycode/src/` — Module Changes

### Remove Entirely

| Path | Reason |
|------|--------|
| `src/account/` | Full OAuth account system — replace with no-op stub |
| `src/control-plane/` | Cloud workspace sync — replace with local-only directory tracking |
| `src/share/` | Session sharing to remote servers |
| `src/sync/` | Multi-device event sync |
| `src/cli/cmd/account.ts` | Console login command |
| `src/cli/cmd/github.ts` | GitHub App integration |
| `src/cli/cmd/stats.ts` | Stats dashboard command |
| `src/cli/cmd/upgrade.ts` | Self-upgrade from `tinycode.dev` |
| `src/cli/cmd/pr.ts` | GitHub PR integration |
| `src/installation/` | Auto-update from `tinycode.dev`/`bobbyjohnstx` |
| `src/acp/` | Cloud-based agent orchestration (replaced by `oh-my-tiny`) |
| `src/acp-next/` | Cloud-based agent orchestration (replaced by `oh-my-tiny`) |

### Gut (Remove Cloud Code, Keep Local Functionality)

**`src/session/retry.ts`**
- Remove `GO_UPSELL_URL` and all `tinycode.dev/go` references
- Remove free-tier limit error handling
- Keep generic retry/backoff logic

**`src/config/config.ts`**
- Remove `$schema: "https://tinycode.dev/config.json"` auto-injection (currently lines 429–466, 579)
- Remove remote config fetching
- Remove `enterprise` config field
- Change default config directory: `~/.config/tinycode/` → `~/.config/tinycode/`

**`src/server/shared/ui.ts`**
- Remove `UI_UPSTREAM = "https://app.tinycode.dev"` proxy fallback
- Require embedded UI or local dev server — no silent remote proxy

**`src/provider/provider.ts`**
- Remove `tinycode` custom provider (currently lines 178–200)
- Remove `HTTP-Referer: "https://tinycode.dev/"` request headers
- Trim `BUNDLED_PROVIDERS` from 24 entries down to 4 (see Provider Simplification below)

**`src/cli/cmd/tui/config/tui-migrate.ts`**
- Remove `TUI_SCHEMA_URL = "https://tinycode.dev/tui.json"` reference

**`src/auth/index.ts`** (rewrite)
- Remove OAuth flow entirely
- API key auth only: read from environment variables and config file
- Store credentials in `auth.json` using `type: "api"` entries only

---

## Provider Simplification

`BUNDLED_PROVIDERS` is cut from 24 entries to 4:

| Package | Purpose |
|---------|---------|
| `@ai-sdk/openai` | OpenAI API |
| `@ai-sdk/openai-compatible` | Ollama, vLLM, and any OpenAI-compatible endpoint |
| `@ai-sdk/anthropic` | Anthropic API |
| `@ai-sdk/google` | Google Gemini API |

**Removed providers:** bedrock, azure, vertex, copilot, gitlab, groq, mistral, cerebras, cohere, deepinfra, togetherai, openrouter, nvidia, xai, perplexity, alibaba, vercel, sap, zenmux, llmgateway, venice, kilo, and the proprietary `tinycode` provider.

Users who need Azure or Bedrock can add the relevant `@ai-sdk/*` package manually — the provider plugin interface (`packages/plugin`) remains available for this.

---

## `packages/core/src/` — Module Changes

### Remove

| Path | Reason |
|------|--------|
| `src/account.ts` | Account management against remote server |
| `src/github-copilot/` | GitHub Copilot auth and provider |

### `src/plugin/provider/` — Trim from 32 to 5 plugins

**Keep:**
- `openai.ts`
- `openai-compatible.ts`
- `anthropic.ts`
- `google.ts`
- `dynamic.ts`

**Remove:** all 27 others — `alibaba.ts`, `amazon-bedrock.ts`, `azure.ts`, `cerebras.ts`, `cloudflare-ai-gateway.ts`, `cloudflare-workers-ai.ts`, `cohere.ts`, `deepinfra.ts`, `gateway.ts`, `github-copilot.ts`, `gitlab.ts`, `google-vertex.ts`, `groq.ts`, `kilo.ts`, `llmgateway.ts`, `mistral.ts`, `nvidia.ts`, `tinycode.ts`, `openrouter.ts`, `perplexity.ts`, `sap-ai-core.ts`, `togetherai.ts`, `venice.ts`, `vercel.ts`, `zenmux.ts`, and any remaining cloud-specific plugins.

### `src/models-dev.ts` — Local-First Model Catalog

- Change default model source from `https://models.dev/api.json` to a bundled local file: `packages/core/src/models-local.json`
- Network fetch remains available opt-in via the `TINYCODE_MODELS_URL` environment variable
- Bundled catalog covers: Ollama/vLLM (via `openai-compatible`), OpenAI, Anthropic, Google

---

## New Modules

### 1. Local LLM Discovery

**File:** `packages/tinycode/src/provider/local-discovery.ts`

Auto-detects local and LAN inference servers at startup and repopulates the provider catalog every 30 seconds.

**Detection targets:**

| Server | Endpoint | Detection method |
|--------|----------|-----------------|
| Ollama | `http://localhost:11434` | `GET /api/tags` |
| vLLM | `http://localhost:8000` | `GET /v1/models` |
| LAN servers | mDNS/Bonjour | `bonjour-service` (already present in deps) |

On success, each discovered model is registered as an `openai-compatible` provider entry using the server's base URL and the model name returned by the API.

### 2. Bundled Model Catalog

**File:** `packages/core/src/models-local.json`

Static JSON shipping with the binary. Contains model IDs, context window sizes, and capability flags for the supported providers. No network request at startup.

### 3. Simplified Auth

**File:** `packages/tinycode/src/auth/index.ts` (rewrite)

- No OAuth. No browser redirect. No token exchange.
- API keys read from environment variables (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GOOGLE_API_KEY`) and from the config file.
- Credentials stored in `~/.config/tinycode/auth.json` using `type: "api"` entries only.

---

## Default Configuration

The default `tinycode.json` config pre-configures local inference. Users do not need to edit anything to talk to a local Ollama or vLLM instance.

**Local providers (default):**

```json
{
  "$schema": "./tinycode-config-schema.json",
  "provider": {
    "ollama": {
      "npm": "@ai-sdk/openai-compatible",
      "options": {
        "baseURL": "http://localhost:11434/v1",
        "name": "ollama"
      }
    },
    "vllm": {
      "npm": "@ai-sdk/openai-compatible",
      "options": {
        "baseURL": "http://localhost:8000/v1",
        "name": "vllm"
      }
    }
  }
}
```

**Cloud provider API keys (user-supplied, optional):**

```json
{
  "provider": {
    "anthropic": { "options": { "apiKey": "$ANTHROPIC_API_KEY" } },
    "openai":    { "options": { "apiKey": "$OPENAI_API_KEY" } },
    "google":    { "options": { "apiKey": "$GOOGLE_API_KEY" } }
  }
}
```

Config schema reference changes from `https://tinycode.dev/config.json` to a local `tinycode-config-schema.json` bundled with the binary.

Config directory: `~/.config/tinycode/` (previously `~/.config/tinycode/`).

---

## `packages/app` Changes

**Replace `ghostty-web` with `xterm.js`**

`ghostty-web` is a private dependency hosted in the `bobbyjohnstx` org. It is replaced with `xterm.js` (open source, MIT licensed, `@xterm/xterm` on npm). The terminal emulation interface is the same; the swap is contained to the terminal component.

**Remove Sentry**

- Remove `@sentry/solid` from dependencies
- Remove `@sentry/vite-plugin` from dev dependencies
- Remove all Sentry initialization and `Sentry.captureException` call sites

No structural changes to the API client, routing, or component hierarchy.

---

## Root-Level Cleanup

### Files and Directories to Remove

| Path | Reason |
|------|--------|
| `sst.config.ts` | SST cloud infrastructure definition |
| `sst-env.d.ts` | SST type declarations |
| `infra/` | All cloud infrastructure configs |
| `nix/` | Nix dev environment |
| `sdks/vscode/` | VS Code extension |
| `perf/` | Cloud performance testing |
| `specs/` | API specification docs |

### Root `package.json` Changes

**Remove from `dependencies`:**
- `@aws-sdk/client-s3`
- `heap-snapshot-toolkit`

**Remove from `devDependencies`:**
- `@actions/artifact`
- `sst`

**Remove from `scripts`:**
- `dev:console`
- `dev:stats`
- `dev:storybook`

**Remove from `patchedDependencies`:** all cloud-only SDK patches.

### `turbo.json`

Remove cloud build targets. Keep only: `build`, `dev`, `typecheck`, `lint`, `test`.

---

## Deployment

### Single Binary (Recommended)

Build a standalone executable. No Bun or Node runtime required on the target machine.

```bash
./packages/tinycode/script/build.ts --single
```

Usage:

```bash
./tinycode           # TUI (default)
./tinycode serve     # Headless API server on port 4096
./tinycode web       # API server + web UI on port 4096
```

### Development Run

```bash
bun install
bun dev              # TUI mode
bun dev serve        # Headless API
bun dev web          # API + web UI
```

### Docker (Air-Gapped LAN Server)

Suitable for a shared inference server on an isolated network.

```dockerfile
FROM oven/bun:latest
COPY . /app
RUN cd /app && bun install --frozen-lockfile
EXPOSE 4096
CMD ["bun", "run", "--cwd", "packages/tinycode", "src/index.ts", "serve", "--hostname", "0.0.0.0"]
```

**Prerequisites for all deployment options:**
- Ollama or vLLM running on localhost or LAN (for local inference)
- `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, or `GOOGLE_API_KEY` set in environment (for cloud fallback, optional)

---

## Trade-off Decisions

| Decision | Pros | Cons |
|----------|------|------|
| Bundle model catalog locally (`models-local.json`) | Air-gapped, fast startup, no DNS at boot | Must update catalog manually; new cloud model IDs not auto-discovered |
| Use `@ai-sdk/openai-compatible` for local LLMs | Single integration covers Ollama and vLLM without custom code | Some Ollama-specific parameters (`keep_alive`, `num_ctx`) require explicit options passthrough |
| Remove 27 provider plugins | Smaller dependency tree, faster builds, no unused auth code | Users wanting Azure, Bedrock, or other cloud providers must add `@ai-sdk/*` packages and configure manually |
| Replace `ghostty-web` with `xterm.js` | Eliminates private `bobbyjohnstx` dependency; fully open source | Potential terminal rendering differences for edge cases |
| Remove account and OAuth system | Eliminates all cloud auth dependency; simpler code path | No multi-device session sync; no session sharing |
| Keep Effect framework | Avoids months-long rewrite of server, session, and tool layers | Effect is a heavy dependency; a ground-up rewrite would produce a leaner binary |
| Local LLM discovery via polling (30-second interval) | Simple, reliable, works air-gapped, no persistent connections | 30-second delay before a newly started inference server appears in the provider list |
| Remove ACP/ACP-next, delegate to `oh-my-tiny` | Clean separation of concerns; removes cloud orchestration code | `oh-my-tiny` (`../oh-my-tiny/`) is a separate tool that must be installed for extended agent workflows |
