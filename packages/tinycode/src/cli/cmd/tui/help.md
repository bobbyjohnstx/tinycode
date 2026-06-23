# tinycode Help

## What is tinycode?
Local-LLM-first AI coding assistant. Runs in your terminal against Ollama, vLLM, MaaS, or cloud API providers.

## Agents
Type `/ask <agent> <prompt>` to invoke a specialized subagent:

### Built-in
| Agent   | Role |
|---------|------|
| explore | Fast codebase search (grep, glob, file reading, bash). Specify thoroughness: "quick", "medium", or "very thorough". |
| scout   | External research — clone repos, read library docs, fetch URLs. Never modifies workspace. |
| general | General-purpose agent for research and multi-step tasks. Fans out work in parallel. |

### Custom agents (`.tinycode/agent/`)
| Agent               | Role |
|---------------------|------|
| agent-reviewer      | Validates agent prompt definitions against the style guide |
| analyst             | Pre-planning requirements analysis — catches gaps before planning |
| architect           | Read-only code analysis and architectural guidance |
| code-reviewer       | Severity-rated code review with SOLID principle checks (READ-ONLY) |
| code-simplifier     | Simplifies recently modified code without changing behavior |
| critic              | Quality gate — multi-perspective review of plans and code (READ-ONLY) |
| debugger            | Root-cause analysis, regression isolation, stack trace analysis |
| designer            | UI/UX designer-developer — production-grade interfaces |
| document-specialist | External documentation and reference specialist |
| executor            | Focused task executor — implement code changes precisely as specified |
| git-master          | Git expert for atomic commits, rebasing, and history management |
| planner             | Strategic planning — interview, gather requirements, produce work plans |
| qa-tester           | Interactive CLI testing — spin up services, verify behavior via tmux |
| scientist           | Data analysis and research — hypothesis-driven, evidence required |
| security-reviewer   | Security vulnerability detection (OWASP Top 10, secrets, CVEs) (READ-ONLY) |
| test-engineer       | Test strategy, integration/e2e coverage, TDD workflows |
| tracer              | Evidence-driven causal tracing with competing hypotheses (READ-ONLY) |
| verifier            | Evidence-based verification — confirm completion with fresh test output (READ-ONLY) |
| writer              | Technical documentation — README, API docs, architecture docs, comments |

Primary agent modes (switch with tab or `<leader>a`):
- build — default, full tool access (read/write/edit/bash)
- plan — planning mode, edit tools disabled; writes plans to .tinycode/plans/

Custom agents can be defined in config under the `agent` key or generated via the command palette.

## Slash Commands
Type `/` to open autocomplete and pick from available commands:

| Command              | Description |
|----------------------|-------------|
| /init                | Guided AGENTS.md setup for the current project |
| /review              | Review changes (commit, branch, or PR); defaults to uncommitted |
| /ask \<agent\> \<prompt\> | Invoke a subagent — see agent list above |
| /swarm               | Launch supervised tmux split-screen workers with shared `.tinycode/swarm/<id>` persistence |
| /compact, /summarize | Compact the session (summarize context to free space) |
| /rename              | Rename the current session |
| /undo                | Revert to the previous user message |
| /redo                | Restore a reverted message |
| /timeline            | Jump to a message in the timeline |
| /fork                | Fork the session from a selected message |
| /export              | Export the session transcript to a file |
| /copy                | Copy the session transcript to clipboard |
| /share               | Share the session (generates a URL) |
| /timestamps          | Toggle message timestamps |
| /thinking            | Toggle thinking block visibility |

MCP prompts and local skills also appear as slash commands.

## @ File References
Type `@` to reference files. The autocomplete shows:
- Files in the current project (fuzzy-matched, frecency-sorted)
- Named references configured under the `reference` key (e.g. `@my-lib/src/index.ts`)
- MCP resources exposed by connected servers

Append `#<line>` or `#<start>-<end>` to reference a line range: `@src/main.ts#10-25`

`@` references files only. To invoke an agent, use `/ask <agent>`.

## Key Keybindings
Leader key defaults to `ctrl+x`. All `<leader>` bindings require pressing the leader first.

| Key              | Action |
|------------------|--------|
| ctrl+p           | Open command palette |
| f1               | Open help |
| escape           | Interrupt current session / close dialog |
| tab              | Cycle to next agent |
| shift+tab        | Cycle to previous agent |
| return           | Submit prompt |
| shift+return     | Insert newline |
| \<leader\>n      | New session |
| \<leader\>l      | List sessions |
| \<leader\>g      | Session timeline (jump to message) |
| \<leader\>c      | Compact session |
| ctrl+r           | Rename session |
| \<leader\>j      | Go to first child (subagent) session |
| \<leader\>k      | Go to parent session |
| \<leader\>u      | Undo last message |
| \<leader\>r      | Redo undone message |
| \<leader\>y      | Copy last assistant message |
| \<leader\>x      | Export session transcript |
| \<leader\>a      | List agents |
| \<leader\>m      | List models |
| \<leader\>b      | Toggle sidebar |
| \<leader\>t      | Switch theme |
| \<leader\>e      | Open external editor |
| \<leader\>s      | View status |
| \<leader\>h      | Toggle code block concealment |
| f2               | Cycle to next recently used model |
| shift+f2         | Cycle to previous recently used model |
| ctrl+t           | Cycle model variants |
| pageup           | Scroll messages up one page |
| pagedown         | Scroll messages down one page |
| ctrl+g / home    | Jump to first message |
| end              | Jump to last message |
| ctrl+alt+k       | Toggle which-key panel (shows all bindings) |
| ctrl+c           | Clear input / exit |

