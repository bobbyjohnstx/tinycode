# 12. Permission System

Package: `internal/permission/`

The permission system controls whether tool executions proceed automatically, require user approval, or are blocked entirely. It uses a rule-based evaluation model with last-wins semantics, layered rulesets, and a blocking ask/reply flow for interactive approval.

## 12.1 Rules

### Action Enum

| Action | Constant | Behavior |
|--------|----------|----------|
| `"allow"` | `ActionAllow` | Tool executes without prompting |
| `"deny"` | `ActionDeny` | Tool blocked immediately, returns `DeniedError` |
| `"ask"` | `ActionAsk` | Blocks until user approves or rejects |

### Rule Structure

```go
type Rule struct {
    Permission string `json:"permission"`
    Pattern    string `json:"pattern"`
    Action     Action `json:"action"`
}

type Ruleset []Rule
```

- **Permission** -- the tool permission name (e.g., `"read"`, `"edit"`, `"shell"`)
- **Pattern** -- glob pattern matched against the tool's argument (e.g., file path, command)
- **Action** -- what happens when this rule matches

### Edit Tool Normalization

Tools `edit`, `write`, `apply_patch`, and `todowrite` all map to the `"edit"` permission when checking disabled status via `Disabled()`. This ensures a single `deny edit *` rule blocks all file-writing tools.

### Permission Aliases

`bash`↔`shell` and `list`↔`glob` are aliases. `Evaluate` and `Disabled` expand both sides so a rule written as `allow bash *` matches an Ask with permission `shell` (and vice versa).

## 12.2 Default Rules

```go
var DefaultRules = Ruleset{
    {Permission: "read",               Pattern: "*",     Action: ActionAllow},
    {Permission: "read",               Pattern: ".env*", Action: ActionAsk},
    {Permission: "webfetch",           Pattern: "*",     Action: ActionAsk},
    {Permission: "external_directory", Pattern: "*",     Action: ActionAsk},
    {Permission: "secret-shell",       Pattern: "*",     Action: ActionAsk},
}
```

DefaultRules are automatically prepended during evaluation. They establish the baseline: file reads are allowed, but `.env*` files, web fetches, external directory access, and shell commands that name a secret file all require user approval.

Doom-loop detection is a hard-stop in the session processor (`checkDoomLoop`), not a permission Ask. Destructive shell commands use the `destructive-shell` permission from the shell/monitor tools. Secret-file shell commands use `secret-shell`.

## 12.3 Evaluation

### Evaluation Chain

Rulesets are evaluated in this order, concatenated left to right:

```
DefaultRules -> baseRules (config) -> agentRuleset -> approved (always-approved)
```

**Last-wins semantics:** `Evaluate()` scans the concatenated list from end to start and returns the first matching rule. Later rulesets override earlier ones.

### Evaluate Function

```go
func Evaluate(permission, pattern string, rulesets ...Ruleset) Rule
```

1. Prepend `DefaultRules` to the supplied rulesets
2. Flatten all rulesets via `Merge()`
3. Walk from the last rule backward
4. Return the first rule where both `WildcardMatch(permission, rule.Permission)` and `WildcardMatch(pattern, rule.Pattern)` succeed
5. If no rule matches, return `Rule{Permission: permission, Pattern: "*", Action: ActionAsk}`

### Merge

```go
func Merge(rulesets ...Ruleset) Ruleset
```

Concatenates multiple rulesets into a single flat slice. Order is preserved -- earlier rulesets appear first, later rulesets last (and thus win on conflict).

### Disabled

```go
func Disabled(tools []string, ruleset Ruleset) map[string]bool
```

Returns the set of tool names that are globally denied (pattern `"*"`, action `"deny"`). For tools in the `editTools` map (`edit`, `write`, `apply_patch`, `todowrite`), the permission is normalized to `"edit"` before checking.

## 12.4 Wildcard Matching

Package: `internal/permission/wildcard.go`

```go
func WildcardMatch(input, pattern string) bool
```

