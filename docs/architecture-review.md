# Architecture Review — tinycode-go

**Date:** 2026-09-08
**Scope:** All packages in `internal/`, `cmd/`, `pkg/` (110+ non-test source files, 20+ packages)

## Summary

| Severity | Count |
|----------|-------|
| HIGH | 0 |
| MEDIUM | 15 |
| LOW | 24 |
| **Total** | **45** |
| Resolved | 11 |

Both HIGH-severity issues have been resolved. The remaining open items are medium-severity correctness issues, unused/dead code, and minor API inconsistencies.

---

## Resolved (fixed in working tree)

These were identified during review and fixed before this document was finalized:

- **statusFromError false positives** — `strings.Contains(msg, "500")` matched any error containing "500" anywhere. Fixed with regex `\bHTTP (\d{3})\b`. (`internal/session/processor.go:584`)
- **IsOverflow prefix mismatch** — `strings.HasPrefix(errMsg, "400 ")` never matched actual `"HTTP 400: ..."` format. Fixed to `HasPrefix(errMsg, "HTTP 400:")`. (`internal/provider/retry.go:113`)
- **buildRequest text overwrite** — Multiple text parts overwritten with `llmMsg.Content = part.Text`. Fixed to accumulate into `[]string` and join with `\n`. (`internal/session/processor.go:317-338`)
- **No iteration limit** — Infinite loop if LLM kept returning tool calls. Fixed with `maxIterations = 200`. (`internal/session/processor.go:20,122-126`)
- **ms.List error ignored** — Error from loading existing messages silently dropped. Fixed with `slog.Warn` and continue with nil. (`internal/server/session_manager.go:787-791`)

---

## HIGH

### H1. Data race: maybeWarmup mutates Model.Capabilities from background goroutine — **[RESOLVED]**

`maybeWarmup` spawns a background goroutine that writes `m.Capabilities.ToolCall = false` on the `Model` struct. The `Registry` protects the map with a mutex, but not the struct fields. Any concurrent reader of `Model.Capabilities` (e.g., provider listing, model selection) races with this write.

- `internal/provider/discovery.go:401`
- **Fix:** Warmup now copies the Model struct before mutating in the goroutine.

### H2. Goroutine leak on RPC timeout in plugin manager — **[RESOLVED in #82]**

When `sendRPC` times out, the goroutine blocked on `p.decoder.Decode()` persists for the process lifetime. Repeated timeouts accumulate leaked goroutines. A timed-out decode goroutine may later write a stale response to the channel, which the next `sendRPC` call reads as a spurious response.

