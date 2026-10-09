# LLM Panel — Design Spec

**Date:** 2026-10-09  
**Status:** Approved for implementation

## Goal

Add a toggleable right-side panel that shows a live, unformatted transcript of
LLM exchanges as they stream — text deltas, tool calls, reasoning blocks, and
step boundaries — giving developers a debug view of the model's output separate
from the conversational chat UI.

## Non-goals

- Showing the raw HTTP wire format (request bodies, headers, token counts).
- Showing the full messages array sent to the model each call.
- A third simultaneous panel (left sidebar + right LLM panel at the same time).

## Architecture

### New component: `internal/tui/llmpanel.go`

`LLMPanel` is a bubbletea component with:

```go
type LLMEntry struct {
    Kind    string // "text" | "tool" | "step" | "reasoning"
    Role    string // "assistant" | "tool"
    Content string
}

type LLMPanel struct {
    entries  []LLMEntry
    viewport viewport.Model
    width    int
    height   int
    open     bool
    atBottom bool
}
```

- `Append(e LLMEntry)` adds an entry and scrolls to bottom if `atBottom`.
- `View()` renders each entry with a dim role prefix (`▸ text`, `⚙ tool`, `──`, `~ thinking`) and no markdown processing.
- Scrollable via `j/k` or arrow keys when the panel has focus (future scope; v1 is display-only, always scrolled to bottom).
- Ring buffer: cap at 500 entries to prevent unbounded memory growth.

### Layout changes: `internal/tui/layout.go`

Add `llmPanelWidth int` and `hasLLMPanel bool` to the `layout` struct.

`calculateLayout` gains a `llmPanelOpen bool` parameter alongside the existing
`sidebarOpen bool`. The two are mutually exclusive: if both are true,
`llmPanelOpen` wins (the user explicitly asked for the debug view).

Panel width: fixed at 44 columns (wider than the sidebar's 28 — tool JSON needs room).

`composeView` updated to:
```
[sidebar?] [chat] [llmPanel?]
[prompt]
[status]
```

When `hasLLMPanel` is true, `chatWidth = totalWidth - llmPanelWidth` and the
panel is placed on the right via `JoinHorizontal`.

Minimum terminal width to show the panel: 120 columns (same threshold as sidebar).

### App integration: `app.go` / `app_update.go`

- `App` gains `llmPanel LLMPanel` field.
- `layout` calculation passes `a.llmPanel.IsOpen()` as the new arg; if LLM panel
  open, `sidebarOpen` is forced false for layout purposes.
- `forwardSSEMessages` routes the following existing messages to `llmPanel.Append`:
  - `MessagePartDeltaMsg` with field `"text"` → `LLMEntry{Kind:"text", Role:"assistant", Content: delta}`
  - `MessagePartUpdatedMsg` where part type is `"reasoning"` → `LLMEntry{Kind:"reasoning", ...}`
  - `MessagePartUpdatedMsg` where part type is `"tool"` → `LLMEntry{Kind:"tool", Role:"tool", Content: formatted JSON}`
  - `SessionStatusMsg` with status change to `busy/idle` → `LLMEntry{Kind:"step", Content: "── step start / step end ──"}`

No new bus events, no server-side changes.

### Keybinding: `internal/tui/keys.go`

Add `ToggleLLMPanel key.Binding` with `ctrl+x l` (l for "log").

```go
ToggleLLMPanel key.Binding // ctrl+x l
```

### Slash command: `internal/tui/commands.go`

Add `{Name: "llm-log", Description: "Toggle LLM stream panel", InPalette: true}`.

Handler in `app_update.go` fires `LLMPanelToggleMsg{}` (new msg type in `msg.go`).

## Data flow

```
processor_llm.go
  └─ bus.Publish("session.text.delta" / "session.tool.begin" / ...)
       └─ server SSE stream
            └─ TUI api.Client.Subscribe()
                 └─ run_handlers.go → MessagePartDeltaMsg / MessagePartUpdatedMsg
                      └─ forwardSSEMessages()
                           ├─ chat.go (existing)
                           └─ llmpanel.Append() (new)
```

## Testing

- `llmpanel_test.go`: unit tests for `Append` (ring buffer cap, entry ordering),
  `View` rendering (role prefix present, no markdown leaking through), scroll
  tracking.
- `layout_test.go`: extend existing layout tests with `llmPanelOpen=true` cases;
  verify mutual exclusion with sidebar.
- No integration test changes required (no server-side changes).

## Files touched

| File | Change |
|------|--------|
| `internal/tui/llmpanel.go` | New — LLMPanel component |
| `internal/tui/llmpanel_test.go` | New — unit tests |
| `internal/tui/layout.go` | Add llmPanel slot to layout calc + composeView |
| `internal/tui/msg.go` | Add `LLMPanelToggleMsg` |
| `internal/tui/app.go` | Add `llmPanel` field, wire into NewApp |
| `internal/tui/app_update.go` | Toggle handler, route events to panel |
| `internal/tui/keys.go` | Add `ToggleLLMPanel` binding (`ctrl+x l`) |
| `internal/tui/commands.go` | Add `llm-log` slash command |

## Open questions (resolved)

- **Raw vs structured data**: Use existing structured events (no server changes).
- **Left vs right placement**: Right side, mutually exclusive with left sidebar.
- **Width**: 44 columns (enough for tool JSON); fixed, not resizable in v1.
- **Scroll**: v1 always follows bottom (auto-scroll); keyboard scroll is future scope.
