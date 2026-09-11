# 14. Storage

Package: `internal/storage/`

SQLite database for persisting projects, sessions, messages, events, and related entities. Uses `modernc.org/sqlite` (pure Go, no CGO dependency).

## 14.1 Database Handle

Source: `db.go`

```go
type DB struct {
    *sql.DB
    path string
}
```

`Open(dbPath)` creates the parent directory if needed, opens the database, applies pragmas, runs migrations, and returns a `*DB`. Supports `:memory:` for testing.

### Connection Pragmas

| Pragma | Value | Purpose |
|--------|-------|---------|
| `journal_mode` | WAL | Write-ahead logging for concurrent reads |
| `synchronous` | NORMAL | Reduced fsync frequency (safe with WAL) |
| `busy_timeout` | 5000 | Wait up to 5 seconds on lock contention |
| `cache_size` | -64000 | 64 MB page cache (negative = KB) |
| `foreign_keys` | ON | Enforce foreign key constraints |

For file-backed databases, pragmas are passed as DSN query parameters. For `:memory:` databases, pragmas are applied via individual `PRAGMA` statements after open (DSN parameters are not preserved across in-memory connections).

**MaxOpenConns = 1** -- serializes all writes through a single connection. Combined with WAL, this avoids `SQLITE_BUSY` errors while still allowing concurrent reads.

### Exported Errors

```go
var ErrNotFound = errors.New("not found")
```

## 14.2 Database Path Resolution

Source: `db.go:DefaultPath()`

Resolution order:

1. `TINYCODE_DB` environment variable -- supports `:memory:` and absolute paths; relative paths are joined with `dataDir()`
2. Fallback: `{dataDir}/tinycode.db`

`dataDir()` resolution:

1. `TINYCODE_DATA_DIR` environment variable
2. `XDG_DATA_HOME/tinycode` (if `XDG_DATA_HOME` is set)
3. `~/.local/share/tinycode`

## 14.3 Migration System

Source: `db.go:migrate()`

Migrations are `.sql` files embedded via `//go:embed migrations/*.sql` and tracked in a `_migrations` table:

```sql
CREATE TABLE IF NOT EXISTS _migrations (
    name       TEXT PRIMARY KEY,
    applied_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);
```

### Execution

1. Ensure `_migrations` table exists (`CREATE TABLE IF NOT EXISTS`)
2. Read all `.sql` files from the embedded filesystem
3. Sort filenames lexicographically (so `001_` runs before `002_`)
4. For each file, check `SELECT COUNT(*) FROM _migrations WHERE name = ?`
5. If not yet applied: execute the full SQL content, then `INSERT INTO _migrations`
6. All migrations use `CREATE TABLE IF NOT EXISTS` / `CREATE INDEX IF NOT EXISTS` for idempotency

Migrations are **not** wrapped in an explicit transaction -- each migration file executes as a batch. The `IF NOT EXISTS` guards provide crash-recovery safety.

## 14.4 Schema: Migration 001 (Initial)

Source: `migrations/001_initial.sql`

### project

Root entity. Represents a directory-based project.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | TEXT | PRIMARY KEY | Project identifier |
| `worktree` | TEXT | NOT NULL | Filesystem path to the project root |
| `vcs` | TEXT | | Version control system (e.g., "git") |
| `name` | TEXT | | Display name |
| `icon_url` | TEXT | | Project icon URL |
| `icon_url_override` | TEXT | | User-overridden icon URL |
| `icon_color` | TEXT | | Icon tint color |
| `time_created` | INTEGER | NOT NULL | Unix timestamp |
| `time_updated` | INTEGER | NOT NULL | Unix timestamp |
| `time_initialized` | INTEGER | | When project was first initialized |
| `sandboxes` | TEXT | NOT NULL, DEFAULT '[]' | JSON array of sandbox configs |
| `commands` | TEXT | | JSON command definitions |

### session

