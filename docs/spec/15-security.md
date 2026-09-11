# 15. Security

Cross-cutting security mechanisms spanning server middleware, tool execution, session processing, storage, and process lifecycle.

---

## 15.1 Server Authentication

Source: `internal/server/middleware/auth.go`, `cmd/tinycode/config.go`

Token-based authentication on every HTTP request.

**Token generation:** `generateToken()` produces a cryptographically random 64-character hex token (32 bytes via `crypto/rand`). A fresh token is generated on each startup for TUI, serve, and web modes.

**Validation flow:**

1. If token is empty string, middleware is pass-through (auth disabled)
2. `OPTIONS` requests pass through unconditionally (CORS preflight)
3. All other requests must include `Authorization: Bearer <token>` header
4. Mismatch returns `401 Unauthorized` with JSON body `{"error": "unauthorized"}`

**Middleware chain order** (outermost to innermost):

```
CORS → TokenAuth → SecurityHeaders → router
```

Source: `internal/server/server.go:105`

## 15.2 CORS Policy

Source: `internal/server/middleware/cors.go`

| Setting | Value |
|---------|-------|
| Allowed origins | `localhost` or `127.0.0.1` on any port (via `isLocalhostOrigin()`) |
| Allowed methods | GET, POST, PUT, PATCH, DELETE, OPTIONS |
| Allowed headers | Content-Type, Authorization, X-Request-ID |
| Max-Age | 86400 seconds (24 hours) |
| Credentials | Allowed |

**Origin validation:** Parses origin URL, extracts hostname. Only `localhost` and `127.0.0.1` pass. Port is ignored (any port accepted). The `CORSConfig` also supports explicit `AllowOrigins` list and wildcard `*`, but the default config uses only the function-based check.

**Behavior on non-matching origin:** Request proceeds without CORS headers (browser blocks cross-origin access).

**Preflight handling:** `OPTIONS` requests with a valid origin receive CORS headers and `204 No Content`. Non-OPTIONS requests receive CORS headers but are forwarded to the next handler.

## 15.3 Security Headers

Source: `internal/server/middleware/security.go`

Applied to every response:

| Header | Value | Purpose |
|--------|-------|---------|
| `X-Content-Type-Options` | `nosniff` | Prevents MIME type sniffing |
| `X-Frame-Options` | `DENY` | Blocks all iframe embedding |
| `X-XSS-Protection` | `1; mode=block` | Enables browser XSS filter |
| `Referrer-Policy` | `strict-origin-when-cross-origin` | Limits referrer to origin on cross-origin |
| `Content-Security-Policy` | `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'` | Restricts resource loading to same origin |

## 15.4 Loopback Binding

Source: `internal/server/server.go`

Default bind address: `127.0.0.1` (loopback only). Configurable via `TINYCODE_HOST` environment variable.

Server constants:

| Constant | Value |
|----------|-------|
| `defaultPort` | 4096 |
| Default hostname | `127.0.0.1` |
| `readHeaderTimeout` | 10 seconds |

## 15.5 SSRF Protection

Source: `internal/tool/webfetch.go`

The `webfetch` tool performs pre-flight DNS resolution against blocked CIDR ranges:

| Range | Description |
|-------|-------------|
| `127.0.0.0/8` | Loopback |
| `10.0.0.0/8` | Private (Class A) |
| `172.16.0.0/12` | Private (Class B) |
| `192.168.0.0/16` | Private (Class C) |
| `169.254.0.0/16` | Link-local |
| `0.0.0.0/8` | Current network |
| `::1/128` | IPv6 loopback |
| `fc00::/7` | IPv6 unique local |
| `fe80::/10` | IPv6 link-local |

Redirect targets are also validated against the blocklist. Max 10 redirects. TLS minimum version: TLS 1.2.

## 15.6 Shell Command Safety

Source: `internal/tool/shell.go`

### Destructive Command Detection

`isDestructive()` checks commands against 10 regex patterns before execution:

