# 8. Tools

Built-in tools the model can call. Plugin tools are specified in [09-plugins.md](09-plugins.md).

Package: `internal/tool/`

## 8.1 Tool Framework

Each tool is defined by a `Def` struct:

```go
type Def struct {
    ID          string
    Description string
    Parameters  map[string]any        // JSON Schema
    Permission  string                // permission category
    Execute     func(ctx, tc, args)   // handler
}
```

**Tool context** (`Context`) provides: `SessionID`, `Directory`, `Perms` (permission service), `Bus` (event bus), `JobManager` (subagent jobs), `SubagentDepth`, `DB` (SQLite handle), `AfterHook` (post-execution modifier).

Tools are registered in a thread-safe `Registry` (sync.RWMutex) that preserves insertion order.

### Execution Flow

1. Look up tool by name; return error if not found or disabled
2. Create fresh `Context` with session-specific fields
3. If permission service exists and tool has a non-empty `Permission`, call `Perms.Ask()` (with agent `Ruleset` when set)
4. Publish `tool.execute.before` bus event
5. Call `def.Execute(ctx, toolCtx, args)`
6. On error: publish `tool.execute.after` with `success: false`, return error string as output
7. On success: apply `AfterHook` if set, publish `tool.execute.after`
8. Truncate via `TruncPreview(output)` (head+tail preview)
9. Return `(output, isError, nil)`

### Registration

`RegisterBuiltins(r)` registers 18 always-available tools (including `plan_enter`/`plan_exit`). `RegisterConditional(r, cfg)` adds conditional tools:

| Tool | Condition |
|------|-----------|
| `skill` | `ConfigDir` or `ProjectDir` is non-empty |
| `websearch` | `EXA_API_KEY` environment variable is set |

Source: `builtin.go`

---

## 8.2 Output Truncation

Source: `truncate.go`

| Constant | Value | Description |
|----------|-------|-------------|
| `MaxLines` | 2,000 | Maximum output lines |
| `MaxBytes` | 51,200 (50 KB) | Maximum output bytes |

**Truncation direction:**
- `TruncHead` — keep tail, prepend `"... [truncated N bytes, showing last M lines] ..."` 
- `TruncTail` — keep head, append `"... [truncated N bytes, showing first M lines] ..."`

**Registry path:** non-error tool output is truncated via `TruncPreview` (head+tail preview), not `Truncate(..., TruncTail)`.

Byte truncation respects UTF-8 rune boundaries.

---

## 8.3 Built-in Tools

### read

Read files with line numbers. Source: `read.go`

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `file_path` | string | yes | — | Absolute path |
| `offset` | integer | no | 0 | Line offset (0-indexed) |
| `limit` | integer | no | 2000 | Max lines to read |

| Constant | Value |
|----------|-------|
| `defaultReadLimit` | 2,000 lines |
| `maxLineLength` | 2,000 characters (truncated with `"..."`) |
| `binaryCheckSize` | 8,192 bytes |
| `binaryThreshold` | 0.30 (30% non-text bytes) |

**Behaviors:**
- Output format: `N\tcontent\n` (numbered lines)
- Image support: `.jpg`, `.jpeg`, `.png`, `.gif`, `.webp` returned as base64 data URIs
- Binary detection: checks first 8,192 bytes; null bytes count as 10 non-text bytes
- On file-not-found: `suggestSimilar()` finds files with >= 3 common prefix characters

**Permission:** `read`

### write

Create or overwrite files. Source: `write.go`

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `file_path` | string | yes | Absolute path |
| `content` | string | yes | File content |

**Behaviors:**
- Creates parent directories with `os.MkdirAll(dir, 0755)`
- Line ending preservation: if existing file has CRLF, converts new content to CRLF
- Writes with mode `0644`
- Publishes `file.modified` bus event with operation `"write"`

**Permission:** `edit`

### edit

Search-and-replace editing with 10-strategy fuzzy cascade. Source: `edit.go`, `edit_replacer.go`

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `file_path` | string | yes | — | Absolute path |
| `old_string` | string | yes | — | Text to find |
| `new_string` | string | yes | — | Replacement text |
| `replace_all` | boolean | no | false | Replace all occurrences |

**Cascade replacement strategies** (tried in order until one succeeds):

