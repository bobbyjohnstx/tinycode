# 10. Skills

Package: `internal/skill/`

Skills are reusable prompt templates that extend tinycode via slash commands. Each skill is a directory containing a `SKILL.md` file with frontmatter metadata and markdown body. Skills are discovered from user and project config directories, surfaced as slash commands in the TUI prompt, and executed via the `skill` tool.

## 10.1 Skill Schema

```go
type Skill struct {
    ID          string   // unique identifier (directory name, or frontmatter name override)
    Name        string   // display name (same as ID)
    Description string   // human-readable description for autocomplete
    Params      []string // positional parameter names
    Source      string   // "user" or "project"
}
```

## 10.2 Discovery

`skill.Discover(configDir, projectDir)` scans two locations in priority order:

| Priority | Path | Source | Description |
|----------|------|--------|-------------|
| 1 (highest) | `<configDir>/skills/*/SKILL.md` | `"user"` | User-global skills |
| 2 | `<projectDir>/.tinycode/skills/*/SKILL.md` | `"project"` | Project-local skills |

### Deduplication

Skills are deduplicated by name. User skills are discovered first, so a project skill with the same name is silently dropped. The `seen` map tracks names across both directories.

### Discovery Algorithm

1. Read entries in `<configDir>/skills/`, skip non-directories
2. For each directory, read `SKILL.md`; skip if missing or unreadable
3. Parse frontmatter via `frontmatter.Parse()`
4. Resolve name: use frontmatter `name` field if present and non-empty, otherwise use directory name
5. Skip if name already in `seen` map
6. Build `Skill` struct from frontmatter fields
7. Repeat steps 1-6 for `<projectDir>/.tinycode/skills/`

## 10.3 SKILL.md Format

Each skill directory must contain a `SKILL.md` file with YAML-like frontmatter and a markdown body:

```yaml
---
name: my-skill
description: One-line description shown in autocomplete
params: [param1, param2]
---

Skill prompt content here. Use $1, $2 for positional parameters
and $ARGUMENTS for the full argument string.
```

### Frontmatter Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | No | Overrides the directory name as the skill identifier |
| `description` | string | No | Displayed in autocomplete and skill listings |
| `params` | string | No | YAML-style list `[param1, param2]` of parameter names |

### Frontmatter Parser

Package: `internal/frontmatter/`

A simple key-value parser (not a full YAML parser):

- Frontmatter delimited by `---\n` and `\n---`
- Each line split on first `:` into key-value pair
- Values are always strings (the `params` field is parsed separately by `parseParamsList`)
- Returns `(map[string]any, body string)`

### Parameter List Parsing

`parseParamsList(s)` converts a YAML-style list string into a `[]string`:

1. Trim whitespace
2. Strip leading `[` and trailing `]`
3. Split on `,`
4. Trim whitespace from each element, discard empty strings

Example: `"[name, language]"` produces `["name", "language"]`.

## 10.4 Parameter Substitution

`substituteSkillParams(content, arguments)` replaces placeholders in the skill body:

| Placeholder | Replaced With | Example |
|-------------|---------------|---------|
| `$1`, `$2`, ... `$N` | Positional argument (space-split) | `"Alice"` for first arg |
| `$ARGUMENTS` | Full argument string | `"Alice Wonderland"` |

### Behavior

- Arguments are split on whitespace via `strings.Fields()`
- Positional placeholders beyond the provided argument count remain unreplaced
- `$ARGUMENTS` is always replaced (with empty string if no arguments provided)
- Substitution uses `strings.ReplaceAll` (all occurrences of each placeholder)

### Examples

```
Content: "Hello $1, welcome to $2!"
Arguments: "Alice Wonderland"
Result:  "Hello Alice, welcome to Wonderland!"

Content: "Name: $1\nAll: $ARGUMENTS"
Arguments: "alpha beta"
Result:  "Name: alpha\nAll: alpha beta"

Content: "$1 and $2 and $3"
Arguments: "a b"
Result:  "a and b and $3"      # $3 unreplaced
```

