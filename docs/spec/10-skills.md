# 10. Skills

Package: `internal/skill/`

Skills are reusable prompt templates that extend tinycode via slash commands. Each skill is a directory containing a `SKILL.md` file with frontmatter metadata and markdown body (user/project/path skills), or an embedded markdown file (bundled builtins). Skills are discovered from config, project, and optional `skills.paths` directories, surfaced as slash commands in the TUI, expanded into the prompt on `/skill-name` (and palette select), and executed via the `skill` tool.

Sources: `discovery.go` (filesystem discovery), `embed.go` (bundled defaults), `content.go` (load + param substitution). There is no `loader.go`.

## 10.1 Skill Schema

```go
type Skill struct {
    ID          string   // match key (frontmatter name, or directory / embed basename)
    Name        string   // display / match name (same as ID for discovered skills)
    Description string   // human-readable description for autocomplete
    Params      []string // positional parameter names
    Source      string   // "user", "project", "path", or "builtin"
    Dir         string   // absolute/relative skills/<dirname> for disk skills; empty for builtin
}
```

Matching is by `Name` or `ID`. Content is read from `Dir/SKILL.md` when `Dir` is set; builtins use `ReadDefaultSkill(ID)` from the embed FS.

## 10.2 Discovery

`skill.Discover(configDir, projectDir)` and `skill.DiscoverWithPaths(configDir, projectDir, extraPaths)` scan locations in priority order (first seen wins):

| Priority | Path | Source | Description |
|----------|------|--------|-------------|
| 1 (highest) | `<configDir>/skills/*/SKILL.md` | `"user"` | User-global skills |
| 2 | `<projectDir>/.tinycode/skills/*/SKILL.md` | `"project"` | Project-local skills |
| 3 | each entry in `skills.paths` → `*/SKILL.md` | `"path"` | Extra directories from config |
| 4 (lowest) | embedded `defaults/*.md` | `"builtin"` | Bundled defaults |

`skills.urls` is parsed in config but **not** fetched at runtime (not implemented).

### Deduplication

Skills are deduplicated by name. Earlier sources win. The `seen` map tracks names across all directories and builtins.

### Discovery Algorithm

1. Read entries in `<configDir>/skills/`, skip non-directories
2. For each directory, read `SKILL.md`; skip if missing or unreadable
3. Parse frontmatter via `frontmatter.Parse()`
4. Resolve name: use frontmatter `name` if present and non-empty, else directory name
5. Skip if name already in `seen`
6. Build `Skill` with `Dir` set to the skill directory path
7. Repeat for project skills, then each `skills.paths` entry
8. Append embedded defaults with empty `Dir`

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

## 10.4 Content Load and Parameter Substitution

`skill.LoadContent(s)` reads the skill file (or embed), strips frontmatter via `frontmatter.Parse`, and returns the markdown body.

`skill.SubstituteParams(content, arguments)` replaces placeholders:

| Placeholder | Replaced With | Example |
|-------------|---------------|---------|
| `$1` … `$N` | Positional argument (space-split), **highest index first** | `"Alice"` for first arg |
| `$ARGUMENTS` | Full argument string | `"Alice Wonderland"` |

### Behavior

- Arguments are split on whitespace via `strings.Fields()`
- Positional placeholders are replaced from highest `$N` down to `$1` so `$10` is not corrupted by `$1`
- Positional placeholders beyond the provided argument count remain unreplaced
- `$ARGUMENTS` is always replaced (with empty string if no arguments provided)

### Examples

```
Content: "Hello $1, welcome to $2!"
Arguments: "Alice Wonderland"
Result:  "Hello Alice, welcome to Wonderland!"

Content: "got $10 and $1"
Arguments: "A B C D E F G H I J"
Result:  "got J and A"
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

### Execution Flow

1. Parse `name` and optional `arguments` from JSON input
2. Call `skill.DiscoverWithPaths(configDir, projectDir, skillPaths)`
3. Match by `Name` or `ID`
4. If not found: return error listing all available skills with descriptions
5. `LoadContent` (Dir/SKILL.md or embed; frontmatter stripped)
6. `SubstituteParams` for `$1`…`$N` and `$ARGUMENTS`
7. Return substituted body as tool output

## 10.6 Slash Expansion and Command Integration

### Slash expansion

`command.ExpandSlashCommand(text, configDir, projectDir, skillPaths...)` (source: `internal/command/expand.go`):

1. Handles `/swarm` and `/work-loop` instruction prefixes
2. Otherwise, if the text is `/name …` and `name` matches a discovered skill, loads the skill body via `LoadContent`, substitutes remaining args, and returns that text for the LLM (`DisplayText` stays the original slash line)
3. Wired from `session_ops` so user messages expand before the model runs

Palette skill selection (TUI `PaletteSelectedMsg`) uses the same expand path and inserts the expanded body into the prompt (not a frecency-only no-op).

### Command discovery

Skills are surfaced as slash commands via `command.Discover` / `DiscoverWithPaths`, which merges:

| Priority | Source | `Source` field | Description |
|----------|--------|----------------|-------------|
| 1 | Built-in commands | `"builtin"` | Hardcoded commands |
| 2 | Agent names | `"builtin"` | Each agent becomes a switchable command |
| 3 | User / project / `skills.paths` skills | `"skill"` | Disk skills |
| 4 | Bundled default skills | `"skill"` | From embed |

Built-in commands and agent names are registered first, so a skill cannot shadow a built-in command or agent name.

### `/debug` vs `/diagnostics`

| Command | Kind | Behavior |
|---------|------|----------|
| `/diagnostics` | Client TUI command | Opens the diagnostics dialog (config, providers, system info) |
| `/debug` | Built-in alias | Tells the current agent to delegate to the debugger agent |

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

Bundled as embedded markdown under `internal/skill/defaults/` (`embed.go`):

| Skill | Description |
|-------|-------------|
| `remember` | Triage findings to memory surfaces |
| `deepinit` | Generate per-directory AGENTS.md files |
| `doctor` | Project / environment health checks |
| `mcp-setup` | Configure MCP servers via guided menu |
| `incident` | Triage a live system failure |
| `change` | Plan one cluster or host change |
| `host` | Inspect a machine, local or over ssh |

`/debug`, `/trace`, `/plan`, `/verify`, `/test`, and `/review` are built-in commands. Each expands to a short instruction to call the task tool with `debugger`, `tracer`, `plan`, `verifier`, `test-engineer`, or `code-reviewer`. A user or project skill of the same name is loaded instead. The copy-paste format is [authoring.md](../authoring.md).

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

Config:

```json
{
  "skills": {
    "paths": ["/shared/skills"],
    "urls": []
  }
}
```

`paths` are scanned for `*/SKILL.md`. `urls` are accepted in config but not downloaded.

## 10.9 HTTP `GET /skill`

Go returns `[]Skill` JSON: `id`, `name`, `description`, `params`, `source`, and optional `dir`.

The TypeScript OpenAPI (`packages/sdk/openapi.json`) historically required `name`, `location`, and `content` per item. That shape is **not** what the Go server returns. Treat the Go struct as authoritative; see [02-api-routes.md](02-api-routes.md) §2.23 and [16-not-implemented.md](16-not-implemented.md) §16.29.

---

Prev: [09-plugins.md](09-plugins.md) | Next: [11-mcp.md](11-mcp.md)
