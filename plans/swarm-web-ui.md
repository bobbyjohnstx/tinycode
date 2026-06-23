# Swarm Web UI Support

## Context

The swarm tool (`packages/tinycode/src/tool/swarm.ts`) creates a tmux session with tiled panes (dashboard, supervisor, N workers) for parallel agent orchestration. This works well in the CLI/TUI but is inaccessible from the web/desktop UI because there is no tmux client to switch to.

The simplest solution: when the swarm tool runs on a web/desktop client, create a single PTY session that runs `tmux attach -t <session>` and auto-open it as a terminal tab. The user sees the same tiled tmux layout rendered through xterm.js. No changes to tmux orchestration, worker scripts, supervisor logic, or shared filesystem coordination.

## Work Objectives

- Swarm tool detects client type and auto-opens a terminal tab for web/desktop users
- CLI/TUI behavior remains completely unchanged
- Minimal code changes -- reuse existing PTY and terminal infrastructure

## Guardrails

**Must Have:**

- CLI/TUI swarm behavior is byte-for-byte identical (no regressions)
- Web/desktop users see the tmux swarm session in a terminal tab automatically
- Clean service dependency declaration (Pty.Service added to tool registry layer)
- The `pty.open` bus event is a general-purpose mechanism, not swarm-specific

**Must NOT Have:**

- Multiple PTY sessions per swarm (one tab with tmux handles the tiling)
- Changes to worker scripts, supervisor logic, dashboard scripts, or shared filesystem
- Changes to the tmux session creation commands
- New REST API endpoints (use existing PTY create + bus events)

## Task Flow

### Step 1: Define `pty.open` bus event and add Pty.Service to tool registry

**Files:**

- `packages/tinycode/src/pty/index.ts` -- Add new `Event.Open` bus event definition alongside existing `Created`, `Updated`, `Exited`, `Deleted` events.
  ```
  Event.Open = BusEvent.define("pty.open", { id: PtyID, title: string })
  ```
- `packages/tinycode/src/tool/registry.ts` -- Add `Pty.Service` to the registry layer's dependency union type (lines 83-108). Import from `@/pty`.

**Acceptance:**

- `pty.open` event is defined and exported from the PTY module
- `Pty.Service` appears in the tool registry layer type alongside `ChildProcessSpawner`, `Bus.Service`, etc.
- Project type-checks (`bun typecheck` from `packages/tinycode`)

---

### Step 2: Add client-aware branching to swarm tool

**Files:**

- `packages/tinycode/src/tool/swarm.ts`

**Changes to the init function (around line 470):**

- Add `yield* RuntimeFlags.Service` to get `flags.client` (RuntimeFlags is already imported/available in registry context)
- Add `yield* Pty.Service` to get the PTY service handle (for web/desktop path)
- Add `yield* Bus.Service` to get the bus handle (for publishing `pty.open`)
- Update the tool's type parameter from `ChildProcessSpawner` to `ChildProcessSpawner | Pty.Service | RuntimeFlags.Service | Bus.Service` (follow the pattern in `write.ts` where multiple services are yielded)

**Changes to the execute function (around lines 520-580):**

After the tmux session is created (after all `runTmux` calls complete), add a client-type branch:

```
if (flags.client === "app" || flags.client === "desktop") {
  // 1. Create a PTY that attaches to the tmux session
  const ptyInfo = yield* pty.create({
    command: "tmux",
    args: ["attach-session", "-t", plan.sessionName],
    cwd: plan.projectDir,
    title: `Swarm: ${plan.sessionName}`,
  })

  // 2. Publish pty.open so the web UI auto-opens a tab
  yield* bus.publish(Event.Open, { id: ptyInfo.id, title: ptyInfo.title })

  // 3. Skip switch_client entirely (no tmux client to switch)
} else {
  // Existing CLI/TUI path: switch_client logic (unchanged)
  const switchExit = plan.switchClient
    ? yield* tmuxExit(["switch-client", "-t", `${plan.sessionName}:swarm`], plan.projectDir)
    : 1
  // ... existing switchedClient metadata
}
```

**Changes to metadata return:**

- Add optional `ptyID` field to the `Metadata` type (for web/desktop, stores the PTY ID)
- Set `switchedClient: true` for web/desktop (since we auto-opened the tab)

**Acceptance:**

- Running swarm with `TINYCODE_CLIENT=cli` produces identical behavior to current code
- Running swarm with `TINYCODE_CLIENT=app` creates a PTY session running `tmux attach-session -t <session>` and publishes `pty.open`
- The `switch_client` parameter is silently ignored for web/desktop clients
- Project type-checks

