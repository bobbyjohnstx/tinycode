# tinycode TUI — UI/UX Review

> Combined findings from architect, critic, and designer agent reviews.
> Use this document to drive planning with the planner agent.

---

## Executive Summary

The tinycode TUI is well-engineered underneath — the rendering stack (SolidJS + opentui), theme system (33 themes), diff viewer, and plugin architecture are all production-quality. However, the user-facing layer has **critical discoverability and accuracy problems** that actively mislead first-time users. The top issues are: help.md documents wrong keybindings, the which-key discoverability panel is disabled by default, and "tinycode" branding still appears in multiple places.

---

## P0 — Critical (Actively Misleads Users)

### 1. help.md documents WRONG keybindings

**All three reviewers flagged this.**

| help.md claims                      | Actual binding                  | Real action                          |
| ----------------------------------- | ------------------------------- | ------------------------------------ |
| `ctrl+p` = Toggle plan mode         | `ctrl+p` = `command_list`       | Opens command palette                |
| `ctrl+c` = Cancel current operation | `ctrl+c` = `input_clear`        | Clears input (or exits app if empty) |
| `ctrl+r` = New session              | `ctrl+r` = `session_rename`     | Renames current session              |
| `ctrl+f` = Search sessions          | `ctrl+f` = `session_pin_toggle` | Pins session in list                 |

**Correct mappings to document:**

- New session = `ctrl+x n` (leader+n)
- Session list = `ctrl+x l` (leader+l)
- Interrupt session = `escape` (press twice)
- Command palette = `ctrl+p`
- Rename session = `ctrl+r`

**Files:** `help.md:40-45`, `keybind.ts:59,90,102,156`

### 2. which-key panel is disabled by default

The which-key panel shows available keybindings when leader key is pressed — it's the primary discoverability mechanism and it's off. Users who press `ctrl+x` get zero feedback about what comes next.

**Fix:** Change `enabled: false` → `enabled: true` at `which-key.tsx:604`

### 3. help dialog renders markdown as plain text

The help dialog dumps raw markdown syntax — headers show as `## text`, tables show as `| pipe | syntax |`. The session view has a proper `<markdown>` renderer but the help dialog doesn't use it.

**Fix:** Replace `<text>{HELP_CONTENT}</text>` with `<markdown>` component at `dialog-help.tsx:70`

---

## P1 — High Priority (Confusing UX)

### 4. Keybinding duplicates

| Key         | Bindings                                                                 | Risk                                        |
| ----------- | ------------------------------------------------------------------------ | ------------------------------------------- |
| `<leader>h` | `messages_toggle_conceal` + `tips_toggle`                                | Same key, different contexts                |
| `ctrl+d`    | `app_exit` + `session_delete` + `stash_delete` + `input_delete`          | 4-way overlap, dangerous for exit vs delete |
| `ctrl+f`    | `session_pin_toggle` + `model_favorite_toggle` + `permission_fullscreen` | 3-way overlap across dialogs                |

**Files:** `keybind.ts:144,217` (leader+h), `keybind.ts:50,91,113,184` (ctrl+d), `keybind.ts:102,115,211` (ctrl+f)

### 5. ctrl+x leader key conflicts with terminal cut

`ctrl+x` is the universal cut shortcut. Using it as a leader key means every user who muscle-memories cut will activate leader mode. The 2-second timeout (`KeymapLeaderTimeoutDefault = 2000`) means 2 seconds of "nothing works." No visual feedback that leader mode is active (unless which-key is enabled).

**Files:** `keybind.ts:43`, `tui-schema.ts:31`

### 6. Subagent navigation paradigm shift

Parent → child: `ctrl+x j` (leader key chord)
Inside child: `up` (parent), `left/right` (prev/next siblings)
Arrow keys work because child view has no prompt, but this is non-obvious and undiscoverable without the SubagentFooter hint text.

**Fix:** Add `<leader>k` for parent (mirror of `<leader>j`) alongside arrow keys.

**Files:** `keybind.ts:98-101`, `subagent-footer.tsx:96-128`, `index.tsx:444-449`

### 7. "tinycode" branding remnants (user-visible)

| Location       | Text                                    | File:Line                    |
| -------------- | --------------------------------------- | ---------------------------- |
| Docs command   | Opens `https://tinycode.dev/docs`       | `app.tsx:805`                |
| Update dialog  | "Successfully updated to tinycode v..." | `app.tsx:1045`               |
| Plugin routes  | Terminal title "OC \|" (should be "TC") | `app.tsx:476`                |
| Sidebar footer | Renders "Open" "Code" as product name   | `sidebar/footer.tsx:70-72`   |
| Sidebar tip    | "tinycode includes free models..."      | `sidebar/footer.tsx:52`      |
| Retry dialog   | Links to `tinycode.dev/go`              | `dialog-retry-action.tsx:10` |

### 8. No help keybinding

`help_show` defaults to `"none"` — no keyboard shortcut to open help. Must type `/help` or find it via `ctrl+p` command palette.

