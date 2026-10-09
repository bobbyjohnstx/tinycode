# 7. Agents

Package: `internal/agent/`

Agents define behavioral presets: system prompts, tool permissions, and model parameters. Every session runs under an agent.

## 7.1 Agent Info

```go
type Info struct {
    Name        string             // unique identifier (registry key is filename stem; Name may differ via frontmatter)
    Description string             // human-readable description
    Mode        Mode               // "primary", "subagent", "all"
    Native      bool               // true for hardcoded agents
    Hidden      bool               // hidden from user lists
    Disabled    bool               // soft-disabled; Get/List skip these
    TopP        *float64           // LLM sampling parameter
    Temperature *float64           // LLM sampling parameter
    Color       string             // TUI display color (hex)
    Permission  permission.Ruleset // tool permission rules
    Prompt      string             // system prompt text
    Compact     bool               // true when compact variant is active
    Steps       *int               // max steps for this agent
    Options     map[string]any     // agent-specific options
    Model       *ModelRef          // preferred model override
    Variant     string             // model variant
}
```

### Agent Modes

| Mode | Constant | Description |
|------|----------|-------------|
| Primary | `ModePrimary` | Can be the main session agent |
| Subagent | `ModeSubagent` | Only available for delegation |
| All | `ModeAll` | Available in both roles |

## 7.2 Agent Registry

### Loading Order

1. **Native agents** — Hardcoded in `registerNativeAgents()` with specific permissions (prompts from `defaults/*.txt`, except `explore` which prefers `explore.md` body)
2. **Bundled agents** — Loaded from `defaults/*.md` via embedded filesystem (`embed.FS`); skipped when a native agent already owns the name
3. **User agents** — `LoadUserAgents`: `~/.config/tinycode/agents/*.md` then `~/.config/tinycode/agent/*.md` (compat)
4. **Project agents** — `.tinycode/agent/*.md` (loaded after user; overwrites non-native)
5. **Config overrides** — Applied from `config.Info.Agents` via `ApplyConfigOverrides()`

User/project `.md` agents may replace bundled (non-native) agents. Native agents are never overwritten by directory loads.

### Native Agents

| Agent | Mode | Description | Key Permissions |
|-------|------|-------------|-----------------|
| `build` | Primary | Default agent, full tool access | `question:allow`, `plan_enter:allow` |
| `plan` | Primary | Planning mode — interviews, researches, writes plans; edits scoped to `plans/*`/`drafts/*` | `read:allow`, `task:allow`, `plan_exit:allow`, `edit "plans/*":allow`, `edit "drafts/*":allow`, `edit:deny` elsewhere |
| `general` | Subagent | Plain assistant. Answers directly and uses a tool only when needed. On the default Tab cycle | `todowrite:deny` |
| `explore` | Subagent | Fast codebase search | Only: `grep`, `glob`, `bash`, `webfetch`, `websearch`, `read` |
| `scout` | Subagent | Upstream dependency source. Official docs go to document-specialist | Only: `grep`, `glob`, `webfetch`, `websearch`, `read` |
| `compaction` | Primary (hidden) | Context summarization | All denied |
| `title` | Primary (hidden) | Session title generation | All denied, temp=0.5 |
| `summary` | Primary (hidden) | Session summary | All denied |

### Bundled Agents (from `defaults/*.md`)

Loaded from embedded markdown files with YAML frontmatter. Actual set in Go `internal/agent/defaults/`:

| Agent | Description |
|-------|-------------|
| `analyst` | Requirements and acceptance criteria before planning |
| `architect` | Code design and architecture review (read-only). Known failures go to debugger |
| `code-reviewer` | Severity-rated code review with SOLID checks |
| `code-simplifier` | Refactoring for clarity (archived: disabled by default) |
| `critic` | Plan and gap review. Code defects go to code-reviewer |
| `debugger` | Root-cause analysis and bug fixing |
| `designer` | UI implementation with framework-aware visual design |
| `document-specialist` | External SDK docs, API references, changelogs, and integration guides |
| `executor` | Focused implementation of scoped tasks |
| `git-master` | Git history, rebasing, atomic commits |
| `ops` | Cluster and host administration. Primary. Read-only first; mutating shell commands ask |
| `qa-tester` | Interactive CLI testing (archived: disabled by default) |
| `scientist` | Data analysis and research (archived: disabled by default) |
| `security-reviewer` | Security vulnerability detection |
| `test-engineer` | Test strategy and TDD workflows |
| `tracer` | Competing-hypothesis causal tracing |
| `verifier` | Completion verification |
| `writer` | Technical documentation |

`explore.md` / `explore.compact.md` also live under defaults; the native `explore` agent uses the `explore.md` body as its prompt (permissions stay native). Compact peers of archived agents are disabled alongside the base agent.

The default Tab cycle is `build`, `general`, `ops`, `plan`, `architect`, `code-reviewer`. **ops** is primary. It allows read-only shell commands and asks for `destructive-shell` and `secret-shell`. It does not edit source. `/incident`, `/change`, and `/host` expand into checklists. An ambiguous cause goes to tracer, product docs to document-specialist, repo config to explore, source exposure to security-reviewer, file edits to executor, and application bugs to debugger.

