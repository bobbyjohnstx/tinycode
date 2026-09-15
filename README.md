# tinycode

AI coding assistant for the terminal. Single binary, no runtime dependencies.

![tinycode TUI](tinycode-screenshot.png)

[![CI](https://github.com/bobbyjohnstx/tinycode-go/actions/workflows/ci.yml/badge.svg)](https://github.com/bobbyjohnstx/tinycode-go/actions/workflows/ci.yml) [![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## What it is

tinycode is a privacy-first, local-first AI coding assistant. A single Go binary embeds everything: HTTP server, terminal UI, session management, LLM client, and tool execution. No separate server process, no Node.js, no runtime dependencies.

It connects to local LLM providers (Ollama, vLLM, LM Studio) or cloud endpoints (OpenRouter, any OpenAI-compatible API). No data leaves your machine unless you configure a cloud provider. No telemetry, no sign-up, no subscription.

tinycode reads your files, runs commands, edits code, and works through multi-step tasks --- the same workflow as cloud AI coding tools, but against your own models on your own hardware.

### Interfaces

The primary interface is the **terminal UI (TUI)** --- a full-featured interactive session with conversation history, model switching, agent/skill invocation, and inline tool approval. The TUI starts instantly and keeps you in the same environment as your code.

tinycode also supports:

- **Headless API server** (`tinycode serve`) --- REST + SSE endpoints for programmatic access
- **Agent Client Protocol** (`tinycode acp`) --- stdio transport for IDE integration (VS Code, Zed, JetBrains)
- **Non-interactive mode** (`tinycode run`) --- run a prompt and exit, for scripts and CI

## Quick start

```bash
# Build from source
git clone https://github.com/bobbyjohnstx/tinycode-go.git && cd tinycode-go
make build
./dist/tinycode

# Or with a specific project directory
./dist/tinycode /path/to/project

# Headless API server
./dist/tinycode serve

# IDE integration (Agent Client Protocol)
./dist/tinycode acp
```

## Architecture

Standard Go layout: `cmd/` for binaries, `internal/` for private packages, `pkg/` for public SDK.

### `cmd/tinycode/` --- Main binary

Single entry point. Subcommands: `tui` (default), `serve`, `web`, `acp`, `run`, `models`, `providers`, `session`, `export`, `agent`, `debug`, `version`.

### `internal/` --- Core packages

| Package        | Description                                                                    |
| -------------- | ------------------------------------------------------------------------------ |
| `tui/`         | Terminal UI ([bubbletea](https://github.com/charmbracelet/bubbletea), Elm architecture) |
| `tui/api/`     | HTTP client for the embedded server API                                        |
| `server/`      | HTTP server (net/http + chi router), REST + SSE endpoints                      |
| `session/`     | Session lifecycle, processor loop, LLM coordination                            |
| `llm/`         | LLM client abstraction, OpenAI-compatible streaming, tool-call JSON repair     |
| `provider/`    | Provider auto-discovery (Ollama, vLLM, LM Studio, OpenRouter)                  |
| `agent/`       | Agent definitions and prompt files                                             |
| `tool/`        | Tool implementations (file ops, shell, grep, glob)                             |
| `config/`      | Config file parsing (`~/.config/tinycode/config.json`), JSONC support          |
| `storage/`     | SQLite via modernc.org/sqlite, migrations                                      |
| `bus/`         | Event bus for inter-component communication                                    |
| `mcp/`         | Model Context Protocol client                                                  |
| `acp/`         | Agent Client Protocol (stdio transport for IDE integration)                    |
| `plugin/`      | Plugin lifecycle management                                                    |
| `permission/`  | Tool permission prompting and rules                                            |
| `skill/`       | Skill discovery and loading                                                    |
| `vcs/`         | Git operations                                                                 |
| `command/`     | Slash command discovery (built-in commands, agents, skills)                     |
| `project/`     | Project metadata, VCS detection, worktree paths                                |
| `frontmatter/` | YAML-like frontmatter parser for agent and skill markdown files                |
| `id/`          | Sortable ID generation with typed prefixes                                     |
| `redhat/`      | Red Hat shared library (OcClient, APIClient, PromQL, Containerfile parser)     |
| `static/`      | Embedded web app file server with SPA fallback                                 |
| `earlyinit/`   | Package-init side effects that run before other imports                        |

### `pkg/plugin/` --- Public Go plugin SDK

Protocol definitions, hook interfaces, and tool registration for building plugins.

## Plugins

36 built-in plugins, each a standalone Go binary communicating over JSON-RPC via stdin/stdout.

| Plugin               | Description                                                             |
| -------------------- | ----------------------------------------------------------------------- |
| `plugin-cluster-ops` | OpenShift cluster operations (oc login, status, cluster info)           |
| `plugin-code-review` | Git diff formatting for code review                                     |
| `plugin-command-inject` | Discovers and exposes project scripts as tools                       |
| `plugin-context-pruning` | Detects duplicate tool calls and prunes redundant context           |
| `plugin-handoff`     | Saves and restores session context for cross-session handoff            |
| `plugin-log-sanitizer` | Redacts secrets and sensitive data from tool output                   |
| `plugin-notify`      | Desktop notifications                                                   |
| `plugin-pilot`       | Issue tracker integration (list, create, update, comment)               |
| `plugin-safety-net`  | Blocks dangerous shell commands before execution                        |
| `plugin-snippets`    | Kubernetes/OpenShift manifest templates                                 |
| `plugin-telemetry`   | Tool call tracking and usage reporting                                  |
| `plugin-web-search`  | Web search via DuckDuckGo and Red Hat knowledge base                    |

#### Red Hat --- OpenShift

| Plugin                        | Description                                                        |
| ----------------------------- | ------------------------------------------------------------------ |
| `plugin-ocp-context-injection` | Injects current OpenShift cluster/project context into sessions   |
| `plugin-ocp-oauth`            | OpenShift OAuth token management and refresh                       |
| `plugin-ocp-obs-logging`      | OpenShift observability: log queries via Loki/LokiStack            |
| `plugin-ocp-obs-metrics`      | OpenShift observability: PromQL queries and alert inspection       |

#### Red Hat --- Ansible

| Plugin                   | Description                                                             |
| ------------------------ | ----------------------------------------------------------------------- |
| `plugin-aap-bridge`     | Ansible Automation Platform bridge (job templates, inventories, credentials) |
| `plugin-eda-events`     | Event-Driven Ansible event stream and rulebook activation               |

#### Red Hat --- RHOAI

| Plugin                          | Description                                                       |
| ------------------------------- | ----------------------------------------------------------------- |
| `plugin-rhoai-eval-trustyai`   | TrustyAI model evaluation (bias, fairness, explainability)         |
| `plugin-rhoai-experiment-tracker` | ML experiment tracking (metrics, parameters, runs)              |
| `plugin-rhoai-mcp-bridge`      | MCP-to-RHOAI bridge for model context protocol integration        |
| `plugin-rhoai-mlflow-tools`    | MLflow experiment and model registry operations                    |
| `plugin-rhoai-model-serving`   | RHOAI model serving management (deploy, scale, monitor)            |
| `plugin-rhoai-pipelines`       | RHOAI/Kubeflow pipeline management (create, run, monitor)          |

#### Red Hat --- Platform

| Plugin                          | Description                                                       |
| ------------------------------- | ----------------------------------------------------------------- |
| `plugin-satellite`              | Satellite administration (hosts, errata, content views, services, REX) |
| `plugin-quay`                   | Quay container registry operations (repos, tags, security scans)  |
| `plugin-rhdh`                   | Red Hat Developer Hub catalog and template operations              |
| `plugin-tekton`                 | Tekton pipeline and task management                                |
| `plugin-rhacm`                  | Red Hat Advanced Cluster Management (fleet, policies, placement)  |
| `plugin-rhacs`                  | Red Hat Advanced Cluster Security (vulnerabilities, compliance)    |
| `plugin-rh-api-catalog`        | Red Hat API catalog discovery and documentation                    |
| `plugin-rh-dev-content`        | Red Hat developer content and learning resources                   |
| `plugin-rh-ecosystem-catalog`  | Red Hat ecosystem and partner integration catalog                  |
| `plugin-rhdp-provisioner`      | Red Hat Developer Platform environment provisioning                |
| `plugin-container-linter`      | Containerfile/Dockerfile linting and best practice checks          |
| `plugin-lightwell`             | Lightwell integration for Red Hat product lifecycle data           |

Plugins use the SDK in `pkg/plugin/`. See [docs/plugin-development.md](docs/plugin-development.md) for building custom plugins.

## Agents

Press **Tab** to cycle through agents, or use `<leader>a` to pick from a list. Use `/ask <agent> <prompt>` to invoke any agent as a one-shot subagent.

### Built-in agents

| Agent               | Description                                                                   |
| ------------------- | ----------------------------------------------------------------------------- |
| `architect`         | Strategic architecture advisor --- analyzes code, diagnoses bugs (read-only)  |
| `code-reviewer`     | Severity-rated code review with logic defect detection and SOLID checks       |
| `code-simplifier`   | Simplifies recently modified code without changing behavior                   |
| `critic`            | Multi-perspective review of plans and code with gap analysis (read-only)      |
| `debugger`          | Root-cause analysis, regression isolation, stack trace analysis                |
| `executor`          | Focused task executor --- smallest viable diff, no scope creep                |
| `explore`           | Fast read-only codebase search (grep/glob)                                    |
| `git-master`        | Git expert for atomic commits, rebasing, and history management               |
| `planner`           | Strategic planning --- gathers requirements, produces actionable work plans   |
| `scientist`         | Data analysis and research --- hypothesis-driven, evidence required           |
| `security-reviewer` | Security vulnerability detection (OWASP Top 10, secrets, CVEs)               |
| `test-engineer`     | Test strategy, coverage authoring, flaky test hardening, TDD workflows        |
| `verifier`          | Evidence-based verification of completion claims                              |
| `writer`            | Technical documentation                                                       |

Agents with a `.compact.md` variant automatically use a smaller prompt for models with limited context windows.

## Configuration

Config lives at `~/.config/tinycode/config.json` (JSONC supported):

```jsonc
{
  "model": "ollama/qwen3.5:9b",
  "small_model": "ollama/qwen3.5:1.7b",
  "default_agent": "build"
}
```

### Provider auto-discovery

tinycode probes local LLM providers at startup:

| Provider   | Default address          | Override                     |
| ---------- | ------------------------ | ---------------------------- |
| Ollama     | `127.0.0.1:11434`       | `OLLAMA_HOST`                |
| vLLM       | (none, requires env var) | `TINYCODE_VLLM_HOST`        |
| LM Studio  | `127.0.0.1:1234`        | `TINYCODE_LMSTUDIO_HOST`    |
| OpenRouter | ---                      | `OPENROUTER_API_KEY`         |

### Environment variables

| Variable             | Purpose                                 |
| -------------------- | --------------------------------------- |
| `TINYCODE_PORT`      | Override default server port (4096)     |
| `TINYCODE_HOST`      | Override default bind address (127.0.0.1) |
| `TINYCODE_DB`        | Override database path                  |
| `TINYCODE_LOG_LEVEL` | Set log level (debug, info, warn, error) |
| `TINYCODE_WEB_DIR`   | Serve web UI from directory (dev mode)  |
| `OLLAMA_HOST`        | Ollama server URL                       |
| `TINYCODE_VLLM_HOST` | vLLM server URL                         |
| `TINYCODE_LMSTUDIO_HOST` | LM Studio server URL               |
| `OPENROUTER_API_KEY` | Enable OpenRouter provider              |

## Building

```bash
make build          # Build for current platform -> dist/tinycode
make build-all      # Cross-compile for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64
make package        # Create release archives for all platforms
make test           # Run all tests
make lint           # Run go vet
make check          # Run vet + tests
make embed-webapp   # Embed SolidJS web app into the binary
make clean          # Remove build artifacts
```

## License

MIT
