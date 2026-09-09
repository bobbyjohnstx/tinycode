# tinycode User Guide

Complete guide from beginner to power user.

**Table of Contents**
1. [Introduction](#introduction)
2. [Installation](#installation)
3. [Running tinycode](#running-tinycode)
4. [The Terminal UI](#the-terminal-ui)
5. [Sessions](#sessions)
6. [Agents](#agents)
7. [Skills](#skills)
8. [Models](#models)
9. [Configuration](#configuration)
10. [Advanced Workflows](#advanced-workflows)
11. [Plugin System](#plugin-system)
12. [Troubleshooting](#troubleshooting)

---

## Introduction

**tinycode** is an open-source AI coding assistant that runs as a standalone Go binary. A single process embeds the HTTP server, bubbletea TUI, session management, LLM client, and tool execution -- no separate server process required.

### What makes it different

- **Single binary**: One `go build` produces a self-contained executable
- **Privacy first**: Your code never leaves your machine when using local models
- **Local LLMs**: Runs models via Ollama, vLLM, or LM Studio on your hardware
- **Full tool access**: Read, write, edit files; run shell commands; search code
- **Agents**: Specialized personas (architect, debugger, executor, etc.) for different tasks
- **Sessions**: Organize work as a tree of conversations
- **Extensible**: Add plugins (Go binaries) and MCP servers

---

## Installation

### Prerequisites

- **Go 1.22+** -- [install here](https://go.dev/dl/)
- **make**
- **Git**
- A running LLM provider (Ollama recommended for getting started)

### Build from source

```bash
git clone https://github.com/bobbyjohnstx/tinycode-go.git
cd tinycode-go
make build
```

The binary is produced at `dist/tinycode`.

### Verify

```bash
./dist/tinycode version
```

---

## Running tinycode

### TUI mode (default)

```bash
./dist/tinycode                 # Current directory
./dist/tinycode /path/to/project  # Specific directory
```

### Headless API proxy

```bash
./dist/tinycode serve
```

Starts the HTTP server without the TUI. Useful for remote access or integrating with other tools.

### Agent Client Protocol (IDE integration)

```bash
./dist/tinycode acp
```

Runs over stdin/stdout using the Agent Client Protocol for IDE integration.

---

## The Terminal UI

### Layout

The TUI has three areas:
- **Session sidebar** (left) -- session tree, toggled with `<leader>b`
- **Conversation area** (center) -- messages, tool output, streaming responses
- **Input prompt** (bottom) -- type prompts, slash commands, file references

### Leader key

The leader key is `Ctrl+X` by default. Press it, then press a follow-up key within 500ms to trigger an action. Throughout this guide, `<leader>` means Ctrl+X.

### Keyboard shortcuts

| Key | Action |
|-----|--------|
| `Ctrl+C` / `Ctrl+D` | Exit tinycode |
| `Ctrl+P` | Command palette |
| `Enter` | Submit prompt |
| `Shift+Enter` / `Alt+Enter` | Insert newline in prompt |
| `Escape` | Interrupt current operation |
| `Tab` / `Shift+Tab` | Next / previous agent |
| `F2` / `Shift+F2` | Next / previous recent model |
| `PgUp` / `PgDn` | Scroll conversation |

### Leader key sequences

| Key | Action |
|-----|--------|
| `<leader>b` | Toggle session sidebar |
| `<leader>n` | New session |
| `<leader>o` | List all sessions |
| `<leader>m` | Switch model |
| `<leader>a` | List agents |
| `<leader>u` | Undo message |
| `<leader>r` | Redo message |

### Prompt input

Reference files in your prompt with `@`:

```
@src/auth.go analyze the login flow
@Dockerfile review for best practices
```

Use `/` to trigger slash commands (skills):

```
/debug why are my tests failing?
/trace the root cause of this error
/verify this change actually works
```

Type `Tab` to autocomplete agents and skills.

---

## Sessions

Sessions are conversations. They form a tree structure where child sessions can inherit context from parents.

### Creating and navigating sessions

| Key | Action |
|-----|--------|
| `<leader>n` | Create new session (as child of current) |
| `<leader>o` | List all sessions |

### Session hierarchy

Sessions organize into a tree. Use this for structured work:

```
Planning Session
  +-- Feature Implementation
  |   +-- UI Component
  |   +-- Tests
  +-- Code Review
```

Create a parent session, then spawn child sessions for subtasks.

### Session compaction

When a session gets long, context approaches the model's limit. Compaction summarizes old messages, preserves recent context, and shrinks the session.

---

## Agents

Agents are specialized personas for different tasks. All agents share the same tools but have distinct system prompts that shape their behavior.

### Built-in agents

| Agent | Purpose |
|-------|---------|
| `build` | General coding (default). Full tool access. |
| `architect` | Analyzing design, reviewing structure. Read-only. |
| `code-reviewer` | Severity-rated code review. Read-only. |
| `code-simplifier` | Refactoring recent changes for clarity. |
| `critic` | Multi-perspective quality review. Read-only. |
| `debugger` | Root-cause analysis for bugs. |
| `executor` | Focused task implementation. |
| `explore` | Fast codebase search. Read-only. |
| `git-master` | Git history, rebasing, atomic commits. |
| `planner` | Breaking down work into steps. |
| `scientist` | Data analysis, evidence-driven research. |
| `security-reviewer` | Vulnerability detection. Read-only. |
| `test-engineer` | Test strategy and TDD. |
| `verifier` | Confirmation that work is actually complete. |
| `writer` | Technical documentation. |

### Switching agents

Press `Tab` to cycle to the next agent. Press `<leader>a` to see all agents and pick one.

Or use `/ask <agent> <prompt>` inline:

```
/ask architect design the auth flow
/ask test-engineer write tests for this component
/ask debugger why is this test failing?
```

---

## Skills

Skills are slash commands that inject specialized instructions. Type `/` in the prompt to see the list.

### Using skills

Type a skill name at the start of your prompt:

```
/debug my tests are failing
/trace the root cause of this error
/verify this change actually works
```

Or type a prompt first, then use a skill to guide the response:

```
My tests are failing. Why?
/debug
```

---

## Models

tinycode supports local models (via Ollama, vLLM, LM Studio) and cloud providers (OpenRouter, Anthropic, OpenAI, etc.).

### Selecting a model

Press `<leader>m` or type `/connect` to open the model selector. This is a two-step dialog:

1. **Select provider** — shows all discovered providers with model counts (e.g., "OpenRouter (430 models)")
2. **Select model** — shows models for the chosen provider in a scrollable list (8 visible at a time)

In the model list, type to search (e.g., "qwen" to filter to Qwen models). Use backspace to clear the filter. Press `Esc` to go back to the provider list.

### Provider auto-discovery

On startup, tinycode probes for local providers:

- **Ollama** at `localhost:11434` (override with `OLLAMA_HOST`) -- queries `/api/tags` for available models
- **vLLM** at the URL in `TINYCODE_VLLM_HOST` -- queries `/v1/models`
- **LM Studio** at `localhost:1234` (override with `TINYCODE_LMSTUDIO_HOST`) -- queries `/v1/models`

Discovery runs on startup (synchronously, so models are available immediately) and then polls every 30 seconds. If a provider fails 3 consecutive health checks, it is removed and polling stops.

### Cloud providers

Set API keys as environment variables:

```bash
export OPENROUTER_API_KEY=your-key    # Auto-discovers models from OpenRouter API
export ANTHROPIC_API_KEY=your-key
export OPENAI_API_KEY=your-key
```

Or use `/connect` in the TUI to enter API keys interactively.

### Model warmup

For Ollama models, tinycode sends a warmup probe on discovery to:
- Pre-load the model into GPU memory
- Verify tool-call support

If a model does not support tool calling, the capability is marked as disabled and the model works in text-only mode.

---

## Configuration

tinycode reads config from multiple locations (JSONC supported):

- **macOS**: `~/Library/Application Support/tinycode/config.json` and `~/.config/tinycode/config.json` (both are loaded, for TS tinycode compatibility)
- **Linux**: `~/.config/tinycode/config.json`
- **Override**: `TINYCODE_CONFIG_DIR` environment variable

Project-level config can be placed in `.tinycode/config.json`.

### Example config

```json
{
  "model": "ollama/qwen3.5:9b",
  "default_agent": "build",
  "shell": "/bin/zsh",
  "logLevel": "info"
}
```

### Common options

| Option | Default | Purpose |
|--------|---------|---------|
| `model` | (none) | Default LLM model (e.g., `ollama/qwen3.5:9b`) |
| `small_model` | (none) | Smaller model for lightweight tasks |
| `default_agent` | `build` | Agent loaded on startup |
| `subagent_depth` | (none) | Maximum nesting depth for subagents |
| `shell` | (none) | Shell for tool execution |
| `logLevel` | (none) | Log verbosity |
| `temperature` | (none) | Default LLM temperature |
| `max_tokens` | (none) | Default max output tokens |

### Provider configuration

```bash
# Ollama (auto-discovered at localhost:11434)
export OLLAMA_HOST=http://your-host:11434

# vLLM
export TINYCODE_VLLM_HOST=http://localhost:8000

# LM Studio
export TINYCODE_LMSTUDIO_HOST=http://localhost:1234

# Cloud providers
export OPENROUTER_API_KEY=your-key
export ANTHROPIC_API_KEY=your-key
export OPENAI_API_KEY=your-key
```

### Provider filtering

Control which providers appear in the model list:

```json
{
  "enabled_providers": ["ollama", "vllm"],
  "disabled_providers": ["openrouter"]
}
```

### MCP server configuration

Add MCP servers in the config:

```json
{
  "mcp": {
    "my-server": {
      "command": "npx",
      "args": ["-y", "@my/mcp-server"],
      "env": {
        "API_KEY": "your-key"
      }
    }
  }
}
```

MCP servers can also use SSE transport:

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

### Plugin configuration

```json
{
  "plugins": [
    "notify",
    "safety-net"
  ]
}
```

See [Plugin Development](plugin-development.md) for details.

### Local storage

| Path | macOS | Linux |
|------|-------|-------|
| **Database** | `~/Library/Application Support/tinycode/tinycode.db` | `~/.local/share/tinycode/tinycode.db` |
| **Log file** | `~/Library/Application Support/tinycode/tinycode.log` | `~/.local/share/tinycode/tinycode.log` |
| **Config** | `~/Library/Application Support/tinycode/config.json` | `~/.config/tinycode/config.json` |

Set `TINYCODE_LOG_LEVEL=debug` to enable verbose logging.

---

## Advanced Workflows

### Multi-agent pipelines

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

### File references

Reference files in any prompt with `@`:

```
Edit @src/auth.go to add a login function
Review @Dockerfile for best practices
Compare @go.mod with @go.sum for consistency
```

### Session hierarchy patterns

**Bug fix workflow:**

```
Production Issue
  +-- Investigate (debugger finds root cause)
      +-- Fix Implementation (executor codes)
          +-- Regression Tests (test-engineer adds tests)
              +-- Verification (verifier confirms)
```

**Feature development:**

```
Feature Design
  +-- API Design (architect)
  +-- Implementation (executor)
  +-- Tests (test-engineer)
  +-- Documentation (writer)
```

---

## Plugin System

Plugins are standalone Go binaries that communicate with tinycode over JSON-RPC via stdin/stdout. They can provide custom tools and lifecycle hooks.

### Installing plugins

Plugins are resolved in this order:
1. `~/.config/tinycode/plugins/<name>` -- binary in the config directory
2. `tinycode-plugin-<name>` on PATH -- binary with the conventional prefix
3. The built-in registry -- known plugins with install instructions

### Configuring plugins

Add plugin names to the config:

```json
{
  "plugins": [
    "notify",
    "safety-net",
    "telemetry"
  ]
}
```

### Available plugins

| Plugin | Description |
|--------|-------------|
| `notify` | Desktop notifications for session events |
| `safety-net` | Pre-execution safety checks for destructive commands |
| `telemetry` | Usage telemetry and analytics |
| `code-review` | Git diff display for code review |
| `handoff` | Session context handoff between sessions |
| `web-search` | Web search via DuckDuckGo |
| `pilot` | Issue tracker integration (GitHub, GitLab, Gitea) |
| `cluster-ops` | OpenShift cluster authentication |
| `snippets` | Kubernetes resource templates |
| `context-pruning` | Deduplicates repeated tool outputs |
| `log-sanitizer` | Strips secrets from tool output |
| `command-inject` | Custom slash command injection |

See [Plugin Development](plugin-development.md) for building your own.

---

## Troubleshooting

### Common issues

| Problem | Solution |
|---------|----------|
| Model not found | `ollama pull <model>` first |
| Cannot connect to Ollama | Verify `ollama serve` is running; check `OLLAMA_HOST` |
| Tool calling not working | Model may not support it; try a larger model (9B+) |
| Provider disappeared | After 3 consecutive failures, providers are removed; restart tinycode |

### Diagnostic steps

1. Check that your LLM provider is running and accessible
2. Verify your config file is valid JSON (see [Local storage](#local-storage) for paths)
3. Check the log file: `TINYCODE_LOG_LEVEL=debug ./dist/tinycode` (see [Local storage](#local-storage) for log path)
4. Try a different model via `/connect` or `<leader>m`

### Further reading

- [Getting Started](getting-started.md) -- quick setup walkthrough
- [Architecture](architecture.md) -- how tinycode works under the hood
- [Plugin Development](plugin-development.md) -- build custom plugins
- [Troubleshooting](troubleshooting.md) -- detailed solutions