| Pattern | Matches |
|---------|---------|
| `rm -rf` / `rm -R` | Recursive file deletion |
| `git push --force` | Force push |
| `git reset --hard` | Hard reset |
| `git clean -f` | Force clean |
| `git branch -D` | Force branch delete |
| `DROP TABLE/DATABASE` | SQL destructive operations |
| `TRUNCATE TABLE` | SQL truncation |
| `kill -9` | Forceful process kill |
| `mkfs` | Filesystem formatting |
| `dd` / `> /dev/sd*` | Raw disk writes |

When a destructive pattern is matched:
- If permission service is available: triggers `permission.Ask()` with permission `"destructive-shell"` and the command as both pattern and metadata
- If no permission service: returns error with warning message

### Secret File Access Detection

`checkSecretAccess()` checks commands against 5 regex patterns and emits `slog.Warn` if matched:

| Pattern | Matches |
|---------|---------|
| `\.env` | `.env` files |
| `\.env\.\w+` | `.env.production`, `.env.local`, etc. |
| `credentials` | Credential files |
| `*.key` | Private key files |
| `*.pem` | PEM certificate/key files |

This is a warning-only check (logged, not blocked). Blocking of `.env*` reads is handled by the permission system's default rules (see section 15.8).

### Audit Trail

The shell tool accepts a `description` parameter for human-readable command descriptions, logged alongside the command for audit purposes.

### Execution Constraints

| Constraint | Value |
|------------|-------|
| Default timeout | 120 seconds |
| Maximum timeout | 600 seconds (10 minutes) |
| Shell | `sh -c` |
| Working directory | Session's project directory |

## 15.7 Binary Detection

Source: `internal/tool/read.go`

The `read` tool detects binary files before reading:
- Checks first 8,192 bytes (`binaryCheckSize`)
- If > 30% (`binaryThreshold`) of bytes are non-text, returns error
- Null bytes count as 10 non-text bytes each (strong binary indicator)
- Prevents accidental exposure of binary file contents to the LLM

## 15.8 Permission-Based Security

Source: `internal/permission/defaults.go`, `internal/session/processor_validation.go`

See [12-permissions.md](12-permissions.md) for the full permission system specification. This section covers the security-relevant defaults and behaviors.

### Default Rules

6 rules applied as the base layer for every session:

| Permission | Pattern | Action | Security Purpose |
|------------|---------|--------|-----------------|
| `read` | `*` | allow | Broad read access for tools |
| `read` | `.env*` | ask | Protects secret files from silent reads |
| `webfetch` | `*` | ask | Gates all external HTTP requests |
| `doom_loop` | `*` | ask | User confirmation on loop detection |
| `guardrail` | `*` | ask | Safety guardrail confirmation |
| `external_directory` | `*` | ask | Protects paths outside project directory |

### Doom Loop Detection

Source: `internal/session/processor_validation.go`

`isDoomLoop()` detects when the last N tool calls have identical signatures (same tool name and arguments). Default threshold: 3 consecutive identical calls (`defaultDoomThreshold`). Triggers `permission.Ask()` with `"doom_loop"` permission.

### Consecutive Failure Tracking

Source: `internal/session/processor.go`, `internal/session/processor_loop.go`

| Constant | Value |
|----------|-------|
| `maxConsecutiveToolFailures` | 10 |
| `consecutiveToolFailureWarnEvery` | 3 |

When a tool call returns an error, the consecutive failure counter increments. On success, it resets to zero. At every 3 failures, a warning event is published suggesting a model switch. At 10 consecutive failures, the session processing loop halts with an error.

### Plugin Deny-Wins

Plugin hooks use deny-wins semantics for `permission.ask`: any plugin denial overrides all allows, ensuring plugins can restrict but never override user denials.

## 15.9 External Directory Protection

Source: `internal/session/processor_validation.go`

`checkExternalDirectory()` prevents tools from silently operating on files outside the project directory.

### Path Extraction

`extractPathsFromArgs()` parses tool call JSON arguments and checks 16 path-related keys:

```
file_path, path, file, directory, dir, folder, target, destination,
source, src, dest, location, root, base_path, working_directory, cwd
```

### Containment Check

`isInsideDirectory()` determines if a path is inside the project directory:

