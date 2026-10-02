# Feature Roadmap — 20 Open Issues in 5 Phases

## Context

20 open issues span 11 active features and 9 deferred items. The primary merge-conflict bottleneck is `app.go` (700 lines) — every new slash command adds entries to three locations: the `clientNames` map (line 375), the `items` palette slice (line 399), and the `handleClientCommand` switch (line 436). Secondary bottleneck is `run_handlers.go` (419 lines) where commands needing inline TUI behavior add prefix-match handlers. `command/discovery.go` (159 lines) also gains a `builtinCommands()` entry per command.

All features adding commands MUST merge sequentially within their phase to avoid conflicts in these three files. Feature *development* can happen in parallel worktrees since the core logic lives in isolated files.

## Work Objectives

- Ship 11 active features across 5 phases with zero merge conflicts
- Maintain a clean main branch — each feature merges and passes `go test ./... -count=1` before the next
- Sequence high-effort features (#343 rewind, #344 goal, #345 hooks, #346 batch) after low-effort wins stabilize
- Document where deferred issues slot in if activated

## Guardrails

**Must Have:**
- Each feature developed in an isolated git worktree
- Sequential merge within each phase (one PR merged at a time)
- `go test ./... -count=1` and `go vet ./...` pass after each merge
- Each feature includes tests for its core logic (not just wiring)

**Must NOT Have:**
- No parallel merges to main within a phase (app.go conflicts guaranteed)
- No changes to processor_loop.go before Phase 3 (#344 owns that file)
- No changes to plugin/hook.go before Phase 4 (#345 owns that file)
- No scope creep from deferred issues into active phases

## File Conflict Map

```
ALWAYS TOUCHED (by every command feature):
  internal/tui/app.go              — clientNames, items, handleClientCommand
  internal/tui/run_handlers.go     — prefix-match handler (if TUI-inline behavior needed)
  internal/command/discovery.go    — builtinCommands() entry

FEATURE-SPECIFIC (safe for parallel development):
  #338  internal/tui/sidebar.go              (ContextStats extension)
  #339  new file or run_handlers.go          (separate LLM call)
  #340  internal/session/session.go          (Store.Create with ParentID)
  #341  internal/tui/clipboard.go            (code-block picker)
  #342  internal/config/config.go            (effort config field)
        internal/tui/run_handlers.go         (like /thinking handler pattern)
  #343  internal/session/ (new message ops)  (conversation truncation)
        internal/tui/ (new picker component) (interactive rollback UI)
  #344  internal/session/processor_loop.go   (tryAutoContinue, checkDoomLoop)
        internal/session/processor.go        (ProcessorConfig fields)
        internal/config/config.go            (goal config)
  #345  internal/plugin/hook.go              (new Dispatch* functions)
        internal/plugin/builtin.go           (hook registration)
        internal/config/config.go            (hook config schema)
  #346  internal/server/session_ops.go       (parallel prompt dispatch)
  #347  new diagnostic file                  (config audit checks)
  #348  internal/session/compaction.go       (trackFiles consumer)
```

---

## Phase 1: Quick-Win Commands

**Issues:** #339 /btw, #342 /effort, #338 /context, #341 /copy enhancement
**Estimated time:** 3-4 dev-days
**Can start before previous phase:** N/A (first phase)

### Parallelism

**Develop in parallel** (different core files):
- #339 /btw — new handler in run_handlers.go (separate LLM call, no conversation state mutation)
- #342 /effort — config.go + run_handlers.go (follows /thinking pattern at line 59)
- #338 /context — sidebar.go (extends ContextStats struct at line 26)
- #341 /copy — clipboard.go (adds Nth-response selection + code-block picker)

**Merge sequentially** (app.go + discovery.go touched by all four):
1. #342 /effort — merge first (enables #316 later, shares run_handlers.go pattern with /thinking)
2. #339 /btw — merge second (simplest, no state)
3. #338 /context — merge third (sidebar-only feature code)
4. #341 /copy — merge last (extends existing /copy case in handleClientCommand)

### Per-Issue Details

**#342 /effort** — Acceptance: `/effort medium` changes the reasoning budget for subsequent prompts; `/effort` with no arg shows current level.
- Consumes: `internal/config/config.go:Info` struct (line 14), `internal/tui/run_handlers.go:thinkingLevelBudget()` pattern (line 235)
- Produces: new `EffortLevel` config field in config.go, `/effort` handler in run_handlers.go, palette entry in app.go
- Note: #342 is distinct from /thinking — /effort controls model-level reasoning budget (extended thinking), /thinking controls per-turn reasoning. Clarify scope with issue author before starting.

**#339 /btw** — Acceptance: `/btw <question>` sends a separate LLM call and displays the response without touching conversation history.
- Consumes: LLM client from processor (or direct API call), current model config
- Produces: `/btw` handler in run_handlers.go, new TUI message display (toast or inline), palette entry in app.go
- Key decision: does /btw response appear as a toast, a sidebar panel, or inline in chat with a "side-channel" marker?

**#338 /context** — Acceptance: `/context` opens a visualization showing token usage breakdown (input/output/cache/reasoning) and context window fill percentage.
- Consumes: `internal/tui/sidebar.go:ContextStats` struct (line 26, already has Tokens/InputTokens/OutputTokens/ContextLimit/Percent/Cost)
- Produces: extended ContextStats or new visualization component, `/context` command handler, palette entry
- Note: sidebar already shows basic context stats. /context should show a richer breakdown — histogram of message sizes, cache hit ratio, proximity to compaction threshold.

**#341 /copy enhancement** — Acceptance: `/copy 2` copies the 2nd-most-recent assistant response; `/copy code` opens a picker for code blocks in the last response; bare `/copy` preserves existing behavior.
- Consumes: `internal/tui/clipboard.go:writeClipboard()` (line ~120), `app.go:lastAssistantText()` (line 575), chat message list from `a.chat.Messages()`
- Produces: extended `/copy` handler (args parsing), code-block extraction logic, picker UI if interactive selection needed

### Deferred issue slot
- **#316 (adaptive planning)** can activate after #342 merges — it depends on the effort framework.

---

## Phase 2: Session & Utility Commands

**Issues:** #340 /branch, #348 /changes, #347 /doctor enhancement
**Estimated time:** 3-4 dev-days
**Can start before Phase 1 is fully merged:** Development YES (core files don't overlap). Merge NO (app.go must incorporate Phase 1 changes first).

### Parallelism

**Develop in parallel:**
- #340 /branch — session.go (uses existing Store.Create with ParentID + Store.Children)
- #348 /changes — compaction.go (calls trackFiles), shells out to git diff
- #347 /doctor — new diagnostic file (config audit, no shared state)

**Merge sequentially:**
1. #340 /branch — merge first (touches session.go which #343 will need later)
2. #348 /changes — merge second (isolated to compaction.go consumer)
3. #347 /doctor — merge last (fully independent diagnostic code)

### Per-Issue Details

**#340 /branch** — Acceptance: `/branch` creates a new session with ParentID set to current session, copies conversation context, and switches to the new session. The branched session appears in session list with parent lineage visible.
- Consumes: `internal/session/session.go:Store.Create()` (line 126, already accepts `ParentID`), `Store.Children()` (line 226), conversation messages from current processor
- Produces: `/branch` command handler, session-copy logic (messages duplication), updated session list display showing parent/child relationships
- Key decision: does /branch copy all messages or just up to the current point? Does it deep-copy or reference?

**#348 /changes** — Acceptance: `/changes` displays a diff of all files modified during the current session, using session-tracked file paths (not just `git diff`).
- Consumes: `internal/session/compaction.go:trackFiles()` (line 81, returns readFiles + modifiedFiles from conversation messages), git CLI
- Produces: `/changes` command handler, diff display component (reuse existing /diff pattern from app.go line 512), session-scoped file filtering
- Note: existing `/diff` shows all uncommitted changes. `/changes` filters to only session-touched files via trackFiles().

**#347 /doctor enhancement** — Acceptance: `/doctor` runs a config audit reporting: missing config files, invalid JSON, deprecated fields, unreachable providers, MCP server health, plugin binary existence.
- Consumes: config loading logic from `internal/config/config.go`, provider registry, MCP server list from sidebar, plugin paths
- Produces: new diagnostic module (e.g., `internal/doctor/`), `/doctor` command handler, structured report output

### Deferred issue slots
- **#295 (TUI cockpit layout)** — ideally activates before Phase 3. By end of Phase 2, six new commands have added TUI surface area. A layout pass would prevent overlay accumulation.
- **#318 (recall_tool_output)** — related to #338 (context). Could slot after Phase 1.

---

## Phase 3: Autonomous Execution

**Issues:** #344 /goal
**Estimated time:** 4-6 dev-days
**Can start before Phase 2 is fully merged:** Development YES (processor_loop.go untouched by Phases 1-2). Merge NO (config.go and app.go must be current).

### Parallelism

Single issue — no internal parallelism. This is medium effort and touches sensitive loop control code.

### Details

**#344 /goal** — Acceptance: `/goal "deploy the fix"` enters autonomous multi-turn mode where the agent continues executing until the goal is achieved or a safety limit is hit. The doom loop detector remains active. A progress indicator shows iteration count and goal status. The user can interrupt with Ctrl+C at any time.
- Consumes:
  - `internal/session/processor_loop.go:tryAutoContinue()` (line 176) — current auto-continue with count limit
  - `internal/session/processor_loop.go:checkDoomLoop()` (line 202) — doom loop detection with tool signature tracking
  - `internal/session/processor.go:ProcessorConfig` (line 21) — `AutoContinueMax` (line 30), `DoomThreshold` (line 29)
  - `internal/config/config.go:Info` (line 14) — `AutoContinue` field (line 175)
- Produces:
  - Goal-aware auto-continue: when /goal is active, `autoContinueLimit()` returns the goal's max iterations (not the default 0)
  - Goal status tracking: new fields on Processor to track goal text, progress, and completion detection
  - Goal completion heuristic: LLM self-assessment or tool-result-based detection that the goal is met
  - `/goal` command handler in run_handlers.go, palette entry in app.go
  - TUI progress indicator: iteration count, elapsed time, token spend while goal is running
  - Safety: doom loop threshold remains at its default; goal mode does NOT disable doom detection. User configurable iteration cap via config (default: 25).
- Risks:
  - Token burn: uncapped goal loops can exhaust context. Mitigation: hard iteration cap + token budget check per iteration via `checkCompaction()` (line 145).
  - Doom loop false positives: legitimate repetitive work (e.g., applying same fix to many files) may trigger doom detection. Mitigation: goal mode could raise the doom threshold by 2x, not disable it.
  - Interrupt handling: must work cleanly — partial tool execution state must be coherent after Ctrl+C.

### Deferred issue slots
- **#332 (typed event bus)** — if activated, should start during Phase 3 development. It's infrastructure that doesn't touch command files, and landing it before Phase 4 (#345 hooks) gives typed events to hook dispatch.

---

## Phase 4: High-Effort Features

**Issues:** #343 /rewind, #345 shell hooks
**Estimated time:** 6-10 dev-days
**Can start before Phase 3 is fully merged:** Development YES (#343 touches session message handling, #345 touches plugin system — neither conflicts with processor_loop.go). Merge NO.

### Parallelism

**Develop in parallel** (completely different subsystems):
- #343 /rewind — session package (message manipulation) + TUI (interactive picker)
- #345 /shell hooks — plugin package (hook dispatch) + config (hook schema)

**Merge sequentially:**
1. #343 /rewind — merge first (touches session.go which may need Phase 2's #340 branch changes)
2. #345 /shell hooks — merge second (isolated to plugin system)

### Per-Issue Details

**#343 /rewind Phase 1** — Acceptance: `/rewind` opens an interactive picker showing conversation turns (user messages as labels). Selecting a turn truncates the conversation to that point. The session state is consistent after rewind — token counts updated, no orphaned tool results.
- Consumes:
  - Conversation messages from `Processor.Messages()` (via mu-locked accessor)
  - Session message persistence (currently in-memory on Processor; may need Store extension for durability)
  - `internal/tui/` component patterns (dialog/picker — follows SessionDialog or CommandPalette patterns)
- Produces:
  - New TUI picker component (`internal/tui/rewind_picker.go`) — lists user messages as selectable turns
  - Message truncation logic: `Processor.RewindTo(messageID string)` — locks mu, truncates messages slice, resets autoContinueCount and recentToolCalls
  - Token recount after truncation (or accept stale counts until next LLM call updates them)
  - `/rewind` command handler, palette entry
  - Edge cases: rewind to before a compaction summary (show warning, allow anyway), rewind in a branched session (affects only the branch), rewind with pending tool execution (abort first)

**#345 Shell hooks in settings.json** — Acceptance: Users can define shell commands in settings.json that execute on session events (start, end, tool-before, tool-after). Hook output is captured and logged. Hook failures are non-fatal (logged, not blocking).
- Consumes:
  - `internal/plugin/hook.go` — existing typed dispatch functions: `DispatchSessionStart`, `DispatchSessionEnd`, `DispatchPermissionAsk`, `DispatchShellEnv`, `DispatchToolExecBefore`, `DispatchToolExecAfter`
  - `internal/plugin/builtin.go:BuiltinManager.DispatchHook()` (line ~50) — generic dispatcher
  - `internal/config/config.go` — needs new `Hooks` config section
- Produces:
  - Config schema: `"hooks": { "session.start": ["cmd1", "cmd2"], "tool.before": [...] }` in settings.json
  - Shell hook executor: spawns `exec.CommandContext` for each registered command, passes event data as JSON on stdin, captures stdout/stderr
  - Integration with existing DispatchHook: shell hooks fire after plugin hooks for the same event
  - Timeout and error handling: per-hook timeout (default 10s), non-fatal on failure, structured logging
  - `/hooks` command to list registered hooks and their last execution status (optional, stretch)

### Deferred issue slots
- **#332 (typed event bus)** — if activated, should land BEFORE #345 starts development. Typed events make hook dispatch cleaner and prevent string-based event name drift.
- **#295 (TUI cockpit layout)** — by Phase 4, the TUI has gained ~8 new commands/overlays. Layout refactoring becomes increasingly valuable.

---

## Phase 5: Advanced Multi-Agent

**Issues:** #346 /batch
**Estimated time:** 6-8 dev-days
**Can start before Phase 4 is fully merged:** Development YES (session_ops.go untouched by Phase 4). Merge NO.

### Details

**#346 /batch** — Acceptance: `/batch "task description"` decomposes the task into subtasks, dispatches them as parallel subagents (extending /swarm), and merges their results with conflict detection. The user sees a progress view showing each subtask's status.
- Consumes:
  - `internal/server/session_ops.go` — existing swarm patterns, `swarmMaxIterations()` (line 308), `StartPrompt()` (line ~84)
  - `/swarm` command template from `command/discovery.go` (line 103)
  - Git worktree support for parallel file changes
- Produces:
  - Task decomposition: LLM-powered planning phase that breaks the batch into independent subtasks
  - Parallel dispatch: extends swarm to run N subagents concurrently with isolated worktrees
  - Merge resolution: conflict detection when subagents modify the same files, interactive merge UI or sequential re-application
  - Progress view: real-time status of each subtask (pending/running/complete/failed)
  - `/batch` command handler, palette entry
- Risks:
  - Merge conflicts between subagents: two agents editing the same file. Mitigation: file-level locking or sequential merge with conflict UI.
  - Resource exhaustion: too many parallel LLM calls. Mitigation: configurable concurrency limit (default: 3).
  - Partial failure: some subtasks succeed, others fail. Mitigation: completed subtasks merge immediately; failed ones report errors and allow retry.

---

## Deferred Issues — Activation Slots

| Issue | Description | Best Activation Point | Dependency |
|-------|-------------|----------------------|------------|
| #286 | MCP-setup skill rewrite | Any time (independent) | None |
| #295 | TUI cockpit layout | Before Phase 4 ideally; prevents overlay accumulation | None, but risk grows with each phase |
| #296 | Web UI design audit | Any time (independent) | None |
| #316 | Adaptive planning | After Phase 1 (#342 merged) | #342 /effort |
| #317 | Extension registry | Any time (independent) | None |
| #318 | recall_tool_output | After Phase 1 (#338 merged) | Related to #338 /context |
| #332 | Typed event bus | Before Phase 4 (#345) | None, but enables cleaner #345 |
| #334 | Metrics endpoint | Any time (independent) | None |
| #337 | Operator Go rewrite | Any time (independent) | None |

### Recommended activations

1. **#332 (typed bus)** — activate during Phase 3 development window. It's infrastructure (bus package), doesn't touch command files, and landing before Phase 4 gives #345 typed events instead of string-keyed dispatch.
2. **#295 (TUI layout)** — activate between Phase 2 and Phase 3. By then, 7 new commands have added TUI surface. Waiting past Phase 4 means retrofitting 9+ command UIs.
3. **#316 (adaptive planning)** — activate after Phase 1 ships. Depends on #342 effort framework being in place.

---

## Success Criteria

- All 11 active features merged to main with passing tests
- Zero merge conflicts during sequential merge within each phase
- Each phase's features are independently functional (no cross-phase runtime dependencies)
- Deferred issues have clear activation points documented
- Total estimated timeline: 22-32 dev-days across all 5 phases

## Phase Overlap Summary

```
Phase 1 (dev)  ████████████
Phase 1 (merge)         ████
Phase 2 (dev)     ████████████        ← dev starts during P1 dev
Phase 2 (merge)              ████     ← merge waits for P1 merge
Phase 3 (dev)          ████████████   ← dev starts during P2 dev
Phase 3 (merge)                  ██   ← merge waits for P2 merge
Phase 4 (dev)               ██████████████  ← dev starts during P3 dev
Phase 4 (merge)                        ████ ← merge waits for P3 merge
Phase 5 (dev)                     ██████████████
Phase 5 (merge)                              ██
```

With overlap, effective wall-clock time is ~18-22 dev-days (not the 22-32 sum).
