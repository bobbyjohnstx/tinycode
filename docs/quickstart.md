# First 5 Minutes with tinycode

A hands-on walkthrough to get productive fast. You should have tinycode built already -- see [install.md](install.md) if not.

## 1. First-run path (diagnose + connect)

There is no `tinycode setup` wizard. First run is intentional and sufficient with two steps:

1. **`tinycode doctor`** — diagnostic only (config, database, providers, agents, plugins). It does not configure models; it tells you what is missing.
2. **`tinycode` (TUI)** — if no model was auto-discovered, the connect dialog opens automatically (same UI as `/connect`). Esc dismisses; run `/connect` anytime to reopen. Or set env vars / config before launch.

```bash
tinycode doctor          # diagnose; exit non-zero if critical checks fail
tinycode                 # connect dialog opens when no model is available
```

## 2. Start tinycode

```bash
# From your project directory
./dist/tinycode

# Or if installed to PATH
tinycode

# Or point it at a specific project
tinycode ~/projects/my-app
```

You see an animated boot sequence that checks each subsystem. Green check marks mean everything is ready. If a provider check fails (red X), tinycode still launches -- you can connect a provider next.

## 3. Connect to a provider

If no model was auto-discovered, type `/connect` and press Enter. This opens a two-step dialog:

1. **Select a provider** -- Ollama, LM Studio, OpenRouter, or any discovered provider
2. **Select a model** -- type to filter the list (e.g., "qwen" or "llama")

