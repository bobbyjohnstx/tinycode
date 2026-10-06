# tinycode Commands

Type `/` in the prompt to see available commands. Type `/help` to open this reference.

## Session Commands

| Command | What it does | When to use it |
|---------|-------------|----------------|
| `/branch [name]` | Copy the current conversation to a new session and switch to it. The original is preserved. | Before trying a risky approach — branch, experiment, `/resume` back if it fails. |
| `/compact [focus]` | Summarize the conversation via `Processor.Compact` to free context window space (works; not a stub). Optional focus instructions guide the summary. | When the model starts forgetting earlier context, or `/context` shows you're near the limit. |
| `/rewind` | Open a picker of conversation turns and roll back to a selected point. Messages after that turn are removed. | When the conversation went in a wrong direction and you want to try again from an earlier point. |
| `/rename [name]` | Set or auto-generate a session name. | To label sessions for easy identification in the session list. |
| `/archive` | Archive the current session (hides from default list). | When you're done with a session but don't want to delete it. |
| `/export` | Export the session as Markdown. | To save a conversation for documentation or sharing. |
| `/export-html` | Export the session as a self-contained HTML file. | For formatted, shareable session transcripts. |

## Execution Commands

| Command | What it does | When to use it |
|---------|-------------|----------------|
| `/goal <condition>` | Keep the model working across turns until a condition is met. Maps known conditions (e.g., "all tests pass") to shell commands for verification. For unrecognized conditions, the model self-assesses. Max 10 iterations. | "Fix all lint errors", "make all tests pass", "get the build working". Hands-off iterative work. |
| `/swarm [--plan] <task>` | Dispatch parallel subagents to work on a task. With `--plan`, shows a decomposition plan for your review before dispatching. | Large refactors, migrations, or multi-file changes that benefit from parallel execution. |
| `/effort [low\|medium\|high\|max]` | Adjust response depth. Low = concise answers, fewer tool calls. Max = exhaustive analysis, many iterations. Default: medium. | `/effort low` for quick questions. `/effort high` before a thorough security review. |
| `/thinking [off\|low\|medium\|high\|max]` | Control the model's extended thinking budget (reasoning tokens). | When you want the model to think harder about a complex problem, or faster on a simple one. |
| `/auto-approve` | Toggle auto-approval of tool calls for this session. When on, the model runs tools without asking permission. | When you trust the task and don't want to approve every shell command or file edit. |
| `/shell` | Drop into an interactive shell session inside tinycode. Type `exit` to return. | Running a quick sequence of shell commands without tool-call overhead. |

## Context & Information

| Command | What it does | When to use it |
|---------|-------------|----------------|
| `/context` | Show a breakdown of what fills the context window — system prompt, conversation, tool results — with token estimates per category. | When the model seems to be forgetting things, or before deciding whether to compact. |
| `/btw <question>` | Ask a side question without adding to the conversation history. The answer appears as a toast. `/btw` alone shows the last side answer. | Quick lookups mid-task: `/btw what type is this field?` without derailing the current conversation. |
| `/changes` | Show a diff of only the files tinycode modified in this session (excludes pre-existing uncommitted changes). | Before committing — see exactly what tinycode changed, not your other uncommitted work. |
| `/diff` | Show all uncommitted changes in the working directory (full git diff). | Quick look at everything that's changed, regardless of source. |
| `/copy [N]` | Copy the last assistant response to clipboard. Pass N for the Nth-latest. Shows a code-block picker if the response contains fenced blocks. | Grabbing code or output to paste elsewhere. `/copy 2` for the second-to-last response. |
| `/editor [@file]` | Open your `$EDITOR` to compose a prompt. With `@file`, opens that file for editing. | Drafting a long or multi-line prompt, or making a quick file edit outside tinycode. |
| `/paste-image` | Paste an image from the system clipboard and attach it to the next prompt. | Adding screenshots or diagrams as context for the model. |

## Configuration

| Command | What it does | When to use it |
|---------|-------------|----------------|
| `/connect` | Open the model/provider picker. | First thing in a new session, or when switching models. |
| `/theme` | Change the color theme. | Visual preference. The "tinycode" theme matches the brand palette. |
| `/mcp` | Manage MCP (Model Context Protocol) server connections. | Setting up external tool servers (web search, databases, etc.). |
| `/hooks` | Show configured lifecycle hooks — both plugin hooks and shell hooks from settings.json. | Checking what automation runs on session events and tool calls. |
| `/scoped-models` | Toggle model scoping (favorites). Only show preferred models in the picker. | When the provider has many models and you only use a few. |
| `/diagnostics` | Show diagnostics info for bug reports. | When something isn't working and you need to report it. |
| `/help` | Show this command reference and keybindings. | When you forget a command. |

## Skills (AI-driven commands)

Skills inject specialized instructions into the conversation. They're prompts, not code.

| Command | What it does | When to use it |
|---------|-------------|----------------|
| `/ask <agent> <message>` | Send a one-shot message to a specific agent (architect, debugger, executor, etc.) | When you want a specific agent's perspective without switching the session agent. |
| `/review` | Structured code review of the current diff with severity-rated findings. | Before committing — catches bugs, style issues, and security problems. |
| `/debug` | Root-cause analysis for a known failure. Reproduces narrowly, gathers evidence. | When something is broken and you need to find why. |
| `/trace` | Evidence-driven causal tracing with competing hypotheses. | When there are multiple possible causes and you need to rank them. |
| `/verify` | Confirm a change works by building and running the app, not just passing tests. | Before claiming a fix is done — proves it works end-to-end. |
| `/plan` | Create a structured implementation plan with acceptance criteria. | Before starting a complex multi-step task. |
| `/test` | Generate comprehensive test cases for a function or module. | Adding test coverage — happy path, edge cases, error conditions. |
| `/doctor` | Check the tinycode environment — provider connectivity, model health, config audit. Also available as CLI: `tinycode doctor`. | First session, or when something seems misconfigured. |
| `/remember` | Save findings from this session to memory for future conversations. | When you learn something that should persist across sessions. |

## Tools Available to the Model

These are not slash commands — they are tools the model can invoke during a conversation.

| Tool | What it does | When the model uses it |
|------|-------------|----------------------|
| `notepad` | Session-scoped scratch storage (read/write/list/delete, 50 entries max). Survives compaction. | To remember intermediate results, plans, or checklists across a long session. |
| `monitor` | Background process watcher with buffered event delivery (ring buffer, concurrent cap). | To watch a running process (e.g., build, test suite) and report output at turn boundaries. |
| `report_findings` | Structured code review output — file, line, severity, summary per finding. | During code reviews to produce machine-readable, consistently formatted results. |
| `notify` | Desktop notification with urgency levels. Supports WSL. Auto-fires on goal/subagent completion. | To alert you when a long-running task finishes or needs attention. |

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Enter` | Submit prompt |
| `Shift+Enter` | New line in prompt |
| `Tab` | Cycle through agents |
| `@` | File path completion |
| `/` | Command completion |
| `Ctrl+P` | Command palette |
| `Ctrl+C` | Clear prompt or quit |
| `Ctrl+D` | Quit |
| `Ctrl+X` | Leader key (then: `b` sidebar, `a` agents, `m` models, `o` sessions, `n` new session, `e` export, `y` copy, `u` undo, `r` redo, `d` diff, `t` theme) |
| `Esc` | Dismiss dialog / interrupt active request |
