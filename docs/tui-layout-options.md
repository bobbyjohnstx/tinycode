# TUI Layout Redesign Options

Four layout concepts for tinycode's TUI, each distinctly different from the current opencode-derived layout.

## Current Layout (opencode-style)

Right sidebar (30 cols), chat in center, prompt + 2-line status bar at bottom.

```
┌────────────────────────────────────────────────────────────────────────────────┬───────────────────────────────┐
│                                                                                │ Context                       │
│  ┃ User message with thick left border                                         │ 12k tokens · 45% used         │
│  ┃ 3:04 PM                                                                     │ $0.0234 spent                 │
│                                                                                │                               │
│  + Thought: 2.3s                                                               │ MCP                           │
│  Assistant response with markdown rendering                                    │ ● filesystem (3 tools)        │
│    $ bash  $ echo hello                                        done            │ ○ slack                       │
│    ▸ read  internal/tui/app.go                                 done            │                               │
│    ← edit  internal/tui/layout.go                              done            │ Sessions                      │
│  ■ Build · claude-4-opus                                                       │                               │
│                                                                                │ ▸ Fix TUI layout              │
│                              (scrollable viewport)                             │   Refactor sidebar            │
│                                                                                │                               │
│                                                                                │ Agent: Build                  │
│                                                                                │ Model: claude-4-opus          │
│                                                                                │                               │
│                                                                                │ ~/projects/tinycode        │
│                                                                                │ • tinycode 0.1.0              │
├────────────────────────────────────────────────────────────────────────────────┤                               │
│                                                                                │                               │
│  ┃ Ask anything... "Fix a TODO in the codebase"                                │                               │
│  ┃                                                                             │                               │
│  ┃  Build · claude-4-opus · anthropic                                          │                               │
│  ╹▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀│                               │
├────────────────────────────────────────────────────────────────────────────────┴───────────────────────────────┤
│ ·······█▓▒·· esc interrupt                                              tab agents  ctrl+p commands          │
│ ~/projects/tinycode                                                        claude-4-opus  anthropic        │
└───────────────────────────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## Option 1: "Cockpit" — Horizontal Instrument Panel

No sidebar. All metadata in a single top header strip (like tmux status or vim airline).
Chat gets full terminal width at all times.

```
┌───────────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│ ■ Build  │  claude-4-opus · anthropic  │  ██████████░░░░░░░░░░ 45%  12k tok  │  ● fs ● mcp  │  $0.02  │ ?  │
├───────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                                               │
│  ┃ Can you fix the broken test in internal/tui/sidebar_test.go?                                                │
│  ┃ 3:04 PM                                                                                                    │
│                                                                                                               │
│  + Thought: 2.3s                                                                                              │
│                                                                                                               │
│  I'll look at the test file and identify the issue.                                                           │
│                                                                                                               │
│    ▸ read  internal/tui/sidebar_test.go                                                        done           │
│    ← edit  internal/tui/sidebar_test.go                                                        done           │
│                                                                                                               │
│  The test was asserting the wrong expected value for the context percentage.                                   │
│  I've updated line 42 to expect `45` instead of `50`.                                                         │
│                                                                                                               │
│    $ bash  go test ./internal/tui/... -count=1                                                 done           │
│    PASS ok  github.com/bobbyjohnstx/tinycode/internal/tui  0.234s                                         │
│                                                                                                               │
│  All tests pass now. The issue was a stale expected value after the context                                    │
│  calculation was updated in the previous commit.                                                               │
│                                                                                                               │
│  ■ Build · claude-4-opus                                                                                      │
│                                                                                                               │
│                                                                                                               │
│                                                                                                               │
├───────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                                               │
│  ┃ _                                                                                                          │
│  ┃                                                                                                            │
│  ┃  Build · claude-4-opus · anthropic                                                                         │
│  ╹▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀│
│  ·······█▓▒·· working                                    ~/projects/tinycode     tab ctrl+p ctrl+x        │
└───────────────────────────────────────────────────────────────────────────────────────────────────────────────┘
```

**What makes it distinctive**: Top header replaces the sidebar entirely. Context usage is a visual
progress bar. MCP servers are colored dots. Everything in one horizontal strip. At 80 cols, the
header wraps to 2 lines, omitting less-critical fields.

**Differs from opencode**: No right sidebar. Information compressed into a status line format
terminal developers already know. Chat gains ~25% more width.

---

## Option 2: "Split Deck" — Conversation vs. Activity

Two vertical panes. Left = conversation prose only. Right = live activity feed of tool calls,
diffs, and shell output. Separates "what was said" from "what was done."

```
┌──────────────────────────────────────────────────────┬────────────────────────────────────────────────────────┐
│  CONVERSATION                                        │  ACTIVITY                                  ■ Build   │
├──────────────────────────────────────────────────────┼────────────────────────────────────────────────────────┤
│                                                      │                                                       │
│  ┃ Can you fix the broken test in sidebar_test.go?   │  3:04:12  ▸ read sidebar_test.go              done    │
│  ┃ 3:04 PM                                           │  ───────────────────────────────────                   │
│                                                      │  func TestContextPercent(t *testing.T) {              │
│                                                      │      stats := ContextStats{                           │
│  I'll look at the test file and identify the issue.  │  -       Percent: 50,                                 │
│                                                      │  +       Percent: 45,                                 │
│  The test was asserting the wrong expected value for  │      }                                                │
│  the context percentage. I've updated line 42 to     │  ───────────────────────────────────                   │
│  expect `45` instead of `50`.                        │                                                       │
│                                                      │  3:04:14  ← edit sidebar_test.go              done    │
│  All tests pass now. The issue was a stale expected  │                                                       │
│  value after the context calculation was updated in  │  3:04:15  $ bash                               done   │
│  the previous commit.                                │  ───────────────────────────────────                   │
│                                                      │  $ go test ./internal/tui/... -count=1                │
│  ■ Build · claude-4-opus                             │  PASS                                                 │
│                                                      │  ok  .../internal/tui  0.234s                         │
│                                                      │  ───────────────────────────────────                   │
│                                                      │                                                       │
│                                                      │                                                       │
│                                                      │                                                       │
│                                                      │                                                       │
├──────────────────────────────────────────────────────┴────────────────────────────────────────────────────────┤
│                                                                                                               │
│  ┃ _                                                                                                          │
│  ┃  Build · claude-4-opus · anthropic              ██████████░░░░░░░░░░ 45%  12k  $0.02                      │
│  ╹▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀│
│  ·······█▓▒·· working                         ~/projects/tinycode          tab ctrl+p ctrl+x o sessions   │
└───────────────────────────────────────────────────────────────────────────────────────────────────────────────┘
```

**What makes it distinctive**: Tool calls never interrupt conversation flow. Left pane reads like
clean dialogue. Right pane is a real-time activity log with timestamps and expanded output.

**Differs from opencode**: Opencode interleaves tool calls into conversation. This separates them
by content type. No session sidebar — sessions via overlay. At 80 cols, falls back to single
column with tool calls collapsed inline.

---

## Option 3: "Ribbon" — Minimal Chrome, Maximum Canvas

Thin breadcrumb bar at top, floating prompt at bottom. 5 lines of chrome total (vs 9 today).
Everything else accessed through overlays.

```
┌───────────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│  tinycode  ›  Fix TUI layout  ›  Build                                              45% ctx   ·······█▓▒·· │
├───────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                                               │
│                                                                                                               │
│  ┃ Can you fix the broken test in internal/tui/sidebar_test.go?                                                │
│  ┃ 3:04 PM                                                                                                    │
│                                                                                                               │
│                                                                                                               │
│  + Thought: 2.3s                                                                                              │
│                                                                                                               │
│  I'll look at the test file and identify the issue.                                                           │
│                                                                                                               │
│    ▸ read  internal/tui/sidebar_test.go                                                        done           │
│    ← edit  internal/tui/sidebar_test.go                                                        done           │
│                                                                                                               │
│  The test was asserting the wrong expected value for the context percentage.                                   │
│  I've updated line 42 to expect `45` instead of `50`.                                                         │
│                                                                                                               │
│    $ bash  go test ./internal/tui/... -count=1                                                 done           │
│    PASS ok  github.com/bobbyjohnstx/tinycode/internal/tui  0.234s                                         │
│                                                                                                               │
│  All tests pass now. The issue was a stale expected value after the context                                    │
│  calculation was updated in the previous commit.                                                               │
│                                                                                                               │
│  ■ Build · claude-4-opus                                                                                      │
│                                                                                                               │
│                                                                                                               │
│                                                                                                               │
│                                                                                                               │
│                                                                                                               │
│                                                                                                               │
│╶─────────────────────────────────────────────────────────────────────────────────────────────────────────────╴│
│  ┃ _                                                                                                          │
│  ┃                                                                                                            │
│  ┃  Build · claude-4-opus · anthropic · ~/projects/tinycode                                                │
│  ╹                                                                                                            │
└───────────────────────────────────────────────────────────────────────────────────────────────────────────────┘
```

**What makes it distinctive**: Breadcrumb bar provides context without a sidebar. Spinner lives in
the breadcrumb. No status bar — cwd folds into prompt metadata. Resembles a focused writing tool.
Works identically at 80 and 120 cols (no sidebar breakpoint).

**Differs from opencode**: Radically less chrome. Selecting breadcrumb segments opens overlays
(session list, agent list). 4 extra lines of chat area vs current layout.

---

## Option 4: "Gutter" — Left Activity Rail

14-column left rail with visual indicators: vertical context gauge, MCP dots, session list,
spinner. Always visible even at 80 cols. VS Code activity-bar energy.

```
┌──────────────┬────────────────────────────────────────────────────────────────────────────────────────────────┐
│              │                                                                                                │
│   ■ Build    │  ┃ Can you fix the broken test in internal/tui/sidebar_test.go?                                │
│              │  ┃ 3:04 PM                                                                                    │
│  ┌────────┐  │                                                                                                │
│  │████████│  │  + Thought: 2.3s                                                                              │
│  │████████│  │                                                                                                │
│  │████████│  │  I'll look at the test file and identify the issue.                                            │
│  │████████│  │                                                                                                │
│  │▓▓▓▓▓▓▓▓│  │    ▸ read  internal/tui/sidebar_test.go                                        done           │
│  │        │  │    ← edit  internal/tui/sidebar_test.go                                        done           │
│  │        │  │                                                                                                │
│  │        │  │  The test was asserting the wrong expected value for the context                                │
│  │        │  │  percentage. I've updated line 42 to expect `45` instead of `50`.                              │
│  │        │  │                                                                                                │
│  │        │  │    $ bash  go test ./internal/tui/... -count=1                                  done           │
│  │        │  │    PASS ok  .../internal/tui  0.234s                                                           │
│  │        │  │                                                                                                │
│  │        │  │  All tests pass now. The issue was a stale expected value after the                             │
│  └────────┘  │  context calculation was updated in the previous commit.                                       │
│   45% 12k    │                                                                                                │
│   $0.02      │  ■ Build · claude-4-opus                                                                      │
│              │                                                                                                │
│  ── MCP ──   │                                                                                                │
│  ● fs     3  │                                                                                                │
│  ○ slack     │                                                                                                │
│              ├────────────────────────────────────────────────────────────────────────────────────────────────┤
│  ── ses ──   │                                                                                                │
│  ▸ Fix TUI   │  ┃ _                                                                                          │
│    Refactor  │  ┃                                                                                             │
│              │  ┃  Build · claude-4-opus · anthropic                                                          │
│  ·······█▓▒  │  ╹▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀│
│ ·tinycode·   │  ~/projects/tinycode                                         tab ctrl+p ctrl+x            │
└──────────────┴────────────────────────────────────────────────────────────────────────────────────────────────┘
```

**What makes it distinctive**: Narrow left rail with a vertical context gauge bar — you see
context usage at a glance. MCP and session lists are compact but always visible. Only 14 cols
wide (vs 30 for current sidebar). Works at 80 cols (66 cols for chat).

**Differs from opencode**: Sidebar moves left and shrinks from 30→14 cols. Uses visual indicators
(gauge, dots) instead of text labels. Always visible (no toggle). IDE activity-bar feel.

---

## Comparison

| Aspect              | Current (opencode) | Cockpit         | Split Deck       | Ribbon          | Gutter          |
|---------------------|--------------------|-----------------|------------------|-----------------|-----------------|
| Sidebar             | Right, 30 cols     | None (top bar)  | None (overlays)  | None (overlays) | Left, 14 cols   |
| Chat width at 120   | 90 cols            | 120 cols        | 56+56 cols       | 120 cols        | 106 cols        |
| Chrome overhead     | 9 lines            | 10 lines        | 10 lines         | 5 lines         | 8 lines         |
| Status location     | Bottom 2 lines     | Top bar         | Bottom 2 lines   | Breadcrumb+meta | Left gutter     |
| 80-col graceful     | No sidebar         | 2-line header   | Single column    | Same as 120     | Same (66 chat)  |
| Unique element      | Session tree       | Progress bar    | Breadcrumb nav   | Minimal chrome  | Vertical gauge  |
| Tool call display   | Inline in chat     | Inline in chat  | Dedicated pane   | Inline in chat  | Inline in chat  |
| Session access      | Sidebar tree       | Overlay only    | Overlay only     | Breadcrumb tap  | Gutter list     |