For local models, [Ollama](https://ollama.com) is the fastest way to start:

```bash
# In another terminal
ollama pull qwen3:8b
ollama serve
```

Then `/connect` in tinycode and select Ollama.

For cloud models via OpenRouter, set your API key before launching:

```bash
OPENROUTER_API_KEY=your-key tinycode
```

Local providers on a non-default host:

```bash
export TINYCODE_OLLAMA_HOST=http://your-host:11434   # preferred; OLLAMA_HOST is the fallback
export TINYCODE_VLLM_HOST=http://localhost:8000
export TINYCODE_LMSTUDIO_HOST=http://localhost:1234
```

## 4. Send your first prompt

Type a question at the bottom and press Enter:

```
what does this project do?
```

The model reads your project files using built-in tools (read, grep, glob, bash) and responds with a summary. Responses stream in real-time. Scroll up with PgUp or mouse wheel to review long answers.

Press **Escape** to interrupt if the model is taking too long. Press **Ctrl+C** to clear the prompt (or quit if the prompt is already empty).

## 5. Try @ file references

Type `@` to trigger file autocomplete. A dropdown shows files in your project directory:

```
@src/main.go explain the entry point
```

- Directories appear with a trailing `/` -- select one to drill into it
- Use Up/Down arrows to navigate, Tab or Enter to confirm
- The file contents are included as context with your prompt

Reference multiple files in one prompt:

```
compare @go.mod with @go.sum and check for issues
```

## 6. Use an agent

Press **Tab** to cycle through agents. The current agent name appears in the status bar on the right. Each agent has a different specialty:

- **build** (default) -- general coding, delegates complex work
- **architect** -- design decisions, system-level analysis (read-only)
- **debugger** -- root-cause analysis, stack traces
- **executor** -- focused implementation, smallest viable diff
- **code-reviewer** -- severity-rated code review

Or invoke an agent for a single question without switching:

```
/ask architect should we split this into microservices?
/ask debugger why is TestAuth failing?
```

Press **Ctrl+X a** to open the full agent list.

## 7. Run /swarm

Swarm mode dispatches parallel subagents for independent tasks:

```
/swarm review the config, provider, and agent packages
```

The build agent splits the task into subtasks and runs them simultaneously. Each subagent works independently, and results are synthesized into a single report.

Swarm auto-approves tool permissions so subagents can work unattended.

## 8. Explore keybindings

Press **Ctrl+X** to see the which-key panel -- a floating overlay showing all leader key follow-ups:

```
Navigation        Edit              Tools             Actions
  b  sidebar        e  $EDITOR        a  agent list      y  copy response
  o  session list   d  diff viewer    m  model list      x  export session
  n  new session    u  undo           t  theme picker
                    r  redo           i  MCP servers
```

Press any follow-up key within 2 seconds. For example, **Ctrl+X d** opens the diff viewer, **Ctrl+X t** opens the theme picker.

Press **Ctrl+P** to open the command palette showing all keybindings and slash commands.

## 9. Search the chat

Press **Ctrl+F** to open the search bar at the top of the chat viewport. Type a query to find text in the conversation:

- **Ctrl+N** or **Enter** -- jump to next match
- **Ctrl+P** -- jump to previous match
- **Escape** -- close search

The search bar shows your position (e.g., "3/12") and auto-scrolls to the message containing the current match.

## 10. Undo changes

If the model edited files and you want to revert:

```
/undo
```

This restores files to their state before the last AI edit. Use `/redo` to bring the changes back. You can also use **Ctrl+X u** and **Ctrl+X r**.

Review what changed before committing:

```
/diff
```

This opens an inline diff viewer showing all uncommitted changes in the working directory. Or use **Ctrl+X d**.

## 11. Workflow commands

A few commands that change how tinycode works during a session:

```
/effort high          # More thorough analysis, more tool calls
/effort low           # Quick concise answers
/goal all tests pass  # Autonomous loop — keeps working until the condition is met
/branch experiment    # Fork this conversation to try a different approach
/rewind               # Roll back to an earlier turn if something went wrong
/context              # See what's filling up the context window
```

`/goal` is especially useful for iterative tasks like fixing lint errors or getting a build working — it maps conditions to shell commands and loops automatically (max 10 iterations).

## 12. Export your session

Save the conversation for reference:

```
/export
```

This writes a Markdown file (`session-<title>.md`) to the working directory. Or use **Ctrl+X x**.

For a formatted version with syntax highlighting:

```
/export html
```

This creates a self-contained HTML file you can open in any browser or share.

## 13. Get help

```
/help
```

Opens the command palette showing every keybinding and slash command. You can also press **Ctrl+P**.

For a list of all CLI subcommands:

```bash
tinycode help
```

### Other useful commands

| Command | What it does |
|---------|--------------|
| `/thinking high` | Enable extended reasoning (1k/4k/16k/128k token budget) |
| `/theme` | Pick a color theme with live preview |
| `/paste-image` | Paste a clipboard image for vision models |
| `/mcp` | Manage MCP server connections |
| `/scoped-models` | Mark favorite models so only they appear in the selector |
| `/archive` | Soft-delete the current session |
| `/shell` | Drop into an interactive shell, return to tinycode on exit |
| `/editor` | Open `$EDITOR` to compose a long prompt |
| `/editor @file` | Open a file in `$EDITOR` for direct editing |
| `/auto-approve` | Toggle auto-approve for tool permissions this session |
| `/diagnostics` | Show diagnostics (config, providers, agents, system info) |

### Bundled skills

Seven built-in skills expand into the current prompt. `/debug`, `/trace`, `/plan`, `/verify`, `/test`, and `/review` instead ask the matching agent to do the job. Adding either kind is described in [authoring.md](authoring.md).

| Command | What it does |
|-------|--------------|
| `/debug` | Ask debugger for one root cause |
| `/verify` | Ask verifier for proof a change works |
| `/trace` | Ask tracer to rank competing explanations |
| `/review` | Ask code-reviewer to review a change |
| `/plan` | Ask the plan agent for a work plan |
| `/test` | Ask test-engineer to write tests |
| `/doctor` | Diagnose the tinycode environment |
| `/mcp-setup` | Guided MCP server configuration |
| `/remember` | Triage session findings across memory surfaces |
| `/deepinit` | Deep project initialization and onboarding |
| `/incident` | Triage a live system failure |
| `/change` | Plan one cluster or host change |
| `/host` | Inspect a machine, local or over ssh |

### Terminal bell

tinycode rings the terminal bell when a task finishes or when a permission prompt appears. If you work in another window while the model is running, you hear the bell when it needs your attention. Configure bell behavior in your terminal settings (audible vs. visual).

### Quick diagnostics

Run `tinycode doctor` at any time to verify all subsystems (config, database, providers, agents, plugins, skills):

```bash
tinycode doctor
```

### Session resume

Resume where you left off without the TUI session picker:

```bash
tinycode -c                        # Continue the most recent session
tinycode -r "my feature work"     # Resume by title substring
```

## Other run modes

```bash
tinycode run -m ollama/qwen3.5:9b "explain the main function"   # one prompt, then exit
tinycode run --multi-turn --format json -m ollama/qwen3.5:9b    # loop on stdin, NDJSON events
tinycode serve    # headless API and the thin ops console
tinycode web      # browser chat UI
tinycode acp      # IDE integration over stdio
```

Flags, NDJSON events, and permission handling are in the [user guide](user-guide.md#run-mode).

## If something fails

- **Model not found.** Pull it first: `ollama pull <model>`.
- **Cannot reach Ollama.** `ollama serve` must be running. On another host, set `TINYCODE_OLLAMA_HOST`.
- **No tool calls.** The warmup probe checks tool-call support. A model that fails it still answers in text. Larger models (9B and up) usually do better.

More cases: [troubleshooting.md](troubleshooting.md).

## Next steps

- Read the full [User Guide](user-guide.md) for detailed coverage of every feature
- See [Plugin Development](plugin-development.md) to build custom tool plugins
- Check [Architecture](architecture.md) for how tinycode works internally
- Run `tinycode init` for optional Red Hat plugin/role setup (models: `/connect`, `OPENROUTER_API_KEY`, or Ollama)