## 10.5 Skill Tool

Source: `internal/tool/skill.go`

The `skill` tool is registered conditionally when a config or project directory is available. It is invokable by the LLM during a session.

### Tool Definition

| Field | Value |
|-------|-------|
| ID | `skill` |
| Description | Execute a skill by name |
| Permission | `read` |

### Input Schema

```json
{
  "type": "object",
  "properties": {
    "name": {
      "type": "string",
      "description": "The name of the skill to execute"
    },
    "arguments": {
      "type": "string",
      "description": "Space-separated arguments to pass to the skill ($1, $2, etc.)"
    }
  },
  "required": ["name"]
}
```

### Execution Flow

1. Parse `name` and optional `arguments` from JSON input
2. Call `skill.Discover(configDir, projectDir)` to find all available skills
3. Match by `Name` or `ID`
4. If not found: return error listing all available skills with descriptions
5. Resolve file path based on `Source` (`"user"` or `"project"`)
6. Read `SKILL.md` content from disk
7. Apply `substituteSkillParams()` to replace `$1`, `$2`, `$ARGUMENTS`
8. Return substituted content as tool output

## 10.6 Command Integration

Package: `internal/command/`

Skills are surfaced as slash commands via `command.Discover()`, which merges four sources in priority order:

| Priority | Source | `Source` field | Description |
|----------|--------|----------------|-------------|
| 1 | Built-in commands | `"builtin"` | Hardcoded commands |
| 2 | Agent names | `"builtin"` | Each agent becomes a switchable command |
| 3 | User skills | `"skill"` | From `<configDir>/skills/*/SKILL.md` |
| 4 | Project skills | `"skill"` | From `<projectDir>/.tinycode/skills/*/SKILL.md` |

### Built-in Commands

| Command | Description | Subtask |
|---------|-------------|---------|
| `/init` | Guided project setup | No |
| `/review` | Review changes (`/review [commit\|branch\|pr]`) | Yes |
| `/ask` | Ask an agent (`/ask <agent> <prompt>`) | Yes |
| `/swarm` | Dispatch parallel subagents for multi-task work | Yes |

### Deduplication

The same `seen` map is used across all sources. Built-in commands and agent names are registered first, so a skill cannot shadow a built-in command or agent name.

### Command Schema

```go
type Command struct {
    Name        string   // slash command name (without /)
    Description string   // shown in autocomplete
    Source      string   // "builtin" or "skill"
    Template    string   // prompt template (built-in only)
    Subtask     bool     // runs as subtask (built-in only)
    Hints       []string // parameter hints for autocomplete
}
```

The merged command list is served via `GET /command` and powers autocomplete in the TUI prompt.

## 10.7 Built-in Skills

The following skills ship as user-installable templates (not compiled into the binary):

| Skill | Description |
|-------|-------------|
| `ai-slop-cleaner` | Clean AI-generated code with regression-safe deletion workflow |
| `configure-notifications` | Set up Telegram, Discord, or Slack notifications |
| `debug` | Isolate single most-likely root cause |
| `deepinit` | Generate per-directory AGENTS.md files |
| `mcp-setup` | Configure MCP servers via guided menu |
| `remember` | Triage findings to memory surfaces |
| `tc-doctor` | 14+ diagnostic checks (pure bash) |
| `trace` | Evidence-driven causal tracing |
| `verify` | Confirm changes work before claiming completion |

## 10.8 Directory Layout

```
~/.config/tinycode/           # configDir (macOS: ~/Library/Application Support/tinycode/)
  skills/
    debug/
      SKILL.md                # user skill
    verify/
      SKILL.md

<project>/
  .tinycode/
    skills/
      deploy/
        SKILL.md              # project skill
      lint-fix/
        SKILL.md
```

---

Prev: [09-plugins.md](09-plugins.md) | Next: [11-mcp.md](11-mcp.md)
