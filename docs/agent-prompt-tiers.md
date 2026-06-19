# Agent Prompt Tiers

Agent prompts are sized to match model context budgets. Two tiers: `default` and `compact`.

## Tier Selection

Reuses the existing model size extraction from `src/session/system.ts`:

| Tier | Model size | Context range | Agent budget |
|------|-----------|---------------|-------------|
| compact | ≤9B params | 4K–8K tokens | ~400 tokens (~300 words) |
| default | ≥10B params | 32K+ tokens | ~1300 tokens (~900 words) |

The threshold is **parameter count**, not context window, because instruction-following ability correlates with model size more than context length. A 9B model with 128K context still can't follow a 900-word agent prompt reliably.

Selection function: `modelSizeB(model)` in `src/session/system.ts` extracts parameter count from model ID strings (e.g. `qwen3-14b` → 14, `llama3.1:8b` → 8). Unknown size falls back to `default`.

## File Resolution

Convention-based with fallback:

```
agents/debugger.md          ← default (used for ≥10B)
agents/debugger.compact.md  ← compact (used for ≤9B)
```

If no `.compact.md` variant exists, the default is used for all sizes. This allows gradual adoption — create compact variants only for agents where the default is too large.

## Token Budget Breakdown (≤9B model, 4K context)

| Component | Tokens | Notes |
|-----------|--------|-------|
| System prompt (`local-small.txt`) | ~350 | Fixed overhead |
| Agent prompt | ~400 | **This is the compact budget** |
| Project instructions (CLAUDE.md) | ~200 | Varies by project |
| Tool schemas | ~500 | Depends on enabled tools |
| Conversation history | ~2500 | What's left for actual work |

Exceeding the agent budget compresses conversation history, which degrades output quality more than a shorter agent prompt would.

## What to Keep in Compact

These sections carry the most value per token for small models:

1. **Role** (2-3 sentences) — Who you are, what you're responsible for, what you're NOT responsible for. Small models drift without role boundaries.
2. **Constraints** (4-5 bullets) — Hard rules. Imperative sentences. No rationale — small models don't benefit from "why" explanations.
3. **How to Work** (3-4 bullets) — Concrete steps, not philosophy. "Read the error, then read the code at that line" not "understand the context deeply before acting."
4. **Output Format** (template) — Keep this. Small models produce much better output with a structural template to fill in.

## What to Cut for Compact

Ranked by token savings vs quality impact:

| Section | Typical size | Cut? | Reason |
|---------|-------------|------|--------|
| `<Why_This_Matters>` | 50-80 words | **Cut** | Motivational context. Small models don't internalize this. |
| `<Failure_Modes_To_Avoid>` | 80-120 words | **Cut** | Negative examples confuse small models — they sometimes reproduce the anti-pattern instead of avoiding it. |
| `<Final_Checklist>` | 60-80 words | **Cut** | Redundant with constraints. Small models rarely self-check. |
| `<Tool_Usage>` | 50-80 words | **Cut** | Tool mappings are in the system prompt. Duplicating here wastes budget. |
| `<Investigation_Protocol>` | 100-200 words | **Collapse** | Keep as 4 bullets, drop numbered sub-steps. |
| `<Execution_Policy>` | 30-50 words | **Cut** | Effort guidance and escalation rules don't work reliably on small models. |
| `<Success_Criteria>` | 50-80 words | **Cut** | Aspirational. Constraints are the enforceable version. |
| Examples in constraints | 20-40 words each | **Cut** | "Adding null checks everywhere instead of asking why is it null?" becomes just "Fix root causes, not symptoms." |

## Derivation Process

To create a compact variant from a default agent:

1. Start with the default agent prompt.
2. Delete the sections marked "Cut" above.
3. Collapse multi-step protocols to 3-4 imperative bullets.
4. Strip rationale from constraints — keep only the rule. "Reproduce BEFORE investigating. If you cannot reproduce, find the conditions first." becomes "Reproduce before investigating."
5. Tighten the output format template — remove optional fields, combine related lines.
6. Verify the result is under 300 words.
7. Test with a ≤9B model on a representative task. If the model ignores a rule, the rule's phrasing is too subtle — make it blunter, not longer.

## Reference Sizes

Current agent prompts across the three sources:

| Source | Word range | Purpose |
|--------|-----------|---------|
| `.tinycode/agent/` (default) | 456–2462 | Full-featured for large models (default tier source) |
| `.tinycode/agent/` (omt) | 227–373 | Already near compact size (compact tier reference) |
| `src/agent/prompt/` (native) | 75–356 | Built-in agents, already minimal |

The omt agents in `.tinycode/agent/` demonstrate what a well-done compact agent looks like — they were written for local models and have been validated against ≤12B models.

## Anti-Patterns

- **Fewer rules ≠ compact.** Small models need *more* guardrails but in *fewer words*. Tighter phrasing, not fewer rules.
- **Don't add "be concise" to agent prompts.** That's in the system prompt. Agent prompts define the agent's specialty, not general behavior.
- **Don't reference other agents in compact prompts.** "Escalate to architect" means nothing if the model doesn't know what architect is. Say "stop and explain the blocker."
- **Don't use XML tags in compact prompts.** Small models handle flat markdown headers better than nested XML. Save the `<Section>` syntax for default tier.
