# Fix Plan — Architecture Review Findings

**Source:** `docs/architecture-review.md` (53 findings across 4 severity levels)
**Issues:** Gitea #82-#107 (20 issues total)
**Date:** 2026-09-08

## Ordering Principles

1. Security fixes first (exploitable vulnerability)
2. Critical protocol breaks second (plugin subsystem is fundamentally broken)
3. Concurrency bugs third (data races and goroutine leaks cause silent corruption)
4. Correctness bugs fourth (wrong behavior, silent data loss)
5. Robustness improvements fifth (hardening, cleanup)
6. Low severity last (dead code, deprecations, cosmetics)

Within each band, issues touching the same files are grouped to reduce merge conflicts.

---

## Phase 1 — Security: Path Traversal

**Issues:** #85 (H1)
**Effort:** Small
**Files:** `internal/server/handler_file.go`
**Dependencies:** None — standalone, must land first

| Issue | Fix | Acceptance Criteria |
|-------|-----|---------------------|
| #85 (H1) | Validate `path` query param in `handleFileRead` (line 12) and `handleFileList` (line 36) against `s.config.Directory`. Use `filepath.EvalSymlinks` + `strings.HasPrefix` to prevent symlink escapes. | 1. Request for `../../etc/passwd` returns 403/400. 2. Request for valid project file succeeds. 3. Symlink pointing outside project directory returns 403/400. |

**Notes:**
- Use `filepath.EvalSymlinks` on the resolved path, then check `strings.HasPrefix(resolved, allowedDir)` where `allowedDir` is also resolved via `EvalSymlinks`.
- Return 403 Forbidden for paths outside the allowed directory. Do not leak the resolved path in the error message.

---

## Phase 2 — Critical: Plugin Protocol & SSE Routing

**Issues:** #82 (C1+C2), #83 (C3), #84 (C4)
**Effort:** Medium
**Files:** `internal/plugin/hook.go`, `internal/plugin/manager.go`, `pkg/plugin/plugin.go`, `pkg/plugin/protocol.go`, `internal/server/router.go`, `internal/server/handler_event.go`, `internal/server/handler_stub.go`, `internal/tui/api/plugin.go`
**Dependencies:** None — but #83 depends on #82 being done first (route is useless if protocol is broken)

### #82 — Plugin protocol alignment (C1 + C2)

**Decision required:** Align Manager to SDK (recommended — preserves public API contract).

| Finding | Fix | Acceptance Criteria |
|---------|-----|---------------------|
| C1: hook method routing broken | Change `internal/plugin/hook.go:77` to send `"hook/invoke"` as the RPC method with the hook name inside `HookParams.Name`, matching the SDK's `dispatch()` routing in `pkg/plugin/plugin.go:144-149`. | A plugin built with the SDK receives hook invocations and can respond. |
| C2: initialize field name mismatch | Change `internal/plugin/manager.go:47-50` to use JSON tags matching SDK: `"version"` instead of `"protocolVersion"`, add `"options"` field. Change `initializeResult` (line 53-55) to capture `ID`, `Tools`, and `Hooks` fields matching `pkg/plugin/protocol.go:39-43`. | Plugin initialize handshake succeeds: server reads plugin ID, tools list, and hooks list from response. |

### #83 — Register plugin event route (C3)

| Finding | Fix | Acceptance Criteria |
|---------|-----|---------------------|
| C3: `POST /plugin/event` does not exist | Add `POST /plugin/event` route in `internal/server/router.go` with a handler that dispatches plugin events. Alternatively, if plugin events should flow through the existing hook system, remove `SendPluginEvent` from `internal/tui/api/plugin.go` and its callers. | Either: the route exists and returns 200, or the dead client method is removed. |

### #84 — SSE route/handler naming inversion (C4)

| Finding | Fix | Acceptance Criteria |
|---------|-----|---------------------|
| C4: `/event` calls `StreamGlobalEvents`, `/global/event` calls `StreamEvents` — names inverted | Swap the handler assignments in `internal/server/router.go` so `/event` calls `StreamEvents` (flat, instance-scoped) and `/global/event` calls `StreamGlobalEvents` (wrapped, directory-scoped). Remove dead `handleGlobalEventStreamWrapped` from `handler_event.go`. Verify TUI client (`internal/tui/api/sse.go`) connects to the correct endpoint for the format it parses. | 1. `/event` returns flat events. 2. `/global/event` returns wrapped events. 3. TUI still receives events correctly. 4. No dead handler code remains. |

