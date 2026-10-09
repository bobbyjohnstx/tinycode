# Authoring agents and skills

Agents and skills are both markdown, and they do different jobs.

An **agent** is a specialist with its own prompt and permissions. Reach it with `/ask <agent>` or the task tool. A **skill** is a procedure the current agent follows in this conversation. Reach it by typing `/<skill>`.

Do not ship the same job as both. If the work needs its own permissions or a compact prompt, make an agent. If it is a checklist for whoever is already active, make a skill.

## Agents

Put a personal agent in `~/.config/tinycode/agent/<name>.md`. Put a project agent in `.tinycode/agent/<name>.md`. The project file wins. Neither file can replace the native agents `build`, `plan`, `general`, `explore`, `scout`, `compaction`, `title`, or `summary`.

A file named `<name>.compact.md` next to it is used when the model is 8B parameters or smaller. Give the compact file the same `mode`, `steps`, and `permission` block as the full file. Shorten the prose. Do not change what the agent is allowed to do.

```markdown
---
name: example
description: One line shown in the agent list
mode: subagent
steps: 8
permission:
  edit: deny
  bash: deny
  read: allow
  glob: allow
  grep: allow
---

## Role

You are Example. Say the mission in one sentence.
You are responsible for the one job this agent exists to do.
You are not responsible for the neighboring jobs (name the agent that owns each one).
You are READ-ONLY: never use Write or Edit tools.

## Why this matters

One short paragraph on the failure this agent prevents.

## Success criteria

- A result a caller can check without re-reading the whole transcript

## Constraints

- The permission block above is the real limit. The prose must match it.
- Name a stop condition so the agent does not keep searching.

## Investigation protocol

1. The first thing to read or search.
2. The decision the agent must make.
3. The handoff when the job belongs to someone else.
```

`mode` is `subagent` or `primary`. Omitting it means `subagent`. `steps` is the iteration cap when this agent is spawned with the task tool. `permission` values are `allow`, `ask`, or `deny`. A missing rule asks. `bash` and `shell` are the same permission. `read` also covers grep, glob, and web search. `webfetch` is separate. `edit` covers write.

Use `steps: 8` for a read-only agent and `steps: 12` for an agent that edits or runs shell commands that change the tree.

Only name handoffs that exist and are enabled. `code-simplifier`, `qa-tester`, and `scientist` ship disabled.

To ship an agent inside the tinycode binary, add `internal/agent/defaults/<name>.md` and `<name>.compact.md`. `LoadDefaults` embeds every `defaults/*.md`.

## Skills

A skill is a directory with one `SKILL.md` file:

- `~/.config/tinycode/skills/<name>/SKILL.md`
- `.tinycode/skills/<name>/SKILL.md`

A personal or project skill with the same name replaces a bundled skill. `skills.paths` entries are scanned the same way (`*/SKILL.md`). `skills.urls` is stored and not fetched.

```markdown
---
name: example
description: One line shown in slash-command autocomplete
params: [target]
---

# Example

## When to use

- The user says "example" or the task matches this procedure

## When not to use

- The job belongs to an agent. Name that agent.

## Workflow

1. First check.
2. Second check. The first argument is $1. The whole argument string is $ARGUMENTS.

## Rules

- Stay inside this procedure. Do not switch jobs halfway through.

## Output contract

- What the reply must contain
```

`$1`, `$2`, and `$ARGUMENTS` are the only placeholders. The frontmatter is removed before the model sees the body.

Bundled skills live as single files in `internal/skill/defaults/<name>.md`. The seven that ship are `doctor`, `mcp-setup`, `remember`, `deepinit`, `incident`, `change`, and `host`. Skills have no compact file. Keep the body short so it still fits when the current agent is already the compact prompt.

`/debug`, `/trace`, `/plan`, `/verify`, `/test`, and `/review` are not skills. Each one tells the current agent to call the task tool with `debugger`, `tracer`, `plan`, `verifier`, `test-engineer`, or `code-reviewer`.
