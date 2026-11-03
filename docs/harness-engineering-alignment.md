# Harness Engineering Alignment Report

How tinycode maps to the findings of "Harness Engineering: Anatomy, Architecture, and Evolution of Coding Agents" (Barbaste et al., arXiv:2609.00006, July 2026) — an 83-page source-code study of 11 production coding agent harnesses.

## Paper Overview

The paper defines an agent as **a model + a harness** and identifies 7 canonical subsystems, 29 recurring design patterns, and 18 design recommendations from studying ~4 million lines of Python/TypeScript/Rust across Claude Code, Codex CLI, Gemini CLI, Mistral Vibe, OpenHands, Aider, Mini-SWE-Agent, Hermes, Pi, OpenCode, and OpenClaw.

Central thesis: "In the first half of 2026 the coding harness completed a turn from tool to platform."

Full reference: [`memory/reference_harness-engineering-paper.md`](../memory/reference_harness-engineering-paper.md)

## 18 Recommendations Scorecard

| # | Recommendation | Score | tinycode Evidence |
|---|---------------|-------|-------------------|
| R1 | Simple iterative action-observation loop | ✅ | `processor.go:176-291` — standard for-loop alternating LLM inference with tool execution |
| R2 | Budget/step limits and stop guards | ✅ | `maxIterations=200`, `TokenBudget` check, doom-loop detector, `maxConsecutiveToolFailures=10` |
| R3 | Single-vendor tight coupling acceptable | ✅ | Multi-provider via owned `llm.Client` interface — not framework-bound |
| R4 | Multi-provider: owned transports | ✅ | `NewClient` selects OpenAI or Anthropic client based on protocol hints; no framework import |
| R5 | Fewer, higher-signal tools | ✅ | 18 builtins (Read/Write/Edit/Shell/Grep/Glob/etc.) — well within guidance |
| R6 | Deferred/lazy tool loading | ✅ | `RegisterConditional` loads Skill and WebSearch only when config/API keys present |
| R7 | Whole-file write + search-replace/diff | ✅ | WriteTool, EditTool (search-replace), ApplyPatchTool (unified diff) |
| R8 | Fuzzy matching cascades | ✅ | `edit_replacer.go` — fuzzy replacement strategy with fallback on exact-match failure |
| R9 | Threshold-triggered compaction | ✅ | `compactionOutputReserve` calculates threshold; `checkCompaction` after each LLM step |
| R10 | Persistent cross-session memory | ⚠️ | Notepad is session-scoped only. Wiki plugin exists but is opt-in. No built-in cross-session memory. |
| R11 | Layer permission checks | ✅ | DefaultRules with read/ask/deny layers; buildPerms adds tool-level allow then ask for sensitive patterns |
| R12 | Hard policy floor surviving YOLO | ⚠️ | Auto-approve exists but no unconfigurable deny floor. `ActionDeny` exists but agents can override via config. |
| R13 | Sub-agent spawning as tool call | ✅ | TaskTool registered; SubagentRunner wired; general/explore/scout subagents with step limits |
| R14 | Fork context for sub-agents | ✅ | `handleForkSession` creates forked sessions; TaskTool spawns independent child sessions |
| R15 | SKILL.md + MCP support | ✅ | SKILL.md discovery from user/project/path dirs; MCP service configured from config |
| R16 | Hooks/lifecycle events | ✅ | Shell hooks + plugin hooks for before/after tool execution and permission ask |
| R17 | No agentic framework imports | ✅ | No LangChain/AutoGen/CrewAI in go.mod; hand-rolled loop |
| R18 | No embedding-based vector retrieval | ✅ | No vector DB; deterministic tools (Grep/Glob/Read) |

**Score: 15/18 met, 2 partially met, 1 N/A (R3 — tinycode is multi-provider by design)**

## Key Alignment with Field Findings

### The Twin Absences ✅
- No agentic frameworks: tinycode's loop is hand-rolled Go, no LangChain/AutoGen
- No vector-embedding code retrieval: deterministic tools only (ripgrep, glob, tree-sitter-like grep)

### Adoption Standards ✅
| Standard | Field Adoption | tinycode |
|----------|---------------|----------|
| SKILL.md | 9/11 | ✅ Implemented |
| MCP | 8/11 | ✅ Implemented (stdio/SSE/streamable-HTTP) |
| ACP | 6/11 | ✅ Implemented |
| Hooks/event bus | 9/11 | ✅ Implemented |
| Deferred tool loading | 8/9 | ✅ Conditional registration |

