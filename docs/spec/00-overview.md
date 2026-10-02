# tinycode Technical Specification

Version: 2.0
Date: 2026-09-11

---

## 1. Overview

tinycode is a Go rewrite of [tinycode](https://github.com/bobbyjohnstx/tinycode) (TypeScript). It is a standalone Go binary — a single process embeds the HTTP server (ephemeral port), terminal UI (bubbletea), session management, LLM client, tool execution, and plugin system.

The system manages AI conversation sessions, coordinates tool execution, handles LLM provider discovery, and provides an extensible plugin and skill framework. It is designed to work well with small local models (8B–14B parameters) while scaling gracefully to larger cloud models.

### 1.1 Target Audience

Developers who want an AI coding assistant that runs primarily against local LLM infrastructure (Ollama, vLLM, LM Studio, or any OpenAI-compatible endpoint), with optional cloud provider support via OpenRouter.

### 1.2 Use Cases

- Interactive AI-assisted coding in a terminal (bubbletea TUI)
- Headless API server for programmatic workflows (`tinycode serve`)
- Non-interactive CLI for scripting and CI/CD (`tinycode run`)
- IDE integration via Agent Client Protocol (`tinycode acp`)
- Multi-agent orchestration with subagent spawning

### 1.3 Installation

```bash
# Shell installer
curl -fsSL https://raw.githubusercontent.com/bobbyjohnstx/tinycode/main/install.sh | sh

# Homebrew
brew install bobbyjohnstx/tap/tinycode

# Build from source
git clone https://github.com/bobbyjohnstx/tinycode.git
cd tinycode && make build
# Binary at dist/tinycode
```

### 1.4 Run Modes

| Mode | Command | Description |
|------|---------|-------------|
| TUI | `tinycode` | Interactive terminal UI (default) |
| TUI (directory) | `tinycode <dir>` | TUI against a specific project |
| Serve | `tinycode serve` | Headless HTTP API server |
| Run | `tinycode run "prompt"` | Non-interactive single prompt |
| Run (multi-turn) | `tinycode run --multi-turn` | Multi-turn stdin loop |
| ACP | `tinycode acp` | Agent Client Protocol (IDE integration, stdio) |

### 1.5 Architecture Summary

```
┌──────────────────────────────────────────────┐
│                  cmd/tinycode                 │
│  main.go → dispatches to TUI / serve / run   │
├──────────────────────────────────────────────┤
│  internal/tui/       Bubbletea TUI            │
│  internal/server/    HTTP server (net/http)    │
│  internal/session/   Session + processor loop  │
│  internal/llm/       OpenAI-compatible client  │
│  internal/provider/  Provider discovery        │
│  internal/agent/     Agent registry            │
│  internal/tool/      Tool implementations      │
│  internal/plugin/    Plugin system (JSON-RPC)   │
│  internal/mcp/       MCP client                │
│  internal/permission/ Permission evaluation     │
│  internal/config/    Config parsing (JSONC)     │
│  internal/storage/   SQLite persistence         │
│  internal/bus/       Event bus                  │
│  internal/skill/     Skill discovery            │
│  internal/lsp/       LSP client                 │
│  internal/vcs/       Git operations             │
│  internal/acp/       ACP stdio transport        │
│  internal/command/   Slash command discovery     │
│  internal/id/        Sortable ID generation     │
│  internal/project/   Project metadata           │
│  internal/static/    Embedded web app server    │
│  internal/earlyinit/ Package-init side effects  │
│  internal/frontmatter/ Frontmatter parser       │
│  internal/redhat/    Red Hat shared library      │
├──────────────────────────────────────────────┤
│  pkg/plugin/         Public plugin SDK          │
│  cmd/plugin-*/       Plugin binaries (36)       │
└──────────────────────────────────────────────┘
```

### 1.6 Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| Single binary | No runtime dependencies, simple deployment |
| Ephemeral port | No port conflicts; TUI discovers via bus events |
| OpenAI-compatible API | Works with Ollama, vLLM, LM Studio, OpenRouter, any compatible endpoint |
| SQLite (modernc.org/sqlite) | Pure Go, no CGO, single-file persistence |
| Bubbletea TUI | Elm architecture (immutable state, Update/View) for reliable terminal rendering |
| Plugin JSON-RPC over stdio | Process isolation, language-agnostic protocol |
| Bus-driven architecture | Loose coupling between server, TUI, plugins via event bus |

### 1.7 Key Constants

| Constant | Value | Location |
|----------|-------|----------|
| Server version | `"0.1.0"` | `handler_health.go` |
| Default port | `4096` | `server.go` |
| Shutdown timeout | `25s` | `server.go` |
| Read header timeout | `10s` | `server.go` |
| SSE heartbeat interval | `10s` | `sse.go` |
| Bus channel capacity | `4096` | `bus.go` |
| Max processor iterations | `200` | `processor.go` |
| Max consecutive tool failures | `10` | `processor.go` |
| Tool failure warn interval | every `3` | `processor.go` |
| Default session list limit | `50` | `handler_session.go` |
| File search max results | `100` | `handler_stub.go` |
| File find max results | `200` | `handler_stub.go` |
| File size skip threshold | `1 MB` | `handler_stub.go` |
| Discovery probe timeout | `2s` | `discovery.go` |
| Discovery poll interval | `30s` | `discovery.go` |
| Max consecutive discovery failures | `3` | `discovery.go` |
| Default compaction max messages | `80` | `compaction.go` |
| Compaction min preserve tokens | `2000` | `compaction.go` |
| Compaction max preserve tokens | `15000` | `compaction.go` |
| LLM header timeout | `5 min` | `openai.go` |
| LLM chunk timeout | `5 min` | `openai.go` |

### 1.8 Spec Organization

| File | Section |
|------|---------|
| [01-interfaces.md](01-interfaces.md) | TUI, Web UI, ACP, Run mode |
| [02-api-routes.md](02-api-routes.md) | All HTTP API endpoints |
| [03-event-system.md](03-event-system.md) | Event bus, SSE, LLM events |
| [04-session-management.md](04-session-management.md) | Session lifecycle, processor loop |
| [05-context-compaction.md](05-context-compaction.md) | Compaction algorithm, observation masking |
| [06-llm-providers.md](06-llm-providers.md) | Provider discovery, model schema, warmup |
| [07-agents.md](07-agents.md) | Agent registry, permission rules, small-model variants |
| [08-tools.md](08-tools.md) | Built-in tools, parameters, truncation |
| [09-plugins.md](09-plugins.md) | Plugin system, SDK, lifecycle hooks |
| [10-skills.md](10-skills.md) | Skill discovery, slash commands |
| [11-mcp.md](11-mcp.md) | Model Context Protocol client |
| [12-permissions.md](12-permissions.md) | Permission evaluation chain, rules, config |
| [13-configuration.md](13-configuration.md) | Config files, merge semantics, JSONC |
| [14-storage.md](14-storage.md) | SQLite schema, migrations, data paths |
| [15-security.md](15-security.md) | Auth, CORS, path validation, middleware |
| [16-not-implemented.md](16-not-implemented.md) | TS features not yet ported |
