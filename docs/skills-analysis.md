# Skills Analysis for Tinycode

Analysis of skills, patterns, and infrastructure worth adopting into tinycode.

Skills and agents have already been selectively adopted into tinycode and oh-my-tiny, but some skills were not included. This document captures which skills and patterns are worth bringing over.

---

## Tier 1: Copy First (High Value, Reasonable to Adapt)

### `ralplan` (plan) — Consensus Planning Loop

Implements a Planner → Architect → Critic consensus loop (up to 5 iterations) that stress-tests plans from multiple perspectives before execution begins.

- **Short mode**: Fast single-pass planning
- **Deliberate mode**: Adds pre-mortem (3 failure scenarios) + expanded test plan (unit/integration/e2e/observability)
- **Output**: RALPLAN-DR structured format with ADR section (Decision, Drivers, Alternatives, Why chosen, Consequences, Follow-ups)
- **Source**: `skills/plan/` (also aliased as `ralplan`)

### `verify` — Evidence-Based Completion Checks

Turns vague "it works" claims into concrete evidence. Enforces a preference order:

1. Existing tests
2. Typecheck / build
3. Narrow targeted checks
4. Manual validation (last resort)

Prevents agents from self-approving their own work. Lightweight and standalone.

- **Source**: `skills/verify/`

### `trace` — Evidence-Driven Causal Debugging

Structured debugging with competing hypotheses ranked by a 6-tier evidence strength hierarchy:

| Tier | Type | Example |
|------|------|---------|
| 1 (strongest) | Controlled reproductions | Isolated repro confirming behavior |
| 2 | Primary source artifacts | Logs, metrics, code-path traces, git history |
| 3 | Convergent sources | Multiple independent sources agreeing |
| 4 | Single-source inference | One log line, one metric |
| 5 | Circumstantial | Timing correlations, naming patterns |
| 6 (weakest) | Speculation | Intuition, guesses |

Forces structured reasoning instead of guess-and-check. Includes falsification rules and rebuttal rounds.

- **Source**: `skills/trace/`

### `ai-slop-cleaner` (deslop) — Regression-Safe AI Cleanup

Mandatory cleanup pass for AI-generated code. Key properties:

- **Deletion-first**: Removes bloat before restructuring
- **Behavior-locked**: Runs regression tests after cleanup to ensure nothing breaks
- **`--review` mode**: Flags issues without fixing (reviewer-only)
- **Ralph integration**: Phase 7.5 in ralph is a mandatory deslop pass
- **Source**: `skills/ai-slop-cleaner/`

### `release` — Repo-Aware Release Assistant

Self-contained release workflow that:

- Detects version source files
- Routes to correct registry/distribution
- Inspects CI workflows
- Caches rules analysis for repeat use
- Includes first-time setup guidance
- **Source**: `skills/release/`

### `skillify` — Extract Skills from Sessions

Meta-skill that watches for repeatable workflows during a session and extracts them as reusable skill drafts. Quality gate: the pattern must be:

- Non-Googleable (not generic knowledge)
- Codebase-specific
- Hard-won (learned through real work)

This is how you grow a skill library organically rather than pre-designing everything.

- **Source**: `skills/skillify/`

### Commit Protocol (CLAUDE.md convention)

Not a skill, but a convention worth copying verbatim. Structured git trailers that preserve decision context:

```
fix(auth): prevent silent session drops during long-running ops

Auth service returns inconsistent status codes on token expiry,
so the interceptor catches all 4xx and triggers inline refresh.

Constraint: Auth service does not support token introspection
Constraint: Must not add latency to non-expired-token paths
Rejected: Extend token TTL to 24h | security policy violation
Rejected: Background refresh on timer | race condition with concurrent requests
Confidence: high
Scope-risk: narrow
Directive: Error handling is intentionally broad (all 4xx) — do not narrow without verifying upstream behavior
Not-tested: Auth service cold-start latency >500ms
```

Trailers: `Constraint:`, `Rejected:`, `Directive:`, `Confidence:`, `Scope-risk:`, `Not-tested:`

### Model Routing Table (CLAUDE.md convention)

Tier-based agent routing:

| Tier | Use Case |
|------|----------|
| LOW | Quick lookups, exploration, writing |
| MEDIUM | Standard implementation, debugging, testing, verification |
| HIGH | Architecture, deep analysis, code review, planning |

---

## Tier 2: Worth Copying (Unique & Clever)

### `deep-interview` — Socratic Ambiguity Gating

Requirements gathering with mathematical convergence tracking.

- **Weighted scoring**: Goal (35-40%), Constraints (25-30%), Criteria (25-30%), Context (15%)
- **Round 0 topology confirmation**: Locks top-level components before depth-first questioning
- **Challenge agents**: Contrarian (round 4), Simplifier (round 6), Ontologist (round 8+)
- **Ontology stability tracking**: Measures entity convergence across rounds — stability ratio = (stable + changed) / total. Proves domain model is converging, not oscillating.
- **Source**: `skills/deep-interview/`

### `wiki` — Persistent Knowledge Base

Markdown knowledge base that compounds across sessions.

- Keyword + tag matching (no embeddings needed)
- `[[page]]` cross-reference syntax
- Category organization (architecture/decision/pattern/debugging)
- Append-only log

Oh-my-tiny already has wiki MCP tools, but the *skill* (when/how to write entries) is the valuable part.

- **Source**: `skills/wiki/` (if present as standalone skill)

### `ccg` — Tri-Model Orchestration

Claude + Codex + Gemini parallel advisor invocation with conflict-resolution synthesis. Since tinycode supports multiple LLM providers natively, this is a natural fit.

- Parallel advisor calls
- Structured conflict resolution when models disagree
- Fallback handling when a provider is unavailable
- **Source**: `skills/ccg/`