| # | Strategy | Behavior |
|---|----------|----------|
| 1 | SimpleReplacer | Exact `strings.Replace` |
| 2 | LineTrimmedReplacer | Trim whitespace per line |
| 3 | BlockAnchorReplacer (strict) | First/last lines exact, interior similarity >= 1.0 |
| 4 | WhitespaceNormalizedReplacer | Collapse all whitespace to single space |
| 5 | IndentationFlexibleReplacer | Strip common indent, match ignoring indent level |
| 6 | EscapeNormalizedReplacer | Normalize `\r\n` → `\n`, `\t` → spaces |
| 7 | TrimmedBoundaryReplacer | Strip blank leading/trailing lines |
| 8 | ContextAwareReplacer | Non-whitespace lines as anchors (>= 3 lines, >= 2 anchors) |
| 9 | BlockAnchorReplacer (relaxed) | Interior similarity >= 0.7 (threshold 0.3) |
| 10 | MultiOccurrenceReplacer | Fuzzy scoring >= 0.8, picks best match |

**Per-file mutex:** `fileMutexMap` serializes edits to the same path. Cleanup at `fileMutexCleanupThreshold = 1000` entries.

**Levenshtein:** 2-row DP algorithm in `edit_levenshtein.go`. `Similarity() = 1.0 - dist/maxLen`.

**On no match:** `fuzzyFind()` searches for the first line of the needle and shows "did you mean" context.

**Permission:** `edit`

### bash

Shell command execution. Tool ID is `bash` (not `shell`). Source: `shell.go`

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `command` | string | yes | — | Shell command |
| `timeout` | integer | no | 120,000 ms | Timeout in milliseconds (max 600,000) |
| `description` | string | no | — | Human-readable description |

| Constant | Value |
|----------|-------|
| `defaultShellTimeout` | 120 seconds |
| `maxShellTimeout` | 600 seconds (10 minutes) |

**Destructive command detection** (triggers `destructive-shell` permission check):
- `rm -rf` / `rm --recursive` variants
- `git push --force`, `git reset --hard`, `git clean -f`, `git branch -D`
- `DROP TABLE/DATABASE`, `TRUNCATE TABLE`
- `kill -9`, `mkfs`, `dd`, `> /dev/sd*`

**Secret file detection** (hard-block — returns error, does not ask): `.env`, `.env.*`, `credentials`, `*.key`, `*.pem`

Executes via `sh -c <command>` in `tc.Directory`. Combines stdout/stderr (stderr prefixed with `"STDERR:\n"`).

**Permission:** `shell` (agent allowlists may use `bash` or `shell` interchangeably)

### grep

Regex content search. Source: `grep.go`

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `pattern` | string | yes | — | Regex pattern |
| `path` | string | no | working dir | Search root |
| `include` | string | no | — | File glob filter (e.g., `*.go`) |
| `max_count` | integer | no | 500 | Max matches |

| Constant | Value |
|----------|-------|
| `maxGrepMatches` | 500 |
| Max file size | 1 MB (skipped if larger) |

Sorted by file modification time (newest first). Output: `relPath:lineNum:lineContent`. Skips: `.git`, `node_modules`, `vendor`, `.tinycode`.

**Permission:** `read`

### glob

File pattern matching. Source: `glob.go`

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `pattern` | string | yes | — | Glob pattern (supports `**`) |
| `path` | string | no | working dir | Search root |

| Constant | Value |
|----------|-------|
| `maxGlobResults` | 1,000 |

Two modes: simple glob (no `**`) uses `filepath.Glob`; double glob (`**`) uses custom recursive `walkDoubleGlob`. Sorted by modification time (newest first). Same skip directories as grep.

**Permission:** `read`

### question

Ask the user a question. Source: `question.go`

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `question` | string | yes | Question text |
| `options` | string[] | no | Optional choices |

Publishes `question.asked` event, subscribes to `question.reply`, and blocks until answer arrives (or context cancellation).

**Permission:** `read`

### webfetch

Fetch URL content with SSRF protection. Source: `webfetch.go`

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `url` | string | yes | — | URL to fetch |
| `method` | string | no | GET | HTTP method |
| `headers` | object | no | — | Additional headers |
| `body` | string | no | — | Request body |
| `format` | enum | no | text | `text`, `markdown`, `html` |