**Fix:** Bind to `?` (when prompt empty) or `F1` at `keybind.ts:60`

---

## P2 — Medium Priority (Polish & Consistency)

### 9. No onboarding for first-time users

New user sees: logo, prompt placeholder ("Fix a TODO"), and maybe a random tip. No guidance on `@` for agents, `/` for commands, `ctrl+p` for palette. The `/connect` hint appears in the footer but auto-dismisses after 5 seconds.

### 10. export command description is misleading

`keybind.ts:84`: "Export session to editor" — actually opens a dialog to save as markdown file with options. Should say "Export session transcript."

### 11. Sidebar session title not truncated

Long auto-generated titles wrap multiple times in the 42-char sidebar, pushing all other content down. No `truncate()` applied.

**File:** `sidebar.tsx:56`

### 12. No visual separator between content and sidebar

Content area and sidebar have no border/divider between them. The sidebar has `backgroundPanel` but blends into the content area on some themes.

### 13. Spinner color doesn't change on retry/error

During retry states, the Knight Rider spinner stays agent-colored (looks "happy") while error text is red. Should use `theme.error` during retries.

**File:** `prompt/index.tsx:1447-1469`

### 14. Single toast at a time

New toasts replace previous ones. During provider connection flows, multiple operations fire toasts in succession and earlier ones get lost.

### 15. ctrl+c overloaded

`ctrl+c` = clear input (when text present) OR exit app (when empty). Users expect `ctrl+c` = interrupt. Actual interrupt is `escape`. The double-press escape behavior ("press again to interrupt") is subtly communicated.

### 16. Session interrupt requires double-escape

First `escape` arms, second `escape` interrupts (within 5s). The hint text changes from "esc interrupt" to "again to interrupt" in `theme.primary` color — easy to miss.

---

## P3 — Low Priority (Minor Polish)

### 17. Tips show only one random tip per session launch

The tip offset is set via `Math.random()` once. With 60+ tips, most users never see the `@` mention tip or leader key tip.

### 18. No model display on home screen

Home footer shows directory, MCP count, and version — but not the currently selected model. Model is only visible in the session footer.

### 19. Remove "ok" button from help dialog

No other dialog has a confirmation button. `esc`/`enter` already close it. Inconsistent with the rest of the UI.

### 20. Unicode character compatibility

`"■"` (filled square) in assistant metadata may not render in all terminal fonts. The logo's half-block sub-pixel rendering can misalign on Windows terminal fonts.

### 21. Some themes lack light-mode definitions

Themes like aura, kanagawa only define dark colors. Fallback may produce poor contrast in light terminals.

### 22. No graceful degradation for 256-color terminals

Themes assume true color (24-bit RGB). 256-color terminals get approximate matching which can look jarring for subtle background differences.

---

## What Works Well (Keep These)

1. **Logo animation** — sub-pixel shimmer, mouse-interactive ripples, idle breathing. Genuine differentiator.
2. **Inline vs block tool rendering** — small operations stay compact, large operations expand. Excellent information hierarchy.
3. **33 themes with dark/light variants** — best-in-class for a terminal tool.
4. **Knight Rider progress animation** — agent-colored trail communicates "which agent is working."
5. **Prompt extmark system** — paste summarization, file badges, agent mentions all keep the input manageable.
6. **Diff renderer** — auto split/unified, syntax highlighting, themed colors. Production-quality.
7. **Plugin slot system** — sidebar, footer, prompt area extensible without sacrificing base quality.
8. **Permission prompt design** — warning color, diff preview, three-option button bar with keyboard nav.
9. **Provider connection flow** — progressive disclosure (provider → auth → model picker).
10. **Agent color-coding** — left border color on prompt immediately shows which agent is active.

---

## Key Files Reference

| File                                                   | What                                    |
| ------------------------------------------------------ | --------------------------------------- |
| `src/cli/cmd/tui/config/keybind.ts`                    | All keybinding definitions              |
| `src/cli/cmd/tui/help.md`                              | Help content (currently wrong)          |
| `src/cli/cmd/tui/ui/dialog-help.tsx`                   | Help dialog renderer                    |
| `src/cli/cmd/tui/feature-plugins/system/which-key.tsx` | Keybinding discoverability panel        |
| `src/cli/cmd/tui/app.tsx`                              | Main TUI app, routes, tinycode branding |
| `src/cli/cmd/tui/routes/session/index.tsx`             | Session view, subagent nav              |
| `src/cli/cmd/tui/routes/session/sidebar.tsx`           | Sidebar layout and footer               |
| `src/cli/cmd/tui/routes/session/subagent-footer.tsx`   | Subagent navigation UI                  |
| `src/cli/cmd/tui/component/prompt/index.tsx`           | Prompt area, spinner, metadata          |
| `src/cli/cmd/tui/config/tui-schema.ts`                 | Leader key timeout config               |
| `src/cli/cmd/tui/component/dialog-retry-action.tsx`    | tinycode Go upsell link                 |
| `src/cli/cmd/tui/feature-plugins/sidebar/footer.tsx`   | "tinycode" sidebar branding             |