### `external-context` — Parallel Documentation Lookup

Decomposes a question into 2-5 facets and spawns parallel document-specialist agents. Synthesizes evidence-backed answers from multiple sources.

- **Source**: `skills/external-context/`

### `ultraqa` — Bounded QA Cycling

Test → diagnose → fix → repeat (max 5 cycles) with smart exit conditions:

- **Early exit on pattern**: 3x same failure = stop (fundamental issue, not a fixable bug)
- **Environment error handling**: Distinguishes test env issues from real failures
- **Architect-driven diagnosis**: Uses architect agent for root cause, not the executor
- **Source**: `skills/ultraqa/`

### `self-improve` — Tournament Selection

Autonomous code improvement with parallel experimentation:

1. Fire N independent executors on separate branches
2. Rank candidates by benchmark score
3. Merge best, verify no regression, accept or revert
4. **Circuit breaker**: Max consecutive failures → early exit
5. **Plateau detection**: Same approach family 3+ iterations → diminishing returns
6. **Approach family taxonomy**: architecture / training_config / data / infrastructure / optimization / testing / documentation / other
- **Source**: `skills/self-improve/`

---

## Tier 3: Situationally Useful

### `configure-notifications`

Interactive wizard for Telegram/Discord/Slack/custom notification setup. Useful for long-running workflows where you want to be notified of completion.

- **Source**: `skills/configure-notifications/`

### `project-session-manager` (psm)

Worktree-first dev environments for issues/PRs/features with optional tmux. Multi-provider support (GitHub, Jira, GitLab, Azure DevOps). Includes a "teleport" command for lightweight worktree switching.

- **Source**: `skills/project-session-manager/`

### `deepinit`

Generates hierarchical AGENTS.md documentation across the entire codebase. Parent references (`<!-- Parent: ../AGENTS.md -->`), manual section preservation, validation checks. One-time setup value.

- **Source**: `skills/deepinit/`

### `visual-verdict`

Screenshot-to-reference comparison with structured JSON pass/fail (score threshold 90+). Only useful for UI work with reference screenshots.

- **Source**: `skills/visual-verdict/`

### `cancel`

Smart cancellation with session-aware state cleanup and dependency-aware cancellation order. Only matters if stateful execution modes (ralph, autopilot) are adopted.

- **Source**: `skills/cancel/`

---

## Design Patterns Worth Stealing

These are embedded in various skills and worth extracting as standalone concepts.

### 1. Deslop as Mandatory Phase

Never ship AI-written code without a cleanup pass + regression re-verification. In ralph this is Phase 7.5 — after architect approval but before completion.

### 2. 3-Point Injection (deep-dive → deep-interview)

Feed trace results into interview initialization so requirements gathering starts with evidence:

1. **Initial-idea enrichment**: "Original problem + trace finding + given this, what should we do?"
2. **Codebase-context replacement**: Skip interview's brownfield exploration; inject trace synthesis
3. **Question-queue injection**: Per-lane critical unknowns become the first 1-3 interview questions

### 3. Evidence Strength Hierarchy

6 tiers from controlled reproduction down to speculation. Used to rank competing hypotheses and prevent false certainty. Down-rank hypotheses when support is mostly weak tiers.

### 4. Ontology Convergence Tracking

Mathematical proof that a domain model is stabilizing across interview rounds:

- Stability ratio = (stable + changed) / total entities
- Renamed entities count as stable (unchanged concept, new name)
- Used to decide when requirements gathering is "done enough"

### 5. Tournament Selection Over Greedy

Parallel candidates ranked by benchmark, best confirmed on merge. Safer than sequential guess-and-check because:

- Multiple approaches explored simultaneously
- Built-in rollback (revert on regression)
- Plateau detection prevents wasted iterations

### 6. Session-Scoped State Isolation

All persistent state under `.tinycode/state/sessions/{sessionId}/`. Parallel sessions get independent state trees. Cancel clears only one session's files.

### 7. Consensus Loop (Planner → Architect → Critic)

Three perspectives prevent single-perspective blindness:

- **Planner**: Proposes approach
- **Architect**: Steelman rebuttal + synthesis
- **Critic**: Enforces principle-option consistency, fair alternatives, concrete verification
- Terminates when Critic approves or max iterations hit

### 8. Separate Authoring and Review Passes

Never self-approve in the same context. Writer pass creates/revises content; reviewer/verifier pass evaluates it in a separate lane.

---

## Recommended Adoption Order

### Phase 1: Immediate (standalone, no dependencies)

1. **Commit protocol trailers** — Add to CLAUDE.md (zero effort)
2. **Model routing table** — Add to CLAUDE.md (zero effort)
3. **`verify`** — Copy skill, use immediately
4. **`skillify`** — Copy skill, let it grow the rest

### Phase 2: Planning & Quality (high-value skills)

5. **`ralplan`** — Consensus planning
6. **`ai-slop-cleaner`** — Code quality gate
7. **`trace`** — Structured debugging
8. **`release`** — Release workflow

### Phase 3: Advanced Workflows

9. **`deep-interview`** — Requirements gathering
10. **`ultraqa`** — Bounded QA cycling
11. **`ccg`** — Multi-model orchestration (natural fit for tinycode's multi-provider architecture)
12. **`external-context`** — Parallel docs lookup

### Phase 4: Experimental

13. **`self-improve`** — Tournament selection (ambitious, high reward)
14. **`project-session-manager`** — Worktree dev environments
15. **`deep-dive`** — Full trace → interview pipeline

---

## Source Reference

Each skill typically has:

- `SKILL.md` — Skill definition with trigger patterns, phases, and instructions
- Supporting files (templates, schemas, examples) vary by skill
