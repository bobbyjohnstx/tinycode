# 7. Agents

Package: `internal/agent/`

Agents define behavioral presets: system prompts, tool permissions, and model parameters. Every session runs under an agent.

## 7.1 Agent Info

```go
type Info struct {
    Name        string             // unique identifier
    Description string             // human-readable description
    Mode        Mode               // "primary", "subagent", "all"
    Native      bool               // true for hardcoded agents
    Hidden      bool               // hidden from user lists
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

1. **Native agents** — Hardcoded in `registerNativeAgents()` with specific permissions
2. **Bundled agents** — Loaded from `defaults/*.md` via embedded filesystem (`embed.FS`)
3. **Config overrides** — Applied from `config.Info.Agents` via `ApplyConfigOverrides()`

### Native Agents

| Agent | Mode | Description | Key Permissions |
|-------|------|-------------|-----------------|
| `build` | Primary | Default agent, full tool access | `question:allow`, `plan_enter:allow` |
| `plan` | Primary | Read-only plan mode | `plan_exit:allow`, `edit:deny` |
| `general` | Subagent | Multi-step task execution | `todowrite:deny` |
| `explore` | Subagent | Fast codebase search | Only: `grep`, `glob`, `bash`, `webfetch`, `websearch`, `read` |
| `scout` | Subagent | External research | Only: `grep`, `glob`, `webfetch`, `websearch`, `read`, `repo_clone`, `repo_overview` |
| `compaction` | Primary (hidden) | Context summarization | All denied |
| `title` | Primary (hidden) | Session title generation | All denied, temp=0.5 |
| `summary` | Primary (hidden) | Session summary | All denied |

### Bundled Agents (from `defaults/*.md`)

Loaded from embedded markdown files with YAML frontmatter:

| Agent | Description |
|-------|-------------|
| `architect` | Code design and architecture review (read-only) |
| `code-reviewer` | Severity-rated code review with SOLID checks |
| `code-simplifier` | Refactoring for clarity and maintainability |
| `critic` | Multi-perspective quality review |
| `debugger` | Root-cause analysis and bug fixing |
| `designer` | Production-grade UI/UX |
| `document-specialist` | External library and API reference |
| `executor` | Focused implementation of scoped tasks |
| `git-master` | Git history, rebasing, atomic commits |
| `planner` | Strategic planning and work breakdown |
| `qa-tester` | Interactive CLI testing |
| `scientist` | Data analysis and research |
| `security-reviewer` | Security vulnerability detection |
| `test-engineer` | Test strategy and TDD workflows |
| `tracer` | Evidence-driven causal tracing |
| `verifier` | Completion verification |
| `workspace` | Development environment setup |
| `writer` | Technical documentation |
| `analyst` | Requirements analysis |
| `cluster-admin` | Kubernetes/OpenShift operations |

## 7.3 Frontmatter Schema

Agent `.md` files use YAML frontmatter:

```yaml
---
description: Human-readable description
mode: primary | subagent | all
hidden: true | false
color: "#ff0000"
steps: 10
permission:
  edit: deny
  read: allow
  bash:
    "*": allow
    "/etc/*": deny
---

System prompt content here...
```

### Permission Rules in Frontmatter

Permissions support two formats:

**Simple (applies to all patterns):**
```yaml
permission:
  edit: deny
  read: allow
```

**Pattern-specific:**
```yaml
permission:
  bash:
    "*": allow
    "/etc/*": deny
```

## 7.4 Small-Model Variants

When a model's parameter count is ≤8B (detected from model name), the registry returns a **compact variant** if available:

```go
func (r *Registry) Get(name string, modelSizeB *float64) *Info {
    if modelSizeB != nil && *modelSizeB <= 8 {
        compact := r.agents[name+".compact"]
        if compact != nil {
            result := *compact
            result.Compact = true
            return &result
        }
    }
    // ... return standard agent
}
```

Compact variants are agent files named `<agent>.compact.md` with simplified prompts optimized for smaller models.

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

Setting `disable: true` removes the agent from the registry.

## 7.7 Default Agent Resolution

`DefaultAgent(configDefault)` resolves the default agent:

1. If `configDefault` is set in config, validate it exists and is a primary, visible agent
2. Otherwise, use `"build"`
3. Fallback: first registered non-subagent, non-hidden agent

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

`LoadUserAgents(configDir, projectDir)` loads user-defined agents from two directories:

1. **User config:** `~/.config/tinycode/agents/*.md`
2. **Project:** `.tinycode/agent/*.md`

Files are parsed identically to bundled agents (frontmatter + body). Agents already in the registry are **not** overwritten — bundled and native agents take precedence.

Compact variants (`*.compact.md`) are loaded alongside their full counterparts.

## 7.10 `/ask` Command Integration

The `/ask <agent> <prompt>` command delegates a prompt to a specific agent:

1. `parseAskCommand()` extracts the agent name and prompt text
2. `isKnownAgent()` validates against the loaded agent list
3. Sent as `agent` field on `PromptInput` to the session handler
4. Prompt autocomplete filters `/ask` suggestions to non-primary agents only
