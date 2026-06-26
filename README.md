# tinycode

[![CI](https://github.com/bobbyjohnstx/tinycode/actions/workflows/ci.yml/badge.svg)](https://github.com/bobbyjohnstx/tinycode/actions/workflows/ci.yml) [![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE) [![Release](https://img.shields.io/github/v/release/bobbyjohnstx/tinycode)](https://github.com/bobbyjohnstx/tinycode/releases)

A slim, local-LLM-first AI coding assistant. Runs air-gapped with zero cloud dependencies.

## What it is

tinycode is an AI coding agent that runs against **your own LLMs** — Ollama, vLLM, or any OpenAI-compatible endpoint on your network. Cloud providers (Anthropic, OpenAI, Google, OpenRouter) are supported via API key as a secondary option, but local inference is the primary use case.

### Interfaces

The primary interface is the **terminal UI (TUI)** — a full-featured interactive session in your terminal with conversation history, model switching, agent/skill invocation, and inline tool approval. The TUI is the fastest way to work: it starts instantly, runs anywhere a terminal does, and keeps you in the same environment as your code.

**TUI Features:**
- **Session tree sidebar** (`<leader>b`): Toggleable ASCII tree showing your session hierarchy — organized by parent-child relationships for easy navigation
- **Session export** (`tinycode export --format html <session-id>`): Export sessions to self-contained HTML files for sharing or archiving

For teams or remote access, tinycode also ships a **web UI** (SolidJS + TailwindCSS) that connects to the tinycode API server. Open `http://localhost:4096` after starting the server, or run `bun dev web` to launch both. The web UI provides the same conversation, agent, and tool capabilities in a browser tab.

A **standalone desktop app** (Electron) is available for macOS, Windows, and Linux. It wraps the web UI in a native window with system tray integration. Run `bun run --cwd packages/desktop dev` to launch in development, or build distributable binaries with `bun run --cwd packages/desktop build`.

## Quick start

```bash
# Install dependencies
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
  "model": "ollama/llama3.2",
  "lsp": true
}
```

For a LAN MaaS server (LiteLLM, LiteMaaS, etc.):

```bash
export TINYCODE_MAAS_HOST=https://your-maas-server
export TINYCODE_MAAS_API_KEY=your-key
```

For OpenRouter (with account balance and cost tracking):

```bash
export OPENROUTER_API_KEY=your-key
```

tinycode auto-discovers Ollama (`localhost:11434`), vLLM (`localhost:8000`), and MaaS servers from environment variables at startup. Use `/connect` in the TUI to manually connect a provider.

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

See [CLAUDE.md](CLAUDE.md) for development guidance and [AGENTS.md](AGENTS.md) for coding style.

## Agents

Type `/ask <agent> <prompt>` to invoke a subagent. Tab or `<leader>a` switches the primary agent (build/plan).

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
| `designer`            | UI/UX designer-developer for production-grade interfaces          |
| `document-specialist` | External documentation and reference specialist                   |
| `executor`            | Focused implementation of scoped tasks                            |
| `explore`             | Fast codebase search (grep/glob)                                  |
| `git-master`          | Git expert for atomic commits, rebasing, and history management   |
| `planner`             | Strategic planning — gather requirements, produce work plans      |
| `qa-tester`           | Interactive CLI testing via tmux                                  |
| `scientist`           | Data analysis and research — hypothesis-driven, evidence required |
| `security-reviewer`   | Security vulnerability detection (OWASP Top 10, secrets, CVEs)    |
| `skills-reviewer`     | Validates skill definitions against the style guide               |
| `test-engineer`       | Test strategy, integration/e2e coverage, TDD workflows            |
| `tracer`              | Evidence-driven causal tracing with competing hypotheses          |
| `verifier`            | Evidence-based verification of completion                         |
| `workspace`           | Workspace setup and environment configuration                     |
| `writer`              | Technical documentation                                           |

Agents with a `.compact.md` variant automatically use the compact prompt for models ≤9B parameters. See [docs/agent-prompt-tiers.md](docs/agent-prompt-tiers.md) for details.

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

## Remote Installation

For production deployments on OpenShift or Kubernetes, the recommended approach is the **tinycode-operator** — it manages `TinycodeInstance` custom resources and handles deployment, storage, routing, and security context automatically. See the [tinycode-operator](https://github.com/bobbyjohnstx/tinycode-operator) repository.

For installing directly on a remote server, clone from your Gitea or GitHub remote and run `bun install`. If you need to transfer via zip instead:

```bash
zip -r tinycode.zip . \
  --exclude "*/node_modules/*" \
  --exclude ".git/*" \
  --exclude "*/dist/*"
```

| Exclusion          | Why                                                        |
| ------------------ | ---------------------------------------------------------- |
| `*/node_modules/*` | npm dependencies — restored by `bun install` on the target |
| `.git/*`           | Git history — not needed to run the server                 |
| `*/dist/*`         | Built binaries and web UI assets — regenerated at runtime  |

On the target server after unzipping:

```bash
# Install Bun (if not already installed)
curl -fsSL https://bun.sh/install | bash

# Install dependencies
bun install

# Run headless server, bound to all interfaces for remote access
bun dev serve --hostname 0.0.0.0

# Or set in ~/.config/tinycode/config.json:
# { "server": { "hostname": "0.0.0.0" } }
```

Set `TINYCODE_SERVER_PASSWORD` before starting — without it the server is unsecured. Open port 4096 in the firewall:

```bash
sudo firewall-cmd --add-port=4096/tcp --permanent && sudo firewall-cmd --reload
```

Access the web UI at `http://<server-ip>:4096`.

## Ecosystem

tinycode is three projects that work together:

| Project | What it does |
| ------- | ------------ |
| **tinycode** (this repo) | Core server, TUI, web UI, desktop app, agents, skills, tools, and LLM provider integrations. Everything you need to run tinycode locally. |
| [**tinycode-container**](https://github.com/bobbyjohnstx/tinycode-container) | Container image that packages tinycode with oh-my-tiny, tmux, git, and optional oc CLI into a single OCI image for Kubernetes and OpenShift deployments. Handles PVC-based config persistence, vLLM auto-discovery, GitOps repo cloning, and OpenShift arbitrary-UID compatibility. |
| [**tinycode-operator**](https://github.com/bobbyjohnstx/tinycode-operator) | Kubernetes Operator for OpenShift that manages `TinycodeInstance` custom resources. Handles deployment, storage provisioning, Route/Ingress creation, SCC binding, declarative vLLM configuration with auto-probing, cross-namespace model discovery, GitOps mode, shared team workspaces with RWX PVCs, and cluster-admin mode with kubeconfig mounting. Installable via OLM/OperatorHub or Helm. |

### Container images

Pre-built container images are published to [Quay.io](https://quay.io/repository/bjohns/tinycode-container):

```bash
# Pull the latest image
podman pull quay.io/bjohns/tinycode-container:latest

# Run locally with Ollama on the host network
podman run -it --network host \
  -e TINYCODE_SERVER_PASSWORD=changeme \
  quay.io/bjohns/tinycode-container:latest

# Run with a remote vLLM endpoint
podman run -it -p 4096:4096 \
  -e TINYCODE_VLLM_URL=http://your-vllm-server:8000 \
  -e TINYCODE_SERVER_PASSWORD=changeme \
  quay.io/bjohns/tinycode-container:latest
```

Images are also mirrored to `ghcr.io/bjohns/tinycode-container`. Both registries receive identical multi-arch builds (amd64 + arm64) on every push to main.

### OpenShift / Kubernetes deployment

The recommended path for cluster deployments is the **tinycode-operator**. It reduces a full deployment to a single CR:

```yaml
apiVersion: tinycode.dev/v1alpha1
kind: TinycodeInstance
metadata:
  name: my-tinycode
spec:
  vllm:
    - name: vllm-qwen3
      url: http://qwen3-30b.qwen3.svc.cluster.local:8080
  model: "vllm-qwen3/qwen3-30b"
  auth:
    passwordSecret: tinycode-password
  storage:
    projectsSize: "20Gi"
```

The operator handles Route creation, PVC provisioning, SCC binding, vLLM model auto-probing, and pod lifecycle. See the [tinycode-operator README](https://github.com/bobbyjohnstx/tinycode-operator) and [RHOAI cluster setup guide](https://github.com/bobbyjohnstx/tinycode-operator/blob/main/docs/rhoai-cluster-setup.md) for full documentation.

## License

MIT