### Threshold Compaction ✅
7/11 systems converged on threshold-triggered LLM summarization. tinycode matches: `compactionOutputReserve` calculates per-model threshold, `checkCompaction` triggers after each LLM step.

## tinycode's Unique Position

Not in the paper's corpus (which covers Python/TypeScript/Rust):

| Differentiator | Detail |
|----------------|--------|
| **Language** | Go — the only Go harness identified in the landscape |
| **Single binary** | No separate server, no Node.js, no Python runtime |
| **Local-LLM-first** | Ollama/vLLM/LM Studio auto-discovery; cloud providers optional |
| **32 plugins as Go binaries** | JSON-RPC over stdin/stdout, not npm packages |
| **Pure Go SQLite** | `modernc.org/sqlite`, no CGO, cross-compilation works |

## Gap Analysis and Implementation Recommendations

### P0: Persistent Cross-Session Memory (R10)

**Gap:** Notepad is session-scoped. The wiki plugin provides persistent knowledge but requires manual install and is not built-in. 9/11 systems in the corpus support persistent memory.

**Recommendation:** Add a built-in `memory` tool backed by SQLite (already in the binary).

**Implementation:**
- New `internal/tool/memory.go` (~150 lines)
  - `memory_write(key, content, tags)` — upsert a memory entry scoped to the project directory
  - `memory_read(key)` — retrieve by key
  - `memory_search(query)` — full-text search across entries
  - `memory_list()` — list entries with metadata
- New table in `internal/storage/migrations/`: `CREATE TABLE memory (id TEXT PK, directory TEXT, key TEXT, content TEXT, tags TEXT, created_at, updated_at)`
- Register in `builtin.go` — always available, no opt-in
- Session-start: inject a system hint listing existing memory keys for the current directory (like the "files already read" hint)

**LOE:** S (1-2 days)

**What the corpus does:**
- Codex: agent-maintained memory pipeline (sandboxed sub-agent consolidates)
- Gemini CLI: extraction sub-agent writes to a patch inbox the user reviews
- Claude Code: character-capped MEMORY.md files
- Hermes/OpenHands: bounded context files

### P1: Hard Policy Floor (R12)

**Gap:** Auto-approve bypasses all permission checks. No unconfigurable deny floor prevents catastrophic operations like `rm -rf /`, credential exfiltration via `curl`, or `chmod 777` on system files.

**Recommendation:** Add `FloorRules` evaluated before user/agent rules in `permission.Evaluate()`. Floor survives auto-approve.

**Implementation:**
- New `internal/permission/floor.go` (~60 lines)
  - `FloorRules []Rule` — hardcoded, unconfigurable deny rules
  - Patterns: `rm -rf /`, `rm -rf ~`, `chmod 777 /`, `curl * | bash`, `wget * | sh`, writing to `/etc/`, `eval(`, credential env vars in command args
- Modify `internal/permission/evaluate.go`: check floor rules FIRST, before user rules. If floor denies, return `ActionDeny` regardless of auto-approve state.
- Add `session.warning` event when floor blocks an action ("Policy floor blocked: [reason]")
- TUI toast surfaces the block

**LOE:** XS (half day)

**What the corpus does:**
- Hermes: "a policy floor that survives YOLO" — explicitly designed to prevent catastrophic commands even in full-auto mode
- Codex: Starlark policy rules → LLM approval reviewer → OS sandbox (three layers)

### P1: Worktree Isolation for Subagents (R14 strengthening)

**Gap:** TaskTool spawns subagents in the same working directory. Parallel subagents can conflict on file mutations. The paper shows Claude Code and Codex both fork context with isolated copies.

**Recommendation:** Add `isolation: "worktree"` option to TaskTool.

**Implementation:**
- Modify `internal/tool/task.go`: when `isolation` arg is `"worktree"`, create a git worktree before spawning the subagent, clean up after
- New `internal/vcs/worktree.go` (~80 lines): `CreateWorktree(dir, branch) (path, cleanup, error)` using `git worktree add`
- Subagent runs in the worktree directory; on completion, changes are available for the parent to merge
- Default: no isolation (current behavior). Opt-in per task call.

**LOE:** M (2-3 days)

### P2: Model-Family Prompt Matrix

