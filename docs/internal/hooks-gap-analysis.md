# Hooks Gap Analysis — Event and Type Coverage

Date: 2026-10-02
Source: Competitive analysis of upstream AI coding assistant hooks reference

## Current Hook Events (6)

| Event | Description | Can abort? |
|-------|-------------|-----------|
| `session.start` | Session created | No |
| `session.end` | Session deleted | No |
| `tool.execute.before` | Before tool executes | Yes |
| `tool.execute.after` | After tool completes | No |
| `permission.ask` | Permission prompt | Yes |
| `shell.env` | Shell environment setup | No |

## Current Hook Types (2)

| Type | Description |
|------|-------------|
| `command` | Shell command (exec.Command) |
| `shell` | Settings.json shell hooks (#345) — lightweight alternative to plugin hooks |

## Hook Events to Add (4 issues created)

### #366 — Stop hook event (HIGH priority)
Fires when the model finishes responding. Hook decides if the model should continue.
- Generalizes `/goal` — any shell command as the stop condition
- Safety: `stop_hook_active` flag + 8-continuation cap
- Receives `last_assistant_message` for inspection

### #367 — PostCompact hook event (MEDIUM priority)
Fires after compaction completes.
- Critical for notepad (#363) — injection point for "your notepad has N entries" hints
- Receives trigger type (manual/auto) and compact summary

### #368 — FileChanged hook event (MEDIUM priority)
Fires when a watched file changes on disk, regardless of source.
- Catches changes from shell commands, external processes, not just tinycode tools
- Use: auto-format, direnv integration, config reload
- Uses `fsnotify` for filesystem watching
- Access to `CLAUDE_ENV_FILE` for environment persistence

### #369 — HTTP hook type (LOW priority)
Send hook events to webhook URLs instead of running shell commands.
- POST event JSON to URL, parse response for decisions
- Environment variable interpolation in headers
- Use: logging, CI/CD integration, custom dashboards

## Hook Events Evaluated and Skipped

### Medium value — could add later

| Event | Description | Why deferred |
|-------|-------------|-------------|
| PostToolBatch | Fire once after all parallel tools complete | Useful for cross-tool context injection. Add when parallel tool execution is more common. |
| ConfigChange | React to settings file changes | Audit/enforcement use case. Add when enterprise deployment needs grow. |
| UserPromptSubmit | Validate/filter prompts before they reach the model | Could block sensitive content. Add when content filtering is needed. |
| PreCompact | Fire before compaction | Could block compaction or modify the summary strategy. Add alongside PostCompact if needed. |
| CwdChanged | React to directory changes | Environment reload, per-project toolchains. Add alongside FileChanged. |

### Skipped — cloud/enterprise-specific or unnecessary

| Event | Why skip |
|-------|----------|
| PreModelSwitch / PostModelSwitch | tinycode doesn't have complex model-switching (no automatic fallbacks, no effort-based model selection). A simple `/model` command is sufficient. |
| Elicitation / ElicitationResult | MCP-specific UI interaction for form-based user input. tinycode uses the question tool for this. |
| WorktreeCreate / WorktreeRemove | Worktree tools (#360) deferred. No need for hooks on nonexistent functionality. |
| SubagentStart / SubagentStop | Subagent lifecycle hooks are niche. The existing `tool.execute.before/after` covers tool-level events. |
| TeammateIdle | Agent teams not implemented in tinycode. |
| TaskCreated / TaskCompleted | TodoWrite is the task mechanism. Hooks on individual task creation/completion add marginal value. |
| MessageDisplay | Would require intercepting the bubbletea render loop. Complex for minimal value. |
| Setup | `--init-only` mode not implemented. Add if CI/CD initialization workflows are needed. |
| InstructionsLoaded | CLAUDE.md loading doesn't need hooks — file is loaded once at startup. |
| StopFailure | API error handling is internal. Hooks on errors add limited value for local-first tool. |
| DirectoryAdded | `/add-dir` not implemented as a command. |
| Notification | Desktop notifications handled by the notify plugin (#361). |
| PostToolUseFailure | Covered by `tool.execute.after` with error status. |

## Hook Types Evaluated and Skipped

| Type | Why skip |
|------|----------|
| Prompt hooks | Requires an LLM call per hook firing — expensive on local models with limited context. `/goal` covers the main use case for LLM-evaluated stop conditions. |
| Agent hooks | Same concern — spawning a subagent per hook is expensive and slow. Niche use case. |
| MCP tool hooks | Useful but niche. MCP servers can expose hooks as tools directly. Defer until MCP hook patterns are established. |

## Hook Configuration Features to Consider

| Feature | Upstream has | tinycode has | Worth adding? |
|---------|-------------|-------------|---------------|
| Matcher patterns | Regex + exact string + pipe-separated | Match filter in HookConfig | Enhance to support pipe-separated tool names |
| `if` condition | Permission rule syntax filter | Not implemented | Medium value — filter hooks by tool arguments |
| Async hooks | `async: true` for background execution | Not implemented | Medium value — long-running hooks shouldn't block |
| `timeout` field | Per-hook timeout (default 600s) | Shell hooks have 10s default | Already sufficient for local-first |
| `additionalContext` | Inject text into model's context | Not implemented | HIGH value — the main way hooks communicate with the model |
| `updatedInput` | Modify tool arguments before execution | Not implemented | Medium value — rewrite tool inputs via hooks |
| `updatedToolOutput` | Replace tool output | Not implemented | Medium value — redaction/transformation |

## Reassessment Criteria

A skipped hook event or type should be reconsidered if:
1. User demand — multiple users request the capability
2. Feature addition — e.g., implementing worktree tools makes WorktreeCreate/Remove hooks relevant
3. Deployment pattern change — e.g., enterprise deployment makes ConfigChange hooks valuable
