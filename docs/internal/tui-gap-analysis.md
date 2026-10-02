# TUI Gap Analysis — Current State vs Approved Mockup

Date: 2026-10-02

## Summary

The TUI has working implementations of all 6 elements, but each diverges from the mockup:

1. **Status bar** — missing effort, context%, agent name display; wrong format
2. **Goal tracker** — text-only, no box-drawing/steps/auto-fade; statusBarHeight is fixed
3. **Chat styling** — user border is red (should be blue), thinking uses per-agent color (should be accent purple), tool name in render/tool.go is never themed
4. **Prompt separator** — doesn't exist
5. **Sidebar** — right side (should be left), 42 cols (should be ~28)
6. **Theme** — tinycode.json palette matches mockup, but ApplyColorTheme only updates 10 of 20+ styles; many hardcoded

## Priority Fixes

### P1: Wire theme colors through all rendering (unblocks everything)
- Extend `ApplyColorTheme` (theme.go:153-180) to cover:
  - `render/tool.go:36-43` (separate styleToolName, never themed)
  - sidebar styles (sidebar.go:167-174)
  - status bar inline colors (statusbar.go:155-158)
  - prompt colors (prompt.go:320,369-372)
  - reasoning label (use accent, not per-agent)
- Fix diff colors: DiffAdded/DiffRemoved are loaded but never applied
- Fix agent color map (styles.go:24-41) to consult active theme

### P2: Redesign status bar format
- Change from `[cwd] [model provider]` to `agent · model · provider · effort · NN% ctx`
- Add effort and contextPercent fields to StatusBar struct
- Move agent/model metadata from prompt.go:renderMetadata into status bar
- Use theme's backgroundPanel (#141414) instead of hardcoded #1A1A1A

### P3: Move sidebar to left, shrink to ~28 cols
- Swap args in layout.go:56-59 JoinHorizontal
- Change sidebarWidth from 42 to 28
- Adjust minSidebarWidth in layout.go:8

### P4: Build goal tracker visual component
- Replace statusText() string with multi-line renderer using box-drawing
- Add Step struct to GoalState (name, status)
- Make statusBarHeight dynamic in layout.go
- Add auto-fade timer for completion state
- Pink border (#c87898) on goal box

### P5: Add prompt separator
- Full-width `─` line in primary color above prompt area
- Render in prompt View() or composeView

### P6: Fix render/tool.go duplicate styleToolName
- Separate package has its own styleToolName that shadows the themed one
- Either export the themed variable or add SetToolStyles function

## Key Files

| File | Lines | What |
|------|-------|------|
| statusbar.go | 15-27, 154-222 | Status bar struct and View |
| goal.go | 1-99 | Goal tracker (text-only) |
| session/goal.go | 6-11 | GoalState (no steps) |
| chat_render.go | 18-31, 79-92, 142-165, 270-274 | Message rendering styles |
| render/tool.go | 36-43 | Unthemed tool styles |
| theme.go | 153-180 | ApplyColorTheme (incomplete) |
| layout.go | 6-9, 53-61 | Fixed heights, sidebar position |
| sidebar.go | 23, 167-174, 277 | Width, styles, border |
| prompt.go | 318-365, 368-399 | Prompt view, metadata |
| styles.go | 9-56 | Hardcoded defaults, agent colors |
| themes/tinycode.json | 1-245 | Palette (matches mockup) |