**WARNING:** #83 and #84 both modify `internal/server/router.go` lines 7-92. They must be implemented together or in immediate sequence to avoid merge conflicts.

---

## Phase 3 — Concurrency: Data Races & Goroutine Leaks

**Issues:** #86 (H2+H3+H4), #87 (H5+H7), #88 (H6), #94 (H14)
**Effort:** Medium
**Files:** `internal/server/session_manager.go`, `internal/provider/discovery.go`, `internal/mcp/mcp.go`, `internal/mcp/sse.go`, `internal/plugin/manager.go`, `internal/permission/permission.go`
**Dependencies:** #82 should land first (both #82 and #87-H7 touch `plugin/manager.go`, though at widely separated locations — lines 47-55 vs 371-394)

### #86 — Three data races (H2 + H3 + H4)

| Finding | Fix | Acceptance Criteria |
|---------|-----|---------------------|
| H2: `Abort()` reads `active.processor` without lock after unlock at line 245 | Hold the lock while reading `active.processor` in `Abort()` (lines 242-255 of `session_manager.go`). Extract the processor reference under the lock, then call methods on it after releasing. | `go test -race ./internal/server/...` passes with no race on `processor` field. |
| H3: `maybeWarmup` mutates `Model.Capabilities` from background goroutine | Make `Capabilities.ToolCall` write in `discovery.go:401` safe. Either: (a) copy the Model struct before mutating, or (b) use an atomic flag, or (c) protect with the Registry mutex. | `go test -race ./internal/provider/...` passes. |
| H4: MCP `Configure` unlock/relock race during map iteration | Collect servers to stop into a slice under the lock, then stop them after releasing the lock. Do not iterate `s.servers` while unlocked. | `go test -race ./internal/mcp/...` passes. |

### #87 — Goroutine and resource leaks (H5 + H7)

| Finding | Fix | Acceptance Criteria |
|---------|-----|---------------------|
| H5: MCP SSE `pending` map entries leak on disconnect | Add a cleanup method that signals all pending channels and removes entries when the SSE connection closes. Call it from the disconnect/reconnect path. | Pending map is empty after disconnect. No goroutines blocked on stale channels. |
| H7: Plugin RPC `sendRPC` goroutine leaks on timeout | Cancel the decode goroutine when timeout fires. Use a `context.Context` with cancel, or close the decoder's reader to unblock `Decode()`. Ensure stale responses on the channel are drained. | Repeated RPC timeouts do not accumulate goroutines (verify with `runtime.NumGoroutine()` in test). |

### #88 — Permission closed flag (H6)

| Finding | Fix | Acceptance Criteria |
|---------|-----|---------------------|
| H6: `s.closed` set in `Close()` but never checked in `Ask()` | Add `if s.closed { return ErrClosed }` at the top of `Ask()` in `permission.go:91`. | After `Close()`, `Ask()` returns an error immediately instead of hanging. |

### #94 — processPrompt context propagation (H14)

**INVESTIGATION NEEDED:** The architecture review claims `context.Background()` is used at `session_manager.go:235,269`, but current code at line 262 appears to use the HTTP request context parameter. Verify whether this finding is stale or references a different call site before implementing.

| Finding | Fix | Acceptance Criteria |
|---------|-----|---------------------|
| H14: HTTP disconnect does not cancel LLM processing | If confirmed: propagate the HTTP request context to `processPrompt` instead of `context.Background()`. If already fixed: close the issue with a note. | Dropping the HTTP connection cancels the LLM request within a reasonable timeout. |

---

## Phase 4 — Correctness: Session & Provider Logic

**Issues:** #89 (H8+H9), #95 (M1), #96 (M2), #100 (M6)
**Effort:** Small
**Files:** `internal/session/processor.go`, `internal/provider/retry.go`, `internal/server/session_manager.go`
**Dependencies:** #86 (H2) should land first — it also modifies `session_manager.go` in the same region as #100

### processor.go fixes (#89, #95, #96 — all in the same file)

