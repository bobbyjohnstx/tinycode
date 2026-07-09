# tinycode Cheat Sheet

Quick reference for the most common keyboard shortcuts, agents, and commands.

## Keyboard Shortcuts

**Leader key** = `Ctrl+X` by default (customize in `~/.config/tinycode/config.json`)

| Key | Action |
|-----|--------|
| `Ctrl+C` or `Ctrl+D` or `<leader>q` | Exit tinycode |
| `F1` | Open help dialog |
| `Ctrl+P` | Show command palette |
| `<leader>l` | List all sessions |
| `<leader>n` | Create a new session |
| `<leader>j` / `<leader>k` | Go to first/parent child session |
| `left` / `right` | Cycle to previous/next child session |
| `<leader>m` | List available models |
| `F2` / `Shift+F2` | Cycle to next/previous recent model |
| `<leader>a` | List available agents |
| `Tab` / `Shift+Tab` | Cycle to next/previous agent |
| `<leader>b` | Toggle session tree sidebar |
| `<leader>e` | Open external editor |
| `<leader>t` | Switch theme |
| `<leader>s` | View status |
| `<leader>c` | Compact the session |
| `<leader>x` | Export session transcript |
| `<leader>y` | Copy message |
| `<leader>u` / `<leader>r` | Undo/redo message |
| `<leader>h` | Toggle code block concealment |
| `Ctrl+R` | Rename session |
| `Ctrl+D` | Delete session |
| `Escape` | Interrupt current session |
| `Ctrl+F` | Pin/unpin session |
| `<leader>1` - `<leader>9` | Quick-switch to session slot 1-9 |
| `Ctrl+Alt+K` | Toggle which-key panel |

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
| `/tc-doctor` | Diagnose tinycode configuration and environment issues |
| `/trace` | Evidence-driven causal tracing with competing hypotheses |
| `/verify` | Confirm changes work before claiming completion |

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

### Session navigation
```
<leader>j          # Go to first child session
<leader>k / up     # Go to parent session
left / right       # Cycle through siblings
```

## Configuration

Edit `~/.config/tinycode/config.json`:

```json
{
  "model": "ollama/llama3.2",
  "lsp": true
}
```

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
- Use `<leader>c` to compact a session before exporting
- Export to HTML with `tinycode export --format html <session-id>`
- Type `@filename` to reference a file in your prompt
- Use `<leader>h` to collapse code blocks and focus on analysis
- Session tree shows hierarchy with `<leader>b`
- **Tool-call warnings:** If you see "Multiple tool call failures detected," switch to a larger model via `<leader>m` — tinycode auto-repairs common JSON issues, but very small models may not support tool calling at all
