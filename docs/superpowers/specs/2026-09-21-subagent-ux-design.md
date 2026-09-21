# Subagent UX Design Spec

## Overview

Improve visibility and control of subagent execution in the TUI. Three areas: progress display, descriptive labels, and auto-approve permissions.

## Display: Option A — Inline Collapsible Groups

Subagent tool calls render as collapsible sections in the chat flow, reusing the existing thinking-block toggle pattern.

### Collapsed State (default)

Each subagent is one line showing: toggle indicator, agent label, description, status, timing, and token usage.

```
+ executor-1  Run wc -l on all files                        done 2.4s [1.2k tok]
+ executor-2  Run file on all files                         done 3.1s [1.8k tok]
+ explore-3   Read README.md and summarize                  ⣷⣧⣇⡇⡄⡀⠀⠀
```

- `+` = collapsed, `-` = expanded (click or keyboard to toggle)
- Agent label colored by agent type (`AgentColor()` from `styles.go`)
- Description comes from the task tool's `description` arg
- Status: `done X.Xs [N tok]` or animated `...` for running
- Running subagents show `...` (not braille wave inline)

### Expanded State

```
- executor-1  Run wc -l on all files                        done 2.4s [1.2k tok]
  ┃ $ wc -l *.md                                            done
  ┃ $ wc -l *.svg                                           done
  ┃ Result: 14 files, 847 total lines

+ executor-2  Run file on all files                         done 3.1s [1.8k tok]
+ explore-3   Read README.md and summarize                  ...
```

- Expanded shows child tool calls indented under a colored left-border guide line (`┃`)
- Tool calls use the existing `inlineDetail` rendering (`render/tool.go`) for descriptive labels (e.g., `$ wc -l *.md`, `▸ README.md`, `← edit config.go`)
- Subagent's final text response shown as "Result:" at the bottom

### Keyboard/Mouse Interaction

- Click on the `+`/`-` line toggles expand/collapse (same as thinking blocks)
- Keyboard shortcut to expand/collapse all subagent groups (reuse `T` key pattern)

## Implementation Plan

### Step 1: Add SubagentLabel to PartView
- Add `SubagentLabel string` to `PartView` in `internal/tui/state.go`
- Extract from `part["subagentLabel"]` in `parsePartView` in `internal/tui/cmd.go`
- Preserve across upserts like `ThoughtExpanded`

### Step 2: Add SubagentExpanded State
- Add `SubagentExpanded map[string]bool` to the chat or message level
- Track which subagent labels are expanded (default: all false = collapsed)
- Add toggle logic similar to `toggleThought` in `chat.go`

### Step 3: Group and Render Subagent Parts
- In `renderAssistantMessage` (`chat_render.go`), detect consecutive tool parts with the same `SubagentLabel`
- Render a collapsible group header per unique subagent label
- If collapsed: render one-line summary (label, description, status, timing, tokens)
- If expanded: render left-border guide + indented child tool calls + result text

### Step 4: Consume subagent.completed Events
- Map `subagent.completed` bus event to a TUI message in `mapSSEToMsg`
- Store completion data (timing, token usage) per subagent label
- Update the collapsed line with final stats when the event arrives

### Step 5: Task Tool Description Propagation
- The task tool's `description` arg (set by the LLM) is already in the tool call args
- Extract it in the rendering layer for the group header
- Also extract `subagent_type` for the agent label coloring

## Auto-Approve Permissions

### Implicit with /swarm
When `expandSlashCommand` detects `/swarm`, set a flag on the session that auto-approves all tool permissions for the duration of that prompt. Implementation:
- Add `AutoApprove bool` to `tool.Context` (propagated to subagents via registry copies)
- In `Registry.Execute()`, skip the `Perms.Ask()` call when `AutoApprove` is true
- Set `AutoApprove = true` on the tool context when processing a swarm-expanded prompt

### Manual Toggle
- `/auto-approve on` / `/auto-approve off` slash command
- Toggles `AutoApprove` on the session's tool context
- Status shown in the prompt metadata line

### Config Setting
- `"autoApprove": true` in `tinycode.jsonc`
- Applied at session creation time
- Overridable per-session via the toggle

## Files Touched

| File | Change |
|------|--------|
| `internal/tui/state.go` | Add `SubagentLabel` to `PartView` |
| `internal/tui/cmd.go` | Extract `subagentLabel` in `parsePartView`, map `subagent.completed` |
| `internal/tui/chat.go` | Add `SubagentExpanded` state, toggle logic |
| `internal/tui/chat_render.go` | Group subagent parts, render collapsed/expanded |
| `internal/tui/render/tool.go` | Extract task description for group header |
| `internal/tool/tool.go` | Add `AutoApprove bool` to `Context`, propagate in copies |
| `internal/tool/tool.go` | Skip `Perms.Ask()` when `AutoApprove` is true |
| `internal/command/expand.go` | Set auto-approve flag when expanding `/swarm` |
| `internal/tui/run.go` | Handle `/auto-approve` command |