| Issue | Finding | Fix | Acceptance Criteria |
|-------|---------|-----|---------------------|
| #89 (H8) | `statusFromError` uses `strings.Contains(msg, "500")` — matches "5003 tokens" | Parse the HTTP status code numerically from the error string instead of substring matching. Use a regex like `^(\d{3})\b` or restructure the error type to carry the status code as an int field. | Error containing "5003" is not classified as HTTP 500. Error "HTTP 500: internal server error" is correctly classified. |
| #89 (H9) | `IsOverflow` checks `HasPrefix(errMsg, "400 ")` but errors are formatted as `"HTTP 400: ..."` | Align the prefix check with the actual error format: `HasPrefix(errMsg, "HTTP 400:")` or, better, use the same structured error approach from H8. | Overflow detection correctly identifies `"HTTP 400: maximum context length"` errors. |
| #95 (M1) | `buildRequest` overwrites `llmMsg.Content` on each text part | Concatenate text parts: accumulate into a `strings.Builder` or slice, then assign the joined result. | Multi-part text messages preserve all text parts in the built request. |
| #96 (M2) | Processor `for {}` loop has no iteration limit | Add a configurable max iteration count (e.g., 200). When exceeded, return an error to the user explaining the model exceeded the iteration limit. The existing `consecutiveToolFailures` counter should also break the loop (e.g., after 10 consecutive failures). | Loop terminates after hitting the iteration limit. User sees an error message, not an infinite hang. |

### session_manager.go fix (#100)

| Issue | Finding | Fix | Acceptance Criteria |
|-------|---------|-----|---------------------|
| #100 (M6) | `existingMsgs, _ := ms.List(sessionID)` silently discards error | Handle the error: log it and either return the error to the caller or proceed with an empty list and a warning log. | Database errors in `ms.List()` are logged. If the error is critical (not just "no rows"), processing does not silently proceed with stale context. |

---

## Phase 5 — Correctness: Scattered HIGH + Related MEDIUM

**Issues:** #90 (H10), #91 (H11), #92 (H12), #93 (H13), #104 (M12), #105 (M15)
**Effort:** Small (each fix is a few lines)
**Files:** `internal/agent/agent.go`, `internal/tool/edit.go`, `internal/acp/event.go`, `internal/tool/tool.go`, `internal/tui/api/sse.go`, `internal/tui/chat.go`, `internal/tui/app.go`, `internal/tui/cmd.go`
**Dependencies:** None — all independent

| Issue | Finding | Fix | Acceptance Criteria |
|-------|---------|-----|---------------------|
| #90 (H10) | `ApplyConfigOverrides` is never called | Find the correct call site where agents are loaded/initialized and call `ApplyConfigOverrides(userConfig)`. Likely in the session setup or agent loading path. | User agent config overrides from `config.json` are applied at runtime. Test with a config override and verify the agent uses the overridden values. |
| #91 (H11) | `fileMutexes` sync.Map grows unbounded | Add an eviction mechanism. Options: (a) use a fixed-size LRU cache, (b) periodically clear the map (e.g., at session end), (c) scope mutexes to the session. | After a session ends or after N unique files, old entries are cleaned up. Memory does not grow proportional to total unique files ever edited. |
| #92 (H12) | ACP `relayToolEvent` reads `props["name"]` but tool.go publishes as `"tool"` | Change `internal/acp/event.go:95` to read `props["tool"]` instead of `props["name"]`. | ACP clients receive the correct tool name in relayed events. |
| #93 (H13) | SSE reconnect backoff never resets | Add `backoff = initialBackoff` after a successful SSE connection is established in `internal/tui/api/sse.go`. | After disconnect + successful reconnect, the next disconnect uses the initial backoff delay, not the escalated one. |
| #104 (M12) | `stickyBottom` never re-engages after scroll up | Re-engage `stickyBottom = true` when the user scrolls to the bottom of the chat viewport, or when a new message arrives while the viewport is within a threshold of the bottom. | User scrolls up (auto-scroll stops), scrolls back to bottom (auto-scroll resumes). New messages while at bottom auto-scroll. |
| #105 (M15) | Escape key does not abort despite status bar hint | Add an escape key handler in `app.go` `handleGlobalKey()` (around line 415) that calls `abortSession()` from `cmd.go:262` when `app.state.Working` is true. | Pressing escape while a session is working sends an abort request. Status bar hint matches actual behavior. |

---

## Phase 6 — Robustness: Remaining MEDIUM Fixes

**Issues:** #97 (M3), #98 (M4), #99 (M5), #101 (M7+M9), #102 (M8+M10), #103 (M11+M13+M14), #106 (M16+M17)
**Effort:** Medium (7 issues, each small individually)
**Files:** `internal/plugin/hook.go`, `internal/server/server.go`, `internal/server/revert.go`, `internal/tool/glob.go`, `internal/tool/truncate.go`, `internal/tool/webfetch.go`, `internal/mcp/oauth.go`, `internal/tui/diffview.go`, `internal/tui/workspace.go`, `internal/tui/theme_loader.go`, `internal/tui/styles.go`, `internal/tui/permission.go`, `internal/id/id.go`
**Dependencies:** #82 should land first — #97 also touches `plugin/hook.go`