Conversation session, scoped to a project.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | TEXT | PRIMARY KEY | Session identifier (e.g., `ses_...`) |
| `project_id` | TEXT | NOT NULL, FK->project CASCADE | Owning project |
| `workspace_id` | TEXT | | Optional workspace grouping |
| `parent_id` | TEXT | | Parent session for sub-conversations |
| `slug` | TEXT | NOT NULL | URL-safe identifier |
| `directory` | TEXT | NOT NULL | Working directory for this session |
| `path` | TEXT | | Session path (routing) |
| `title` | TEXT | NOT NULL | Display title |
| `version` | TEXT | NOT NULL | Schema version |
| `share_url` | TEXT | | Public sharing URL |
| `summary_additions` | INTEGER | | Lines added (aggregate) |
| `summary_deletions` | INTEGER | | Lines deleted (aggregate) |
| `summary_files` | INTEGER | | Files changed (aggregate) |
| `summary_diffs` | TEXT | | JSON diff summaries |
| `cost` | REAL | NOT NULL, DEFAULT 0 | Accumulated API cost |
| `tokens_input` | INTEGER | NOT NULL, DEFAULT 0 | Prompt tokens used |
| `tokens_output` | INTEGER | NOT NULL, DEFAULT 0 | Completion tokens used |
| `tokens_reasoning` | INTEGER | NOT NULL, DEFAULT 0 | Reasoning/thinking tokens |
| `tokens_cache_read` | INTEGER | NOT NULL, DEFAULT 0 | Cache hit tokens |
| `tokens_cache_write` | INTEGER | NOT NULL, DEFAULT 0 | Cache write tokens |
| `revert` | TEXT | | Revert metadata |
| `permission` | TEXT | | Permission context |
| `agent` | TEXT | | Active agent name |
| `model` | TEXT | | Model identifier |
| `time_created` | INTEGER | NOT NULL | Unix timestamp |
| `time_updated` | INTEGER | NOT NULL | Unix timestamp |
| `time_compacting` | INTEGER | | When context compaction started |
| `time_archived` | INTEGER | | When session was archived |

**Indexes:** `project_id`, `workspace_id`, `parent_id`

### message

Individual messages within a session. The `data` column stores the full message payload as JSON.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | TEXT | PRIMARY KEY | Message identifier |
| `session_id` | TEXT | NOT NULL, FK->session CASCADE | Owning session |
| `time_created` | INTEGER | NOT NULL | Unix timestamp |
| `time_updated` | INTEGER | NOT NULL | Unix timestamp |
| `data` | TEXT | NOT NULL | JSON message payload |

**Indexes:** `(session_id, time_created, id)` composite -- optimizes chronological message retrieval within a session

### part

Sub-parts of a message (text blocks, tool calls, tool results, reasoning). The `data` column stores the part payload as JSON.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | TEXT | PRIMARY KEY | Part identifier |
| `message_id` | TEXT | NOT NULL, FK->message CASCADE | Parent message |
| `session_id` | TEXT | NOT NULL | Session (denormalized for query efficiency) |
| `time_created` | INTEGER | NOT NULL | Unix timestamp |
| `time_updated` | INTEGER | NOT NULL | Unix timestamp |
| `data` | TEXT | NOT NULL | JSON part payload |

**Indexes:** `(message_id, id)` composite, `session_id`

### todo

Task items tracked within a session.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `session_id` | TEXT | NOT NULL, FK->session CASCADE | Owning session |
| `content` | TEXT | NOT NULL | Task description |
| `status` | TEXT | NOT NULL | Status (e.g., "pending", "done") |
| `priority` | TEXT | NOT NULL | Priority level |
| `position` | INTEGER | NOT NULL | Display order |
| `time_created` | INTEGER | NOT NULL | Unix timestamp |
| `time_updated` | INTEGER | NOT NULL | Unix timestamp |

**Primary key:** `(session_id, position)` composite

**Indexes:** `session_id`

### permission

Persisted permission rulesets, one per project.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `project_id` | TEXT | PRIMARY KEY, FK->project CASCADE | Owning project |
| `time_created` | INTEGER | NOT NULL | Unix timestamp |
| `time_updated` | INTEGER | NOT NULL | Unix timestamp |
| `data` | TEXT | NOT NULL | JSON permission ruleset |

## 14.5 Schema: Migration 002 (Additional Tables)

Source: `migrations/002_missing_tables.sql`

### session_message

Structured message representation with explicit role and token tracking.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | TEXT | PRIMARY KEY | Message identifier |
| `session_id` | TEXT | NOT NULL, FK->session CASCADE | Owning session |
| `role` | TEXT | NOT NULL | Message role (user, assistant, system, tool) |
| `content` | TEXT | NOT NULL | Message content |
| `tool_call_id` | TEXT | | Tool call identifier (for tool results) |
| `tool_calls` | TEXT | | JSON array of tool call requests |
| `token_count` | INTEGER | NOT NULL, DEFAULT 0 | Token count for this message |
| `time_created` | INTEGER | NOT NULL | Unix timestamp |
| `time_updated` | INTEGER | NOT NULL | Unix timestamp |

**Indexes:** `session_id`, `(session_id, time_created)` composite

### workspace

