# 2. API Routes

All routes are served by `internal/server/` using Go's `net/http.ServeMux` with method-pattern routing (Go 1.22+). The server binds to `127.0.0.1:4096` by default, falling back to a random port if 4096 is in use.

## 2.1 Middleware Stack

Applied in order (outermost wraps innermost):

```
CORS → TokenAuth → SecurityHeaders → Handler
```

1. **CORS** (`middleware/cors.go`) — Allows origins matching `localhost` or `127.0.0.1` on any port (`isLocalhostOrigin()`). Methods: `GET, POST, PUT, PATCH, DELETE, OPTIONS`. Headers: `Content-Type, Authorization, X-Request-ID`. Max-Age: `86400s`. Credentials allowed.
2. **TokenAuth** (`middleware/auth.go`) — If a token is configured, validates `Authorization: Bearer <token>`. Disabled (pass-through) when token is empty. `OPTIONS` requests always pass through for CORS preflight.
3. **SecurityHeaders** (`middleware/security.go`) — Sets `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `X-XSS-Protection: 1; mode=block`, `Referrer-Policy: strict-origin-when-cross-origin`, `Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'`.

Source: `internal/server/middleware/`

## 2.2 Global Routes

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/global/health` | `handleHealth` | Returns `{"healthy": true, "version": "0.1.0"}` |
| GET | `/global/version` | `handleVersion` | Returns `{"version": "0.1.0"}` |
| GET | `/global/event` | `handleGlobalEventStream` | SSE stream for all bus events (wildcard subscription) |
| GET | `/global/config` | `handleConfigGet` | Get merged config for a directory (`?directory=`) |
| PATCH | `/global/config` | `handleConfigUpdate` | Update config fields |
| POST | `/global/dispose` | `handleGlobalDispose` | Trigger graceful shutdown |

## 2.3 Event Routes

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/event` | `handleEventStream` | SSE stream (same as global event stream) |

## 2.4 Session Routes

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| POST | `/session` | `handleSessionCreate` | Create a new session. Query: `?directory=`. Body: `{title, agent, model}` |
| GET | `/session` | `handleSessionList` | List sessions. Query: `?directory=&limit=&offset=` |
| GET | `/session/status` | `handleSessionStatus` | Get status of all active sessions |
| GET | `/session/{id}` | `handleSessionGet` | Get session by ID |
| PATCH | `/session/{id}` | `handleSessionUpdate` | Update session fields (title, agent, model) |
| DELETE | `/session/{id}` | `handleSessionDelete` | Delete a session and its messages |

## 2.5 Session Action Routes

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| POST | `/session/{id}/message` | `handleSessionPrompt` | Synchronous prompt submission (blocks until complete) |
| POST | `/session/{sessionID}/prompt_async` | `handleSessionPromptAsync` | Async prompt submission (returns immediately, stream via SSE) |
| POST | `/session/{id}/abort` | `handleSessionAbort` | Abort the active processor for a session |
| POST | `/session/{id}/fork` | `handleSessionFork` | Fork a session (creates child with copied messages) |
| POST | `/session/{id}/init` | `handleSessionInit` | Initialize a session (load agent, set system prompt) |
| POST | `/session/{id}/summarize` | `handleSessionSummarize` | Trigger context compaction |
| POST | `/session/{id}/command` | `handleSessionCommand` | Execute a client command (e.g., `connect`, `compact`) |
| POST | `/session/{id}/revert` | `handleSessionRevert` | Revert the last assistant turn |
| POST | `/session/{id}/unrevert` | `handleSessionUnrevert` | Undo a revert |

## 2.6 Message Routes

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/session/{id}/message` | `handleMessageList` | List messages for a session |
| GET | `/session/{id}/message/{messageID}` | `handleMessageGet` | Get a specific message |
| DELETE | `/session/{sessionID}/message/{messageID}` | `handleMessageDelete` | Delete a message and its parts |
| GET | `/session/{id}/children` | `handleSessionChildren` | List child sessions |
| GET | `/session/{id}/todo` | `handleSessionTodo` | Get todo items for a session |
| GET | `/session/{id}/diff` | `handleSessionDiff` | Get file diffs for a session |
| GET | `/session/{id}/event` | `handleSessionEventStream` | SSE stream filtered to a specific session |

## 2.7 Permission Routes

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| POST | `/session/{sessionID}/permissions/{permissionID}` | `handleSessionPermissionReply` | Reply to a session-scoped permission request |
| GET | `/permission` | `handlePermissionList` | List all pending permission requests |
| POST | `/permission/{id}/reply` | `handlePermissionReply` | Reply to a global permission request |

### Permission Reply Body

```json
{
  "reply": "once" | "always" | "reject",
  "message": "optional feedback on rejection"
}
```

## 2.8 Question Routes

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/question` | `handleQuestionList` | List pending user questions |
| POST | `/question/{id}/reply` | `handleQuestionReply` | Reply to a user question |