Glob-style matching via regex conversion:

| Glob | Regex | Matches |
|------|-------|---------|
| `*` | `.*` | Any sequence of characters |
| `?` | `.` | Any single character |

### Behavior

1. Path separators normalized: `\` replaced with `/` in both input and pattern
2. Pattern is `regexp.QuoteMeta`-escaped, then `*` and `?` placeholders are restored
3. Trailing ` *` in the pattern is made optional: `"perm *"` becomes `"perm( .*)?$"`, matching both `"perm"` and `"perm arg"`
4. Match is anchored: `^pattern$`
5. Case-insensitive on Windows (`(?si)` flags), case-sensitive elsewhere (`(?s)` flags)

## 12.5 Permission Service

### Error Types

| Error | Sentinel | Description |
|-------|----------|-------------|
| `ErrClosed` | `"permission service closed"` | Service shut down, no new requests accepted |
| `ErrDenied` | `"permission denied by rule"` | A rule explicitly denied the request |
| `ErrRejected` | `"user rejected permission"` | User chose to reject the request |
| `ErrNotFound` | `"permission request not found"` | Reply references an unknown request ID |

### Typed Errors

| Type | Carries | Description |
|------|---------|-------------|
| `CorrectedError` | `Feedback string` | User rejected with corrective feedback message |
| `DeniedError` | `Ruleset` | Denied by rule, includes the matching ruleset |

### Reply Types

| Reply | Constant | Behavior |
|-------|----------|----------|
| `"once"` | `ReplyOnce` | Allow this single request |
| `"always"` | `ReplyAlways` | Allow and persist rules for future requests |
| `"reject"` | `ReplyReject` | Deny this request and cascade-reject same-session pending asks |

### Request

```go
type Request struct {
    ID         string         `json:"id"`
    SessionID  string         `json:"sessionID"`
    Permission string         `json:"permission"`
    Patterns   []string       `json:"patterns"`
    Metadata   map[string]any `json:"metadata"`
    Always     []string       `json:"always"`
    Tool       *ToolRef       `json:"tool,omitempty"`
}

type ToolRef struct {
    MessageID string `json:"messageID"`
    CallID    string `json:"callID"`
}
```

- **Always** -- patterns to add to the approved ruleset if the user replies `"always"`
- **Tool** -- optional back-reference to the originating tool call

### AskInput

```go
type AskInput struct {
    ID         string
    SessionID  string
    Permission string
    Patterns   []string
    Metadata   map[string]any
    Always     []string
    Tool       *ToolRef
    Ruleset    Ruleset
}
```

`AskInput.Ruleset` carries agent-level permission rules that apply to this specific request.

### Service

```go
type Service struct {
    bus       *bus.Bus
    pending   map[string]*pendingEntry
    baseRules Ruleset
    approved  Ruleset
    closed    bool
    store     RuleStore
    projectID string
    mu        sync.Mutex
}
```

## 12.6 Ask Flow

`Service.Ask(ctx, input)` is the main entry point for permission checks.

```
1. Lock mutex
2. If service is closed -> return ErrClosed
3. If Patterns is empty -> unlock, return ErrDenied (fail closed)
4. For each pattern in input.Patterns:
   a. Evaluate(permission, pattern, baseRules, input.Ruleset, approved)
   b. If any rule -> deny: unlock, return DeniedError (with matching rules)
   c. If any rule -> ask: mark needsAsk = true
5. If all rules -> allow: unlock, return nil
6. Generate request ID (ascending sortable, "permission" prefix)
7. Create pending entry with buffered reply channel (cap 1)
8. Store in pending map
9. Unlock mutex
10. Publish "permission.asked" event on bus
11. Select:
    - Reply received on channel -> return result.err
    - Context cancelled -> clean up pending entry, return ctx.Err()
