# tinycode User Guide

Complete guide from beginner to power user. Read this first to understand tinycode, then reference other docs (cheatsheet, troubleshooting, architecture) as needed.

**Table of Contents**
1. [Introduction](#introduction)
2. [Quick Start](#quick-start)
3. [The Terminal UI](#the-terminal-ui)
4. [Sessions](#sessions)
5. [Agents](#agents)
6. [Skills](#skills)
7. [Models](#models)
8. [Configuration](#configuration)
9. [Web UI & Desktop](#web-ui--desktop)
10. [Advanced Workflows](#advanced-workflows)
11. [Plugin System](#plugin-system)
12. [Performance](#performance)
13. [Troubleshooting](#troubleshooting)
14. [Further Reading](#further-reading)

---

## Introduction

**tinycode** is an open-source AI coding assistant that runs on your machine. It gives you full access to your code, no cloud lock-in, and complete control over which models you use.

### What makes it different

- **Privacy first**: Your code never leaves your machine by default
- **Local LLMs**: Runs models via Ollama, vLLM, or ramalama on your hardware
- **Full tool access**: Read, write, edit files; run shell commands; search code
- **Agents**: Specialized personas (architect, debugger, executor, etc.) for different tasks
- **Sessions**: Organize work as a tree of conversations with parent-child relationships
- **Extensible**: Add plugins, MCP servers, and custom tools

### System Requirements

- **Bun 1.1+** — [install here](https://bun.sh)
- **Git**
- One of:
  - **Ollama** (recommended) — `ollama.ai` for local inference
  - **vLLM** — fast inference via OpenAI-compatible API
  - **API key** to OpenRouter, Anthropic, OpenAI, Google, or other cloud provider

---

## Quick Start

### 1. Install tinycode

```bash
# npm (requires Node.js 20+)
npx tinycode-ai@latest

# Homebrew (macOS/Linux)
brew install bobbyjohnstx/tap/tinycode

# From source (for development)
git clone https://github.com/bobbyjohnstx/tinycode.git
cd tinycode
bun install
bun dev
```

### 2. Start an LLM (local option)

If you prefer to run locally without cloud:

```bash
# In a separate terminal
ollama serve

# In another terminal, pull a model (3–5 min)
ollama pull qwen3.5:9b  # recommended — 14/15 on benchmark, excellent tool calling
# Alternatives: north-mini-code-1.0, gemma4:12b, mistral, neural-chat
```

**Why qwen3.5:9b?** It scores highest on tinycode's 5-task benchmark (14/15) with strong tool-call accuracy. Smaller models (<7B) may not support tool calling at all. See [Model Compatibility](docs/model-compatibility.md) for the full benchmark.

**Skip this** if you're using cloud providers. tinycode auto-discovers Ollama on startup.

### 3. Run tinycode

```bash
tinycode
```

You'll see:
- **Session tree** on the left (toggled with `<leader>b`)
- **Conversation area** in the center
- **Input prompt** at the bottom

On startup, tinycode:
- Auto-discovers Ollama models
- Warms up your configured model (pre-loads to GPU memory, verifies tool-call support)
- Loads the default **build** agent (full tool access)

### 4. Select a model

Press `<leader>m` (Ctrl+X, then M):

```
List Models
────────────
ollama/qwen3.5:9b
ollama/mistral
anthropic/claude-opus
```

Select with arrow keys, press Enter.

### 5. Your first conversation

Type a simple prompt:

```
Explain what this repository does in 2 sentences.
```

Press Enter. tinycode will:
1. Read files in your current directory
2. Ask the LLM to analyze them
3. Stream the response in real time

Try more:

```
What files are in src/?
Write a bash script that lists all .ts files
```

---

## First 5 Minutes

Four workflows that show what tinycode does in practice. Copy-paste these after installing.

### Debug a failing test

```bash
$ tinycode /path/to/project

# In the TUI, type:
The test in auth_test.go is failing with "token expired."
Find the root cause and fix it.

# Or use the debug skill directly:
/debug auth_test.go is failing with "token expired"
```

### Plan a feature before writing code

```bash
$ tinycode /path/to/project

# Press Tab to switch to the plan agent, then:
Add a /health endpoint that returns pod name,
uptime, and connected LLM providers.

# tinycode writes a plan to .tinycode/plans/
# Review it, approve, switch to build mode to execute
```

### Review a pull request

```
# Ask the code-reviewer agent:
/ask code-reviewer Review the diff on the current branch
against main. Focus on correctness and security.
```

### Explore an unfamiliar codebase

```
# Ask the architect agent:
/ask architect How is authentication handled in this project?
Trace from the HTTP handler to the token validation.
```

---

## The Terminal UI

### Keyboard Shortcuts

**Leader key** = `Ctrl+X` by default (customize in `~/.config/tinycode/config.json`).

| Key | Action |
|-----|--------|
| `Ctrl+C` / `Ctrl+D` / `<leader>q` | Exit tinycode |
| `F1` | Help dialog |
| `Ctrl+P` | Command palette (commands, agents, sessions, skills) |
| `<leader>a` | List agents |
| `Tab` / `Shift+Tab` | Next/previous agent |
| `<leader>m` | Switch model |
| `F2` / `Shift+F2` | Cycle next/previous recent model |
| `<leader>o` | List all sessions |
| `<leader>n` | New session |
| `<leader>h` / `<leader>l` | Previous / next sibling session |
| `<leader>j` / `<leader>k` | Go to first child / parent session |
| `<leader>b` | Toggle session tree sidebar |
| `<leader>e` | Open external editor (your `$EDITOR`) |
| `<leader>t` | Switch theme |
| `<leader>s` | View status |
| `<leader>c` | Compact session (removes noise) |
| `<leader>d` | Toggle diff viewer |
| `<leader>x` | Export session to JSON/HTML |
| `<leader>y` | Copy message |
| `<leader>u` / `<leader>r` | Undo/redo message |
| `<leader>;` | Toggle code block concealment (collapse code, focus on text) |
| `Ctrl+R` | Rename session |
| `Ctrl+D` | Delete session |
| `Escape` | Interrupt current operation |
| `Ctrl+F` | Pin/unpin session (keeps visible when switching) |
| `<leader>1` – `<leader>9` | Quick-switch to session slot 1–9 |
| `Ctrl+Alt+K` | Toggle which-key panel (shows available shortcuts) |

### The Which-Key Panel

Press `Ctrl+Alt+K` to see all available keyboard shortcuts for your current context. This is the fastest way to discover commands.

### Prompt Input

When typing a prompt, you can reference files:

```
@src/auth.ts analyze the login flow
@Dockerfile review for best practices
```

Use `/` to trigger slash commands (skills):

```
/debug why are my tests failing?
/trace the root cause of this error
/verify this change actually works
```

Type `Tab` to autocomplete agents and skills.

### Status Line

Press `<leader>s` to see:
- Current model and warmup status
- Active session and path
- Token usage (context window percentage)
- Server health
- Recent errors (if any)

---

## Sessions

Sessions are conversations. They form a **tree structure** where child sessions can inherit context from parents.

### Creating & Navigating Sessions

| Key | Action |
|-----|--------|
| `<leader>n` | Create new session (as child of current) |
| `<leader>o` | List all sessions |
| `<leader>h` / `<leader>l` | Previous / next sibling session |
| `<leader>j` | Go to first child session |
| `<leader>k` | Go to parent session |

### Session Types

- **Regular sessions**: Named conversations stored in the database
- **Pinned sessions**: Visible in the session sidebar even when you switch away (press `Ctrl+F` to pin)
- **Quick-switch slots**: Slots 1–9 accessible via `<leader>1` through `<leader>9`

### Session Hierarchy

Sessions organize into a tree. Use this for structured work:

```
Planning Session
  ├─ Feature Implementation
  │   ├─ UI Component
  │   └─ Tests
  └─ Code Review
```

Create a parent session, then spawn child sessions for subtasks. Parent context flows down, so children inherit the conversation state.

### Session Operations

| Key | Action |
|-----|--------|
| `Ctrl+R` | Rename |
| `Ctrl+D` | Delete |
| `Ctrl+F` | Pin/unpin |
| `<leader>c` | Compact (summarize old turns to save tokens) |
| `<leader>x` | Export to JSON or HTML |

### Compaction

When a session gets long, context approaches the model's limit. Press `<leader>c` to **compact**: summarize old messages, preserve recent context, and shrink the session. Compaction:
- Preserves file read/write operations (tracks which files were modified)
- Masks old observations (replaces long tool outputs with summaries)
- Issues a warning after 3+ compactions (suggesting a new session or subagent approach)
- Logs token counts for diagnostics

---

## Agents

Agents are specialized personas for different tasks. All agents share the same tools but have distinct system prompts.

### Built-in Agents

| Agent | Use When | Tools |
|-------|----------|-------|
| `build` | General coding (default) | Full access (read, write, edit, shell, grep, glob, LSP, MCP) |
| `plan` | Breaking down work | Read-only, writes only to `.tinycode/plans/` |
| `architect` | Analyzing design, reviewing structure | Read-only (grep, glob, read, bash) |
| `code-reviewer` | Severity-rated code review with SOLID checks | Read-only |
| `code-simplifier` | Refactoring recent changes for clarity | Full access to edit/write |
| `critic` | Multi-perspective quality review | Read-only |
| `debugger` | Root-cause analysis for bugs | Full access |
| `designer` | Building UI/UX | Full access |
| `document-specialist` | Understanding external libraries and APIs | Read-only + web fetch |
| `executor` | Focused task implementation | Full access |
| `explore` | Fast codebase search | Read-only + grep/glob |
| `git-master` | Git history, rebasing, atomic commits | Git + shell |
| `scientist` | Data analysis, evidence-driven research | Full access |
| `security-reviewer` | Vulnerability detection | Read-only |
| `test-engineer` | Test strategy and TDD | Full access |
| `tracer` | Evidence-driven causal tracing | Full access |
| `verifier` | Confirmation that work is actually complete | Full access |
| `workspace` | Development environment setup | Full access |
| `writer` | Technical documentation | Full access |

### Switching Agents

Press `Tab` to cycle to the next agent. Press `<leader>a` to see all agents and pick one.

Or use `/ask <agent> <prompt>` inline:

```
/ask architect design the auth flow
/ask test-engineer write tests for this component
/ask debugger why is this test failing?
```

### Two Special Modes

**build**: Full tool access. Use for general coding.

**plan**: Read-only mode that only writes to `.tinycode/plans/`. Use this when you want the LLM to plan without modifying code:

```
<leader>a
Select 'plan'
Break down the refactor into steps
(writes to .tinycode/plans/refactor-plan.md)
```

Then spawn a child session and use `build` to execute the plan.

---

## Skills

Skills are slash commands that inject specialized instructions. Type `/` to see the list.

### Built-in Skills

| Skill | Purpose |
|-------|---------|
| `/debug` | Isolate the single most-likely root cause for a known failure |
| `/trace` | Evidence-driven causal tracing with competing hypotheses |
| `/verify` | Confirm changes work before claiming completion |
| `/ai-slop-cleaner` | Clean up AI-generated code with regression-safe deletion-first workflow |
| `/council` | Answer a question through five advisory lenses, then converge |
| `/configure-notifications` | Set up Telegram, Discord, or Slack alerts |
| `/deepinit` | Generate per-directory AGENTS.md files across the codebase |
| `/mcp-setup` | Configure MCP servers via guided menu |
| `/remember` | Triage findings to memory surfaces |
| `/tc-doctor` | Full diagnostic (14 checks: Ollama, model health, tool-call probe, RAM fit, etc.) |

### Skill Examples

**Debug a failing test:**

```
My tests are failing. Why?
/debug
```

The debugger will focus on root-cause analysis without trying to fix everything.

**Verify a change:**

```
I think this fix works. Can you check?
/verify
```

The verifier will run tests, check build, and report only what was actually proven.

**Diagnose system issues:**

```
/tc-doctor
```

Runs 14 checks in pure bash: directory structure, agents, system tools, Ollama install/health, model availability, tool-call support, Mac-specific checks (Metal, Rosetta), vLLM/custom provider health, and disk space. No Python required.

---

## Models

tinycode supports local models (via Ollama, vLLM, ramalama) and cloud providers (Anthropic, OpenAI, Google, OpenRouter, etc.).

### Selecting a Model

Press `<leader>m` to list available models. Arrow keys to navigate, Enter to select.

**Model warmup**: On startup, tinycode sends a warmup probe to Ollama to:
- Pre-load the model into GPU memory
- Verify tool-call support
- Display result in a toast or footer message

Look for: `"qwen3.5:9b ready — tool calling supported"`.

### Model Compatibility

tinycode runs a 5-task benchmark to measure model quality. See [Model Compatibility](docs/model-compatibility.md) for full results.

**Reference scores** (from M1 Pro 32GB):

| Model | Score | Speed | Best For |
|-------|-------|-------|----------|
| qwen3.5:9b | 14/15 | 5.5 min | Recommended for 32GB+ RAM |
| north-mini-code-1.0 (MoE) | 12/15 | 5.5 min | Trade-off: speed vs accuracy |
| qwen2.5:7b | 9/15 | 2 min | 16GB RAM |
| llama3.2:3b | 6/15 | 2.5 min | 8GB RAM or less |
| Claude Opus 4 | 15/15 | Instant | Cloud ceiling (reference) |

**Tier definitions**:
- **Full Agentic (12–15)**: Reliable tool calling, suitable for production workflows
- **Limited (8–11)**: Inconsistent tool usage, may require manual intervention
- **Chat Only (4–7)**: Minimal tool usage, primarily text responses
- **Not Recommended (0–3)**: Unreliable or non-functional

### Model Shortcuts

After listing models (`<leader>m`):

| Key | Action |
|-----|--------|
| `Ctrl+F` | Toggle favorite status |
| `Ctrl+A` | Show provider list |
| `Return` | Select |

### Favorites

Mark frequently used models as favorites. They appear at the top of the list.

### Tool-Call Support

tinycode detects tool-call capability during warmup. Models with `capabilities.toolcall=false` are used without tools (text-only responses). The warmup probe verifies support and logs the result.

**Models with tool calling**: qwen3.5:9b, north-mini-code-1.0, gemma4:12b.

**Models without**: granite, codellama, deepseek-r1 (distilled) — these all score 5/15 with zero tool calls.

---

## Configuration

tinycode reads config from three sources (in order of precedence):

1. Environment variables (`TINYCODE_*`)
2. Global config: `~/.config/tinycode/config.json` (or `tinycode.json`, `tinycode.jsonc`)
3. Project config: `.tinycode/tinycode.json` (in the current or parent directory)

### Global Config Example

```json
{
  "model": "ollama/qwen3.5:9b",
  "lsp": true,
  "animations": true,
  "server": {
    "port": 4096,
    "hostname": "127.0.0.1"
  },
  "keybinds": {
    "leader": "ctrl+x"
  }
}
```

### Common Options

| Option | Default | Purpose |
|--------|---------|---------|
| `model` | `ollama/qwen3.5:9b` | Default LLM model |
| `lsp` | `true` | Enable Language Server Protocol (code intelligence) |
| `animations` | `true` | Animate TUI transitions |
| `server.port` | `4096` | HTTP API server port |
| `server.hostname` | `127.0.0.1` | Server bind address |
| `keybinds.leader` | `ctrl+x` | Leader key for shortcuts |
| `subagent_depth` | `1` | Maximum nesting depth for subagents (prevents infinite recursion) |

### Provider Configuration

#### Ollama (auto-discovered)

tinycode auto-detects Ollama at `localhost:11434`. To use a different host:

```bash
export TINYCODE_OLLAMA_HOST=http://your-host:11434
```

#### ramalama (container-based)

```bash
export TINYCODE_RAMALAMA_HOST=http://localhost:8080
ramalama serve ollama://qwen3.5:9b
```

#### vLLM (fast inference)

```bash
# Start vLLM on port 8000
python -m vllm.entrypoints.openai.api_server --model qwen3.5:9b

# Set env var if not on localhost:8000
export TINYCODE_VLLM_URLS=http://your-vllm-host:8000
```

#### Cloud Providers

Set API keys as environment variables, or use `/connect` in the TUI to enter them interactively:

```bash
export OPENROUTER_API_KEY=your-key    # auto-discovers 300+ models
export ANTHROPIC_API_KEY=your-key
export OPENAI_API_KEY=your-key
```

Then select the model via `<leader>m`.

**OpenRouter** is auto-discovered when `OPENROUTER_API_KEY` is set — tinycode fetches available models from the OpenRouter API, filters to tool-capable models, and maps pricing and capabilities automatically. Generation costs are tracked per-request via OpenRouter's billing API and shown in the sidebar.

### Provider Filtering

Control which providers appear in the model list:

```json
{
  "enabled_providers": ["ollama", "vllm"],
  "disabled_providers": ["anthropic"]
}
```

Filters apply during discovery, completely hiding disabled providers from the list.

### Local Storage

- **Database**: `~/.local/share/tinycode/tinycode.db` (XDG_DATA_HOME)
- **Config**: `~/.config/tinycode/config.json` (XDG_CONFIG_HOME)
- **Logs**: `~/.config/tinycode/logs/` (if enabled)

### LSP (Language Server Protocol)

Enable code intelligence with `"lsp": true`. This provides:
- Symbol definitions
- Type checking
- Go-to-definition
- Rename refactoring

---

## Web UI & Desktop

### Web UI

The TUI is fast, but tinycode also runs in a browser:

```bash
# Start server (if not already running)
tinycode serve

# In another terminal, start web UI
bun run --cwd packages/app dev
# or: tinycode web (starts both)
```

Open `http://localhost:4096`. Same features as TUI: agents, skills, sessions, model switching.

**Web UI features:**
- REST API for commands
- SSE (Server-Sent Events) for real-time updates
- WebSocket for terminal I/O
- Responsive design for mobile

### Desktop App (Electron)

For macOS/Linux/Windows:

```bash
# From repo root
bun run --cwd packages/desktop dev

# Or after building
tinycode desktop
```

**Features:**
- Native window chrome
- System tray integration
- Menu bar (macOS/Linux/Windows)
- Auto-update notifications
- Dock/taskbar integration
- Persistent window state (size, position, zoom)

**Security**:
- Content Security Policy headers
- No uncontrolled window open
- Shell.openExternal only allows http/https/mailto

### Syncing Between TUI and Web

All instances (TUI, web, desktop) share the same database and can connect to the same server. Changes sync in real time via the event bus.

```bash
# Terminal 1: Start the server
tinycode serve

# Terminal 2: TUI against same server
TINYCODE_SERVER_URL=http://localhost:4096 tinycode

# Browser: Web UI
open http://localhost:4096
```

---

## Advanced Workflows

### Multi-Agent Pipelines

Use agents in sequence for complex tasks:

```
<leader>n                              # New session
/ask planner break down the refactor

<leader>n                              # Child session
/ask executor implement step 1

<leader>n                              # Another child
/ask code-reviewer review the code

<leader>n                              # Child for tests
/ask test-engineer write tests
```

Navigate the tree with `<leader>h`/`<leader>l` (siblings), `<leader>j` (to child), `<leader>k` (to parent).

### Using Plan Mode

1. **Create a plan**:
   ```
   <leader>a
   Select 'plan'
   Break down this 2000-line refactor into small, atomic steps
   (writes to .tinycode/plans/refactor-plan.md)
   ```

2. **Execute step-by-step**:
   ```
   <leader>n                           # New child session
   <leader>a
   Select 'build' (full tool access)
   Implement step 1 from the plan
   ```

3. **Repeat for each step**, creating child sessions as you go.

### Session Hierarchy Patterns

**Code review workflow:**

```
Original Code
  └─ Code Review (architect reads, suggests)
      └─ Refactor (executor applies suggestions)
          └─ Tests (test-engineer writes)
```

**Bug fix workflow:**

```
Production Issue
  └─ Investigate (debugger finds root cause)
      └─ Fix Implementation (executor codes)
          └─ Regression Tests (test-engineer adds tests)
              └─ Verification (verifier confirms)
```

**Feature development:**

```
Feature Design
  ├─ API Design (architect)
  ├─ UI Implementation (designer + executor)
  ├─ Tests (test-engineer)
  └─ Documentation (writer)
```

### Quick-Switch Slots

Assign sessions to slots 1–9 for quick access:

```
<leader>1     # Jump to session 1
<leader>2     # Jump to session 2
# ... etc
```

Useful for toggling between active work sessions without scrolling the list.

### Pinned Sessions

Pin a session to keep it visible:

```
Ctrl+F        # Toggle pin on current session
```

Pinned sessions float to the top of the sidebar and stay visible when switching.

---

## Plugin System

Plugins extend tinycode with custom tools, providers, and features.

### Installing Plugins

Search the registry:

```bash
tinycode plugin-search code-formatter
tinycode plugin-search linter
```

Install by name or npm specifier:

```bash
tinycode plugin my-plugin-name          # Resolves via registry
tinycode plugin @myorg/custom-tool      # Raw npm specifier
```

Plugins install to `.tinycode/node_modules/` and are auto-loaded.

### Plugin Configuration

In `~/.config/tinycode/config.json`:

```json
{
  "plugin": [
    "npm-package-name",
    { "npm": "package-name", "options": { "key": "value" } },
    "./local-plugin.ts"
  ]
}
```

Plugins can:
- Define custom tools (read, write, shell, etc.)
- Add LLM providers
- Hook into session lifecycle
- Extend the TUI with custom components

### Built-in Plugins

**oh-my-tiny (omt)**: Extended orchestration tools

- `notepad` — persistent notes per session
- `wiki` — knowledge base
- `state` — session state management
- `ast-grep` — advanced code searching

Already included and active by default.

### Plugin Lifecycle Hooks

Plugins observe four session events:

- `session.start` — when a session is created
- `session.end` — when a session is deleted
- `session.switch` — when user switches to a different session
- `session.model.change` — when the model is changed

### Creating Plugins

Plugins are npm packages that export a `PluginModule`. The module's `server` function receives a `PluginInput` context and returns `Hooks` — an object declaring tools, event handlers, and lifecycle callbacks:

```typescript
import type { PluginModule } from "@tinycode/plugin"
import { tool } from "@tinycode/plugin/tool"

export default {
  server: async (ctx) => {
    return {
      tool: {
        "my-tool": tool({
          description: "Does something useful",
          args: {
            query: tool.schema.string().describe("The search query"),
          },
          async execute(args) {
            return `Result for: ${args.query}`
          },
        }),
      },
    }
  },
} satisfies PluginModule
```

The `tool()` helper uses Zod schemas for argument validation (available as `tool.schema`). The `execute` function receives validated args and an optional `ToolContext` with session metadata.

See [@tinycode/plugin](packages/plugin) for the full SDK.

---

## Performance

### Model Warmup

On startup, tinycode sends a warmup probe to Ollama via `/api/chat` with:
- A dummy tool-call request
- `keep_alive: "30m"` (keeps model loaded for 30 minutes)
- Results logged to footer/toast

This pre-loads the model into GPU memory so responses are fast.

### Context Compaction

When context approaches the model's limit, tinycode automatically compacts:

1. **Summarizes** old messages
2. **Masks** verbose tool outputs
3. **Preserves** recent context and file operations
4. **Tracks** which files were modified (deterministic, no LLM dependency)

Monitor token usage in `<leader>s` (View Status).

### Per-Agent Tool Scoping

Each agent declares which tools it needs. This reduces prompt size:

- **Read-only agents** (architect, critic): grep, glob, read, bash (~1,800 tokens)
- **Write agents** (executor, debugger): + edit (~2,700 tokens)

Smaller prompts = faster processing (4–8s vs 38s on 9B models).

### Tips for Speed

- Use **qwen3.5:9b** or **north-mini-code-1.0** for best speed/accuracy
- Disable animations: `"animations": false` in config
- Collapse code blocks: `<leader>;` to focus on text
- Compact sessions regularly: `<leader>c`
- Close memory-heavy apps (Docker, Chrome) to free RAM
- Use vLLM for faster inference than Ollama
- On Mac, verify Ollama is native arm64 (not Rosetta)

---

## Troubleshooting

See [troubleshooting.md](troubleshooting.md) for detailed solutions.

### Quick Diagnostics

**Run the full diagnostic:**

```
/tc-doctor
```

Checks 14 areas in pure bash:
- Directory structure
- Agents and skills
- System tools (git, bun, npm, oc)
- Ollama install/health
- Model availability (configured, recommended, RAM fit)
- Tool-call support probe
- Mac-specific checks (Metal, Rosetta, swap)
- vLLM/custom provider health
- tmux (for TUI)
- Disk space

**Common issues:**

| Problem | Solution |
|---------|----------|
| Model not found | `ollama pull <model>` first |
| Can't connect to Ollama | Verify `ollama serve` is running; check `TINYCODE_OLLAMA_HOST` |
| Tool calling not working | Run `/tc-doctor` to check model capability; try qwen3.5:9b |
| Responses are slow | `/tc-doctor` checks RAM vs model size, GPU acceleration, swap pressure |
| TUI is sluggish | Disable animations in config; collapse code blocks with `<leader>;` |
| Session not found | Check database: `ls ~/.local/share/tinycode/tinycode.db` |
| Permission prompts too frequent | Review what's being approved; or configure auto-approval in config |

### When You're Really Stuck

1. Run `/tc-doctor` — diagnoses most issues
2. Check [troubleshooting.md](troubleshooting.md) — detailed solutions for all categories
3. Search [GitHub Discussions](https://github.com/bobbyjohnstx/tinycode/discussions)
4. File an issue with:
   - Output of `/tc-doctor`
   - Exact steps to reproduce
   - Your config (`~/.config/tinycode/config.json`)
   - Relevant logs (if available)

---

## Further Reading

### Core Docs

- [Architecture](architecture.md) — How tinycode works under the hood (monorepo, session processor, tools, plugins, providers)
- [Getting Started](getting-started.md) — Step-by-step setup (condensed version of Sections 1–2 above)
- [Cheat Sheet](cheatsheet.md) — Quick keyboard reference and agent/skill lists
- [Troubleshooting](troubleshooting.md) — Detailed solutions for connection, performance, and tool-call issues

### Models & Benchmarks

- [Model Compatibility](docs/model-compatibility.md) — Full benchmark results (16 models tested, scoring breakdown)
- [Benchmark Guide](benchmark-guide.md) — How to run the 5-task benchmark on your hardware

### Advanced Topics

- [ACP Integration](acp-integration.md) — Agent Client Protocol for IDE integration
- [Use Cases](use-cases.md) — Deployment patterns (remote server, containers, OpenShift)
- [Why tinycode](why-tinycode.md) — Design philosophy vs alternatives

### Developer Docs

- See the repo's [CLAUDE.md](CLAUDE.md) for architecture patterns, testing, and development commands

---

**Ready to code?** Start with `tinycode` and pick a model via `<leader>m`. Or explore the [Cheat Sheet](cheatsheet.md) for keyboard shortcuts. Have questions? Run `/tc-doctor` or check [troubleshooting.md](troubleshooting.md).
