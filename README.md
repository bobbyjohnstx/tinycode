# tinycode

A slim, local-LLM-first AI coding assistant. Runs air-gapped with zero cloud dependencies.

## What it is

tinycode is a terminal UI (TUI) coding agent that runs against **your own LLMs** — Ollama, vLLM, or any OpenAI-compatible MaaS server on your LAN. Cloud providers (Anthropic, OpenAI, Google, OpenRouter) are supported via API key as a secondary option, but local inference is the primary use case.

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

| Agent     | Role                                                                  |
| --------- | --------------------------------------------------------------------- |
| `explore` | Fast codebase search (grep/glob)                                      |
| `scout`   | External research — clone repos, read docs. Never modifies workspace. |
| `general` | General-purpose agent, fans out work in parallel                      |

### Custom agents (via `.tinycode/agent/`)

| Agent                 | Role                                                              |
| --------------------- | ----------------------------------------------------------------- |
| `agent-reviewer`      | Validates agent prompt definitions against the style guide        |
| `analyst`             | Pre-planning requirements analysis — catches gaps before planning |
| `architect`           | Read-only code analysis and architectural guidance                |
| `code-reviewer`       | Severity-rated code review with SOLID principle checks            |
| `code-simplifier`     | Simplifies recently modified code without changing behavior       |
| `critic`              | Quality gate — multi-perspective review of plans and code         |
| `debugger`            | Root-cause analysis and bug fixing                                |
| `designer`            | UI/UX designer-developer for production-grade interfaces          |
| `document-specialist` | External documentation and reference specialist                   |
| `executor`            | Focused implementation of scoped tasks                            |
| `git-master`          | Git expert for atomic commits, rebasing, and history management   |
| `planner`             | Strategic planning — gather requirements, produce work plans      |
| `qa-tester`           | Interactive CLI testing via tmux                                  |
| `scientist`           | Data analysis and research — hypothesis-driven, evidence required |
| `security-reviewer`   | Security vulnerability detection (OWASP Top 10, secrets, CVEs)    |
| `test-engineer`       | Test strategy, integration/e2e coverage, TDD workflows            |
| `tracer`              | Evidence-driven causal tracing with competing hypotheses          |
| `verifier`            | Evidence-based verification of completion                         |
| `writer`              | Technical documentation                                           |

Agents with a `.compact.md` variant automatically use the compact prompt for models ≤9B parameters. See [docs/agent-prompt-tiers.md](docs/agent-prompt-tiers.md) for details.

`@` references files only. To invoke an agent, use `/ask <agent>`.

## Skills

Type `/` to see available slash commands. Skills (marked `:skill`) inject specialized instructions:

| Skill        | Purpose                                                                                    |
| ------------ | ------------------------------------------------------------------------------------------ |
| `/work-loop` | Iterate on a task until complete                                                           |
| `/plan`      | Strategic planning protocol                                                                |
| `/wiki`      | Wiki knowledge base operations                                                             |
| `/deepinit`  | Deep project initialization                                                                |
| `/swarm`     | Launch a supervised tmux split-screen swarm with shared `.tinycode/swarm/<id>` persistence |
| `/cancel`    | Cancel current operation                                                                   |

## oh-my-tiny

oh-my-tiny is a companion plugin providing extended orchestration tools built in to tinycode. It adds notepad, wiki, project memory, state management, and AST grep tools — all stored under `.tinycode/` in the project directory. The omt agents (architect, debugger, executor, etc.) are available via `/ask <agent>` once the plugin is active.

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

| Project                                                                | Description                                                              | Repository                                  |
| ---------------------------------------------------------------------- | ------------------------------------------------------------------------ | ------------------------------------------- |
| [tinycode-container](https://github.com/bobbyjohnstx/tinycode-container)       | Container image packaging tinycode + oh-my-tiny for Kubernetes/OpenShift | `github.com/bobbyjohnstx/tinycode-container`    |
| [tinycode-operator](https://github.com/bobbyjohnstx/tinycode-operator) | OpenShift Operator managing `TinycodeInstance` CRs                       | `github.com/bobbyjohnstx/tinycode-operator` |

## License

MIT
