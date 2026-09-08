# Architecture Review — tinycode-go

**Date:** 2026-09-08
**Scope:** All packages in `internal/`, `cmd/`, `pkg/` (110+ non-test source files, 20+ packages)

## Summary

| Severity | Count |
|----------|-------|
| CRITICAL | 4 |
| HIGH | 14 |
| MEDIUM | 17 |
| LOW | 18 |
| **Total** | **53** |

The most consequential issues are: (1) the plugin subsystem is fundamentally broken due to protocol mismatches between the internal Manager and the public SDK, (2) a missing server route makes `SendPluginEvent` always 404, (3) a path traversal vulnerability in the file handler allows reading arbitrary system files, and (4) multiple data races in the session manager, MCP client, and provider warmup code.

---

## CRITICAL

### C1. Plugin protocol mismatch: hook method routing is broken

The internal Manager sends hook names directly as the JSON-RPC method (e.g., `"session.start"`, `"permission.ask"`), but the SDK's `dispatch()` function routes on `"hook/invoke"` and expects the hook name inside `HookParams.Name`. Every hook call from the server will receive an "unknown method" error from any plugin built with the SDK. Hooks silently fail because the Manager only logs warnings.

- `internal/plugin/hook.go:77` — sends `"session.start"` as method
- `pkg/plugin/plugin.go:144-149` — dispatch routes on `"hook/invoke"`, not `"session.start"`
- `pkg/plugin/protocol.go:71-76` — `HookParams` struct expects name inside params

### C2. Plugin protocol mismatch: initialize handshake field names differ

The server sends `initializeParams` with JSON field `"protocolVersion"`, but the SDK's `InitializeParams` expects `"version"`. The plugin receives an empty version field. Additionally, the server's `initializeResult` only declares `Hooks []string`, silently ignoring the plugin's `ID` and `Tools` fields. Plugin-provided tools are never registered.

- `internal/plugin/manager.go:47-50` — sends `"protocolVersion"`, `"directory"`
- `pkg/plugin/protocol.go:31-34` — expects `"version"`, `"directory"`, `"options"`
- `internal/plugin/manager.go:53-55` — only captures `Hooks`, drops `ID` and `Tools`
- `pkg/plugin/protocol.go:39-43` — sends `ID`, `Tools`, and `Hooks`

### C3. SendPluginEvent calls a nonexistent server route

The TUI API client's `SendPluginEvent` posts to `POST /plugin/event`, but no such route exists in the router. Every call returns a 404 error, silently failing all plugin lifecycle notifications from the TUI.

- `internal/tui/api/plugin.go:17` — posts to `/plugin/event`
- `internal/server/router.go:89-93` — only has `/plugin`, `/plugin/load`, `/plugin/unload`, `/plugin/registry`

### C4. SSE event format mismatch between routes

The instance-scoped route `GET /event` calls `StreamGlobalEvents` (directory-wrapped envelope format), while the global route `GET /global/event` calls `StreamEvents` (flat format). The function names are inverted from the route names. The TUI client connects to `/event` and parses the wrapped format, so the TUI works. However, `handleGlobalEventStreamWrapped` in `handler_event.go:15-17` is dead code (never registered in the router), and any client connecting to `/global/event` expecting the global wrapped format will get flat events and fail to parse.

- `internal/server/router.go:7` — `GET /global/event` -> `handleGlobalEventStream`
- `internal/server/handler_stub.go:18-19` — `handleGlobalEventStream` calls `StreamEvents` (flat)
- `internal/server/router.go:12` — `GET /event` -> `handleEventStream`
- `internal/server/handler_event.go:11-12` — `handleEventStream` calls `StreamGlobalEvents` (wrapped)
- `internal/server/handler_event.go:15-17` — `handleGlobalEventStreamWrapped` is dead code

---

## HIGH

### H1. Path traversal vulnerability in handleFileRead and handleFileList