1. Resolve both paths to absolute via `filepath.Abs()`
2. Compute relative path via `filepath.Rel(directory, path)`
3. If the relative path starts with `..`, the path is outside -- triggers permission check

### Permission Flow

When an external path is detected:
1. Generates a permission request ID via `id.Ascending("perm")`
2. Calls `permission.Ask()` with permission `"external_directory"`
3. Includes tool name and target path in metadata
4. On denial, returns a formatted error message with tool name, path, and project directory

## 15.10 Path Validation

### HTTP File API — Symlink-Aware Traversal Prevention

Source: `internal/server/handler_file.go`

`validatePath(requested)` prevents directory traversal attacks on the HTTP file endpoints:

1. `filepath.Clean()` the requested path
2. If relative, join with the server's working directory
3. `filepath.EvalSymlinks()` on the working directory to resolve to its real path
4. `filepath.EvalSymlinks()` on the requested path
5. If the requested path doesn't exist, resolve the parent directory via `EvalSymlinks()` and re-append the filename (catches symlink escapes for new file creation)
6. Verify the resolved path equals or is a child of the resolved working directory (prefix check with path separator)
7. On failure: return `os.ErrPermission` → `403 Forbidden`

This is **symlink-aware**: symlinks that resolve outside the working directory are rejected even if the unresolved path appears to be within it.

`handleFileList()` additionally filters dot-prefixed entries — files and directories starting with `.` are excluded from listings.

### Tool-Level Path Defaults

Source: `internal/tool/`

- Tool paths are resolved relative to `tc.Directory` (the session's working directory)
- The `read` tool resolves relative paths against the working directory
- The `grep` and `glob` tools default to the working directory if no explicit path is given
- Directories `.git`, `node_modules`, `vendor`, and `.tinycode` are skipped during search operations

## 15.11 Database Security

Source: `internal/storage/db.go`

SQLite pragmas applied on every connection:

| Pragma | Value | Purpose |
|--------|-------|---------|
| `journal_mode` | WAL | Crash-safe write-ahead logging |
| `synchronous` | NORMAL | Balanced durability/performance |
| `busy_timeout` | 5000 ms | Prevents lock contention errors |
| `cache_size` | -64000 (64 MB) | In-memory page cache |
| `foreign_keys` | ON | Referential integrity enforcement |

Connection pool: `MaxOpenConns = 1` serializes all writes through a single connection, preventing concurrent write conflicts.

Pragmas are set both via DSN query parameters and explicit `PRAGMA` statements to ensure they take effect regardless of driver behavior.

## 15.12 Plugin Security

Source: `internal/plugin/manager.go`

- Plugins run as separate processes with stdin/stdout JSON-RPC communication (process isolation)
- Plugin hooks have a 5-second timeout to prevent hanging
- Plugin processes are monitored for unexpected exits
- The `permission.ask` hook uses deny-wins semantics (see section 15.8)
- Plugin kill timeout: 3 seconds before force-kill on shutdown

## 15.13 Graceful Shutdown

Source: `internal/server/server.go`, `cmd/tinycode/tui.go`, `cmd/tinycode/serve.go`, `internal/plugin/manager.go`

### Signal Handling

All entry points (TUI, serve, web, ACP, run) register `signal.NotifyContext()` for `SIGINT` and `SIGTERM`. The resulting context cancellation triggers orderly shutdown.

### Shutdown Sequence

1. Context cancellation propagates to all goroutines
2. `SessionManager.Shutdown()` -- cancels all active session processors, waits for completion
3. `Bus.Publish("global.disposed")` -- notifies all subscribers
4. `httpServer.Shutdown(ctx)` with 25-second timeout (`shutdownTimeout`) -- drains in-flight HTTP requests
5. Deferred cleanup in entry points:
   - `Bus.Close()` -- rejects all pending bus subscribers
   - `PermService.Close()` -- rejects all pending permission requests
   - `MCPService.Close()` -- stops all MCP server connections
   - `PluginManager.Shutdown()` -- stops all plugin processes (3-second kill timeout)

---

Prev: [14-storage.md](14-storage.md) | Next: [16-not-implemented.md](16-not-implemented.md) | [Back to overview](00-overview.md)
