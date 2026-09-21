# Agent Orchestration Reference

How Claude Code decides when to execute directly vs spawn subagents, and patterns for tinycode-go's own multi-agent system.

## Decision Tree

| Situation | Approach | Why |
|-----------|----------|-----|
| Quick fix, tight feedback loop | Direct execution | No startup cost, output stays in context |
| Side investigation / research | Fresh subagent | Isolation, keeps main context clean |
| Implement from parent's plan | Fork | Inherits context, faster startup |
| Parallel review (3+ lenses) | Multiple agents | Independent perspectives |
| Repeatable multi-step pipeline | Workflow | Script holds the plan, not model judgment |

## Fork vs Fresh Agent

**Fork** — inherits full conversation as a point-in-time snapshot. Cheap (shares prompt cache). Use when the subtask needs what the parent already knows. Can't see subsequent parent changes; parent can't see fork's tool calls until completion.

**Fresh agent** — clean context window. Gets only: system prompt, delegation message, CLAUDE.md files, git status. Use for true isolation or independent perspective (reviews, audits). Slower startup but prevents context pollution.

## Key Rules

- **Don't peek** — Never read a fork's output mid-flight. Wait for the completion notification.
- **Don't race** — After launching, you know nothing about results. Never fabricate or predict. Say "still running" if asked.
- **Don't duplicate** — If an agent is working on files, don't touch those same files. Parallel agents on same files need worktree isolation.

## Writing Good Prompts

**For forks** (inherits context): short directive — "Implement the auth refactor. Run tests. Fix failures."

**For fresh agents** (no context): full briefing — explain what you're doing, what you've learned, what to look at, what output format. "Brief like a smart colleague who just walked into the room."

## Anti-Patterns

- Don't spawn an agent for a 2-minute task — overhead exceeds the work
- Don't write "based on your findings, fix the bug" — pushes synthesis onto the agent
- Don't flood context with many agents returning detailed results
- Don't use agents for iterative refinement — tight loops are better inline

## Concurrency & Depth

- Default 20 concurrent subagents max
- Default 3 layers of nesting depth
- Forks bypass concurrency limits but can't spawn their own subagents
- When limits hit, fail loudly — don't retry silently

## Applying to tinycode-go

Five levers to expose: **direct**, **fork** (inherit context), **fresh** (clean start), **parallel dispatch**, and **scripted workflows**.

The fundamental tradeoff: **isolation vs context sharing**.

- `/swarm` → parallel fresh agents (independence)
- `/chain` → sequential forks (shared context)
- `/work-loop` → single agent with iteration protocol

### Concurrency Model

- Configurable cap (default 5-10 parallel agents)
- Fail loudly when limits hit
- Depth limit enforced via per-subagent tool registry copy
- Per-session spawn budget prevents runaway cost

### Subagent Naming

Sequential labels: `executor-A`, `executor-B`, `explore-C`. Session IDs: `parentID:executor-A`. Readable in logs and UI.

### Output Handling

Subagents return summaries, not full transcripts. Keep detailed results in the subagent; return only what the parent needs for synthesis.