| Issue | Finding | Fix | Acceptance Criteria |
|-------|---------|-----|---------------------|
| #97 (M3) | `DispatchShellEnv` sends original env, not accumulated | Change `hook.go:162` to pass `merged` (the accumulated map) to each plugin instead of `input.Env`. | Plugin B sees env vars added by plugin A in `DispatchShellEnv`. |
| #98 (M4) | `wirePluginHooks` goroutines leak on shutdown | Store subscription cancel functions and call them in `Shutdown()`. Or use a context that is canceled on shutdown. | Server shutdown does not leave orphaned subscription goroutines. |
| #99 (M5) | `revert Pop` pops wrong stash if stack changed | Use `git stash pop stash@{N}` with the specific stash reference, or store the stash SHA and pop by ref. | Unrevert pops the correct stash even when other stashes were created between revert and unrevert. |
| #101 (M7) | Glob `**` suffix matching fails for multi-component suffixes | For patterns like `**/src/*.go`, split the suffix on `/` and match against the corresponding trailing path components, not just the filename. | `**/src/*.go` matches `foo/src/main.go` but not `foo/main.go`. |
| #101 (M9) | `truncateBytes` splits multi-byte UTF-8 | After byte-level truncation, scan backward from the cut point to find a valid UTF-8 boundary. Use `utf8.Valid` or `strings.ToValidUTF8`. | Truncated output is always valid UTF-8. No partial codepoints at the end. |
| #102 (M8) | `webfetch` uses `http.DefaultClient` | Create a dedicated `http.Client` with a configured `http.Transport` that sets `TLSHandshakeTimeout`, `ResponseHeaderTimeout`, `IdleConnTimeout`, and `MaxIdleConns`. | Web fetch requests have transport-level timeouts. Slow TLS handshakes time out instead of hanging. |
| #102 (M10) | MCP OAuth `saveOAuthState` has read-modify-write race | Add file locking (e.g., `flock`) around the read-modify-write, or use an atomic write pattern (write to temp file, rename). | Concurrent `saveOAuthState` calls for different client IDs do not lose writes. |
| #103 (M11+M13+M14) | DiffView, Workspace, Theme styles are dead code | **Decision required:** Wire them into the app, or remove them. Recommendation: remove DiffView and Workspace (not ready for integration), keep Theme but wire `styles.go` to use it. | No dead component code remains. If wired: components are accessible in the UI. If removed: no references remain and the build succeeds. |
| #106 (M16) | Permission prompt reject/allow styling identical | Differentiate reject styling (e.g., red background/text) from allow styling (e.g., green) in `permission.go:165-169`. | Reject and allow actions are visually distinct in the permission prompt. |
| #106 (M17) | `id.Create(prefix, true)` double-negation API | Rename the parameter from `ascending bool` to `descending bool` to match internal usage, or invert the internal logic. Update all callers. | The API clearly communicates intent. `Create(prefix, true)` produces descending IDs (or rename to eliminate confusion). |

---

## Phase 7 — Low Severity: Cleanup & Polish

**Issues:** #107 (L1-L18)
**Effort:** Medium (18 items, each trivial individually)
**Files:** Scattered across 15+ files
**Dependencies:** None — can be done in any order after higher-severity phases

These are grouped by subsystem for convenience. Each is a standalone fix.