```

The ask is a blocking call -- the goroutine waits on a channel until either the user replies or the context is cancelled.

Tool execution passes path or command strings as Patterns (via `askPatterns` in `internal/tool`), not just the tool name, so rules like `read .env*` match correctly.

## 12.7 Reply Flow

`Service.RespondToAsk(input)` handles user replies.

### Reject

1. Remove request from pending map
2. Publish `permission.replied` event
3. Send error to reply channel:
   - `CorrectedError` if `input.Message` is non-empty (user provided feedback)
   - `ErrRejected` otherwise
4. **Cascade:** reject all other pending asks for the same session (each gets `ErrRejected` and a `permission.replied` event)

### Once

1. Remove request from pending map
2. Publish `permission.replied` event
3. Send `nil` to reply channel (allow this request)

### Always

1. Remove request from pending map
2. Publish `permission.replied` event
3. Send `nil` to reply channel (allow this request)
4. Add rules to `approved` list: one `Rule{Permission, Pattern, ActionAllow}` for each pattern in `Request.Always`
5. Persist via `RuleStore.SaveRules()` if store is configured
6. **Auto-resolve:** scan remaining pending asks for the same session; if all patterns now evaluate to allow (against the updated `approved` list), resolve them automatically with `ReplyAlways`

## 12.8 Persistence

### RuleStore Interface

```go
type RuleStore interface {
    SaveRules(projectID string, rules Ruleset) error
    LoadRules(projectID string) (Ruleset, error)
}
```

### Lifecycle

- `SetStore(store, projectID)` configures persistence and loads any previously saved rules into the `approved` list
- "Always" approvals are saved on each `ReplyAlways` response when a store is configured
- Rules are scoped to a project (identified by `projectID`)
- Persisted rules survive process restarts

### SQLite persistence

`permission.SQLiteRuleStore` persists the `approved` ruleset in the `permission` table (`project_id`, `data` JSON). `initTooling` wires `SetStore` when a database is open (TUI / serve / web / ACP / run), after ensuring the project row exists. `"always"` approvals then survive restarts for that project.

If no store is configured (tests or callers that omit a DB), `"always"` approvals stay in-memory for the process lifetime only. `RespondToAsk` logs a warning when persisting without a store so this is not silent.

## 12.9 Config Integration

```go
func FromConfig(allow, deny []string) Ruleset
```

Converts config file permission entries into a `Ruleset`.

### Pattern Format

```
"permission pattern"   ->  Rule{Permission: "permission", Pattern: "pattern", Action: allow/deny}
"permission"           ->  Rule{Permission: "permission", Pattern: "*",       Action: allow/deny}
```

Space-separated: the first token is the permission name, the rest is the glob pattern. If no space is present, the pattern defaults to `"*"`.

### Path Expansion

Patterns support home directory expansion:

| Input | Expanded |
|-------|----------|
| `~/path` | `/home/user/path` |
| `~` | `/home/user` |
| `$HOME/path` | `/home/user/path` |
| `$HOME` | `/home/user` |

### Config Rules as Base Rules

`FromConfig()` output is passed to `Service.SetBaseRules()`, placing config rules between DefaultRules and agent/approved rulesets in the evaluation chain.

## 12.10 Bus Events

| Event | Publisher | Payload | Description |
|-------|-----------|---------|-------------|
| `permission.asked` | `Service.Ask()` | `Request` struct | Permission request created, waiting for reply |
| `permission.replied` | `Service.RespondToAsk()` | `{sessionID, requestID, reply}` | Permission request answered |

## 12.11 Service Lifecycle

### Construction

```go
func NewService(b *bus.Bus) *Service
```

Creates a new service with an empty pending map and no rules beyond `DefaultRules`.

### Shutdown

`Service.Close()` sets `closed = true` and rejects all pending requests with `ErrRejected`. After close, `Ask()` returns `ErrClosed` immediately.

### Listing Pending Requests

`Service.List()` returns a snapshot of all currently pending permission requests.

---

Prev: [11-mcp.md](11-mcp.md) | Next: [13-configuration.md](13-configuration.md)
