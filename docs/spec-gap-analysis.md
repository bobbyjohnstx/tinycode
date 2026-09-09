# Spec Gap Analysis — tinycode-go vs. tc-spec.md

**Date:** 2026-09-09
**Reviewers:** Architect agent, Critic agent (parallel independent reviews)
**Spec:** `tc-spec.md` (functional specification of the original TypeScript tinycode)
**Verdict:** Architecturally sound, ~30-45% feature complete against the full spec.

---

## Coverage Summary

| Category | Spec Count | Implemented | Coverage |
|----------|-----------|-------------|----------|
| Tools | 19+ | 8 | 42% |
| HTTP routes | 114+ | ~57 | ~50% |
| LLM protocols | 7 | 1 (OpenAI-compatible) | 14% |
| SSE event types | 56+ | ~15 | ~27% |
| CLI subcommands | 19 | 5 | 26% |
| DB tables | 13 | 6 | 46% |
| TUI keybindings | 30+ | 5 | 17% |
| Built-in agents | 30+ | 22 | 73% |
| Agent compact variants | 14 | 14 | 100% |
| Config fields | 35+ | ~20 | 57% |
| Provider types | 7+ | 4 | 57% |
| Built-in skills | 11 | 0 (discovered, no execution) | 0% |
| Environment vars | 35+ | ~10 | 29% |

---

## P0 — Core Usability Blockers

### 1. No `task` tool (subagent spawning)

Config fields (`SubagentDepth`, `MaxSubagents`) exist but the tool itself is absent. Blocks all multi-agent workflows — the primary differentiator of tinycode.

- **Requires:** Background job system, child session creation, permission derivation (spec 5.7, 14.5)
- **Location:** `internal/tool/task.go` (new file)
- **Complexity:** Large

### 2. Only OpenAI-compatible LLM protocol

The LLM client (`internal/llm/openai.go`) supports only the OpenAI Chat Completions wire format. The spec (Section 28) defines 7 protocol implementations:

- OpenAI Chat Completions — **implemented**
- OpenAI Compatible Chat — **implemented** (same client)
- Anthropic Messages API — **missing**
- AWS Bedrock Converse — **missing**
- AWS Bedrock Event Stream — **missing**
- Google Gemini API — **missing**
- OpenAI Responses API — **missing**

Cannot use cloud providers that don't offer OpenAI-compatible endpoints. OpenRouter provides a workaround for some.

- **Location:** `internal/llm/anthropic.go` etc. (new files)
- **Complexity:** Large

### 3. Edit tool has 1 of 9 fuzzy matching strategies

`internal/tool/edit.go:138-166` implements only a single strategy (first-line prefix match via `fuzzyFind()`). The spec requires 9 cascading strategies:

1. SimpleReplacer (exact match)
2. LineTrimmedReplacer
3. BlockAnchorReplacer (Levenshtein similarity)
4. WhitespaceNormalizedReplacer
5. IndentationFlexibleReplacer
6. EscapeNormalizedReplacer
7. TrimmedBoundaryReplacer
8. ContextAwareReplacer
9. MultiOccurrenceReplacer

Small models (8-14B) produce whitespace-mismatched edits constantly. Each failure wastes a full inference cycle. This is the highest-ROI improvement for small model reliability.

- **Location:** `internal/tool/edit.go`
- **Complexity:** Large

### 4. No `tinycode run` command

Zero flag parsing — the CLI is a simple `os.Args[1]` switch. No non-interactive mode, no interactive direct mode, no `--format json` for CI output. Blocks CI/CD integration and scripting.

- **Location:** `cmd/tinycode/main.go`
- **Complexity:** Medium

### 5. No `invalid` tool

When JSON repair fails on malformed tool calls, there's no graceful fallback. The repaired-but-still-bad JSON gets passed to the real tool and fails confusingly. The spec routes these to an `invalid` tool that returns a helpful error to the LLM.

- **Location:** `internal/tool/invalid.go` (new file)
- **Complexity:** Small

### 6. No doom-loop detection

No detection of identical consecutive tool calls with the same name and input. Small models can loop infinitely calling the same tool, burning inference budget. The spec requires a configurable threshold (default 3) that triggers a permission prompt.

- **Location:** `internal/session/processor.go` (in the main loop)
- **Complexity:** Small

### 7. No auto-continue for small models

When small models stop prematurely after tool calls (finish_reason=stop instead of tool_calls), the spec injects a nudge message to continue. Max nudges: 3 (configurable via `experimental.auto_continue`). Without this, small models frequently stop mid-task.

- **Location:** `internal/session/processor.go:198-204`
- **Complexity:** Small

### 8. No permission prompting in processor

Tools execute directly without an ask/approve cycle in the session processor. The permission service exists (`internal/permission/`) and the TUI handles permission UI, but the processor doesn't call `permission.Ask()` before tool execution. All tools are effectively auto-approved, with only simple regex detection for destructive shell commands.

- **Location:** `internal/session/processor.go` (before `executeTools`)
- **Complexity:** Medium

