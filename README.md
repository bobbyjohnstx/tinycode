# tinycode

![tinycode TUI](tinycode-screenshot.png)

[![CI](https://github.com/bobbyjohnstx/tinycode/actions/workflows/ci.yml/badge.svg)](https://github.com/bobbyjohnstx/tinycode/actions/workflows/ci.yml) [![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE) [![Release](https://img.shields.io/github/v/release/bobbyjohnstx/tinycode)](https://github.com/bobbyjohnstx/tinycode/releases) [![Website](https://img.shields.io/badge/Website-tinycode-blue)](https://bobbyjohnstx.github.io/tinycode.html) [![Discussions](https://img.shields.io/github/discussions/bobbyjohnstx/tinycode)](https://github.com/bobbyjohnstx/tinycode/discussions)

An open-source AI coding assistant that keeps your code on your machine. No cloud accounts, no API keys, no data leaving your network. Just you, your code, and your local models.

## What it is

tinycode fills the gap between cloud-only AI coding tools and privacy. It's the same kind of tool as Claude Code or Codex — reads your files, runs commands, edits code, and works through multi-step tasks — except it runs against **your own LLMs** on your own hardware. Point it at Ollama, vLLM, or any OpenAI-compatible endpoint and start coding. No sign-up, no subscription, no telemetry by default.

Cloud providers (Anthropic, OpenAI, Google, OpenRouter) are also supported if you want them, but they're optional — not required.

> **Privacy:** The CLI, TUI, and server make zero outbound network calls except to your configured model provider. The desktop app supports optional crash reporting (Sentry) when configured by the deployer via `VITE_SENTRY_DSN`.

### Interfaces

The primary interface is the **terminal UI (TUI)** — a full-featured interactive session in your terminal with conversation history, model switching, agent/skill invocation, and inline tool approval. The TUI is the fastest way to work: it starts instantly, runs anywhere a terminal does, and keeps you in the same environment as your code. tinycode automatically handles model limitations: if a model doesn't support tool calling, tinycode detects it and works without tools; if tool calls are malformed, tinycode auto-repairs common JSON issues and warns after 3+ consecutive failures.

**TUI Features:**
- **Session tree sidebar** (`<leader>b`): Toggleable ASCII tree showing your session hierarchy — organized by parent-child relationships for easy navigation
- **Session export** (`tinycode export --format html <session-id>`): Export sessions to self-contained HTML files for sharing or archiving

For teams or remote access, tinycode also ships a **web UI** (SolidJS + TailwindCSS) that connects to the tinycode API server. Open `http://localhost:4096` after starting the server, or run `bun dev web` to launch both. The web UI provides the same conversation, agent, and tool capabilities in a browser tab.

A **standalone desktop app** (Electron) is available for macOS, Windows, and Linux. It wraps the web UI in a native window with system tray integration, auto-updates via GitHub Releases, and platform-specific features. Run `bun run --cwd packages/desktop dev` to launch in development, or build distributable binaries with `bun run --cwd packages/desktop build`.

**Desktop app features:**
- **System tray integration**: Access tinycode from the menu bar (macOS) or system tray (Windows/Linux) with Show/Hide and Quit actions
- **Global hotkey** (Cmd/Ctrl+Shift+T): Bring the window to front or minimize it without closing the app
- **Auto-updates**: Automatically checks for new releases on GitHub and notifies you with an in-app banner
- **Platform-specific behaviors**: 
  - macOS: Closing the window keeps the app running in the dock; Cmd+Q quits fully. Dock badge shows notification count
  - Windows/Linux: App menu with Help links to GitHub (repo, discussions, issues). Taskbar flashes on background notifications
- **Persistent settings**: Zoom level and window state persist across app restarts
- **Theme sync**: Automatically detects OS dark/light mode changes and updates the app appearance

### IDE Integration (ACP)

tinycode supports the [Agent Client Protocol](https://agentclientprotocol.com) for IDE integration. Run `tinycode acp --cwd /path/to/project` to start an ACP server that editors (VS Code, Zed, JetBrains) can connect to via stdio.

A reference [VS Code extension](packages/vscode-extension/) is included. See [docs/acp-integration.md](docs/acp-integration.md) for building custom IDE integrations.

## Quick start

```bash
# Install tinycode — pick one:
npx tinycode-ai                          # or: npm install -g tinycode-ai
brew install bobbyjohnstx/tap/tinycode   # macOS / Linux

# Alternative (deprecated — use npm or Homebrew instead):
# curl -fsSL https://raw.githubusercontent.com/bobbyjohnstx/tinycode/main/install.sh | sh

# Or install from source (development)
bun install

# Run (TUI mode, against current directory)
bun dev

# Run against a specific project
bun dev /path/to/project

# Headless API server
bun dev serve

# Server + web UI
bun dev web
```

## Configuration

Config lives at `~/.config/tinycode/config.json`:

```json
{
  "model": "ollama/qwen3.5:9b",
  "lsp": true
}
```

> Local 9B models need ~6GB VRAM or ~12GB RAM; expect 3-8 minutes per task on CPU. See [model-compatibility.md](docs/model-compatibility.md) for hardware tiers.

For a LAN MaaS server (LiteLLM, LiteMaaS, etc.):

```bash
export TINYCODE_MAAS_HOST=https://your-maas-server
export TINYCODE_MAAS_API_KEY=your-key
```

For OpenRouter (with account balance and cost tracking):

```bash
export OPENROUTER_API_KEY=your-key
```

tinycode auto-discovers Ollama (`localhost:11434`), vLLM (`localhost:8000`), LM Studio (`localhost:1234`), and MaaS servers from environment variables at startup. Use `/connect` in the TUI to manually connect a provider.

## Documentation

- [Getting Started](docs/getting-started.md)
- [User Guide](docs/user-guide.md)
- [Architecture Overview](docs/architecture.md)
- [Model Compatibility](docs/model-compatibility.md) — hardware tiers & benchmarks
- [Plugin Development](docs/plugin-development.md) (npm: `tinycode-plugin`)
- [Adding a Tool](docs/adding-a-tool.md)
- [Deployment Guide](docs/deployment.md)
- [OpenShell Integration](docs/openshell-integration.md) — sandboxed execution with NVIDIA OpenShell

## Architecture

Bun monorepo with Turborepo. Key packages:

| Package             | Description                                                                         |
| ------------------- | ----------------------------------------------------------------------------------- |
| `packages/tinycode` | Core server, HTTP API, TUI, session processor, provider abstraction, tools, plugins |
| `packages/app`      | SolidJS + TailwindCSS v4 web UI                                                     |
| `packages/desktop`  | Electron desktop app                                                                |
| `packages/llm`      | LLM protocol implementations (Anthropic, OpenAI, Bedrock, Gemini)                   |
| `packages/ui`       | Shared SolidJS component library (icons, themes, i18n)                              |
| `packages/plugin`   | Plugin SDK (`@tinycode/plugin`)                                                     |
| `packages/sdk/js`   | Auto-generated TypeScript SDK                                                       |
| `packages/vscode-extension` | Reference VS Code extension for ACP integration                          |

See [CLAUDE.md](CLAUDE.md) for development guidance and [AGENTS.md](AGENTS.md) for coding style.

## Agents

Press **Tab** to cycle through agents, or `<leader>a` to pick from a list. Use `/ask <agent> <prompt>` to invoke any agent as a one-shot subagent without switching.

tinycode has two modes with distinct behavior, plus specialized agent personas:

- **build** (default) — Full tool access. Reads, writes, edits files, runs shell commands, executes tools. This is the normal working mode.
- **plan** — **Read-only mode with hard permission enforcement.** The LLM can explore the codebase and write only to a plan file (`.tinycode/plans/*.md`). All other edits are blocked at the tool level, not just by prompt instruction. When the plan is ready, `plan_exit` prompts you to approve and switch to build mode for execution.

All other agents (architect, debugger, executor, etc.) are **personas** — they share the same tool permissions as build mode but have specialized system prompts that guide their behavior. An architect agent is *instructed* to be read-only and analytical, but it is not *prevented* from editing files if you ask it to. The permission prompt system provides the safety gate for all tool executions regardless of which agent is active.

### Built-in agents

| Agent                 | Role                                                              |
| --------------------- | ----------------------------------------------------------------- |
| `agent-reviewer`      | Validates agent prompt definitions against the style guide        |
| `analyst`             | Pre-planning requirements analysis — catches gaps before planning |
| `architect`           | Read-only code analysis and architectural guidance                |
| `cluster-admin`       | Cluster administration and Kubernetes/OpenShift operations        |
| `code-reviewer`       | Severity-rated code review with SOLID principle checks            |
| `code-simplifier`     | Simplifies recently modified code without changing behavior       |
| `critic`              | Quality gate — multi-perspective review of plans and code         |
| `debugger`            | Root-cause analysis and bug fixing                                |
| `deep-explore`        | Thorough codebase exploration with multi-file analysis            |
| `designer`            | UI/UX designer-developer for production-grade interfaces          |
| `document-specialist` | External documentation and reference specialist                   |
| `executor`            | Focused implementation of scoped tasks                            |
| `explore`             | Fast codebase search (grep/glob)                                  |
| `git-master`          | Git expert for atomic commits, rebasing, and history management   |
| `planner`             | Strategic planning — gather requirements, produce work plans      |
| `qa-tester`           | Interactive CLI testing via tmux                                  |
| `rules-reviewer`      | Validates rule definitions against the style guide                |
| `scientist`           | Data analysis and research — hypothesis-driven, evidence required |
| `security-reviewer`   | Security vulnerability detection (OWASP Top 10, secrets, CVEs)    |
| `skills-reviewer`     | Validates skill definitions against the style guide               |
| `test-engineer`       | Test strategy, integration/e2e coverage, TDD workflows            |
| `tracer`              | Evidence-driven causal tracing with competing hypotheses          |
| `verifier`            | Evidence-based verification of completion                         |
| `workspace`           | Workspace setup and environment configuration                     |
| `writer`              | Technical documentation                                           |

Agents with a `.compact.md` variant automatically use the compact prompt for models ≤8B parameters. See [docs/internal/agent-prompt-tiers.md](docs/internal/agent-prompt-tiers.md) for details.

`@` references files only. To invoke an agent, use `/ask <agent>`.

## Skills

Type `/` to see available slash commands. Skills (marked `:skill`) inject specialized instructions:

| Skill                      | Purpose                                                                                       |
| -------------------------- | --------------------------------------------------------------------------------------------- |
| `/ai-slop-cleaner`         | Clean AI-generated code slop with regression-safe, deletion-first workflow                    |
| `/configure-notifications` | Configure notification integrations (Telegram, Discord, Slack) via natural language           |
| `/debug`                   | Isolate a single most-likely root cause for a known failure                                   |
| `/deepinit`                | Deep codebase initialization — generates per-directory AGENTS.md files                        |
| `/mcp-setup`               | Configure MCP servers via guided menu                                                         |
| `/remember`                | Triage session findings across memory surfaces (project memory, CLAUDE.md, session notes)     |
| `/tc-doctor`               | Diagnose tinycode configuration and environment issues                                        |
| `/trace`                   | Evidence-driven causal tracing with competing hypotheses                                      |
| `/verify`                  | Confirm a change works before claiming completion — runs tiered evidence ladder               |

## oh-my-tiny

oh-my-tiny is a built-in plugin providing extended orchestration tools. It adds notepad, wiki, project memory, state management, and AST grep tools — all stored under `.tinycode/` in the project directory. The agents listed above ship with tinycode; omt provides the tools they use for state management and knowledge persistence.

## Building

```bash
# Build standalone binary for current platform
bun ./packages/tinycode/script/build.ts --single

# Output: packages/tinycode/dist/tinycode-darwin-arm64/bin/tinycode
```

## Deployment

For remote servers, containers, and OpenShift/Kubernetes clusters, see the [Deployment Guide](docs/deployment.md). The recommended path for cluster deployments is the [tinycode-operator](https://github.com/bobbyjohnstx/tinycode-operator), which manages `TinycodeInstance` custom resources and handles storage, routing, and security context automatically.

## Ecosystem

tinycode is a family of projects that work together:

| Project | What it does |
| ------- | ------------ |
| **tinycode** (this repo) | Core server, TUI, web UI, desktop app, agents, skills, tools, and LLM provider integrations. Everything you need to run tinycode locally. |
| [**tinycode-container**](https://github.com/bobbyjohnstx/tinycode-container) | Container image that packages tinycode with oh-my-tiny, tmux, git, and optional oc CLI into a single OCI image for Kubernetes and OpenShift deployments. Handles PVC-based config persistence, vLLM auto-discovery, GitOps repo cloning, and OpenShift arbitrary-UID compatibility. |
| [**tinycode-operator**](https://github.com/bobbyjohnstx/tinycode-operator) | Kubernetes Operator for OpenShift that manages `TinycodeInstance` custom resources. Handles deployment, storage provisioning, Route/Ingress creation, SCC binding, declarative vLLM configuration with auto-probing, cross-namespace model discovery, GitOps mode, shared team workspaces with RWX PVCs, and cluster-admin mode with kubeconfig mounting. Installable via OLM/OperatorHub or Helm. |
| [**tinycode-plugins**](https://github.com/bobbyjohnstx/tinycode-plugins) | Community plugin registry and supporting materials. Publish your plugins here for discovery via `tinycode plugin-search`. |
| [**tinycode-plugin-template**](https://github.com/bobbyjohnstx/tinycode-plugin-template) | Starter template for building tinycode plugins. Clone it to scaffold a new plugin with the correct structure, dependencies, and example tool. |

## Acknowledgments

tinycode is built on [opencode](https://github.com/sst/opencode) by [SST](https://github.com/sst). The core architecture — session processor, provider abstraction, tool system, and TUI — originates from the opencode project. tinycode extends it with local-LLM-first design, bundled agents and skills, MCP integration, web/desktop UIs, container packaging, and a Kubernetes operator for OpenShift deployment.

## License

MIT