## 2.9 Provider Routes

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/provider` | `handleProviderList` | List all registered providers and their models |
| GET | `/provider/{id}` | `handleProviderGet` | Get a specific provider |
| GET | `/provider/{id}/model` | `handleModelList` | List models for a provider |
| GET | `/provider/{providerID}/model/{modelID}` | `handleModelGet` | Get a specific model |

## 2.10 Auth Routes

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| PUT | `/auth/{providerID}` | `handleAuthPut` | Store credentials for a provider |
| DELETE | `/auth/{providerID}` | `handleAuthDelete` | Remove stored credentials |

## 2.11 Config Routes

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/config` | `handleConfigGet` | Get merged config |
| PATCH | `/config` | `handleConfigUpdate` | Update config fields |
| GET | `/config/providers` | `handleConfigProviders` | Get config-defined provider settings |

## 2.12 File Routes

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/file` | `handleFileList` | List files in a directory (`?path=`) |
| GET | `/file/content` | `handleFileRead` | Read file content (`?path=`) |
| GET | `/file/status` | `handleFileStatus` | Get git status for files |
| GET | `/find` | `handleFileSearch` | Search file contents (`?q=`) |
| GET | `/find/file` | `handleFileFind` | Find files by name pattern (`?q=`) |

## 2.13 Project Routes

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/project` | `handleProjectList` | List all known projects |
| GET | `/project/current` | `handleProjectCurrent` | Get current project info |

## 2.14 Metadata Routes

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/path` | `handlePathGet` | Resolve and validate a filesystem path |
| GET | `/agent` | `handleAgentList` | List all registered agents |
| GET | `/skill` | `handleSkillList` | List available skills |
| GET | `/command` | `handleCommandList` | List all slash commands |

## 2.15 VCS Routes

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/vcs` | `handleVCSInfo` | Get VCS (git) info for project |
| GET | `/vcs/status` | `handleVCSStatus` | Get git status |
| GET | `/vcs/diff` | `handleVCSDiff` | Get parsed diff |
| GET | `/vcs/diff/raw` | `handleVCSDiffRaw` | Get raw diff output |

## 2.16 LSP & Formatter Routes

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/lsp` | `handleLSP` | LSP server status |
| GET | `/formatter` | `handleFormatter` | Formatter availability |

## 2.17 MCP Routes

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/mcp/status` | `handleMCPStatus` | MCP server connection status |

## 2.18 Plugin Routes

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/plugin` | `handlePluginList` | List loaded plugins |
| POST | `/plugin/load` | `handlePluginLoad` | Load a plugin by path or name |
| POST | `/plugin/unload` | `handlePluginUnload` | Unload a plugin |
| POST | `/plugin/event` | `handlePluginEvent` | Send an event to plugins |
| GET | `/plugin/registry` | `handlePluginRegistry` | Search the curated plugin registry |

## 2.19 Static File Serving

If `ServeWebUI` is enabled, a catch-all `GET /` handler serves the embedded web app (or a dev directory override) via `internal/static/`. SPA fallback returns `index.html` for non-asset paths.

## 2.20 Response Format

All JSON endpoints use `application/json` content type. Errors return:

```json
{
  "error": "human-readable error message"
}
```

HTTP status codes follow REST conventions: 200 (OK), 201 (Created), 204 (No Content), 400 (Bad Request), 404 (Not Found), 500 (Internal Server Error).

## 2.21 Route Count Summary

| Category | Count |
|----------|-------|
| Global | 6 |
| Event | 1 |
| Session | 6 |
| Session Actions | 9 |
| Messages | 7 |
| Permissions | 3 |
| Questions | 2 |
| Providers | 4 |
| Auth | 2 |
| Config | 3 |
| File | 5 |
| Project | 2 |
| Metadata | 4 |
| VCS | 4 |
| LSP/Formatter | 2 |
| MCP | 1 |
| Plugin | 5 |
| **Total** | **66** |