Grouping of sessions within a project.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | TEXT | PRIMARY KEY | Workspace identifier |
| `project_id` | TEXT | NOT NULL, FK->project CASCADE | Owning project |
| `name` | TEXT | NOT NULL | Display name |
| `description` | TEXT | | Optional description |
| `directory` | TEXT | NOT NULL | Working directory |
| `time_created` | INTEGER | NOT NULL | Unix timestamp |
| `time_updated` | INTEGER | NOT NULL | Unix timestamp |

**Indexes:** `project_id`

### account

User account information for authenticated providers.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | TEXT | PRIMARY KEY | Account identifier |
| `provider` | TEXT | NOT NULL | Authentication provider |
| `email` | TEXT | | User email |
| `name` | TEXT | | Display name |
| `avatar_url` | TEXT | | Profile image URL |
| `token` | TEXT | | OAuth access token |
| `refresh_token` | TEXT | | OAuth refresh token |
| `token_expiry` | INTEGER | | Token expiration timestamp |
| `time_created` | INTEGER | NOT NULL | Unix timestamp |
| `time_updated` | INTEGER | NOT NULL | Unix timestamp |

### account_state

Tracks account lifecycle state.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `account_id` | TEXT | PRIMARY KEY, FK->account CASCADE | Owning account |
| `state` | TEXT | NOT NULL | Current state |
| `metadata` | TEXT | | JSON state metadata |
| `time_created` | INTEGER | NOT NULL | Unix timestamp |
| `time_updated` | INTEGER | NOT NULL | Unix timestamp |

### event_sequence

Groups events into ordered sequences per session.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | TEXT | PRIMARY KEY | Sequence identifier |
| `session_id` | TEXT | NOT NULL, FK->session CASCADE | Owning session |
| `seq` | INTEGER | NOT NULL, DEFAULT 0 | Current sequence counter |
| `time_created` | INTEGER | NOT NULL | Unix timestamp |
| `time_updated` | INTEGER | NOT NULL | Unix timestamp |

**Indexes:** `session_id`

### event

Individual events within a sequence.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | TEXT | PRIMARY KEY | Event identifier |
| `sequence_id` | TEXT | NOT NULL, FK->event_sequence CASCADE | Owning sequence |
| `session_id` | TEXT | NOT NULL | Session (denormalized) |
| `type` | TEXT | NOT NULL | Event type identifier |
| `data` | TEXT | NOT NULL | JSON event payload |
| `seq` | INTEGER | NOT NULL | Position within sequence |
| `time_created` | INTEGER | NOT NULL | Unix timestamp |

**Indexes:** `(sequence_id, seq)` composite, `session_id`

### data_migration

Tracks data-level migrations (distinct from schema migrations).

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | TEXT | PRIMARY KEY | Migration identifier |
| `name` | TEXT | NOT NULL | Migration name |
| `status` | TEXT | NOT NULL, DEFAULT 'pending' | Status: pending, running, completed, failed |
| `error` | TEXT | | Error message if failed |
| `time_started` | INTEGER | | When execution began |
| `time_completed` | INTEGER | | When execution finished |
| `time_created` | INTEGER | NOT NULL | Unix timestamp |

## 14.6 Cascade Delete Chains

All foreign keys use `ON DELETE CASCADE`. Deleting a parent row automatically removes all dependent rows:

```
project
  +-> session           (project_id)
  |     +-> message     (session_id)
  |     |     +-> part  (message_id)
  |     +-> session_message  (session_id)
  |     +-> todo        (session_id)
  |     +-> event_sequence   (session_id)
  |           +-> event      (sequence_id)
  +-> workspace         (project_id)
  +-> permission        (project_id)

account
  +-> account_state     (account_id)
```

Deleting a project cascades through all sessions, messages, parts, todos, events, workspaces, and permissions for that project.

## 14.7 Timestamps

All timestamp columns use `INTEGER` type storing Unix timestamps. The `_migrations.applied_at` column uses `strftime('%s', 'now')` (Unix seconds). Application-level timestamps may use milliseconds depending on the caller.

## 14.8 JSON Columns

Several columns store structured data as JSON text:

| Table | Column | Content |
|-------|--------|---------|
| `project` | `sandboxes` | Array of sandbox configurations |
| `project` | `commands` | Command definitions |
| `session` | `summary_diffs` | Diff summaries |
| `message` | `data` | Full message payload |
| `part` | `data` | Part payload (text, tool call, reasoning) |
| `permission` | `data` | Permission ruleset (see [12. Permissions](12-permissions.md)) |
| `session_message` | `tool_calls` | Tool call request array |
| `account_state` | `metadata` | State metadata |
| `event` | `data` | Event payload |

---

Prev: [13. Configuration](13-configuration.md) | Next: [15. Security](15-security.md)