---

### Step 3: Web UI subscribes to `pty.open` and auto-opens terminal tab

**Files:**

- `packages/app/src/context/terminal.tsx`

**Changes:**

- In the same block where `pty.exited` is subscribed (around line 211), add a subscription to `pty.open`:
  ```
  sdk.event.on("pty.open", (event) => {
    const { id, title } = event.properties
    // Add the PTY to the local terminal list
    setStore("all", (prev) => [
      ...prev,
      { id, title, titleNumber: nextNumber() },
    ])
    // Set as active tab
    setStore("active", id)
    // Open the terminal panel if not already open
    // (follow the existing pattern for panel visibility)
  })
  ```
- The terminal panel auto-opens because setting `active` to a valid ID triggers the panel to show (existing behavior when a terminal is active).

**Acceptance:**

- When a `pty.open` event arrives via SSE, a new terminal tab appears in the web UI
- The tab title matches what was passed in the event (e.g., "Swarm: tiny-swarm-...")
- The terminal connects to the PTY via WebSocket and renders the tmux session
- Closing the tab works normally (PTY is removed, tmux session continues running independently)

---

### Step 4: Auto-reattach on reconnect

**Files:**

- `packages/app/src/context/terminal.tsx`

**Changes:**

When the web UI reconnects after a WebSocket drop, check for orphaned swarm tmux sessions and reattach:

1. On SSE/WebSocket reconnect, read the swarm manifest from the shared filesystem (the session name and shared dir are stored in tool metadata from step 2)
2. Check if the tmux session still exists (`tmux has-session -t <name>`)
3. If it does, create a new PTY running `tmux attach -t <name>` and emit `pty.open`

This can piggyback on the existing reconnect logic in the terminal context.

**Acceptance:**

- After a WebSocket reconnect, if the swarm tmux session is still running, a terminal tab auto-opens and reattaches
- If the swarm has already completed (tmux session gone), no tab opens

---

### Step 5: Cleanup and edge case handling

**Files:**

- `packages/tinycode/src/tool/swarm.ts`

**Changes:**

1. **tmux availability check** -- The existing code already runs `runTmux(["-V"])` which fails if tmux is not installed. No additional check needed, but ensure the error message is clear.

2. **Natural cleanup** -- The swarm self-cleans: workers finish → supervisor detects all done → dashboard exits after 10s → all tmux panes exit → tmux kills the session automatically. No manual `tmux kill-session` needed for normal completion.

3. **Tool output adaptation** -- For web/desktop, adjust the tool output text:
   - Replace "Run `tmux attach -t ...` to view" with "Swarm opened in terminal tab"
   - Note that closing the tab detaches but the swarm continues working (and will self-terminate when all workers complete)

**Acceptance:**

- If tmux is not installed, the tool returns a clear error before creating any filesystem state
- Tool output for web/desktop explains that closing the tab detaches (swarm continues and self-terminates on completion)
- Tool output for CLI is unchanged

---

## Edge Cases and Notes

**Terminal tab limit:** The web UI has a 20-tab LRU cache (`MAX_TERMINAL_SESSIONS`). A swarm terminal counts toward this limit. If at capacity, the LRU eviction will close the oldest terminal. This is acceptable -- 20 terminals is generous.

**Concurrent swarms:** Each swarm creates a separate tmux session and a separate PTY. Multiple swarm tabs can coexist. The tmux session names are unique (timestamp-based).

**tmux keybindings in xterm.js:** tmux's `ctrl+b` prefix works through xterm.js. Arrow keys, pane navigation (`ctrl+b q`), and detach (`ctrl+b d`) all function normally. No special handling needed.

**WebSocket disconnect:** If the WebSocket drops, the PTY dies, but the tmux session survives (it's a separate process). On reconnect, the web UI auto-reattaches by creating a new PTY running `tmux attach -t <session>` and emitting `pty.open` (step 4).

**Natural self-cleanup:** When all workers complete → supervisor detects all done/exited → writes `supervisor/done` → dashboard sees it, waits 10s, exits → all tmux panes have exited → tmux kills the session automatically. No manual cleanup needed for normal completion. The attached PTY exits when the tmux session ends, and the web UI receives `pty.exited` normally.

## Success Criteria

- A user running tinycode in the web UI can invoke the swarm tool and see the tmux dashboard/workers in a terminal tab without manual intervention
- A user running tinycode in the CLI/TUI sees zero behavior change
- The implementation touches 4-5 files with roughly 50-80 lines of new code total
- No new REST API endpoints, no new database schema, no new npm dependencies