---

## P1 — Significant Feature Gaps

### 9. 11 missing tools

| Tool | Spec Section | Notes |
|------|-------------|-------|
| task (subagent) | 10.2 | See P0-1 above |
| websearch | 10.2 | Requires external search provider integration (Exa/Parallel) |
| skill | 12 | Skills are discovered but no tool to invoke them from sessions |
| todowrite | 10.2 | Schema exists (todo table) but tool is missing |
| lsp | 10.2 | 9 LSP operations; experimental |
| swarm | 10.3 | tmux-based multi-agent coordination |
| apply_patch | 10.2 | GPT-specific unified patch tool |
| repo_clone | 10.2 | Experimental, scout agent dependency |
| repo_overview | 10.2 | Experimental |
| plan_exit | 10.2 | Experimental plan mode |
| invalid | 10.2 | See P0-5 above |

### 10. Partially implemented tools

- **edit:** Missing BOM handling, CRLF detection/preservation, auto-formatting after write, per-file semaphore locking.
- **bash/shell:** Regex-based destructive detection instead of AST-based parsing. No secrets file detection. No cross-platform PowerShell support.
- **webfetch:** 512KB limit vs spec's 5MB. No HTML-to-markdown conversion. No PDF text extraction. No Cloudflare bot detection retry. Missing `format` parameter.
- **read:** No binary detection with byte analysis. No image base64 return. No PDF support.
- **grep/glob:** No modification-time sorting (spec says newest first).

### 11. ~57 missing HTTP routes

| Category | Missing Count | Key Routes |
|----------|--------------|------------|
| PTY | 8 | Full WebSocket terminal section |
| TUI Control | 14 | External TUI control API |
| Experimental | 11 | Worktree, console, cross-project sessions |
| MCP management | 7 | Add, auth, connect, disconnect servers |
| Auth | 2 | PUT/DELETE `/auth/:providerID` |
| Session | 6 | Share, shell, message detail, part update |
| Provider OAuth | 3 | Authorize, callback, auth listing |
| V2 API | 3 | Versioned model/provider endpoints |
| Other | 3+ | Logging, formatter status, LSP status |

### 12. Partially implemented routes

- `PATCH /session/{id}` — only supports `title`; spec also requires `permissions` and `archiveTime`
- `POST /session/{id}/fork` — copies ALL messages; ignores `messageID` parameter for fork-point truncation
- `POST /session/{id}/message` (sync) — publishes bus event but does NOT stream response back via HTTP
- `POST /session/{id}/summarize` — publishes events but does NOT run LLM compaction (`handler_stub.go`)

### 13. Session fork ignores messageID

Fork copies all messages instead of truncating at the specified message. The fork feature is unusable for its intended purpose (branching at a specific point).

- **Location:** `internal/server/handler_session.go`
- **Complexity:** Small

### 14. Plugin system broken

Confirmed in CLAUDE.md known issues:
- Wire protocol mismatch between `internal/plugin/` (server) and `pkg/plugin/` (SDK) — different JSON-RPC method names, mismatched initialize field names
- Plugin-provided tools are silently dropped (tool manifests captured but never registered)
- oh-my-tiny plugin (23 tools) not implemented

- **Complexity:** Large

### 15. Compaction incomplete

- No compaction model selection (always uses session model; spec says: compaction agent model -> `small_model` config -> session model)
- No auto-continue after compaction (replay user message or inject synthetic continue)
- No pruning system (separate from compaction; prune old tool outputs based on token budget)
- No text truncation limits during serialization (2000 chars for text, 500 for tool JSON, 2000 for tool output)
- No telemetry logging of compaction metrics

- **Location:** `internal/session/compaction.go`
- **Complexity:** Medium

### 16. System prompt assembly incomplete

- No AGENTS.md discovery (only CLAUDE.md)
- No tool descriptions injected into system prompt
- No environment/context injection (git status, platform info, directory)
- No wiki hints injection

- **Location:** `internal/session/prompt.go`
- **Complexity:** Medium

### 17. Missing providers

- ramalama (via `TINYCODE_RAMALAMA_HOST`)
- MaaS/LiteLLM (via `TINYCODE_MAAS_HOST`)
- Kubernetes in-cluster discovery (Services API, KServe annotation detection, port probing)
- Remote model catalog fetching (`TINYCODE_MODELS_URL` with caching, TTL, fallback chain)

- **Location:** `internal/provider/discovery.go`
- **Complexity:** Medium

### 18. Thread-unsafe MCP tool registration

`session_manager.go:812-816` registers MCP tools into the shared `tool.Registry` during `processPrompt`. Not thread-safe across concurrent sessions. Each session should get its own tool registry copy.

- **Complexity:** Small

### 19. shouldPoll prevents model list updates

`discovery.go:97-104` returns false once a provider is registered, preventing detection of newly pulled or removed Ollama models. Should re-poll registered providers to detect model changes.

- **Complexity:** Small

### 20. No snapshot/revert tracking

