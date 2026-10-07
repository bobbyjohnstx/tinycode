# tinycode Cheat Sheet

Quick reference for the most common keyboard shortcuts, agents, and commands.

## Keyboard Shortcuts

**Leader key** = `Ctrl+X` by default (500ms timeout for follow-up key)

| Key | Action |
|-----|--------|
| `Ctrl+C` | Clear prompt / quit (if empty) |
| `Ctrl+D` | Quit tinycode |
| `Ctrl+P` | Command palette (commands, agents, sessions, skills) |
| `Enter` | Submit prompt |
| `Shift+Enter` / `Alt+Enter` | Insert newline in prompt |
| `Escape` | Interrupt current session |
| `Tab` / `Shift+Tab` | Cycle to next/previous agent |
| `F2` / `Shift+F2` | Cycle to next/previous recent model |
| `PgUp` / `PgDown` | Scroll chat history |
| `<leader>b` | Toggle session tree sidebar |
| `<leader>o` | List all sessions |
| `<leader>n` | Create a new session |
| `<leader>m` | List available models |
| `<leader>a` | List available agents |

## Agents

Press **Tab** to cycle, or `<leader>a` to list. Use `/ask <agent> <prompt>` to invoke any agent inline.

| Agent | Use when... |
|-------|------------|
| `architect` | Need to analyze code design, architecture review, or technical guidance (read-only) |
| `code-reviewer` | Need severity-rated code review with SOLID principle checks |
| `code-simplifier` | Need to refactor recent changes for clarity and maintainability |
| `critic` | Need multi-perspective quality review of plans and code |
| `debugger` | Need root-cause analysis or bug fixing |
| `designer` | Need to build production-grade UI/UX |
| `document-specialist` | Need to understand external libraries or API references |
| `executor` | Need focused implementation of a scoped task |
| `explore` | Need fast codebase search (grep/glob) |
| `git-master` | Need help with git history, rebasing, or atomic commits |
| `planner` | Need strategic planning and work breakdown |
| `qa-tester` | Need interactive CLI testing |
| `scientist` | Need data analysis or evidence-driven research |
| `security-reviewer` | Need security vulnerability detection |
| `test-engineer` | Need test strategy or TDD workflows |
| `tracer` | Need evidence-driven causal tracing with hypotheses |
| `verifier` | Need to verify work is actually complete |
| `workspace` | Need to set up development environment |
| `writer` | Need technical documentation |

**Special agents:**
- `build` — Full tool access (default)
- `plan` — Read-only, write-protected plan mode
- `cluster-admin` — Kubernetes/OpenShift cluster operations
- `analyst` — Requirements analysis before planning

## Skills (Slash Commands)

Type `/` to autocomplete. Use before or after your prompt.

| Command | Purpose |
|---------|---------|
| `/ai-slop-cleaner` | Clean up AI-generated code with regression-safe deletion-first workflow |
| `/configure-notifications` | Set up Telegram, Discord, or Slack notifications |
| `/debug` | Isolate a single most-likely root cause for a failure |
| `/deepinit` | Generate per-directory `AGENTS.md` files across the codebase |
| `/mcp-setup` | Configure MCP servers via guided menu |
| `/remember` | Triage findings to memory surfaces (project memory, CLAUDE.md, session notes) |
| `/doctor` | Full diagnostic skill — provider connectivity, model health, config audit (also: CLI `tinycode doctor`) |
| `/trace` | Evidence-driven causal tracing with competing hypotheses |
| `/verify` | Confirm changes work before claiming completion |

**CLI commands:**

| Command | Purpose |
|---------|---------|
| `tinycode run "prompt"` | Run a prompt non-interactively and exit |
| `tinycode run --format json` | NDJSON event output (for programmatic consumers) |
| `tinycode run --multi-turn` | Multi-turn mode: loop on stdin after initial prompt |
| `tinycode run --max-iterations N` | Cap processor iterations per prompt |
| `tinycode run --permissions json` | Programmatic permission handling via stdin/stdout |
| `tinycode run --dangerously-skip-permissions` | Auto-approve all tool permissions |
| `tinycode doctor` | Headless health check (config, DB, providers, agents, plugins, skills) |
| `tinycode mcp list` | List configured MCP servers + connection status |
| `tinycode mcp add NAME -- CMD…` | Add stdio MCP server (use `--project` for project config) |
| `tinycode mcp add --transport http NAME URL` | Add remote streamable-http MCP server |
| `tinycode mcp auth NAME --env VAR` | Set Bearer `{env:VAR}` (or `--token`) |
| `tinycode mcp logout NAME` | Clear Authorization header |
| `tinycode mcp debug NAME` | Handshake / tools diagnostics |
| `tinycode plugin list` | List installed / available plugins |
| `tinycode plugin install <name>` | Install a plugin binary (`--from` for a local path) |
| `tinycode plugin uninstall <name>` | Remove an installed plugin |

## Common Workflows

### Start a new conversation
```
<leader>n          # Create session
Type your prompt
Return             # Submit
```

### Switch models
```
<leader>m          # List models
Select one
```

### Use an agent for a specific task
```
/ask architect write a summary of the auth flow
/ask test-engineer write unit tests for this component
```

### Review and refactor code
```
<leader>a          # Switch to code-reviewer
Paste or reference the code
Return
<leader>a          # Then switch to code-simplifier
Follow the suggestions
```

### Debug a failing test
```
/ask debugger why is src/session/processor.test.ts failing?
/debug              # (if the agent found a likely cause)
```

### Generate documentation
```
/ask writer        # Switch agent
Write guide for new developers on session architecture
```

### Session navigation (vim-style hjkl)
```
<leader>h          # Go to previous sibling session
<leader>j          # Go to first child session
<leader>k          # Go to parent session
<leader>l          # Go to next sibling session
```

### Scripted / CI usage (run mode)
```bash
# Single prompt, auto-approve tools
tinycode run --dangerously-skip-permissions -m ollama/qwen3:8b "fix lint errors"

# Pipe input, JSON output
echo "explain main.go" | tinycode run --format json -m ollama/qwen3:8b

# Multi-turn with iteration cap
tinycode run --multi-turn --max-iterations 20 -m ollama/qwen3:8b
```

## Configuration

Edit `~/.config/tinycode/config.json`:

```json
{
  "model": "ollama/qwen2.5:latest"
}
```

LSP is enabled by default; set `"lsp": false` to disable.

Set leader key to something else (e.g., space):

```json
{
  "keybinds": {
    "leader": "space"
  }
}
```

## Model Shortcuts

After listing models (`<leader>m`):
- `Ctrl+F` — Toggle favorite status
- `Ctrl+A` — Show provider list
- `Return` — Select

## Tips

- Press `?` in diff viewer for more navigation shortcuts
- Sessions form a tree — child sessions inherit context from parent
- Use `<leader>c` or `/compact` to compact a session (runs real summarization via `Processor.Compact`) before exporting
- Export to HTML with `tinycode export --format html <session-id>`
- Type `@filename` to reference a file in your prompt
- Use `<leader>;` to collapse code blocks and focus on analysis
- Session tree shows hierarchy with `<leader>b`
- **Model warmup:** On startup, tinycode probes the Ollama model to verify tool-call support and pre-load it into GPU memory. Look for "qwen3.5:9b ready — tool calling supported" in the toast/footer
- **Tool-call warnings:** If you see "Multiple tool call failures detected," switch to a larger model via `<leader>m` — tinycode auto-repairs common JSON issues, but very small models may not support tool calling at all