- `internal/plugin/manager.go:387-409`
- **Fix:** Plugin subsystem rewritten with protocol alignment between `internal/plugin/` and `pkg/plugin/` (#82).

---

## MEDIUM

### M1. DispatchShellEnv sends original env to all plugins, not accumulated

Each plugin receives `input.Env` (the original), not the `merged` map built from prior plugins' responses. Plugin B will not see variables added by plugin A.

- `internal/plugin/hook.go:202-204`

### M2. Goroutine leaks in wirePluginHooks on server shutdown

Four subscription goroutines in `wirePluginHooks` run forever. They are only cleaned up if the bus closes their channels. The `Shutdown` method only shuts down the HTTP server and session manager, never unsubscribes these.

- `internal/server/server.go:174-240`

### M3. Revert Pop pops wrong stash if stack has changed

`Pop` runs plain `git stash pop` which pops whatever is on top of the stash stack, not the specific session stash. If another stash was created between revert and unrevert, the wrong stash gets popped.

- `internal/server/revert.go:42-63`

### M4. Glob double-glob suffix matching limited

For patterns like `**/src/*.go`, the suffix `src/*.go` is matched via `filepath.Match(suffix, info.Name())` against just the filename. The `**` implementation doesn't handle multi-component suffixes correctly.

- `internal/tool/glob.go:88-89`

### M5. webfetch uses http.DefaultClient with no transport-level timeouts

Per-request timeout is set via context, but `http.DefaultClient` has no TLS handshake, idle connection, or other transport-level timeouts for arbitrary user-supplied URLs.

- `internal/tool/webfetch.go:84`

### M6. truncateBytes can split multi-byte UTF-8 characters

Raw byte-level truncation can produce invalid UTF-8 at the cut point.

- `internal/tool/truncate.go:65-70`

### M7. MCP OAuth saveOAuthState has read-modify-write race

Two concurrent calls for different client IDs could lose writes. The file is read, modified in memory, and written back without file locking.

- `internal/mcp/oauth.go:297-316`

### M8. Theme ApplyTheme builds styles that are never used by components — **[RESOLVED]**

`ApplyTheme` constructs a full `Theme` struct with per-component styles, but the actual rendering code uses package-level `style*` variables from `styles.go`. Only `Toast` actually uses its Theme. The rest is dead code.

- `internal/tui/theme_loader.go` vs `internal/tui/styles.go`
- **Fix:** Dead theme code removed; Toast theme retained.

### M9. DiffView is unused — defined but never integrated — **[RESOLVED]**

`DiffView`, `DiffOpenMsg`, and `DiffClosedMsg` are defined but never referenced in `app.go`, `run.go`, or any other TUI file. The component is complete but not wired into the app.

- `internal/tui/diffview.go`
- **Fix:** Dead component removed.

### M10. Workspace is unused — defined but never referenced — **[RESOLVED]**

`Workspace` and `WorkspaceChangedMsg` are defined but never created, updated, or read anywhere in the TUI.

- `internal/tui/workspace.go`
- **Fix:** Dead component removed.

### M11. Permission prompt reject/allow styling is identical

The `if a.action == PermissionReject` branch produces the exact same output as the `else` branch. This appears to be a stub for distinct reject styling that was never completed.

- `internal/tui/permission.go:165-169`

### M12. id.Create() inverts the ascending flag via double negation

`Create(prefix, ascending=true)` internally passes `descending=!ascending`, producing ascending IDs. The double-negation API is confusing and a bug waiting to happen for new callers.

- `internal/id/id.go:80-81`

### M13. API client appends directory to URL without encoding

`CreateSession` and `ListSessions` append `?directory=` + raw directory path to the URL with no `url.QueryEscape()`. Paths containing spaces, `&`, `#`, or other special characters will break the URL.

- `internal/tui/api/client.go:34` — `"/session?directory="+c.directory`
- `internal/tui/api/client.go:43` — same pattern in `ListSessions`

### M14. Duplicate JSON-RPC types across MCP and plugin packages

`jsonrpcRequest`, `jsonrpcResponse`, and `jsonrpcError` are defined independently in both `internal/mcp/stdio.go:314-332` and `internal/plugin/manager.go:26-44`, with slightly different field types (`ID int64` vs `ID int`). Changes to the protocol in one package won't propagate.

- `internal/mcp/stdio.go:314-332`
- `internal/plugin/manager.go:26-44`

### M15. fileMutexes sync.Map grows unboundedly

Every unique file path edited gets a permanent entry in the `fileMutexes` sync.Map. Over a long-running session with many file edits, this leaks memory. No eviction or cleanup mechanism.

- `internal/tool/edit.go:12`

### M16. subscribeSummarize is a stub

Publishes `session.status` and `session.compacted` events without performing actual LLM-driven compaction. The "Manual summarize requested" message is cosmetic — no summarization occurs.

- `internal/server/session_manager.go:940-970`

### M17. WildcardMatch compiles a new regex every call

`WildcardMatch` constructs and compiles a fresh regex on every invocation. No caching. Called in a hot path during permission evaluation — multiple patterns per tool call, every tool call.

- `internal/permission/wildcard.go:9-31`

### M18. handleConfigUpdate shallow merge may lose nested structure

The config update handler does a top-level key merge (`existing[k] = v`). If a client sends `{"server": {"port": 8080}}`, it replaces the entire `server` key, losing any other nested fields like `host`.

- `internal/server/handler_config.go:41-43`

---

## LOW

### L1. Dead darwin branch in ConfigDir() and DataDir()

Both branches return the same path (`~/.config/tinycode`). The darwin-specific branch is dead code, or should use `~/Library/Application Support/tinycode`.

- `internal/config/paths.go:17-21,31-35`

### L2. DB.mu field declared but never used

`DB` embeds `sync.RWMutex` as field `mu` but no method ever locks it. Dead field.

- `internal/storage/db.go:28`

### L3. SubstituteEnvVars silently swallows unresolved placeholders

If `{env:VAR_NAME}` is found but the variable doesn't exist, the placeholder silently disappears. Could lead to empty API keys or URLs.

- `internal/config/jsonc.go:130-141`

### L4. No directory argument support despite documentation

CLAUDE.md says `./dist/tinycode <directory>` works, but `main()` treats any argument as a subcommand and will print "unknown command".

- `cmd/tinycode/main.go:46-69`

### L5. MCP StreamableHTTPTransport missing onNotification callback

`setTransportCallbacks` does not set `onNotification` on StreamableHTTPTransport. Tool-change notifications from streamable-HTTP servers are silently ignored.

- `internal/mcp/mcp.go:179`

### L6. Ollama methods use http.DefaultClient instead of configured client

`ShowModel`, `CreateProfile`, and `DeleteModel` bypass custom timeout/transport configuration.

- `internal/provider/ollama.go:169,238,266`

### L7. WarmupProbe uses http.DefaultClient too

Same issue as L6 — bypasses any proxy or timeout configuration.

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

When `sql.ErrNoRows`, the error includes `""` instead of the actual session ID.

- `internal/session/session.go:281`

### L12. Duplicated parseFrontmatter function (3 copies)

Identical implementations in `command`, `skill`, and `agent` packages. Changes to one won't propagate.

- `internal/command/discovery.go:139-168`
- `internal/skill/discovery.go:84-113`
- `internal/agent/agent.go:455`

### L13. AppState contains several unused fields

`Route`, `Focus`, `Parts`, `Permissions` are declared and initialized but never read or set elsewhere.

- `internal/tui/state.go:95-113`

### L14. Deprecated strings.Title usage

`strings.Title(agent)` is deprecated since Go 1.18. Flagged by `go vet` / staticcheck.

- `internal/tui/chat_render.go:108`

### L15. Deprecated lipgloss Copy() usage

`lipgloss.Copy()` is deprecated in favor of direct chaining.

- `internal/tui/palette.go:183,186`

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

### L19. PluginHooks.fire() uses log.Printf instead of slog

All other code uses `slog` for structured logging, but `plugin_hooks.go` uses `log.Printf`. Inconsistent.

- `internal/tui/plugin_hooks.go:48`

### L20. connectedApp type-asserts tea.Model to App without safety check

`c.app = model.(App)` will panic if any component's `Update` returns a different type. A defensive comma-ok assertion would prevent panics.

- `internal/tui/run.go:85,159,169,209,228,239,252`

### L21. MCP OAuth port parsing ignores Sscanf error

`fmt.Sscanf(u.Port(), "%d", &port)` silently keeps the default port if the URL port string is malformed.

- `internal/mcp/oauth.go:219`

### L22. StreamGlobalEvents accesses zero-value Event when channel closed

When `sub.C` is closed (`ok=false`), the handler reads `evt.ID` from a zero-value `bus.Event`, sending a `global.disposed` event with empty fields. Only occurs during shutdown.

- `internal/server/handler_event.go:52-57`

### L23. Manager.sendHook (Manager-level) is dead code

`Manager.sendHook` at `manager.go:328` accepts a `method` string and forwards it directly via `sendRPC`. It is never called anywhere — all hook dispatch goes through `pluginProcess.sendHook` in `hook.go:22` which correctly wraps with `"hook/invoke"`.

- `internal/plugin/manager.go:328-342`

### L24. json.Marshal error silently dropped in session store

`modelJSON, _ = json.Marshal(input.Model)` drops the error. If the model struct contains non-serializable fields (unlikely but possible), the session is created with empty model JSON.

- `internal/session/session.go:117`

### L25. SSE route naming is confusing — **[RESOLVED]**

`GET /event` calls `StreamEvents` (flat format), while `GET /global/event` calls `StreamGlobalEvents` (envelope format). The TUI connects to `/global/event` and works correctly, but the naming inverts expectations — the route NOT named "global" uses the handler named with global semantics. No functional bug, but confusing for new contributors.

- `internal/server/router.go:7,12`
- `internal/server/handler_event.go:11-12`
- `internal/server/handler_stub.go:18-19`
- **Fix:** Handler names aligned with route semantics.

---

## Root Cause Analysis

The codebase has two systemic patterns:

1. **Several TUI components were built in isolation and never wired into the app.** DiffView, Workspace, Theme system, and several AppState fields all have complete implementations that are disconnected from the running application. This suggests parallel development where integration was deferred.

2. **Shared abstractions are duplicated instead of extracted.** `parseFrontmatter` appears in 3 packages, JSON-RPC types in 2 packages, and `http.DefaultClient` is used in places that should share a configured client. This creates maintenance burden when behavior needs to change.

---

## Recommendations

| # | Action | Severity | Effort | Impact | Status |
|---|--------|----------|--------|--------|--------|
| 1 | Fix maybeWarmup race — copy Model before goroutine or sync field access (H1) | HIGH | Low | High | **DONE** |
| 2 | Fix plugin RPC timeout goroutine leak — cancel decode goroutine or use deadline on stream (H2) | HIGH | Medium | Medium | **DONE (#82)** |
| 3 | Fix DispatchShellEnv to pass accumulated env to subsequent plugins (M1) | MEDIUM | Low | Medium | Open |
| 4 | URL-encode directory in API client (M13) | MEDIUM | Low | Medium | Open |
| 5 | Cache compiled regexes in WildcardMatch (M17) | MEDIUM | Low | Medium | Open |
| 6 | Wire DiffView, Workspace, Theme into the app — or remove (M8-M10) | MEDIUM | Medium | Medium | **DONE** (removed) |
| 7 | Extract shared parseFrontmatter and JSON-RPC types (M14, L12) | MEDIUM | Low | Low | Open |
| 8 | Add stash ref tracking to revert (M3) | MEDIUM | Medium | Medium | Open |
| 9 | Implement actual summarization in subscribeSummarize or document as intentional (M16) | MEDIUM | High | Medium | Open |
| 10 | Clean up dead code: Manager.sendHook, AppState unused fields, DB.mu (L2, L13, L23) | LOW | Low | Low | Open |
