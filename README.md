# tinycode

A slim, local-LLM-first AI coding assistant. Runs air-gapped with zero cloud dependencies.

## What it is

tinycode is a terminal UI (TUI) coding agent that runs against **your own LLMs** — Ollama, vLLM, or any OpenAI-compatible MaaS server on your LAN. Cloud providers (Anthropic, OpenAI, Google) are supported via API key as a secondary option, but local inference is the primary use case.

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

tinycode auto-discovers Ollama (`localhost:11434`), vLLM (`localhost:8000`), and MaaS servers from environment variables at startup. Use `/connect` in the TUI to manually connect a provider.

## Architecture

Bun monorepo with Turborepo. Key packages:

| Package | Description |
|---|---|
| `packages/tinycode` | Core server, HTTP API, TUI, session processor |
| `packages/core` | Shared utilities, models, permissions |
| `packages/app` | SolidJS web UI |
| `packages/desktop` | Electron desktop app |
| `packages/plugin` | Plugin SDK (`@opencode-ai/plugin`) |
| `packages/sdk/js` | Auto-generated JS SDK |

See [CLAUDE.md](CLAUDE.md) for development guidance and [AGENTS.md](AGENTS.md) for coding style.

## Agents

Type `@` in the prompt to invoke a subagent:

| Agent | Role |
|---|---|
| `@explore` | Fast codebase search (grep/glob) |
| `@deep-explore` | LSP+AST aware search (symbols, references) |
| `@scout` | External research (clone repos, read docs) |
| `@architect` | Read-only code analysis and architectural guidance |
| `@debugger` | Root-cause analysis and bug fixing |
| `@executor` | Precise implementation of scoped tasks |
| `@general` | General-purpose assistant |

## Skills

Type `/` to see available slash commands. Skills (marked `:skill`) inject specialized instructions:

| Skill | Purpose |
|---|---|
| `/work-loop` | Iterate on a task until complete |
| `/plan` | Strategic planning protocol |
| `/wiki` | Wiki knowledge base operations |
| `/deepinit` | Deep project initialization |
| `/cancel` | Cancel current operation |

## oh-my-tiny

[oh-my-tiny](https://localhost:3000/bjohns/oh-my-tiny) is a companion tool providing extended orchestration. It runs as an MCP server providing state management, wiki, LSP, and AST grep tools. Configured in `.opencode/opencode.jsonc`.

## Building

```bash
# Build standalone binary for current platform
bun ./packages/tinycode/script/build.ts --single

# Output: packages/tinycode/dist/tinycode-darwin-arm64/bin/tinycode
```

## License

MIT