The `path` query parameter is passed directly to `os.ReadFile` / `os.ReadDir` with no validation against `s.config.Directory`. An attacker with network access can read any file on the system.

- `internal/server/handler_file.go:12-18` — `handleFileRead` uses raw `path` query param
- `internal/server/handler_file.go:36-42` — `handleFileList` uses raw `path` query param

### H2. Data race on activeSession.processor in Abort()

`StartPrompt` creates an `activeSession` with nil `processor`, unlocks, then spawns a goroutine that later sets `processor` under the lock. `Abort()` reads `active.processor` at line 252 *after* releasing its own lock at line 245. This is a data race per Go's memory model. The nil check prevents a panic, but concurrent read/write without synchronization is undefined behavior and will trigger the race detector.

- `internal/server/session_manager.go:242-255` — `Abort()` reads `active.processor` without lock
- `internal/server/session_manager.go:257-269` — `StartPrompt` creates session with nil processor
- `internal/server/session_manager.go:814-819` — goroutine sets processor under lock

### H3. Data race: maybeWarmup mutates Model.Capabilities from background goroutine

`m.Capabilities.ToolCall = false` is written from a background goroutine spawned in `maybeWarmup`. The `Model` struct is read concurrently from the `Registry` which only protects the map, not the struct fields.

- `internal/provider/discovery.go:401`

### H4. MCP Configure has unlock/relock race during map iteration

`Configure` unlocks `s.mu`, calls `s.stopServer(conn)`, then re-locks. During the unlock window, another goroutine could modify `s.servers`, invalidating the iterator.

- `internal/mcp/mcp.go:65-76`

### H5. MCP SSE pending map entries leak on disconnect

If the SSE stream disconnects while requests are pending, the `pending` map entries and their channels are never cleaned up or signaled. The goroutine in `sendRequest` hangs until timeout, and entries accumulate on rapid reconnect.

- `internal/mcp/sse.go:122-125`

### H6. Permission Service: closed flag is write-only

`s.closed` is set to `true` in `Close()` but never checked in `Ask()`. After `Close()`, new permission requests are still accepted, their `replyCh` will never be drained, and the requesting goroutine hangs until context cancellation.

- `internal/permission/permission.go:91,266`

### H7. Goroutine leak on RPC timeout in plugin manager

When `sendRPC` times out, the goroutine blocked on `p.decoder.Decode()` persists for the process lifetime. Repeated timeouts accumulate leaked goroutines. A timed-out decode goroutine may later write a stale response to the channel, which the next `sendRPC` call reads.

- `internal/plugin/manager.go:377-394`

### H8. statusFromError uses overly broad string matching

`strings.Contains(msg, "500")` matches any error containing "500" anywhere, e.g., "generated 5003 tokens" would trigger 500 detection, "completed in 429ms" would trigger 429 detection. This can cause incorrect retry/overflow classification.

- `internal/session/processor.go:563-578`

### H9. Retry IsOverflow prefix mismatch with actual error format

`strings.HasPrefix(errMsg, "400 ")` checks for "400 " prefix, but the OpenAI client formats errors as `"HTTP 400: <body>"`. The prefix will never match, meaning overflow detection silently fails.

- `internal/provider/retry.go:113`

### H10. Agent ApplyConfigOverrides is never called

The method exists and is tested, but no caller invokes it. User agent config overrides from `config.json` are silently ignored at runtime.

- `internal/agent/agent.go:144`

### H11. fileMutexes sync.Map grows unboundedly

Every unique file path edited gets a permanent entry in the `fileMutexes` sync.Map. Over a long-running session with many file edits, this leaks memory. No eviction or cleanup mechanism.

- `internal/tool/edit.go:12`

### H12. ACP event key mismatch: tool name always empty

`relayToolEvent` reads `props["name"]` but the bus publish in `tool.go` uses `"tool"` as the key, not `"name"`. ACP clients never see tool names in relayed events.