| ID | Finding | Subsystem | Fix |
|----|---------|-----------|-----|
| L1 | Dead darwin branch in `ConfigDir`/`DataDir` | config | Either use `~/Library/Application Support/tinycode` on darwin, or remove the branch |
| L2 | `DB.mu` field declared but never used | storage | Remove the unused `sync.RWMutex` field |
| L3 | `SubstituteEnvVars` swallows unresolved placeholders | config | Log a warning or return an error when `{env:VAR}` references an undefined variable |
| L4 | No directory argument support despite docs | cmd | Add directory argument handling in `main()`, or update CLAUDE.md to remove the claim |
| L5 | MCP `StreamableHTTPTransport` missing `onNotification` | mcp | Set `onNotification` callback in `setTransportCallbacks` |
| L6 | Ollama methods use `http.DefaultClient` | provider | Use the configured client instance instead of `http.DefaultClient` |
| L7 | `WarmupProbe` uses `http.DefaultClient` | provider | Same fix as L6 — use configured client |
| L8 | Token cost always 0 | server | Pass actual cost from LLM response to `store.UpdateCost` |
| L9 | Pending stores grow without bounds | server | Add TTL-based cleanup or size cap to `PermissionStore` and `QuestionStore` |
| L10 | Unused `scanner` interface in session package | session | Remove the dead interface |
| L11 | `scanSession` reports empty session ID in error | session | Pass the actual session ID to the error message |
| L12 | Duplicated `parseFrontmatter` function | command/skill | Extract into a shared utility package, or have one package import the other |
| L13 | `AppState` contains unused fields | tui | Remove `Route`, `Focus`, `Parts`, `Permissions` fields |
| L14 | Deprecated `strings.Title` usage | tui | Replace with `cases.Title(language.English).String()` from `golang.org/x/text` |
| L15 | Deprecated `lipgloss.Copy()` usage | tui | Replace with direct style chaining (remove `.Copy()` call) |
| L16 | `PluginHooks.fire()` is synchronous | tui | Wrap the HTTP call in a goroutine or use `tea.Cmd` to avoid blocking the TUI |
| L17 | `isTerminalEscape` false-positive risk | tui | Narrow the regex to anchor at string start, or only apply the filter during the startup guard window |
| L18 | Autocomplete results cmd never consumed | tui | Wire `AutocompleteResultsMsg` from `prompt_autocomplete.go` through `prompt.go` Update, or remove the dead return value |

### Additional findings (added during Phase 1 review)

| ID | Finding | Subsystem | Fix |
|----|---------|-----------|-----|
| L19 | `PluginHooks.fire()` uses `log.Printf` instead of `slog` | tui | Switch to `slog` for consistent structured logging |
| L20 | cluster-ops hardcodes `--insecure-skip-tls-verify` | plugin | Make TLS skip configurable, default to secure |
| L21 | `connectedApp` type-asserts `tea.Model` to `App` without safety check | tui | Use comma-ok assertion or type switch to prevent panics |
| L22 | MCP OAuth port parsing ignores `Sscanf` error | mcp | Check `Sscanf` return value, error on malformed port |

**Acceptance criteria for Phase 7:** All 22 items addressed (fixed or explicitly deferred with justification). `go vet ./...` passes. `go test ./... -count=1` passes. No new deprecation warnings from `go vet`.

---

## Decisions (Resolved 2026-09-08)

| Decision | Resolution | Affects |
|----------|-----------|---------|
| Plugin protocol alignment direction | **(A) Align Manager to SDK** — preserves public API contract | #82 |
| `POST /plugin/event` route | **(A) Add the route** | #83 |
| Dead TUI components (DiffView, Workspace) | **(A) Wire into app** | #103 |
| Theme system | **(A) Wire `styles.go` to use Theme** | #103 |

## Investigation Results

| Item | Result | Action |
|------|--------|--------|
| #94 (H14): `context.Background()` claim | **CONFIRMED.** `handler_session.go:235` passes `context.Background()` instead of `r.Context()`. The plumbing in `StartPrompt` already derives a cancellable child context — it just needs a real parent. The bus-driven call at `session_manager.go:173` is acceptable (no HTTP request). | Fix in Phase 3: change `handler_session.go:235` to pass `r.Context()`. |

---

## Execution Summary

| Phase | Issues | Severity | Effort | Files touched |
|-------|--------|----------|--------|---------------|
| 1. Security | #85 | HIGH | Small | 1 |
| 2. Critical protocol | #82, #83, #84 | CRITICAL | Medium | 8 |
| 3. Concurrency | #86, #87, #88, #94 | HIGH | Medium | 6 |
| 4. Session correctness | #89, #95, #96, #100 | HIGH+MED | Small | 3 |
| 5. Scattered correctness | #90, #91, #92, #93, #104, #105 | HIGH+MED | Small | 8 |
| 6. Robustness | #97-#103, #106 | MEDIUM | Medium | 13 |
| 7. Cleanup | #107 | LOW | Medium | 15+ |
| **Total** | **20 issues** | | | |

Phases 4 and 5 can run in parallel (no file overlap). Phase 6 can partially overlap with Phase 5 (no shared files except `plugin/hook.go` between #97 and earlier phases, but #82 should have landed by then). Phase 7 can begin as soon as Phase 6 is underway.
