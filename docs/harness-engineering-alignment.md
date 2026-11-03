# Harness Engineering Alignment Report

How tinycode maps to the findings of "Harness Engineering: Anatomy, Architecture, and Evolution of Coding Agents" (Barbaste et al., arXiv:2609.00006, July 2026) — an 83-page source-code study of 11 production coding agent harnesses.

This report was reviewed and corrected against Section 16 of the paper. The paper does not rank harnesses.

## Paper Overview

The paper defines an agent as **a model + a harness** and identifies 7 canonical subsystems, 29 recurring design patterns, and 18 design recommendations from studying ~4 million lines of Python/TypeScript/Rust across Claude Code, Codex CLI, Gemini CLI, Mistral Vibe, OpenHands, Aider, Mini-SWE-Agent, Hermes, Pi, OpenCode, and OpenClaw.

Central thesis: "In the first half of 2026 the coding harness completed a turn from tool to platform."

Full reference: see `memory/reference_harness-engineering-paper.md` in the project memory directory.

## Scorecard Against Section 16

Scored against what the paper actually recommends, not a rewritten checklist.

| # | Paper Recommendation | What It Actually Says | tinycode | Score |
|---|---------------------|----------------------|----------|-------|
| 1 | Loop | Linear loop is the right start. Pull policies into middleware only after several pile up. | `Processor.ProcessWithID` is that loop. Iteration cap, compaction, doom-loop, tool-failure caps in the loop body. | ✅ |
| 2 | Providers | Multi-provider systems should pay for per-model metadata: cache, thinking, prompts. | `llm.NewClient` picks OpenAI or Anthropic. No per-family prompt or cache matrix. | ⚠️ |
| 3 | Tools | Start from bash and add tools when something fails. | Met for the core set (18 builtins). | ✅ |
| 4 | Deferred tools | Defer tool schemas once the catalog passes ~15 tools. Below that, skip the machinery. | `RegisterConditional` only gates `skill` and `websearch`. 17 builtins always sent. Plugin/MCP tools can push past threshold with no `tool_search`. | ⚠️ |
| 5 | Edits | Exact replace for frontier models, fuzzy cascade for weaker ones. | `edit_replacer.go` always runs the cascade — fits a local-model-first harness. | ✅ |
| 6 | Context files | Auto-discover `AGENTS.md`/`CLAUDE.md`, let model persist durable facts in a bounded, reviewable place. | Discovery met: `BuildSystemPrompt` walks those files. Durable model-written facts is the gap — narrower than originally stated. | ⚠️ |
| 7 | Compaction | Threshold, verbatim tail, incremental summary merge, same path on overflow. | `checkCompaction` uses `compactionOutputReserve`; compact receives `priorSummary`. | ✅ |
| 8, 16 | No code RAG | Do not embed the codebase. Embeddings for conversation memory are a separate, optional choice. | Nothing retrieves code with vectors. | ✅ |
| 9 | Approval modes | Plan / default / YOLO for a developer tool. | Plan agent, ask-by-default, auto-approve cover the shape. No named modes. | ⚠️ |
| 10 | OS sandbox | For enterprise and unattended runs, not for every local developer tool. | Correctly skipped — local-first, single-binary design. | N/A (deployment choice) |
| 11 | Policy floor | Rules as data, and a floor that still denies under YOLO. | **The important gap.** Auto-approve in `Registry.Execute` skips `permission.Ask` entirely. No floor survives this path. | ❌ |
| 12 | Sub-agents | Stay single-agent until parallel work needs isolated context. | `task` tool exists with depth 5, 8-way concurrency, same directory. Paper treats this as "something you earn," not a checkmark for having it. | ✅ |
| 13 | ACP | Ship an ACP server. Keep sub-agents in-process. | `internal/acp` is the server. `RunSubagent` stays in-process. | ✅ |
| 14 | Skills, then MCP | Skills for procedures, MCP for external systems. | SKILL.md discovery + MCP service configured from config. | ✅ |
| 15 | No frameworks | No LangChain, AutoGen, CrewAI. | No framework imports in `go.mod`. Hand-rolled loop. | ✅ |
| 17 | No 1:1 API tools | Do not expose every upstream API as its own tool. | Core is fine. The 32-plugin registry has many one-API-surface plugins, but they stay out of the prompt until installed — the mitigating fact. An installed pile of them is what the paper would call a smell. | ⚠️ |
| 18 | Stuck detection | Ship cheap caps. Do not build a classifier. | Doom threshold 3 identical calls, 10 consecutive tool failures, 200 iterations. | ✅ |

**Honest score: 10 met, 5 partially met, 1 not met, 1 deployment choice, 1 combined with #8**

## The Policy Floor Gap (the real priority)

Auto-approve is two mechanisms, and the floor sketch in the original report missed both:

1. **`tool.Context.AutoApprove`** skips the outer `permission.Ask` in `Registry.Execute` before the tool runs. A floor implemented only inside `permission.Evaluate` never runs for an ordinary shell command on this path.

2. **`executeShell` destructive/secret patterns** still call `Ask` even with AutoApprove, but the TUI auto-replies with "allow" as soon as `PermissionRequestedMsg` arrives — the user never sees the prompt.