- `internal/acp/event.go:95` — reads `props["name"]`
- `internal/tool/tool.go:106` — publishes `"tool": name`

### H13. SSE reconnect backoff never resets after successful connection

After any disconnect+reconnect cycle, the escalated backoff persists. Subsequent disconnects use the escalated delay even though the connection was healthy in between.

- `internal/tui/api/sse.go:38-66` — `backoff` never reset to `initialBackoff`

### H14. processPrompt uses context.Background() — HTTP disconnect does not cancel LLM

The HTTP request context is not propagated to `processPrompt`. If the HTTP connection drops, LLM processing continues indefinitely.

- `internal/server/session_manager.go:235,269` — `context.Background()` used at call site

---

## MEDIUM

### M1. buildRequest loses earlier text parts in multi-text-part messages

When an assistant message has multiple text parts, only the last text part wins (`llmMsg.Content = part.Text` overwrites the previous). Earlier text is silently lost.

- `internal/session/processor.go:302-303`

### M2. Processor loop has no iteration upper bound

The `for {}` loop has no iteration limit. If an LLM repeatedly returns tool calls that all fail, the loop continues indefinitely. The `consecutiveToolFailures` counter only logs warnings, never breaks.

- `internal/session/processor.go:117`

### M3. DispatchShellEnv sends original env to all plugins, not accumulated

Each plugin receives the original `input.Env`, not the accumulated `merged` map. If plugin A adds `FOO=bar`, plugin B will not see it, despite the comment saying "Later plugins override earlier ones."

- `internal/plugin/hook.go:162`

### M4. Goroutine leaks in wirePluginHooks on server shutdown

The four subscription goroutines run forever and are only cleaned up if the bus closes their channels. The `Shutdown` method only shuts down the HTTP server and session manager, never unsubscribes these.

- `internal/server/server.go:174-240`

### M5. Revert Pop pops wrong stash if stack has changed

`Pop` runs plain `git stash pop` which pops whatever is on top of the stash stack, not the specific session stash. If another stash was created between revert and unrevert, the wrong stash gets popped.

- `internal/server/revert.go:42-63`

### M6. existingMsgs error silently discarded

The error from `ms.List(sessionID)` is ignored with `_`. If the database query fails, the processor proceeds with an empty message list, potentially losing conversation history context.

- `internal/server/session_manager.go:780-781`

### M7. Glob double-glob suffix matching doesn't handle multi-component suffixes

For patterns like `**/src/*.go`, the suffix `src/*.go` is matched via `filepath.Match(suffix, info.Name())` against just the filename. The `**` implementation is a simplification that doesn't handle multi-component suffixes.

- `internal/tool/glob.go:88-89`

### M8. webfetch uses http.DefaultClient with no transport-level timeouts

Per-request timeout is set via context, but `http.DefaultClient` has no TLS handshake, idle connection, or other transport-level timeouts for arbitrary user-supplied URLs.

- `internal/tool/webfetch.go:84`

### M9. truncateBytes can split multi-byte UTF-8 characters

Raw byte-level truncation can produce invalid UTF-8 at the cut point.

- `internal/tool/truncate.go:65-70`

### M10. MCP OAuth saveOAuthState has read-modify-write race

Two concurrent calls for different client IDs could lose writes. The file is read, modified in memory, and written back without file locking.

- `internal/mcp/oauth.go:297-316`

### M11. Theme ApplyTheme builds styles that are never used by components

`ApplyTheme` constructs a full `Theme` struct with per-component styles, but the actual rendering code uses package-level `style*` variables from `styles.go`. Only `Toast` actually uses its Theme. The rest is dead code.

- `internal/tui/theme_loader.go:47-162` vs `internal/tui/styles.go`

### M12. ChatView stickyBottom never re-engages after user scrolls up

When the user scrolls up, `stickyBottom` is set to `false`. No code path sets it back to `true`. Auto-scroll is permanently disabled once the user scrolls.

