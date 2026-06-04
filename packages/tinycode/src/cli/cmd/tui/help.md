# tinycode Help

## What is tinycode?
Local-LLM-first AI coding assistant. Runs in your terminal against Ollama, vLLM, MaaS, or cloud API providers.

## Agents
Type `@agent-name` to invoke a specialized subagent:

| Agent | Role |
|-------|------|
| @explore | Fast codebase search (grep, glob, file reading) |
| @deep-explore | LSP+AST aware search — finds symbols, references, call graphs. Steps limit controls depth (see docs/DEEP-EXPLORE-GUIDE.md) |
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

## Deep-Explore Tips

### Split technique for large codebases
A single @deep-explore session on a large project covers ~20-70% of the codebase depending
on the model's context window. For comprehensive architecture docs, split into two sessions:

Session 1 — Structure:
  @deep-explore Map every top-level directory in src/ — list what each contains and its
  purpose in one sentence. Cover ALL directories, not just the obvious ones.

Session 2 — Patterns:
  @deep-explore Identify the key design patterns: how services are defined, how dependency
  injection works, how errors are handled, how data flows from user input to LLM to tools.
  Read actual code examples for each pattern.

Then combine:
  @writer Combine these two exploration results into a complete ARCHITECTURE.md:
  [paste session 1 output]
  ---
  [paste session 2 output]

### Steps by model
The steps setting in .opencode/agent/deep-explore.md controls depth:
- Local 8B models (llama3.2, qwen2.5): 30-35 steps (context fills before steps exhaust)
- Mid models (gemma4:27b, qwen3-14b): 50-60 steps
- Large context (llama-scout-17b): 80-100 steps
See docs/DEEP-EXPLORE-GUIDE.md for the full lookup table.

## Tips & Techniques

### Context window management
- Run /compact before large tasks — don't wait for auto-compaction
- Split long tasks across multiple sessions; each starts with fresh context
- Use @executor for scoped implementation — smaller footprint than @build
- Prefer lsp_document_symbols over reading whole files — structure without token cost

### Agent selection
- @architect for analysis — read-only, zero risk of unwanted edits
- @debugger for root cause → @executor for the fix (two-agent pattern beats one long session)
- @general auto-fans out subtasks in parallel — good for "audit all X" requests
- @explore before @executor — let explore do the reading, executor just writes

### Prompting local models
- Be explicit about output format: "list the files", "respond in JSON", "one sentence per item"
- End prompts with a stop condition: "stop when complete, don't ask what's next"
- Avoid open-ended questions — local models ramble and waste context
- State the constraint when needed: "be concise, this model has a small context window"

### Skills
- /work-loop for iterative tasks: failing tests, debug loops, multi-step refactors
- /plan before any multi-file change — forces structure before the model starts editing
- /cancel immediately when the model goes off-track — stops before it exhausts context

### Air-gapped / local environments
- Set OPENCODE_DISABLE_LSP_DOWNLOAD=1 in .env — prevents hang on LSP binary fetch
- Pull all Ollama models in advance: ollama pull qwen2.5 llama3.2 gemma4
- config.json model setting is more reliable than TUI selection for cold starts
- Use MaaS for complex reasoning; Ollama for sensitive code that stays fully local

### Multi-agent teams (/team)
- Use for genuinely parallel workloads — not for sequential steps
- Workers don't share memory; each is an independent session with full tool access
- 2-3 workers is the practical limit for local 8B models (context × workers)
- See script/team.ts for invocation; docs/DEEP-EXPLORE-GUIDE.md for agent depth tuning

## Getting Help
Type a question in the prompt — tinycode's AI knows its own features.