The former standalone `planner` bundled agent was merged into the native `plan` agent (its methodology now lives in `defaults/plan.txt`). Switching between `build` and `plan` mid-session goes through the `plan_enter` / `plan_exit` tools (see [08-tools.md](08-tools.md)) rather than the `/ask` or Tab mechanisms alone — both tools require `permission.Ask` approval, and an approved call updates the session's active agent (`Processor.applyPendingAgentSwitch`), taking full effect (new system prompt, new permission set) starting with the next turn.

## 7.3 Frontmatter Schema

Agent `.md` files use YAML frontmatter. Shared parsing via `applyFrontmatter`:

```yaml
---
name: optional-display-name
description: Human-readable description
mode: primary | subagent | all   # omitted → subagent (bundled/user .md defaults)
hidden: true | false
color: "#ff0000"
steps: 10
temperature: 0.7
top_p: 0.9
model: ollama/qwen3:8b
permission:
  edit: deny
  read: allow
  bash:
    "*": allow
    "/etc/*": deny
---

System prompt content here...
```

The registry map key is always the filename stem. If `name` is set in frontmatter, it updates `Info.Name` only. Omitted `mode` defaults to `ModeSubagent` (not `ModeAll`).

### Permission Rules in Frontmatter

Permissions support two formats:

**Simple (applies to all patterns):**
```yaml
permission:
  edit: deny
  read: allow
```

**Pattern-specific (nested maps):**
```yaml
permission:
  bash:
    "*": allow
    "/etc/*": deny
```

## 7.4 Small-Model Variants

When a model's parameter count is ≤8B (detected from model name), the registry returns a **compact variant** if available and not disabled:

```go
func (r *Registry) Get(name string, modelSizeB *float64) *Info {
    if modelSizeB != nil && *modelSizeB <= 8 {
        compact := r.agents[name+".compact"]
        if compact != nil && !compact.Disabled {
            result := *compact
            result.Name = name       // requested base name, not "*.compact"
            result.Compact = true
            return &result
        }
    }
    // ... return standard agent (nil if missing or Disabled)
}
```

Compact variants are agent files named `<agent>.compact.md` with simplified prompts optimized for smaller models. `Get` never serves disabled agents (base or compact).

## 7.5 Permission Merging

Agent permissions are built by merging multiple rulesets:

```
DefaultRules → agentPerm → userPerms
```

For native agents, additional rules are inserted between default and user:
```
DefaultRules → nativeAgentRules → userPerms
```

For config overrides:
```
existingAgentPermission → configPermission
```

See [12-permissions.md](12-permissions.md) for the full evaluation chain.

## 7.6 Config Overrides

Users can modify agents via config:

```json
{
  "agents": {
    "build": {
      "temperature": 0.7,
      "model": "ollama/qwen3:8b"
    },
    "my-agent": {
      "prompt": "You are a custom agent...",
      "description": "Custom agent",
      "mode": "primary",
      "permission": {
        "edit": "allow",
        "bash": "ask"
      }
    },
    "debugger": {
      "disable": true
    }
  }
}
```

Override fields: `model`, `variant`, `prompt`, `description`, `temperature`, `top_p`, `mode`, `color`, `hidden`, `name`, `steps`, `options`, `permission`, `disable`.

Setting `disable: true` sets `Disabled` on the agent (soft disable). It does **not** remove the agent from the registry. Native agents cannot be disabled. `Get` and `List` skip disabled agents; `ListAll` includes them.

## 7.7 Default Agent Resolution

`DefaultAgent(configDefault)` resolves the default agent:

1. If `configDefault` is set in config, validate it exists and is a primary, visible, non-disabled agent
2. Otherwise, use `"build"` if not disabled
3. Fallback: first registered non-subagent, non-hidden, non-disabled agent

## 7.8 Agent Model Override

Agents can specify a preferred model:

```go
type ModelRef struct {
    ProviderID string
    ModelID    string
}
```

`ParseModel("ollama/qwen3:8b")` → `{ProviderID: "ollama", ModelID: "qwen3:8b"}`

When an agent has a `Model` set, the session manager uses that model instead of the session's default.

## 7.9 User Agent Loading

Source: `internal/agent/loader.go`

`LoadUserAgents(configDir, projectDir)` loads user-defined agents from:

1. **User config:** `~/.config/tinycode/agents/*.md`
2. **User compat:** `~/.config/tinycode/agent/*.md`
3. **Project:** `.tinycode/agent/*.md`

Files are parsed identically to bundled agents (frontmatter + body via `applyFrontmatter`). Non-native agents already in the registry **may be overwritten** (project wins over user over bundled). Native agents are never overwritten. Load errors are logged, not silently discarded.

Compact variants (`*.compact.md`) are loaded alongside their full counterparts.

## 7.10 `/ask` Command Integration

The `/ask <agent> <prompt>` command delegates a prompt to a specific agent:

1. `parseAskCommand()` extracts the agent name and prompt text
2. `isKnownAgent()` validates against the loaded agent list
3. Sent as `agent` field on `PromptInput` to the session handler
4. Prompt autocomplete filters `/ask` suggestions to non-primary agents only