- `internal/tui/chat.go:119`

### M13. DiffView is unused — defined but never integrated

`DiffView`, `DiffOpenMsg`, and `DiffClosedMsg` are defined but never referenced in `app.go`, `run.go`, or any other TUI file. The component is complete but not wired into the app.

- `internal/tui/diffview.go`

### M14. Workspace is unused — defined but never referenced

`Workspace` and `WorkspaceChangedMsg` are defined but never created, updated, or read anywhere in the TUI.

- `internal/tui/workspace.go`

### M15. Escape key doesn't abort running session despite hint

The status bar shows "esc interrupt" in the hints line, and `abortSession` exists in `cmd.go:262`, but no key handler in `app.go` or `run.go` wires escape to abort. Pressing escape does nothing when a session is working.

- `internal/tui/statusbar.go:86` — shows "esc interrupt"
- `internal/tui/cmd.go:262` — `abortSession` function exists
- `internal/tui/app.go:416-429` — no escape handler for abort

### M16. Permission prompt reject/allow styling is identical

The `if a.action == PermissionReject` branch produces the exact same output as the `else` branch. This appears to be a stub for distinct reject styling that was never completed.

- `internal/tui/permission.go:165-169`

### M17. id.Create() inverts the ascending flag in a confusing way

`Create(prefix, true)` internally passes `descending=false`, producing ascending IDs. The double-negation API is confusing and a bug waiting to happen for new callers.

- `internal/id/id.go:80-81`

---

## LOW

### L1. Dead darwin branch in ConfigDir() and DataDir()

Both branches return the same path (`~/.config/tinycode`). The darwin-specific branch is dead code, or should use `~/Library/Application Support/tinycode`.

- `internal/config/paths.go:17-21,31-35`

### L2. DB.mu field declared but never used

`DB` embeds `sync.RWMutex` as field `mu` but no method ever locks it. Dead field.

- `internal/storage/db.go:28-29`

### L3. SubstituteEnvVars silently swallows unresolved placeholders

If `{env:VAR_NAME}` is found but the variable doesn't exist, the placeholder silently disappears. Could lead to empty API keys or URLs.

- `internal/config/jsonc.go:130-141`

### L4. No directory argument support despite documentation

CLAUDE.md says `./dist/tinycode <directory>` works, but `main()` treats any argument as a subcommand.

- `cmd/tinycode/main.go:46-69`

### L5. MCP StreamableHTTPTransport missing onNotification callback

`setTransportCallbacks` does not set `onNotification` on StreamableHTTPTransport. Tool-change notifications from streamable-HTTP servers are silently ignored.

- `internal/mcp/mcp.go:156-179`

### L6. Ollama methods use http.DefaultClient instead of configured client

`ShowModel`, `CreateProfile`, and `DeleteModel` bypass custom timeout/transport configuration.

- `internal/provider/ollama.go:169,238,266`

### L7. WarmupProbe uses http.DefaultClient too

Same issue as L6.

- `internal/provider/warmup.go:31`

### L8. Token cost always 0

`store.UpdateCost(sessionID, 0, ...)` always passes 0 for cost. Cost tracking is incomplete.

- `internal/server/session_manager.go:853`

### L9. Pending stores grow without bounds

`PermissionStore` and `QuestionStore` entries are only removed via explicit `Remove()`. No TTL or cleanup for abandoned entries.

- `internal/server/pending_store.go`

### L10. Unused scanner interface in session package

Declared but never used anywhere.

- `internal/session/session.go:259-261`

### L11. scanSession reports empty session ID in error

When `sql.ErrNoRows`, the error includes `""` instead of the actual session ID (which is not passed as a parameter).

- `internal/session/session.go:281`

### L12. Duplicated parseFrontmatter function

Identical implementations in both `command` and `skill` packages. Changes to one won't propagate.

- `internal/command/discovery.go:139-168`
- `internal/skill/discovery.go:84-113`

