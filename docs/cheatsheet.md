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
| `↑` / `↓` | Navigate prompt history (in composer) |
| `Ctrl+R` | Open prompt history browser (pick a past prompt) |
| `Ctrl+S` | Stash current draft (save) / restore it (press again when empty) |
| `<leader>b` | Toggle session tree sidebar |
| `<leader>o` | List all sessions |
| `<leader>n` | Create a new session |
| `<leader>m` | List available models |
| `<leader>a` | List available agents |
| `f` | Fork session at selected turn (in `/rewind` dialog) |

## Agents

Press **Tab** to cycle, or `<leader>a` to list. Use `/ask <agent> <prompt>` to invoke any agent inline.

Tab cycles `build`, `general`, `ops`, `plan`, `architect`, and `code-reviewer`. `<leader>a` lists the rest. `code-simplifier`, `qa-tester`, and `scientist` ship disabled.

| Agent | Use when... |
|-------|------------|
| `build` | Doing ordinary coding. Small work stays here; the rest is delegated |
| `general` | Asking a direct question |
| `ops` | Working on a cluster, a host, or an ssh target. Read-only first |
| `plan` | Writing a work plan. Edits stay in `plans/` and `drafts/` |
| `analyst` | Turning decided scope into acceptance criteria |
| `architect` | Choosing a design. Known failures go to debugger |
| `code-reviewer` | Reviewing a code change |
| `critic` | Reviewing a plan for gaps |
| `debugger` | Finding one root cause in application code |
| `designer` | Building UI |
| `document-specialist` | Reading external SDK docs or a changelog |
| `executor` | Applying a scoped code change |
| `explore` | Finding a file or symbol in this repo |
| `git-master` | Committing, rebasing, or reading history |
| `scout` | Reading upstream dependency source |
| `security-reviewer` | Looking for exposure in source |
| `test-engineer` | Writing tests |
| `tracer` | Ranking competing explanations |
| `verifier` | Proving a change works |
| `writer` | Writing technical documentation |

## Skills (Slash Commands)

Type `/` to autocomplete. Use before or after your prompt.

| Command | Purpose |
|---------|---------|
| `/debug` | Ask debugger for one root cause |
| `/trace` | Ask tracer to rank competing explanations |
| `/plan` | Ask the plan agent for a work plan |
| `/verify` | Ask verifier for proof a change works |
| `/test` | Ask test-engineer to write tests |
| `/review` | Ask code-reviewer to review a change |
| `/incident` | Triage a live system failure: impact, evidence, one next command |
| `/change` | Plan one cluster or host change: the command, the check, and the undo |
| `/host` | Inspect this machine, or another host over ssh |
| `/doctor` | Diagnose the tinycode environment (also: `tinycode doctor`) |
| `/mcp-setup` | Configure MCP servers |
| `/remember` | Triage findings to memory surfaces |
| `/deepinit` | Generate per-directory `AGENTS.md` files |

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
```

### Debug a failing test
```
/ask debugger why is internal/session/processor_test.go failing?
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
- **Prompt stash:** `Ctrl+S` parks your current draft (clearing the composer) so you can run a side command or wait on a permission prompt; press `Ctrl+S` again (with an empty composer) to restore it. The stash is in-memory only — session-local, not persisted to disk, and lost on restart — but it does survive the composer being cleared after a submit
- **Prompt history browser:** `Ctrl+R` opens a scrollable list of past prompts (newest first) instead of relying on bare `↑`/`↓`; a status hint below the composer also shows the entry count once you have history
- **Model warmup:** On startup, tinycode probes the Ollama model to verify tool-call support and pre-load it into GPU memory. Look for "qwen3.5:9b ready — tool calling supported" in the toast/footer
- **Tool-call warnings:** If you see "Multiple tool call failures detected," switch to a larger model via `<leader>m` — tinycode auto-repairs common JSON issues, but very small models may not support tool calling at all
