# Agent Prompt Tier Selection

## Context

Small models (<=9B params) can't follow 900-word agent prompts reliably. System prompt tiers already exist (`modelSizeB()` in `system.ts` selects `local-small/medium/large.txt`). Agent prompts need the same treatment: a `default` tier (~900 words) and a `compact` tier (~300 words), selected automatically based on model size.

## Work Objectives

- Automatically use compact agent prompts for small models (<=9B params)
- Fall back to default when no compact variant exists
- Keep the agent list identical regardless of active model

## Guardrails

**Must Have:** Transparent tier selection; compact frontmatter (steps, permissions) applied when compact tier active; works for both native and custom agents.
**Must NOT Have:** Separate agent list entries for compact variants; changes to the UI layer; new config options for tier threshold.

## Task Flow

### 1. Filter compact variants during agent config loading

**File:** `packages/tinycode/src/config/agent.ts` `load()` (lines 106-130)

Currently `configEntryNameFromPath` turns `debugger.compact.md` into name `debugger.compact`, which creates a separate agent entry. Change `load()` to:

- After deriving the name, check if it ends with `.compact`
- If so, strip the `.compact` suffix and store in a separate `compact` map (keyed by base name)
- Return `{ agents: Record<string, Info>, compact: Record<string, Info> }` instead of flat `Record<string, Info>`

**Acceptance:** `load()` on a directory containing `debugger.md` and `debugger.compact.md` returns two maps: `agents` has one entry `debugger`, `compact` has one entry `debugger` with the compact prompt/frontmatter.

### 2. Merge compact variants into Agent.Info

**File:** `packages/tinycode/src/agent/agent.ts` state initialization (lines 281-308)

After merging custom agent configs into the `agents` registry:

- Iterate the `compact` map from step 1
- For each compact entry, attach it to the matching agent as a new optional field `compact?: { prompt?: string; permission?: Permission.Ruleset; steps?: number }`
- If no matching base agent exists, log a warning and skip

Also add `compact` as an optional field to the `Info` schema (line 30).

**Acceptance:** `Agent.list()` returns the same agents as before. An agent with a `.compact.md` file has a populated `compact` field. An agent without one has `compact: undefined`.

### 3. Add tier-aware prompt selection in request preparation

**File:** `packages/tinycode/src/session/llm/request.ts` `prepare()` (line 58)

Import `modelSizeB` from `system.ts` (export it if not already exported). In `prepare()`:

- Call `modelSizeB(input.model)` to get the parameter count
- If size is defined and <=9B, and `input.agent.compact?.prompt` exists, use the compact prompt
- If compact tier is active, also apply `compact.steps` and `compact.permission` (merge permissions, override steps)
- Otherwise fall back to `input.agent.prompt` (existing behavior)

Replace line 58:

```
...(input.agent.prompt ? [input.agent.prompt] : SystemPrompt.provider(input.model)),
```

with a helper that checks compact tier first.

**Acceptance:** Given model `ollama/qwen3:8b` and an agent with a compact variant, `prepare()` uses the compact prompt. Given `ollama/qwen3:14b`, it uses the default prompt. Given a cloud model, it uses the default prompt.

### 4. Export `modelSizeB` and add unit tests

**Files:** `packages/tinycode/src/session/system.ts`, new test file

- Export `modelSizeB` from `system.ts` so `request.ts` can import it
- Write tests covering: compact selection for <=9B, default selection for >=10B, fallback when no compact exists, cloud model always gets default, partial compact (only prompt, no permissions) inherits default permissions

**Acceptance:** All new tests pass. Existing tests remain green. `modelSizeB` is importable from `system.ts`.

## Success Criteria

- Running with `ollama/qwen3:8b` and an agent that has `agent.compact.md` uses the compact prompt
- Running with `ollama/qwen3:14b` uses the default prompt
- Agent list in UI/TUI shows identical entries regardless of model
- Agents without compact variants work unchanged
