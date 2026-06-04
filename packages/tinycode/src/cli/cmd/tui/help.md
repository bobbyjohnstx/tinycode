# tinycode Help

## What is tinycode?
Local-LLM-first AI coding assistant. Runs in your terminal against Ollama, vLLM, MaaS, or cloud API providers.

## Agents
Type `@agent-name` to invoke a specialized subagent:

| Agent | Role |
|-------|------|
| @explore | Fast codebase search (grep, glob, file reading) |
| @deep-explore | LSP+AST aware search — finds symbols, references, call graphs |
| @scout | External research — clone repos, read library docs |
| @architect | Read-only code analysis and architectural guidance (never modifies files) |
| @debugger | Root-cause analysis and bug fixing |
| @executor | Implement a scoped task precisely — smallest viable diff |
| @planner | Structured planning interview — writes plans to .omc/plans/ |
| @general | General-purpose assistant for questions and parallel tasks |

Primary agent modes (switch in the agent bar):
- build — default, full tool access (read/write/edit/bash)
- plan — planning mode, disables all edit tools

## Skills
Type `/skill-name` or use `/skills` to pick from the list:

| Skill | Purpose |
|-------|---------|
| /work-loop | Iterate on a task until complete — read, act, verify, repeat |
| /plan | Strategic planning protocol before implementing |
| /wiki | Knowledge base — store and retrieve project notes |
| /deepinit | Deep codebase initialization and analysis |
| /cancel | Cancel the current operation |
| /team | Launch a multi-agent team for parallel work |
| /improve-codebase-architecture | Architecture review and deepening opportunities |

## Key Keybindings
| Key | Action |
|-----|--------|
| ctrl+p | Toggle plan mode |
| ctrl+x j | Switch to first child subagent session |
| ctrl+c | Cancel current operation |
| esc | Close dialog / go back |
| ctrl+r | New session |
| ctrl+f | Search sessions |

## Connecting Providers
Type `/connect` to open the provider connection dialog.

Local providers (auto-discovered):
- Ollama: runs on localhost:11434 — just start `ollama serve`
- vLLM: runs on localhost:8000
- MaaS: set TINYCODE_MAAS_HOST + TINYCODE_MAAS_API_KEY env vars

Cloud providers (API key required):
- Anthropic, OpenAI, Google — enter API key in /connect dialog

Config file (~/.config/tinycode/config.json):
{
  "model": "ollama/llama3.2",
  "provider": {
    "maas": {
      "npm": "@ai-sdk/openai-compatible",
      "options": { "baseURL": "https://your-server/v1", "apiKey": "your-key" }
    }
  }
}

## Multi-Agent Teams
Launch a team of parallel workers:
  bun script/team.ts --team my-team --workers 3 --task "Audit all files for X"

Or from the TUI, invoke `/team` and describe your task.

## Configuration
Config merges from: ~/.config/tinycode/ (global) -> .opencode/ (project)

Key config options:
- model — default model (e.g. "maas/qwen3-14b", "ollama/llama3.2")
- lsp — enable language servers (true/false or per-server config)
- skills.paths — additional skill directories
- permission — tool permission rules (allow/ask/deny per tool type)

## LSP Support
Set "lsp": true in config.json. Supported servers: typescript, pyright, gopls,
rust-analyzer, clangd, bash, yaml, dockerfile, and more.
Install language servers via npm/brew/cargo as needed.

## Permissions & Guardrails
tinycode asks for approval before:
- Destructive commands (rm, git reset --hard, git push --force)
- Reading secrets files (.env, *.key, *.pem)
- Accessing files outside the project directory

Configure via the "permission" key in config.json:
{ "permission": { "guardrail": "allow" } }

## Getting Help
Type a question in the prompt — tinycode's AI knows its own features.