### L13. AppState contains several unused fields

`Route`, `Focus`, `Parts`, `Permissions` are declared and initialized but never read or set.

- `internal/tui/state.go:7-11,18-24,102-105,112`

### L14. Deprecated strings.Title usage

`strings.Title(agent)` is deprecated since Go 1.18. Will be flagged by `go vet`.

- `internal/tui/chat_render.go:108`

### L15. Deprecated lipgloss Copy() usage

`lipgloss.Copy()` is deprecated in favor of direct chaining.

- `internal/tui/palette.go:184`

### L16. PluginHooks.fire() is synchronous despite "fire-and-forget" intent

Makes a blocking HTTP call that can freeze the TUI if the server is slow.

- `internal/tui/plugin_hooks.go:47`

### L17. isTerminalEscape has false-positive risk

Matching `"11;"` or `"10;"` filters user input containing these substrings (e.g., "10;00 AM").

- `internal/tui/prompt.go:244`

### L18. Autocomplete results cmd returned but never consumed

`AutocompleteResultsMsg` is defined and returned from `Autocomplete.Update` but the caller in `prompt.go` discards the third return value.

- `internal/tui/prompt_autocomplete.go:121-128`
- `internal/tui/prompt.go:141-158`

---

## Root Cause Analysis

The codebase has two systemic issues:

1. **The plugin subsystem was developed as two halves that never integrated.** The internal `Manager` (server-side) and the `pkg/plugin` SDK (plugin-side) define incompatible wire protocols. They share conceptual intent (JSON-RPC, hooks, tools) but disagree on method names, field names, and what the initialize response contains. This means any plugin built with the SDK cannot successfully communicate with the server.

2. **Several TUI components and server features were built in isolation and never wired into the app.** DiffView, Workspace, Theme system, escape-to-abort, and SendPluginEvent all have complete implementations that are disconnected from the running application. This suggests parallel development where integration was deferred.

---

## Recommendations

| # | Action | Severity | Effort | Impact |
|---|--------|----------|--------|--------|
| 1 | Fix plugin protocol alignment (C1, C2) | CRITICAL | Medium | High |
| 2 | Add path validation to file handlers (H1) | HIGH | Low | High |
| 3 | Register `POST /plugin/event` route or remove client method (C3) | CRITICAL | Low | Medium |
| 4 | Fix data races in session_manager.Abort() (H2) | HIGH | Low | High |
| 5 | Fix SSE route/handler naming inversion (C4) | CRITICAL | Low | Medium |
| 6 | Wire DiffView, Workspace, escape-to-abort, Theme into the app — or remove (M13-M15, M11) | MEDIUM | Medium | Medium |
| 7 | Fix statusFromError string matching (H8) | HIGH | Low | Medium |
| 8 | Add cleanup to MCP SSE pending map and tool fileMutexes (H5, H11) | HIGH | Medium | Medium |
| 9 | Fix SSE backoff reset (H13) | HIGH | Low | Low |
| 10 | Check permission.closed in Ask() (H6) | HIGH | Low | Medium |

### Trade-offs

| Option | Pros | Cons |
|--------|------|------|
| Fix plugin protocol in Manager (align to SDK) | Preserves public SDK API contract; plugins already built against SDK will work | Requires changes to internal Manager code; existing non-SDK plugins (if any) would break |
| Fix plugin protocol in SDK (align to Manager) | No server changes needed | Breaks the public API contract; less standard (direct method names vs "hook/invoke" envelope) |
| Remove dead TUI components | Reduces code surface, easier to maintain | Loses implemented features that may be wanted soon |
| Wire dead TUI components into app | Complete feature set | Increases integration risk; needs testing; some components may not be ready |
| Add path validation via filepath prefix check | Simple, fast | Symlinks can still escape the directory boundary |
| Add path validation via filepath.EvalSymlinks + prefix check | Symlink-safe | Slightly more complex; requires the path to exist before checking |
