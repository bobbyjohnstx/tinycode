---
name: plan
description: Create a structured implementation plan with acceptance criteria, risk assessment, and ordered tasks
---

# Plan

Create a detailed implementation plan before writing code. Think through the approach, identify risks, and break work into ordered tasks with clear acceptance criteria.

## When to Use

Use this skill when:
- The user says "plan", "create a plan", "how should we implement this"
- Starting a non-trivial feature or refactor (3+ files, unclear scope)
- The approach has multiple viable strategies that need comparison
- The task involves coordination across multiple subsystems

## When Not to Use

- The fix is obvious and scoped to 1-2 files — just implement it
- The user wants architecture advice without an implementation plan — use an architect agent
- The user wants to debug or trace a problem — use `debug` or `trace`

## Workflow

1. **Understand the goal**: Restate what needs to be accomplished and why.
2. **Explore the codebase**: Identify existing patterns, relevant files, and dependencies.
3. **Identify approach options**: List 2-3 viable strategies with trade-offs.
4. **Select approach**: Recommend one with rationale.
5. **Break into tasks**: Create ordered, testable task list.
6. **Identify risks**: What could go wrong, what is uncertain.

## Rules
- Explore the codebase before proposing an approach — do not plan from assumptions.
- Each task must have concrete acceptance criteria that can be verified.
- Tasks must specify which files they modify.
- Order tasks so dependencies are satisfied (no task depends on a later task).

## Output Contract

### Goal
[1-2 sentence restatement of what and why]

### Approach
[Selected strategy with brief rationale]

#### Alternatives Considered
| Alternative | Why rejected |
|------------|-------------|
| ... | ... |

### Tasks
For each task:
- **Task N**: [description]
  - Files: [list of files to modify/create]
  - Acceptance criteria: [concrete, testable criteria]
  - Dependencies: [which prior tasks must complete first]

### Risks
| Risk | Likelihood | Mitigation |
|------|-----------|------------|
| ... | ... | ... |

### Estimated Scope
- Files modified: [count]
- New files: [count]
- Tests needed: [count]