| Constant | Value |
|----------|-------|
| `maxFetchSize` | 5 MB |
| `fetchTimeout` | 30 seconds |
| Max redirects | 10 |
| TLS minimum | TLS 1.2 |

**SSRF protection:** Pre-flight DNS check against blocked CIDRs: `127.0.0.0/8`, `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `169.254.0.0/16`, `0.0.0.0/8`, `::1/128`, `fc00::/7`, `fe80::/10`; dial-time IP checks in `ssrfSafeTransport`; `CheckRedirect` limits redirects and schemes. Full scope/non-goals: [15-security.md §15.6](15-security.md).

**Permission:** `webfetch`

### websearch

Web search via Exa API. Source: `websearch.go`

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `query` | string | yes | — | Search query |
| `num_results` | integer | no | 8 | Number of results |

**Conditional:** Only registered when `EXA_API_KEY` env var is set. API endpoint: `https://api.exa.ai/search`. Timeout: 30 seconds.

**Permission:** `read`

### task

Spawn subagent sessions. Source: `task.go`

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `description` | string | yes | — | Short task description |
| `prompt` | string | yes | — | Task prompt |
| `subagent_type` | string | no | — | Agent type for subagent |
| `task_id` | string | no | — | Resume existing task |
| `background` | boolean | no | false | Run in background |

| Constant | Value |
|----------|-------|
| `maxSubagentDepth` | 5 |

**Status check mode:** When `task_id` is provided, returns JSON with job status/result.

**Permission:** `shell`

### todowrite

Manage session todo list. Source: `todowrite.go`

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `todos` | array | yes | Array of `{content, status, priority}` |

Status: `pending`, `in_progress`, `done`. Priority: `high`, `normal`, `low`. Replaces all existing todos. Persisted to SQLite. Publishes `session.todos.updated` event.

**Permission:** `edit`

### skill

Execute a skill by name. Source: `skill.go`

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Skill name or ID |
| `arguments` | string | no | Space-separated arguments (`$1`, `$2`, `$ARGUMENTS`) |

**Conditional:** Registered when `ConfigDir` or `ProjectDir` is non-empty. Discovers skills via `skill.Discover()`. On not-found, lists all available skills.

**Permission:** `read`

### plan_enter

Switch the session into plan mode. Source: `plan_switch.go`.

No parameters. On approval, records a pending switch to the `plan` agent (via a shared `SafeAgentSwitch`); the session processor applies it after the current tool round (`Processor.applyPendingAgentSwitch`), publishes `session.agent.switched`, and the caller persists the new agent so the next turn resolves `plan`'s permissions and system prompt.

**Permission:** `plan_enter` (native `build` agent allows it by default; other agents fall back to `ask`)

### plan_exit

Switch the session back to the `build` agent. Source: `plan_switch.go`.

No parameters. Same approval/apply flow as `plan_enter`, targeting `build` instead of `plan`.

**Permission:** `plan_exit` (native `plan` agent allows it by default; other agents fall back to `ask`)

### invalid

Fallback for malformed tool calls. Source: `invalid.go`

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `error` | string | yes | JSON parse error |
| `original_name` | string | yes | Original tool name |
| `original_args` | string | no | Raw arguments |

**Never exposed to the LLM** — `ToolDefs()` explicitly filters out `name == "invalid"`. Always returns `IsError: true`.

**Permission:** `read`

---

## 8.4 Permission Categories

| Category | Tools |
|----------|-------|
| `read` | read, grep, glob, question, websearch, invalid, skill |
| `edit` | write, edit, apply_patch, todowrite |
| `shell` | bash, task, monitor |
| `webfetch` | webfetch |
| `plan_enter` | plan_enter |
| `plan_exit` | plan_exit |
| `destructive-shell` | (secondary check within bash/monitor for destructive commands) |

---

## 8.5 Skip Directories

Shared between grep and glob (`glob.go`):

```go
var skipDirs = map[string]bool{
    ".git": true, "node_modules": true, "vendor": true, ".tinycode": true,
}
```

---

## 8.6 Malformed Tool-Call Repair

Source: `internal/llm/openai.go`

When the LLM generates invalid JSON for tool call arguments:
1. Strip markdown fences (`` ```json ... ``` ``)
2. Remove trailing commas
3. Route to `invalid` tool if repair fails
