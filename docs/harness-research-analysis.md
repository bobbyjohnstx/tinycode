# Harness Research Analysis — What tinycode-go Can Learn

Research conducted 2026-09-28. Based on three sources analyzing agent harness design.

## Sources

1. **Earendil — "What is a Harness?"** (https://earendil.com/posts/what-is-a-harness/)
   Philosophy and market positioning. Defines harness = model + system prompt + tools + agentic loop + translation layer. Emphasizes user ownership, model-agnosticism, local-first, community extensibility.

2. **NVIDIA NOOA — "Six Agent Harness Capabilities"** (https://developer.nvidia.com/blog/six-agent-harness-capabilities-for-higher-model-performance)
   Six architectural capabilities that create "double-digit swings in benchmark results": typed I/O, pass by reference, code as action, programmable loops, explicit object state, model-callable harness APIs. Achieved 82.2% on SWE-bench with 50% fewer tokens than competitors.

3. **Academic Paper — "An Empirical Study of Harness Design for Coding Agents"** (https://arxiv.org/abs/2609.20804)
   176 experiments decomposing harness into planning, action space, context management. Proves: staged elision→summarization is optimal, planning helps weak models but costs strong ones, tool set should adapt to model capability.

---

## Architecture (High Impact)

### 1. Staged Elision Before Summarization

**Idea**: Add a two-stage context management strategy. At a soft threshold (~60% of context window), replace old tool results with compact stubs (elision). Only at the hard threshold (~85%) trigger full LLM summarization. This keeps transcripts append-only and cache-valid for longer.

**Evidence**: 
- NVIDIA: Append-only transcripts with bounded previews compound prefill cache hits across entire sessions. No compaction needed in most sessions.
- Academic paper: T4 strategy (elision then summarization) had the lowest cost per task at every window budget across all models tested. Managed tiers had zero overflow failures vs 78.7% overflow at 32k without management.

**What tinycode has**: `maskObservations()` in `compaction.go` replaces old tool results with stubs. `checkCompaction()` triggers proactive compaction based on input tokens.

**Gap**: No two-stage strategy. `maskObservations` only runs during full compaction, not as a lighter incremental step. No soft/hard threshold separation. Single-stage = expensive LLM call every time.

**Effort**: M

---

### 2. Model-Adaptive Tool Presentation

**Idea**: Adapt the tool set based on model capability. For strong models (70B+, cloud APIs), expose fewer structured tools and let bash handle more. For weak models (7-8B local), expose the full structured tool set with read-before-write enforcement and post-edit diagnostics.

**Evidence**:
- Academic paper: Predefined tools raised success by +15pp for 30B models on SWE-bench. Bash-only reduced cost by 53% for 550B models with +3.6pp accuracy. The crossover depends on both model capability and task type.
- 66% of weak model bash-only runs terminated after out-of-interface emissions, shortening trajectories from 71 to 15 turns. Strong models issued 32% fewer calls with bash-only.

**What tinycode has**: `model.SizeB()` for size detection. Compact agent variants for small models. Agent-level permission overrides (`WithOnlyTools`).

**Gap**: No automatic tool profile selection. Same 19 tools exposed regardless of model size. No "bash-preferred" mode for capable models.

**Effort**: L

---

### 3. Bounded Tool Result Previews

**Idea**: Instead of injecting full tool outputs into context, show bounded previews (first/last N lines + summary stats like line count, file size) and let the model request full content via a `recall_tool_output(call_id)` tool if needed.

**Evidence**:
- NVIDIA NOOA: "Pass by reference" — model sees bounded previews instead of serialized dumps. This is the #1 mechanism behind their 50% token reduction (29 calls/1.1M tokens vs competitors at 66 calls/2.2M tokens).
- Academic paper: Recoverable recall (T2) added machinery models rarely used (56.3% of sessions never called recall). But bounded previews (not full dumps) are universally beneficial.

**What tinycode has**: `truncate.go` truncates large outputs at ~100 lines. `maskObservations` replaces old results with stubs during compaction.

**Gap**: Truncation is coarse (100 lines). No preview+recall pattern. Full tool results still go into context immediately. No token-budget-aware result sizing.

**Effort**: M

---

## Quick Wins (Medium Impact, Low Effort)

### 4. Read-Before-Write Enforcement

**Idea**: Track which files the model has read in a session. When it tries to `write` or `edit` a file it hasn't read, inject a warning or automatically read the file first. Use content hashing to detect stale reads.

**Evidence**:
- Academic paper: Read-before-write enforcement with content hashing was a key difference between structured tools and bash-only. Reduced broken edits — the #1 source of wasted iterations with local models.
- NVIDIA: Safety gates include "read-before-write checks with content hashing."

**What tinycode has**: `edit.go` reads the file to find `old_string` (implicit read). `compaction.go:trackFiles()` identifies read vs modified files. `getFileMutex()` prevents concurrent edits.

**Gap**: No enforcement. The model can write to files it's never explicitly read. No warning or auto-read on first write. No content hashing for stale detection.

**Effort**: S

---

### 5. Model-Neutral Positioning

**Idea**: Lead README and marketing with "bring any model" messaging. Position tinycode as the model-agnostic alternative to Claude Code (which started single-model). Emphasize the translation layer as a differentiator.

**Evidence**:
- Earendil: Model-agnosticism is the core value prop against cloud-locked tools. Claude Code "was not built to provide an agnostic AI translation layer." The trend is toward open, neutral harnesses.

**What tinycode has**: OpenAI + Anthropic LLM clients, 6+ auto-discovered providers (OpenRouter, LM Studio, Ollama, vLLM), config-defined custom providers, RHOAI integration.

**Gap**: README doesn't lead with model-neutrality. No automated model fallback chains. No "works with any OpenAI-compatible endpoint" headline.

**Effort**: S

---

### 6. Data Sovereignty Emphasis

**Idea**: Add explicit "your data never leaves your machine" positioning. Add a `/privacy` or `/data` command showing exactly what's stored and where. Frame as a compliance/enterprise feature.

**Evidence**:
- Earendil: "Users retain the freedom to make their tools their own, and keep local copies of the sessions." Local ownership is the core differentiator vs cloud-dependent tools.

**What tinycode has**: SQLite local DB, local config files, no telemetry, no cloud calls unless user configures a cloud provider.

**Gap**: No explicit privacy statement. No `/privacy` command. Not marketed as a data sovereignty tool.

**Effort**: S

---

## Medium Term

### 7. Post-Edit Automated Diagnostics

**Idea**: After every `edit`, `write`, or `apply_patch` tool execution, automatically run a lightweight diagnostic (syntax check, linter) for the file's language and append findings to the tool result. Catches errors in the same turn instead of a later iteration.

**Evidence**:
- Academic paper: The harness ran ruff/pyflakes after Python edits as part of the tool result pipeline. This reduces the edit→error→fix cycle from 3 turns to 1.
- NVIDIA: Post-edit diagnostics are part of the structured tool protocol.

**What tinycode has**: `afterHook` system in tool execution (`wireToolAfterHook` in `config.go`). LSP integration for language servers. `go vet` / linter support in the codebase.

**Gap**: `afterHook` is for plugins, not built-in diagnostics. No automatic syntax/lint check after edits. LSP exists but isn't wired into the tool result pipeline.

**Effort**: M

---

### 8. Adaptive Planning by Model Strength

**Idea**: Inject structured planning prompts for weak models (accuracy scaffold), strip them for strong models (cost saver). Use `model.SizeB()` or a capability flag to select planning depth. For weak models: inject step-by-step workflow instructions, require explicit plans before action. For strong models: omit planning overhead.

**Evidence**:
- Academic paper: Planning raised accuracy +11.6pp for 30B models on SWE-bench. For 550B models, planning had -2pp accuracy but -30% cost. Without planning, 68.6% of weak model runs terminated without making an edit (vs 27.8% with planning).

**What tinycode has**: Compact agent variants (`*.compact.md`) for small models. `model.SizeB()` for size detection. Build agent has a structured "Task workflow" section.

**Gap**: Compact variants trim length but don't adjust planning strategy. No automatic planning injection/removal. Same workflow instructions for all models.

**Effort**: M

---

### 9. Community Extension Registry

**Idea**: Create a shareable registry where users can publish and install agents, skills, MCP configs, and tool configs via `tinycode install <name>` or a curated index. Start with a git-based registry (JSON index + hosted .md files).

**Evidence**:
- Earendil: Pi has 5,000+ community-shared extensions. This is their #1 differentiator — network effects make the tool stickier. Users share custom agents for specific workflows.

**What tinycode has**: Agent .md files, skills as SKILL.md, plugins as JSON-RPC binaries, MCP servers — all file-based and portable. User agents in `~/.config/tinycode/agents/`, user skills in `~/.config/tinycode/skills/`.

**Gap**: No `tinycode install`, no registry index, no way to discover or share configurations between users. Each user must manually create or copy agent/skill files.

**Effort**: M

---

## Summary Matrix

| Rank | Band | Idea | Impact | Effort |
|------|------|------|--------|--------|
| 1 | Architecture | Staged elision before summarization | High | M |
| 2 | Architecture | Model-adaptive tool presentation | High | L |
| 3 | Architecture | Bounded tool result previews | High | M |
| 4 | Quick Win | Read-before-write enforcement | Medium | S |
| 5 | Quick Win | Model-neutral positioning | Medium | S |
| 6 | Quick Win | Data sovereignty emphasis | Medium | S |
| 7 | Medium Term | Post-edit automated diagnostics | Medium | M |
| 8 | Medium Term | Adaptive planning by model strength | Medium | M |
| 9 | Medium Term | Community extension registry | High | M |

## The Biggest Gap

**Context efficiency.** tinycode's single-stage compaction (mask then full LLM summarization) is expensive and loses information. The research converges: **elide early, summarize late, keep previews bounded**. Implementing #1 + #3 would match state-of-the-art harness design and directly improve performance with local models where every token matters.