3. **`safety-net` plugin** is closer to the Hermes floor than originally acknowledged, but it only runs as a `permission.ask` hook, only if installed, and only when evaluation already decided to ask. An allow rule returns before the hook fires.

**A floor that matches the paper must:**
- Deny in `Registry.Execute` BEFORE the auto-approve skip
- Deny in `executeShell` BEFORE the process starts
- Not be overturnable by the TUI auto-reply
- Normalize quotes and command substitution before matching (raw regex misses obfuscated forms)

This is the change with the best trust-to-effort ratio. A short denylist is a reasonable first cut. Treat it as a backstop, not a sandbox.

## Persistent Memory (corrected shape)

The original recommendation proposed a SQLite `memory` tool. This is the wrong shape.

**What the paper's examples actually are:**
- Claude Code: character-capped markdown files
- Hermes: two small injected files
- Gemini CLI: extraction sub-agent writes an inbox the user reviews
- Recommendation 16 cites SQLite FTS as a recall index for transcripts, not as the memory document itself

**What tinycode already has:**
- `BuildSystemPrompt` injects `AGENTS.md` and `CLAUDE.md` — this is the part of R6 the corpus converged on
- Wiki plugin stores project markdown under `.tinycode/wiki/` — human-readable, diffable
- Notepad is weaker than "session-scoped" — `NewRegistry` creates one `SafeNotepad` shared across all sessions in the process, and it dies with the process

**Right shape:** A bounded project markdown file (e.g., `.tinycode/LEARNINGS.md`) the model can edit via the existing Write tool, with a size cap and a prompt injection of the keys. This matches both the paper and the wiki plugin pattern. Not a hidden SQLite table.

**Priority:** After the policy floor. The wiki plugin already covers the 80% case for users who install it.

## Other Recommendations (corrected)

### Worktree Isolation (follow-on, not R12)

Reasonable opt-in once parallel `task` calls can write the same tree. Sub-agents start a fresh prompt in `tc.Directory`; they do not inherit the parent transcript. `handleForkSession` is a user transcript copy, not a sub-agent context fork.

`git worktree add` is the small part. Deciding what merges back is the product work — 2-3 days is tight for the full solution. Leave the default as the current shared directory.

### Model-Family Prompts (belongs under R2)

Worth doing. The compact switch is `modelSizeB <= 14` in `agent.Get`. Start with agents whose tool-calling behavior actually changes across model families (code-reviewer, build). Keep the described fallback chain: family-specific → compact → full.

Note: `docs/authoring.md` still says 8B for the compact threshold (should say 14B).

### Event Bus as Public Extension (weakest item)

Plugin hooks already cover `session.start`, `session.end`, `permission.ask`, tool before/after. ACP relays `session.error`, `session.status`, tool begin/end. A compaction hook is a small addition if a plugin needs one. A public event bus is a new API surface with maintenance cost — defer until a concrete consumer needs it.

## Patterns tinycode Implements

| Pattern | Status |
|---------|--------|
| Iterative action-observation loop | ✅ |
| Threshold compaction with prior summary | ✅ |
| Fuzzy matching cascade for edits | ✅ |
| Coordinator-worker delegation | ✅ |
| Session branching/forking | ✅ |
| Deferred conditional tool registration | ✅ (partial — not schema-level deferral) |
| Streaming with concurrent tool batching | ✅ |
| Stuck detection (doom loop, failure caps) | ✅ |
| Skills + MCP as dual extensibility | ✅ |

## What NOT to Adopt

| Pattern | Why skip |
|---------|----------|
| OS-level sandbox (Bubblewrap/Seatbelt) | Single-binary, local-first. Permission floor is the right tradeoff. |
| LLM approval reviewer (Guardian) | Latency and token cost per tool call. Permission rules + floor sufficient for local use. |
| Vector-embedding retrieval | 0/11 systems use it. Field validates deterministic approach. |
| General-purpose agentic framework | 0/11 systems use one. Field validates hand-rolled loops. |
| Public event bus | Plugin hooks already cover the needed lifecycle events. New API surface cost not justified. |

## Implementation Priority

1. **Policy floor** — XS effort, highest trust-to-effort ratio. Deny in `Registry.Execute` before auto-approve skip. Short denylist with command normalization.
2. **Model-family prompts** — S effort. Extend compact variant system with `{agent}.{family}.md` overrides.
3. **Worktree isolation** — M effort. Opt-in for parallel task calls. Product design for merge-back is the hard part.
4. **Bounded LEARNINGS.md** — S effort. Only if wiki plugin adoption proves insufficient.

## tinycode's Unique Position

Not in the paper's corpus (Python/TypeScript/Rust only):

| Differentiator | Detail |
|----------------|--------|
| Go language | Only Go harness identified in the landscape |
| Single binary | No separate server, no Node.js, no Python runtime |
| Local-LLM-first | Ollama/vLLM/LM Studio auto-discovery; cloud optional |
| 32 plugins as Go binaries | JSON-RPC over stdin/stdout, not npm packages |
| Pure Go SQLite | `modernc.org/sqlite`, no CGO, cross-compilation works |