**Gap:** tinycode has compact variants (≤14B models get simpler prompts) but not per-model-family prompts. The paper shows OpenCode dispatches 9 different prompts per model family; Aider has 13 edit-format strategies; Codex uses server-delivered model-catalog prompts.

**Recommendation:** Extend the compact variant system to support family-specific overrides.

**Implementation:**
- Extend `internal/agent/agent.go`: beyond `{name}.compact.md`, look for `{name}.{family}.md` (e.g., `code-reviewer.qwen.md`, `code-reviewer.llama.md`)
- Family detected from model ID prefix (already parsed in provider)
- Fallback chain: family-specific → compact (if small) → full
- Start with code-reviewer and build agents — the most model-sensitive

**LOE:** S (1 day)

### P2: Event Bus as Public Extension Substrate

**Gap:** `internal/bus` is internal. External consumers (ACP clients, plugins) can only react via hooks (before/after tool calls). The paper identifies hooks+event buses as a platform-turn signal (9/11 systems).

**Recommendation:** Expose session lifecycle events via ACP and plugin hooks.

**Implementation:**
- Add ACP method `session/subscribe` that streams select bus events to the IDE: `session.start`, `session.end`, `session.compaction`, `session.error`, `session.warning`, `tool.begin`, `tool.end`
- Add plugin hook phases beyond tool before/after: `session.start`, `session.end`, `compaction.before`, `compaction.after`
- Modify `internal/plugin/manager.go`: new hook dispatch points in processor lifecycle
- Modify `internal/acp/event.go`: relay additional bus events

**LOE:** S (1-2 days)

## Patterns tinycode Implements

From the paper's 29 recurring patterns, tinycode implements or partially implements:

| Pattern | Status | Notes |
|---------|--------|-------|
| Streaming loop with concurrent tool batching | ✅ | `processor_tools.go` — parallel tool execution with semaphore |
| Deferred tool loading | ✅ | Conditional registration |
| Threshold compaction | ✅ | `compactionOutputReserve` |
| Prompt-cache-sharing forks | Partial | Subagents share system prompt but don't exploit provider cache prefixes |
| Coordinator-worker pattern | ✅ | Build agent delegates to executor/architect/critic |
| Verify-on-stop guards | Partial | Doom-loop detector, but no verification step before stop |
| Session-tree version control | ✅ | Session branching/forking |
| Fuzzy matching cascade | ✅ | `edit_replacer.go` |
| Everything-is-an-extension | Partial | Plugins + skills + hooks, but core tools are hardcoded |

## Patterns Worth Adopting

| Pattern | From | Value for tinycode |
|---------|------|--------------------|
| Agent-maintained memory pipelines | Codex | The model itself writes/reads persistent memory — higher quality than explicit tool calls |
| Lineage compaction | Hermes | Preserves genealogical thread structure during compaction — better for multi-agent sessions |
| Syntax-aware command permissioning | OpenCode | Parse shell commands to AST before permission check — catches obfuscated dangerous patterns |
| Self-improving skill loop | Hermes | Agent can author its own skills from successful sessions — evolving capability |
| Active-Memory sub-agent | OpenClaw | Run a memory retrieval step before each reply — automatic context enrichment |

## What NOT to Adopt

| Pattern | From | Why skip |
|---------|------|----------|
| OS-level sandbox (Bubblewrap/Seatbelt) | Codex, Gemini CLI | tinycode is single-binary, local-first. OS sandbox adds deployment complexity antithetical to the design. Permission floor (R12) is the right tradeoff. |
| LLM approval reviewer (Guardian) | Codex | Adds latency and token cost to every tool call. Permission rules + floor are sufficient for local-first use. |
| Vector-embedding retrieval | None (0/11) | The entire field validates tinycode's deterministic approach. |
| General-purpose agentic framework | None (0/11) | The entire field validates tinycode's hand-rolled loop. |

## Summary

tinycode scores **15/18** on the paper's recommendations — stronger than most systems in the corpus. The three gaps (persistent memory, policy floor, sandbox isolation) are ordered by impact and feasibility. The policy floor (P1, XS effort) should ship first as it's the highest trust-to-effort ratio. Persistent memory (P0, S effort) closes the largest functional gap. Both can ship in a single release cycle.

tinycode's unique position (Go, single binary, local-LLM-first) is not represented in the corpus, making it complementary rather than competitive with the studied systems.
