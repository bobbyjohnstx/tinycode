# Tinycode User Guide

A complete guide to tinycode -- the local-first AI coding assistant for the terminal.

**Table of Contents**

1. [Getting Started](#getting-started)
2. [The Interface](#the-interface)
3. [Slash Commands](#slash-commands)
4. [Keyboard Shortcuts](#keyboard-shortcuts)
5. [File References](#file-references)
6. [Agents](#agents)
7. [Subagents and Swarm Mode](#subagents-and-swarm-mode)
8. [Providers and Models](#providers-and-models)
9. [Sessions](#sessions)
10. [Configuration](#configuration)
11. [MCP Integration](#mcp-integration)
12. [LSP Integration](#lsp-integration)
13. [Plugins](#plugins)
14. [Web UI](#web-ui)
15. [Run Mode](#run-mode)
16. [CLI Reference](#cli-reference)
17. [Troubleshooting](#troubleshooting)

---

## Getting Started

### Prerequisites

- **Go 1.27+** -- [install here](https://go.dev/dl/)
- **make**
- A running LLM provider. [Ollama](https://ollama.com) is the easiest way to start:

```bash
ollama pull qwen3:8b    # Download a model
ollama serve            # Start the server (if not already running)
```

### Build and run

```bash
git clone https://github.com/bobbyjohnstx/tinycode-go.git
cd tinycode-go
make build
./dist/tinycode
```

The binary at `dist/tinycode` is self-contained -- no runtime dependencies, no Node.js, no separate server process.

### First launch

On startup you see an animated boot sequence that verifies each subsystem:

```
  ▀█▀ ▄  █▀▀▄ █  █  ╲
   █  █  █  █  ▀▀█    ╲
   ▀  ▀  ▀  ▀    ▀      ╲    █▀▀▀ █▀▀█ █▀▀█ █▀▀█
                           ╲  █    █  █ █  █ █
                             ╲▀▀▀▀ ▀▀▀▀ ▀▀▀▀ ▀▀▀▀

   ✓  Loading configuration
   ✓  Connecting to server
   ✓  Discovering providers · Ollama / qwen3:8b
   ✓  Loading agents · 15 agents
   ✓  Loading plugins · 2 plugins
   ✓  Loading sessions · 12 sessions
   ✓  Checking MCP servers · 1 servers
```

Each check mark confirms a subsystem is ready. If a check fails (e.g., no providers found), it shows a red X but tinycode still launches -- you can connect a provider later.

After boot, tips appear at the bottom:

```
→  / commands  ·  @ files  ·  tab agents  ·  ctrl+p palette
   ctrl+x sidebar/sessions  ·  shift+enter newline  ·  /help reference
```

### Connecting to a provider

If no model was auto-discovered, type `/connect` to open the provider/model selector. This is also how you switch models at any time.

### Your first conversation

Type a prompt at the bottom and press Enter:

```
explain what this project does
```

The model reads your project files using its built-in tools (read, grep, glob, bash, edit, write, apply_patch) and responds in the chat area. The `apply_patch` tool applies atomic multi-file edits via unified diff format. You can scroll up with PgUp to review long responses.

---

## The Interface

### Chat viewport

The main area shows the conversation: your prompts, the model's responses, tool calls, and their output. Responses stream in real-time with markdown rendering. Scroll with PgUp/PgDn or mouse wheel.

### Prompt input

At the bottom of the screen. Supports:

- **Enter** to submit
- **Shift+Enter** or **Alt+Enter** to insert a newline (for multi-line prompts)
- **/** to trigger slash command autocomplete
- **@** to trigger file path autocomplete
- **Tab/Shift+Tab** to cycle through agents
- **Up/Down arrow** in autocomplete to navigate suggestions
- **Ctrl+C** to clear input (or quit if already empty)

The prompt line shows the current agent name and model on the right side.

### Status bar

Below the prompt. Shows:

- Current working directory
- Active model and provider
- Agent name (color-coded per agent)
- An animated braille-wave spinner while the model is working
- "ctrl+x ..." hint when the leader key is pending
- **SAFE MODE** indicator (orange, bold) when `--safe-mode` is active

### Sidebar

Toggle with **Ctrl+X b**. The sidebar shows:

- **Context** -- token usage (input/output), percentage of context window used, cost spent, and provider balance remaining (if applicable)
- **MCP** -- connected MCP servers with status indicators (green dot = connected, red = error, gray = disconnected) and tool counts
- **Sessions** -- the 5 most recent titled sessions; click to switch
- **Metadata** -- current agent and model
- **Footer** -- working directory and tinycode version

---

## Slash Commands

Type `/` in the prompt to see the autocomplete list. Commands are handled either client-side (instant) or server-side (sent to the model as instructions).

### Client-side commands

These execute immediately without sending anything to the model.

| Command | Description |
|---------|-------------|
| `/connect` | Open the provider/model selector |
| `/theme` | Open the theme picker (with live preview) |
| `/compact` | Compact the current session context (summarize older messages to free tokens) |
| `/export` | Export the current session as a Markdown file in the working directory |
| `/export html` | Export the current session as an HTML file with syntax highlighting (alias: `/export-html`) |
| `/archive` | Soft-delete the current session (removes from session list, recoverable) |
| `/copy` | Copy the last assistant response to the clipboard |
| `/rename <title>` | Rename the current session |
| `/editor` | Open `$EDITOR` to compose a long prompt; contents are submitted on save+quit |
| `/editor @file.md` | Open a file in `$EDITOR` for direct editing |
| `/shell` | Drop into an interactive shell session; return to tinycode on exit |
| `/debug` | Open a diagnostics dialog showing config, paths, providers, and system info |
| `/thinking <level>` | Set the reasoning level: `off`, `low` (1k tokens), `medium` (4k), `high` (16k), `max` (128k) |
| `/thinking` | Show the current reasoning level |
| `/scoped-models` | Toggle model scoping -- mark favorite models so the model list only shows those |
| `/auto-approve` | Toggle auto-approve for the current session (skips tool permission prompts) |
| `/undo` | Revert the last AI file changes (snapshot-based) |
| `/redo` | Restore previously reverted changes |
| `/diff` | Show uncommitted git changes in the working directory |
| `/paste-image` | Paste an image from the clipboard for multimodal input (alias: `/image`) |
| `/mcp` | Open the MCP server management dialog (reconnect, view status) |
| `/help` | Open the command palette showing all keybindings and commands |
| `/exit` | Quit tinycode |

### Server-side commands

These are processed by the model. They show up in autocomplete alongside client commands.

| Command | Description |
|---------|-------------|
| `/ask <agent> <message>` | Route a prompt to a specific agent (e.g., `/ask architect design the auth flow`) |
| `/swarm <task>` | Split a task into subtasks and dispatch parallel subagents (see [Swarm Mode](#subagents-and-swarm-mode)) |
| `/work-loop <task>` | Iterate on a task autonomously until complete or blocked |
| `/review [target]` | Review changes (commit, branch, or PR) |
| `/init` | Guided project setup for AI-assisted development |

### Custom commands (skills)

If you have skill files in `~/.config/tinycode/skills/` or `.tinycode/skills/`, they appear as additional slash commands. Skills are markdown files with a `SKILL.md` in a named directory that inject specialized instructions into the prompt.

tinycode bundles 10 default skills that are always available (user/project skills override them by name):

| Skill | Description |
|-------|-------------|
| `debug` | Systematic debugging with reproduction steps and root-cause analysis |
| `verify` | Evidence-based completion checks before claiming work is done |
| `trace` | Causal tracing with competing hypotheses and discriminating probes |
| `remember` | Triage session findings across memory surfaces |
| `deepinit` | Deep project initialization and onboarding |
| `doctor` | Diagnose project health issues |
| `mcp-setup` | Guided MCP server configuration |
| `review` | Code review workflow |
| `plan` | Multi-step implementation planning |
| `test` | Test-driven development workflow |

### Image paste (multimodal input)

Use `/paste-image` (or `/image`) to paste an image from the system clipboard into the conversation. The image is encoded and sent as a multimodal content block alongside your next prompt, enabling the model to see screenshots, diagrams, or error output.

Requirements: the model must support vision/multimodal input, and the clipboard must contain image data (not a file path).

### Shell escape

Prefix a command with `!` to run it locally and feed the output to the model:

```
!git log --oneline -10
```

The shell command runs in the working directory and its output is included as context for the model's next response.

---

## Keyboard Shortcuts

### Global keys

| Key | Action |
|-----|--------|
| Ctrl+P | Open command palette |
| Ctrl+F | Open in-transcript search |
| Ctrl+C | Clear prompt input, or quit if empty |
| Ctrl+D | Quit |
| Escape | Interrupt the current model operation |

### Prompt input

| Key | Action |
|-----|--------|
| Enter | Submit prompt |
| Shift+Enter | Insert newline |
| Alt+Enter | Insert newline (alternative) |
| Tab | Cycle to next agent |
| Shift+Tab | Cycle to previous agent |
| F2 | Cycle to next recent model |
| Shift+F2 | Cycle to previous recent model |

### Navigation

| Key | Action |
|-----|--------|
| PgUp | Scroll conversation up |
| PgDn | Scroll conversation down |
| Mouse wheel | Scroll conversation |

### Leader key sequences

The leader key is **Ctrl+X**. Press it, then press a follow-up key within 500ms.

| Sequence | Action |
|----------|--------|
| Ctrl+X b | Toggle sidebar |
| Ctrl+X n | New session |
| Ctrl+X o | Open session list |
| Ctrl+X m | Open model selector |
| Ctrl+X a | Open agent list |
| Ctrl+X x | Export session as Markdown |
| Ctrl+X e | Open `$EDITOR` to compose a prompt |
| Ctrl+X d | Open diff viewer (uncommitted changes) |
| Ctrl+X t | Open theme picker |
| Ctrl+X i | Open MCP server management dialog |
| Ctrl+X y | Copy last response to clipboard |
| Ctrl+X u | Undo last AI file changes |
| Ctrl+X r | Redo reverted changes |

### In-transcript search

Press **Ctrl+F** to open the search bar at the top of the chat viewport. Search is case-insensitive and scans all message text and reasoning blocks.

| Key | Action |
|-----|--------|
| Ctrl+F | Open search (or close if already open) |
| Ctrl+N / Enter | Jump to next match |
| Ctrl+P | Jump to previous match |
| Escape / Ctrl+C | Close search |

The search bar shows the current match position (e.g., "3/12") and auto-scrolls the viewport to the message containing the match.

### Which-key panel

When you press **Ctrl+X** (the leader key), a floating panel appears in the bottom-right corner showing all available follow-up keys grouped by category:

- **Navigation** -- `b` (sidebar), `o` (session list), `n` (new session)
- **Edit** -- `e` ($EDITOR), `d` (diff viewer), `u` (undo), `r` (redo)
- **Tools** -- `a` (agent list), `m` (model list), `t` (theme picker), `i` (MCP servers)
- **Actions** -- `y` (copy last response), `x` (export session)

The panel is non-modal -- any keypress hides it and the key is forwarded normally.

### Terminal bell

tinycode rings the terminal bell (audible or visual, depending on your terminal settings) in two situations:

- When a task completes (the model finishes working)
- When a permission prompt appears (a tool needs approval)

This is useful when you switch to another window while the model is working -- you hear the bell when it needs attention.

### Dialog keys

When a dialog is open (agents, models, sessions, themes):

| Key | Action |
|-----|--------|
| Up / k | Move selection up |
| Down / j | Move selection down |
| Enter | Confirm selection |
| Esc / q | Close dialog |
| d | Toggle enable/disable (agent dialog only, non-native agents) |

---

## File References

Type `@` followed by a path to reference files in your prompt. An autocomplete dropdown shows files in the working directory.

```
@src/main.go explain the startup sequence
@internal/config/ what config options are available?
fix the bug in @pkg/plugin/protocol.go
```

How it works:

- **Autocomplete** triggers when you type `@` at the start of the input or after a space
- **Directories** appear with a trailing `/` and are shown first, highlighted in blue -- select one to drill down into its contents
- **Hidden files** (dotfiles) are excluded from the listing
- **Up to 20** items are shown at once
- Select with Up/Down arrows, confirm with Tab or Enter
- The file's contents are read and included as context when you submit the prompt

You can reference multiple files in one prompt:

```
compare @go.mod with @go.sum and check for inconsistencies
```

---

## Agents

Agents are specialized personas that share the same tools but have different system prompts and permission sets. The default agent is **build**, which handles general coding tasks and knows when to delegate.

### Built-in agents

| Agent | Mode | Description |
|-------|------|-------------|
| **build** | primary | Default agent. Full tool access. Handles simple tasks inline, delegates complex work to executor (implementation), architect (design), or critic (review) subagents. |
| **plan** | primary | Plan mode. Same prompt as build but all edit tools are denied -- for thinking without changing. |
| **architect** | all | Design decisions, API design, system-level trade-offs. Read-only analysis. |
| **code-reviewer** | all | Severity-rated code review with SOLID checks, logic defect detection, performance analysis. |
| **critic** | all | Multi-perspective quality review with gap analysis and pre-mortem. |
| **debugger** | all | Root-cause analysis. One hypothesis at a time, minimal diff fixes. |
| **executor** | all | Focused task implementation. Smallest viable diff, no scope creep. |
| **explore** | subagent | Fast codebase search. Read-only: grep, glob, read, bash only. |
| **general** | subagent | General-purpose research and multi-step tasks. |
| **git-master** | all | Git history management, rebasing, atomic commits. |
| **planner** | all | Interviews user, researches codebase, produces actionable work plans. |
| **scout** | subagent | External research. Clones dependency repos, fetches docs. |
| **security-reviewer** | all | OWASP Top 10, secrets detection, unsafe patterns, dependency CVEs. |
| **test-engineer** | all | Test strategy, coverage authoring, TDD workflows. |
| **verifier** | all | Evidence-based completion checks. No approval without fresh evidence. |
| **writer** | all | Technical documentation with verified examples. |

Hidden utility agents (compaction, title, summary) handle internal tasks and are not selectable.

Some agents (code-simplifier, qa-tester, scientist) are disabled by default but can be enabled in config.

### Agent modes

- **primary** -- Can be set as your active agent to handle the conversation directly
- **all** -- Can be used as either a primary agent or spawned as a subagent
- **subagent** -- Designed to be spawned by the build agent (or invoked via `/ask`) for specific tasks; cannot be set as the default agent

### Compact variants

Each agent has a `.compact` variant that is automatically used when the model has 8B parameters or fewer. Compact prompts are shorter and simpler, tuned for smaller models.

### Switching agents

**Tab/Shift+Tab** -- Cycle through enabled agents in the prompt. The agent name and its color update in the status bar.

**Ctrl+X a** -- Open the agent dialog showing all agents (including disabled ones). Navigate with j/k or arrows, press Enter to select.

**/ask** -- Route a single message to a specific agent without switching:

```
/ask architect should we use a message queue here?
/ask debugger why is TestAuth failing?
```

The `/ask` command has its own autocomplete -- after typing `/ask `, it shows agent names filtered by what you type.

### Enabling and disabling agents

In the agent dialog (Ctrl+X a), press **d** on any non-native agent to toggle it on/off. Disabled agents show `[off]` and are not included in Tab cycling or autocomplete. Native agents (build, plan, general, explore, scout) cannot be disabled.

You can also disable agents in config:

```json
{
  "agents": {
    "scientist": { "disable": true },
    "my-custom-agent": {
      "prompt": "You are a documentation specialist.",
      "description": "Custom docs agent",
      "mode": "subagent"
    }
  }
}
```

---

## Subagents and Swarm Mode

### How subagents work

The build agent (default) can spawn subagents for complex tasks. When you ask for something that involves multiple files or systems, build may delegate to executor, architect, or other agents internally using a `task` tool.

You can also explicitly request delegation:

```
/ask executor implement the login form across all 5 files
```

Subagent output appears in the conversation as nested messages showing which agent handled what.

### Swarm mode (/swarm)

Swarm mode splits a task into independent subtasks and runs them in parallel via multiple subagents.

```
/swarm run tests on internal/config, internal/provider, and internal/agent packages
```

What happens:

1. The build agent receives your task with swarm instructions prepended
2. It analyzes the task and creates 2-4 independent subtasks
3. Each subtask is dispatched to a subagent (executor or explore) via the `task` tool
4. Subagents run in parallel, each with its own tool access
5. After all subagents complete, the build agent synthesizes a report

Swarm mode automatically enables auto-approve for tool permissions (the subagents need to run without waiting for approval).

Constraints:

- Subagents get one round of work -- they do not retry on failure
- The coordinator does not use tools directly -- it only dispatches via the task tool
- If a subagent times out or errors, the coordinator reports what failed and synthesizes partial results

### Work-loop mode (/work-loop)

Work-loop mode iterates autonomously on a task until it is complete or blocked:

```
/work-loop fix all lint errors in this project
```

The agent cycles through: understand, plan, act, verify, assess. It continues without asking for confirmation until the task is done or the same action fails 3 times.

---

## Providers and Models

### Supported providers

| Provider | Type | Discovery |
|----------|------|-----------|
| **Ollama** | Local | Auto-discovered at `localhost:11434` (override with `OLLAMA_HOST`) |
| **vLLM** | Local | Set `TINYCODE_VLLM_HOST` to enable (e.g., `http://localhost:8000`) |
| **LM Studio** | Local | Auto-discovered at `localhost:1234` (override with `TINYCODE_LMSTUDIO_HOST`) |
| **OpenRouter** | Cloud | Set `OPENROUTER_API_KEY` to enable |

Any OpenAI-compatible API endpoint (Anthropic, OpenAI, Azure, etc.) can also be configured as a custom provider via the `provider` config block -- see [Configuration](#configuration).

### Provider auto-discovery

On startup, tinycode probes local providers synchronously so models are available immediately. It then polls every 30 seconds in the background. If a provider fails 3 consecutive health checks, it is removed and polling stops. Restarting tinycode re-enables discovery.

For Ollama models, tinycode sends a warmup probe to pre-load the model into GPU memory and verify tool-call support. Models that do not support tool calling are flagged and work in text-only mode.

### Selecting a model

**Ctrl+X m** or **/connect** -- Opens the model selector. This is a two-step dialog:

1. **Select provider** -- shows all discovered providers with model counts
2. **Select model** -- scrollable list of models for the chosen provider

In the model list, type to search (e.g., "qwen" to filter). Backspace clears the filter. Esc goes back to the provider list.

**F2 / Shift+F2** -- Cycle through recently used models without opening a dialog.

**Tab** in the model list -- moves between available models.

### Model scoping (favorites)

Use `/scoped-models` to mark specific models as favorites. When scoping is active, the model selector only shows your favorites instead of the full list from every provider.

Configure in config:

```json
{
  "scopedModels": [
    "ollama/qwen3:8b",
    "ollama/qwen3.5:9b",
    "openrouter/anthropic/claude-sonnet-4-20250514"
  ]
}
```

### Thinking level (extended reasoning)

Control how much the model "thinks" before responding:

```
/thinking off       # No reasoning (default)
/thinking low       # 1k token budget
/thinking medium    # 4k token budget
/thinking high      # 16k token budget
/thinking max       # 128k token budget
```

Higher thinking levels give better results on complex tasks but use more tokens and take longer. The current level is shown in the prompt and persists for the session.

---

## Sessions

Sessions are conversations. Each session maintains its own message history, agent selection, and model choice.

### Auto-titling

When you send the first prompt in a new session, tinycode automatically generates a title based on the content. The title appears in the sidebar and session list.

### Creating sessions

- **Ctrl+X n** -- Create a new session
- Starting tinycode always begins with a clean session

### Switching sessions

- **Ctrl+X o** -- Open the session list dialog. Navigate with arrows, press Enter to switch.
- **Sidebar** -- Click a session title in the sidebar to switch to it.

### Renaming sessions

```
/rename my feature branch work
```

### Archiving sessions

**/archive** -- Soft-deletes the current session. The session is removed from the session list and sidebar but can be recovered. After archiving, a new session is created automatically.

### Exporting sessions

**Ctrl+X x** or **/export** -- Writes the current session transcript as a Markdown file (`session-<title>.md`) in the working directory. Includes all messages, tool calls, and their output.

**/export html** -- Exports the session as an HTML file with syntax highlighting. The HTML export is self-contained and can be shared or viewed in any browser.

### Session management from CLI

```bash
tinycode session list              # List sessions for the current project
tinycode session delete <id>       # Delete a session by ID
```

### Compaction

When a conversation grows long and approaches the model's context limit, tinycode automatically compacts the session. Compaction summarizes old messages while preserving recent context, keeping the conversation usable within the token budget. You can also trigger compaction manually with the `/compact` command.

Configure compaction behavior in config:

```json
{
  "compaction": {
    "auto": true,
    "tail_turns": 4,
    "preserve_recent_tokens": 8000
  }
}
```

---

## Configuration

### Config file locations

tinycode reads config from multiple locations, merging them in order (later files override earlier ones):

1. **Global config**: `~/.config/tinycode/tinycode.jsonc` (also checks `tinycode.json` and `config.json`)
2. **Project config**: `tinycode.jsonc` or `tinycode.json` files walking up from the working directory (innermost wins)
3. **Project dot-directory**: `.tinycode/` directory in the project

Override the config directory with `TINYCODE_CONFIG_DIR` or `XDG_CONFIG_HOME`.

Config files support JSONC (JSON with comments) and environment variable substitution.

### Example config

```jsonc
{
  // Default model (provider/model format)
  "model": "ollama/qwen3:8b",

  // Default agent on startup
  "default_agent": "build",

  // Shell for tool execution
  "shell": "/bin/zsh",

  // Log level: debug, info, warn, error
  "logLevel": "info",

  // Model favorites for /scoped-models
  "scopedModels": [
    "ollama/qwen3:8b",
    "ollama/qwen3.5:9b"
  ],

  // Color theme
  "theme": "dracula",

  // Permission rules
  "permission": {
    "allow": ["read", "grep", "glob"],
    "deny": ["shell:rm *"]
  },

  // Custom instructions included in every prompt
  "instructions": [
    "Always use Go standard library where possible"
  ],

  // Agent overrides
  "agents": {
    "scientist": { "disable": true },
    "my-agent": {
      "prompt": "You are a Go expert.",
      "description": "Custom Go agent",
      "mode": "primary"
    }
  }
}
```

### Key config options

| Option | Default | Description |
|--------|---------|-------------|
| `model` | *(auto)* | Default model in `provider/model` format |
| `small_model` | *(none)* | Smaller model for lightweight tasks (titles, summaries) |
| `default_agent` | `build` | Agent loaded on startup |
| `shell` | *(system)* | Shell for tool execution |
| `logLevel` | *(none)* | Log verbosity |
| `theme` | *(default)* | Color theme name |
| `temperature` | *(none)* | Default LLM temperature |
| `top_p` | *(none)* | Default nucleus sampling |
| `max_tokens` | *(none)* | Default max output tokens |
| `subagent_depth` | *(none)* | Maximum nesting depth for subagents |
| `autoApprove` | `false` | Auto-approve all tool permissions globally |
| `scopedModels` | `[]` | List of model favorites |
| `instructions` | `[]` | Custom instructions prepended to system prompt |
| `disabled_providers` | `[]` | Providers to hide |
| `enabled_providers` | `[]` | Providers to show (if set, only these appear) |

### Provider environment variables

```bash
# Local providers
OLLAMA_HOST=http://localhost:11434       # Ollama URL (default)
TINYCODE_VLLM_HOST=http://localhost:8000 # vLLM URL
TINYCODE_LMSTUDIO_HOST=http://localhost:1234  # LM Studio URL (default)

# Cloud providers
OPENROUTER_API_KEY=your-key

# Server settings
TINYCODE_PORT=4096           # API server port
TINYCODE_HOST=127.0.0.1      # Bind address
TINYCODE_DB=/path/to/db      # Database path override
TINYCODE_LOG_LEVEL=debug      # Log level
TINYCODE_WEB_DIR=./packages/app/dist  # Web UI directory (dev mode)
TINYCODE_AUTH_TOKEN=my-token  # Auth token for serve/web mode
TINYCODE_NO_AUTH=1            # Disable auth entirely
```

### Local storage paths

| What | Path |
|------|------|
| Config | `~/.config/tinycode/tinycode.jsonc` |
| Database | `~/.local/share/tinycode/tinycode.db` |
| Log file | `~/.local/share/tinycode/tinycode.log` |
| Plugins | `~/.config/tinycode/plugins/` |
| Skills | `~/.config/tinycode/skills/` |

Override data directory with `TINYCODE_DATA_DIR` or `XDG_DATA_HOME`.

---

## MCP Integration

[Model Context Protocol](https://modelcontextprotocol.io/) (MCP) servers provide additional tools to the model. For example, an MCP server could provide database queries, API access, or custom integrations.

### Configuration

Add MCP servers in your config file:

```jsonc
{
  "mcp": {
    "my-server": {
      "command": "npx",
      "args": ["-y", "@my/mcp-server"],
      "env": {
        "API_KEY": "${MCP_API_KEY}"
      }
    }
  }
}
```

The `command` field accepts either a string or an array (`["npx", "-y", "@my/mcp-server"]`).

### Transport types

**Stdio (default)** -- tinycode spawns the MCP server as a child process and communicates over stdin/stdout:

```json
{
  "mcp": {
    "local-server": {
      "command": "my-mcp-server",
      "args": ["--port", "0"]
    }
  }
}
```

**SSE** -- connect to a remote MCP server over HTTP Server-Sent Events:

```json
{
  "mcp": {
    "remote-server": {
      "url": "https://mcp.example.com/sse",
      "transport": "sse"
    }
  }
}
```

**Streamable HTTP** -- the newer MCP transport:

```json
{
  "mcp": {
    "streamable-server": {
      "url": "https://mcp.example.com/mcp",
      "transport": "streamable"
    }
  }
}
```

### OAuth support

MCP servers that require OAuth authentication:

```json
{
  "mcp": {
    "oauth-server": {
      "url": "https://mcp.example.com/sse",
      "transport": "sse",
      "oauth": {
        "client_id": "my-client-id",
        "auth_url": "https://auth.example.com/authorize",
        "token_url": "https://auth.example.com/token",
        "scopes": ["read", "write"]
      }
    }
  }
}
```

### Managing MCP servers

Use `/mcp` or **Ctrl+X i** to open the MCP server management dialog. This shows all configured servers with their connection status and tool counts. From the dialog you can:

- View each server's status (connected, error, disconnected)
- Trigger a reconnect for failed or disconnected servers
- See the number of tools each server provides

### Status monitoring

MCP server status also appears in the sidebar (Ctrl+X b):

- Green dot -- connected, with tool count
- Red dot -- error (hover for details)
- Gray dot -- disconnected or connecting

Tinycode automatically reconnects MCP servers that disconnect.

---

## LSP Integration

Language Server Protocol (LSP) integration provides code intelligence to the model -- go-to-definition, diagnostics, hover info.

### Configuration

Enable globally:

```json
{
  "lsp": true
}
```

Or with per-server overrides:

```json
{
  "lsp": {
    "enabled": true,
    "timeout": 10,
    "servers": {
      "go": {
        "command": "gopls",
        "args": ["serve"],
        "env": {}
      },
      "typescript": {
        "disabled": true
      }
    }
  }
}
```

### Auto-detection

When enabled, tinycode detects available language servers on your PATH (gopls for Go, typescript-language-server for TypeScript, etc.) and starts them lazily when a relevant file is opened.

---

## Plugins

Plugins are standalone Go binaries that extend tinycode with custom tools and lifecycle hooks. They communicate over JSON-RPC via stdin/stdout.

### Installing plugins

```bash
# List available plugins
tinycode plugin list

# List by category
tinycode plugin list --category general

# Install from source (if cmd/plugin-<name> exists in the repo)
tinycode plugin install notify

# Install from a pre-built binary
tinycode plugin install notify --from /path/to/binary
```

Plugins are installed to `~/.config/tinycode/plugins/<name>`. tinycode also checks for `tinycode-plugin-<name>` on your PATH.

### Configuring plugins

Add plugin names to your config:

```json
{
  "plugins": [
    "notify",
    "safety-net"
  ]
}
```

### Available plugins

Plugins are organized by category. Run `tinycode plugin list` for the full list. Highlights:

| Plugin | Category | Description |
|--------|----------|-------------|
| notify | general | Desktop notifications for session events |
| safety-net | general | Pre-execution safety checks for destructive commands |
| web-search | general | Web search via DuckDuckGo |
| pilot | general | Issue tracker integration (GitHub, GitLab, Gitea) |
| ocp-context-injection | openshift | Cluster context injection |
| container-linter | platform | Containerfile linting and bootc support |

### Interactive setup

```bash
tinycode init
```

Walks you through model selection, username, and role-based plugin selection (OpenShift SRE, Security, AI/ML, Platform, Developer). Writes the config file for you.

### Uninstalling

```bash
tinycode plugin uninstall notify
```

---

## Web UI

### Starting the web UI

```bash
tinycode web
```

This starts the API server and opens the embedded web interface in your browser. The web UI is a single-page app built with TypeScript/React that communicates with the same backend as the TUI.

Alternatively, start the headless server and access the web UI manually:

```bash
tinycode serve
# Open http://localhost:4096 in your browser
```

### Authentication

By default, the web/serve mode generates an auth token printed to stderr on startup. Set a custom token:

```bash
TINYCODE_AUTH_TOKEN=my-secret tinycode web
```

Or disable auth entirely (for local-only use):

```bash
TINYCODE_NO_AUTH=1 tinycode serve
```

### Web UI keybindings

The web UI has its own set of keyboard shortcuts:

| Key | Action |
|-----|--------|
| Mod+Shift+P | Open command palette |
| Mod+N | New session |
| Mod+Shift+A | Archive session |
| Mod+. | Open settings |
| Mod+/ | Toggle sidebar |
| Mod+Shift+1 | Focus file tree |
| Mod+Shift+M | Select model |
| Mod+Shift+E | Select agent |
| Mod+` | Toggle terminal |
| Enter | Send message |
| Shift+Enter | Newline in prompt |
| Escape | Cancel / close dialog |

("Mod" is Cmd on macOS, Ctrl on Linux/Windows.)

### Web UI features

The web UI provides:

- **File tree** -- browse and navigate project files in the sidebar; click to reference in prompt
- **Session management** -- create, archive, fork, and switch sessions
- **Agent system** -- same agent selection as the TUI
- **VCS integration** -- view git status and diffs
- **Settings** -- configure model, theme, and preferences visually

### Help API endpoint

`GET /help` returns a JSON response with keybindings, commands, and features:

```bash
curl http://localhost:4096/help
```

Response structure:

```json
{
  "keybindings": [{"key": "mod+shift+p", "description": "Open command palette", "category": "General"}],
  "commands": [{"name": "review", "description": "Review changes", "source": "builtin"}],
  "features": [{"name": "@ File References", "description": "Type @ to reference files..."}]
}
```

---

## Run Mode

The `run` subcommand runs tinycode non-interactively -- process a prompt and exit. Same agents, tools, and permissions as the TUI.

### Basic usage

```bash
# Prompt from arguments
tinycode run -m ollama/qwen3:8b "explain the main function"

# Pipe from stdin
echo "explain this codebase" | tinycode run -m ollama/qwen3:8b

# Specific directory
tinycode run ~/projects/myapp -m ollama/qwen3:8b "add validation"

# Use a specific agent
tinycode run --agent debugger -m ollama/qwen3:8b "why is TestFoo failing?"

# Continue an existing session
tinycode run -c -m ollama/qwen3:8b "now add tests for that"
```

### Flags

| Flag | Description |
|------|-------------|
| `-m, --model` | Model to use (provider/model) |
| `--agent` | Agent to use (default: build) |
| `--format` | Output format: `default` (text) or `json` (NDJSON events) |
| `-c, --continue` | Continue the most recent session |
| `-s, --session` | Session ID to continue |
| `--title` | Session title |
| `--dangerously-skip-permissions` | Auto-approve all tool permissions |
| `-i, --interactive` | Show permission prompts on stderr |
| `--permissions` | Permission handling: `default` or `json` |
| `--max-iterations` | Max processor iterations (default: 200) |
| `--multi-turn` | Loop on stdin after initial prompt |

### Permission modes

| Mode | Flag | Behavior |
|------|------|----------|
| Auto-deny | *(default)* | All tool permissions rejected (safe for automation) |
| Auto-approve | `--dangerously-skip-permissions` | All permissions approved |
| Interactive | `-i` | Prompts on stderr, reads yes/no from stdin |
| JSON | `--permissions json` | NDJSON permission protocol via stdin/stdout |

### NDJSON output (`--format json`)

All output is emitted as newline-delimited JSON events:

| Event type | Description |
|------------|-------------|
| `text` | Text delta from the model |
| `tool_begin` | Tool execution started |
| `tool_end` | Tool execution completed |
| `reasoning` | Model thinking/reasoning block |
| `step_start` | Processor iteration started |
| `step_finish` | Processor iteration completed |
| `warning` | Non-fatal warning |
| `compacted` | Context compaction occurred |

### Multi-turn mode

With `--multi-turn`, tinycode loops on stdin after the initial prompt:

```bash
tinycode run --multi-turn -m ollama/qwen3:8b "explain main.go"
# After response, type next prompt:
# > now add error handling
# > (Ctrl+D to exit)
```

In JSON mode, use structured messages:

```
→ stdin:  {"type":"prompt","text":"explain main.go"}
← stdout: {"type":"ready"}
→ stdin:  {"type":"prompt","text":"now add tests"}
← stdout: {"type":"ready"}
→ stdin:  {"type":"exit"}
```

---

## CLI Reference

```
tinycode [command|directory] [flags]
```

Running with no command starts the TUI.

### Commands

| Command | Description |
|---------|-------------|
| `tui` | Start terminal UI (default) |
| `run` | Run a prompt non-interactively and exit |
| `serve` | Start headless API server (port 4096) |
| `web` | Start server and open web interface |
| `acp` | Agent Client Protocol mode (stdio, for IDE integration) |
| `models` | List available models |
| `providers` | List discovered providers |
| `session` | Manage sessions (`list`, `delete`) |
| `export` | Export session messages as JSON |
| `agent` | List available agents |
| `plugin` | Manage plugins (`list`, `install`, `uninstall`) |
| `init` | Interactive setup wizard |
| `doctor` | Run diagnostics and check system health (providers, config, database, agents) |
| `debug` | Debug info (`config`, `paths`) |
| `status` | Show server health and version info |
| `version` | Print version |
| `help` | Show usage |

### TUI and common flags

These flags apply to the TUI (default mode) and `run` mode:

| Flag | Description |
|------|-------------|
| `-m, --model` | Model to use (provider/model) |
| `--title` | Set the session title |
| `-c, --continue` | Continue the most recent session |
| `-r, --resume <id>` | Resume a session by ID or title substring |
| `--append-system-prompt <text>` | Append text to the system prompt |
| `--append-system-prompt-file <path>` | Append file contents to the system prompt |
| `--max-tokens <n>` | Cumulative token budget (input+output); session aborts when exceeded |
| `--safe-mode` | Skip plugins, MCP servers, and user-defined agents |

### Token budget ceiling

The `--max-tokens` flag sets a cumulative token ceiling for a session. The processor tracks total input and output tokens across all iterations; when the sum exceeds the budget, the session stops with a "token budget exceeded" error. This is useful for unattended runs (`tinycode run`) where you want to cap cost.

```bash
# Abort after 50k total tokens
tinycode run --max-tokens 50000 -m ollama/qwen3:8b "refactor main.go"

# Also works in TUI mode
tinycode --max-tokens 100000
```

### Safe mode

`--safe-mode` starts tinycode without loading plugins, MCP servers, or user-defined agents. Only built-in agents and tools are available. The status bar shows a bold orange **SAFE MODE** indicator when active.

```bash
tinycode --safe-mode
```

### Session resume

Resume a previous session from the command line:

```bash
# Continue the most recent session
tinycode -c

# Resume a specific session by ID or title substring
tinycode -r "auth refactor"
tinycode -r ses_01HQXY...
```

In `run` mode, the same flags work:

```bash
tinycode run -c -m ollama/qwen3:8b "now add tests for that"
```

### Examples

```bash
# TUI
tinycode                              # Current directory
tinycode ~/projects/myapp             # Specific directory
tinycode -m ollama/qwen3:8b           # With specific model

# Non-interactive
tinycode run -m ollama/qwen3:8b "fix the bug"
echo "explain main.go" | tinycode run -m ollama/qwen3:8b

# Server
tinycode serve -m ollama/qwen3:8b     # Headless API
tinycode web                          # Web UI

# Inspection
tinycode models                       # List models
tinycode providers                    # List providers
tinycode agent                        # List agents
tinycode status                       # Health check
tinycode doctor                       # Full diagnostics check
tinycode debug config                 # Dump merged config as JSON
tinycode debug paths                  # Show all config/data paths
```

---

## Troubleshooting

### Logs

Logs are written to `~/.local/share/tinycode/tinycode.log`. For verbose output:

```bash
TINYCODE_LOG_LEVEL=debug tinycode
```

### tinycode doctor

Run `tinycode doctor` for a headless health check that verifies every subsystem without starting the TUI:

```bash
tinycode doctor
```

It checks: version, Go runtime, config validity, data directory writability, database access, agent loading, provider connectivity, MCP servers, plugins, skills, and log file writability. Each check shows a green check, red X, or yellow warning. Non-zero exit code if any critical check fails.

### /debug command

Type `/debug` in the TUI to open a diagnostics dialog showing:

- Merged config
- Provider status
- Active agents and plugins
- MCP server connections
- System info (Go version, OS, architecture)

From the CLI:

```bash
tinycode debug config    # Print merged config as JSON
tinycode debug paths     # Show all file paths
```

### Common issues

| Problem | Solution |
|---------|----------|
| "No models discovered" | Make sure Ollama (or your provider) is running. Check with `ollama list` or `curl http://localhost:11434/api/tags`. |
| Model not found | `ollama pull <model>` to download it first. |
| Tool calling not working | The model may not support function calling. Try a larger model (8B+). Tinycode probes for this on startup and disables the capability if unsupported. |
| Provider disappeared | After 3 consecutive failures, providers are removed. Restart tinycode to re-discover. |
| Spinner frozen | Make sure to propagate the `tea.Cmd` returned by `SetWorking(true)`. If you are developing tinycode, this is a known pitfall. |
| MCP server not connecting | Check the command path and args in your config. Run the MCP server manually to verify it starts. Status is shown in the sidebar. |
| Config not loading | Config files are checked as `tinycode.jsonc`, `tinycode.json`, then `config.json` in each directory. Run `tinycode debug paths` to see which files are found. |
| Wrong config applied | Project config files walk up the directory tree, with innermost overriding outermost. Use `tinycode debug config` to see the merged result. |

### Getting help

- `/help` in the TUI shows the command palette with all keybindings and commands
- `tinycode help` shows CLI usage
- [Architecture docs](architecture.md) for how tinycode works internally
- [Plugin Development](plugin-development.md) for building custom plugins
- [Troubleshooting guide](troubleshooting.md) for more detailed solutions