All keybindings can be overridden in `tui.json` under the `keybinds` key.

## Connecting Providers
Open the model picker with `<leader>m` to select a model and connect providers.

Local providers (auto-discovered):
- Ollama: runs on localhost:11434 — just start `ollama serve`
- vLLM: runs on localhost:8000
- MaaS: set TINYCODE_MAAS_HOST + TINYCODE_MAAS_API_KEY env vars

Cloud providers (API key required):
- Anthropic, OpenAI, Google — enter API key via the model picker

## Configuration
Config merges from: `~/.config/tinycode/` (global) → `.tinycode/` (project)

Key config fields in `config.json`:
```json
{
  "model": "ollama/llama3.2",
  "small_model": "ollama/llama3.2",
  "default_agent": "build",
  "provider": {
    "my-server": {
      "npm": "@ai-sdk/openai-compatible",
      "options": { "baseURL": "https://your-server/v1", "apiKey": "your-key" }
    }
  },
  "mcp": {
    "my-server": { "type": "stdio", "command": "npx", "args": ["-y", "my-mcp-server"] }
  },
  "lsp": true,
  "skills": { "paths": ["~/my-skills"] },
  "permission": { "guardrail": "allow" },
  "instructions": ["Always use TypeScript strict mode"],
  "reference": {
    "my-lib": { "type": "local", "path": "/path/to/my-lib" }
  }
}
```

- `model` — default model (e.g. "maas/qwen3-14b", "ollama/llama3.2")
- `small_model` — model for lightweight tasks like title generation
- `default_agent` — which primary agent to start with (default: build)
- `provider` — custom/MaaS provider definitions
- `mcp` — MCP server configurations
- `lsp` — enable language servers (true/false or per-language config)
- `skills.paths` — additional directories to load skills from
- `permission` — tool permission rules (allow/ask/deny per tool type)
- `instructions` — extra system prompt instructions appended to all agents
- `reference` — named external directories or git repos for `@alias` autocomplete
- `command` — custom slash commands with template strings
- `agent` — per-agent overrides (model, prompt, steps, temperature, etc.)

TUI-specific settings (theme, keybinds, layout) go in `tui.json`, not `config.json`.

## MCP Support
tinycode supports the Model Context Protocol for connecting external tools and data sources.

Configure MCP servers in config.json under the `mcp` key:
```json
{
  "mcp": {
    "filesystem": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/path/to/dir"]
    }
  }
}
```

MCP tools become available to agents automatically. MCP prompts appear as slash commands. MCP resources appear in `@` autocomplete.

## LSP Support
Set `"lsp": true` in config.json. Supported servers: typescript, pyright, gopls,
rust-analyzer, clangd, bash, yaml, dockerfile, and more.
Install language servers via npm/brew/cargo as needed.
Set TINYCODE_DISABLE_LSP_DOWNLOAD=1 to prevent automatic binary downloads (air-gapped environments).

## Permissions & Guardrails
tinycode asks for approval before:
- Destructive commands (rm, git reset --hard, git push --force)
- Reading secrets files (.env, *.key, *.pem)
- Accessing files outside the project directory

Configure via the `permission` key in config.json:
```json
{ "permission": { "guardrail": "allow" } }
```

## Skills
Skills are slash commands backed by prompt templates. They appear in `/` autocomplete.
Load additional skills by setting `skills.paths` in config.json.
Skills can be local (`.tinycode/skills/`) or global (`~/.config/tinycode/skills/`).

## Tips & Techniques

### Context window management
- Use /compact before large tasks — don't wait for auto-compaction
- Split long tasks across multiple sessions; each starts with fresh context
- Use /ask explore before /ask general — let explore do the reading, general just orchestrates
- Prefer lsp_document_symbols over reading whole files — structure without token cost

### Agent selection
- /ask explore for codebase search — read-only, fast, no risk of edits
- /ask scout to research external repos or library docs without cloning into your workspace
- /ask general to fan out parallel subtasks ("audit all X" requests)

### Prompting local models
- Be explicit about output format: "list the files", "respond in JSON", "one sentence per item"
- End prompts with a stop condition: "stop when complete, don't ask what's next"
- Avoid open-ended questions — local models ramble and waste context
- State the constraint when needed: "be concise, this model has a small context window"

### Air-gapped / local environments
- Set TINYCODE_DISABLE_LSP_DOWNLOAD=1 — prevents hang on LSP binary fetch
- Pull all Ollama models in advance: ollama pull qwen2.5 llama3.2
- config.json model setting is more reliable than TUI selection for cold starts
- Use MaaS for complex reasoning; Ollama for sensitive code that stays fully local

### Which-key
Press `ctrl+alt+k` to open the which-key panel — shows all registered keybindings grouped
by category. Use `ctrl+alt+left/right` to navigate groups.

## Getting Help
Type a question in the prompt — tinycode's AI knows its own features.