Git stash workaround exists (`internal/server/revert.go`) but no proper per-message filesystem snapshots. Cannot revert individual message changes reliably. Spec requires pre-step filesystem capture.

- **Location:** `internal/session/processor.go` + `internal/session/snapshot.go` (new)
- **Complexity:** Medium

---

## P2 — Nice-to-Have

| # | Gap | Notes |
|---|-----|-------|
| 21 | Missing DB tables (7) | session_message, workspace, account, account_state, event, event_sequence, data_migration |
| 22 | Missing config fields (~15) | experimental flags, formatter, lsp, attachment.image, command map, skills.paths/urls |
| 23 | No proxy support | http_proxy, NO_PROXY env vars |
| 24 | No OpenTelemetry | OTEL_EXPORTER_OTLP_ENDPOINT, span tracing |
| 25 | Missing TUI keybindings (25+) | Only 5 of 30+ wired; sidebar selection broken |
| 26 | No file watcher | experimental feature, platform-native backends |
| 27 | Missing CLI subcommands (14) | export, models, agent, providers, session, setup, status, mcp, plugin, debug, generate, uninstall, db, import |
| 28 | Retry-after header parsing | Currently only exponential backoff, no retry-after-ms or HTTP date |
| 29 | No compression middleware | gzip/deflate for responses over 1024 bytes |
| 30 | Shell tool AST parsing | Regex-based destructive detection should be AST-based with bash/PowerShell grammars |
| 31 | Temperature/topP/maxTokens not wired | Request struct has fields but never populated from agent config or model defaults |
| 32 | No step-start/step-finish events | Processor treats each LLM call as opaque; no per-step token usage tracking |
| 33 | No delta batching (16ms) | Text/reasoning deltas published individually, not batched for SSE efficiency |
| 34 | Default permission rules incomplete | Missing doom_loop, guardrail, external_directory, .env file rules from spec |
| 35 | Permission persistence | No storage of "always" approvals to database per project |
| 36 | Dead TUI components | DiffView, Workspace, PluginHooks, ThemeLoader, render/diff.go — exist but never wired |

---

## P3 — Can Defer

| # | Gap | Notes |
|---|-----|-------|
| 37 | Internationalization | N/A for TUI; web UI is TypeScript |
| 38 | Workspaces (experimental) | Gated behind feature flag |
| 39 | V2 event/schema system | Migration path; V1 routes work |
| 40 | OpenAPI spec generation | `tinycode generate` |
| 41 | Named references/directories | `@alias` in conversations |
| 42 | Custom tools discovery | Auto-load from `tool/*.{js,ts}` directories |
| 43 | Image processing | Attachment resize/normalization |
| 44 | oh-my-tiny plugin | 23 tools across state/notepad/wiki/ast-grep |

---

## Architectural Risks

### Thread-unsafe MCP tool registration
`session_manager.go:812-816` registers MCP tools into the shared `tool.Registry` during `processPrompt`. Concurrent sessions would cause data races. Fix: per-session registry copy or immutable snapshot pattern.

### shouldPoll blocks re-polling
`discovery.go:97-104` returns false once a provider is registered. This prevents detecting new or removed models from Ollama/vLLM/LM Studio. The original fix for noisy polling went too far — it should still re-poll registered providers.

### Proactive overflow detection missing
Overflow is only detected when the LLM returns an error. The spec checks after each step: tokens used >= (model input limit - reserved buffer). This wastes an inference call on every overflow.

### No step boundary tracking
The processor treats each LLM call as opaque. No step-start/step-finish events means no per-step token usage display and no proactive overflow check between steps.

---

## What Works Well

Both reviewers noted these strengths:

- **Clean Go architecture** following standard layout with well-separated packages
- **Session processor** core loop with concurrent tool execution (WaitGroup), retry logic (29 retryable patterns), and compaction
- **Provider discovery** with Ollama auto-profiling (GPU detection, KV cache math, profile creation), warmup probes, and failure tracking with dormant state
- **Event bus** with sliding-window buffer (4096 capacity), typed + wildcard subscriptions
- **Config system** with multi-source merging, JSONC support, forward compatibility
- **Agent system** with 22 agents, 14 compact variants, per-agent permissions, frontmatter parsing
- **MCP client** with all 3 transports (stdio, SSE, StreamableHTTP), OAuth PKCE flow
- **TUI** working end-to-end with chat, prompt, sidebar, status bar, permission dialog, toast overlay

---

## Recommended Priority Order

Based on both reviews' convergence:

1. **Small wins:** `invalid` tool, doom-loop detection, auto-continue (all Small, big reliability impact)
2. **Edit fuzzy matching** (Large, highest ROI for small model usability)
3. **Permission integration in processor** (Medium, safety-critical)
4. **`task` tool + background job system** (Large, unlocks multi-agent)
5. **`tinycode run` with basic flags** (Medium, unlocks CI/scripting)
6. **Missing tools:** skill, todowrite, websearch (Medium each)
7. **Anthropic Messages API** (Large, opens direct cloud access)
8. **Remaining routes and CLI subcommands** (incremental)
