# tinycode: TypeScript to Go Migration Plan

## Decisions

These four gating decisions were made 2026-09-03 after architect, analyst, and critic review of the full codebase:

1. **TUI stays TypeScript.** The TUI is a network client (HTTP+SSE on port 4096). It doesn't import server internals. Keeping it unchanged gives us an integration test harness from day one. A minimal Go TUI (bubbletea) is a stretch goal after the server is complete.

2. **Plugins deferred.** The 28+ hook plugin system with dynamic JS loading is architecturally incompatible with Go's static binary model. Design decision postponed until the core server is stable.

3. **LLM providers: Ollama, LM Studio, vLLM, OpenAI-compatible, OpenRouter.** These cover local-first use cases. Cloud providers (Anthropic native, Bedrock, Google, Azure) added incrementally later. All five share the OpenAI-compatible chat completions wire protocol, except OpenRouter which adds discovery and cost tracking.

4. **No protobuf/gRPC.** Keep REST+SSE+OpenAPI. No current client speaks gRPC. The existing API contract (114 endpoints, 56 SSE event types) is the migration target.

## Migration Strategy: Parallel Rewrite

Strangler Fig is impractical here. The TypeScript server uses Effect as its runtime framework (300+ files, 1,864 usages of core patterns like `Context.Service`, `Layer.effect`, `Effect.gen`). Effect's Layer composition creates dependency trees where every service requires 5-12 other services — you cannot extract one module without its entire graph.

Instead: build the Go server as a parallel replacement. Validate by running the existing TypeScript TUI and web frontend against the Go server. The OpenAPI spec (`packages/sdk/openapi.json`, 18,400 lines) and SSE event contracts are the acceptance criteria.

**What stays TypeScript:**
- `packages/app/` — SolidJS web frontend (64K lines)
- `packages/desktop/` — Electron shell (4K lines)
- `packages/tinycode/src/cli/cmd/tui/` — TUI (46K lines including run/)
- `packages/sdk/` — auto-generated TypeScript SDK (regenerated from Go server's OpenAPI)
- `packages/ui/` — shared component library

**What gets rewritten in Go:**
- HTTP API server (114 endpoints, SSE, WebSocket)
- Session processor (LLM streaming, tool call coordination, retry, compaction)
- Agent system (8 native agents, custom agent loading, permission scoping)
- Tool system (19 core tools)
- Provider system (5 providers for v1, discovery, Ollama profiling)
- LLM client (OpenAI-compatible streaming, tool call JSON repair)
- Storage (SQLite via sqlc, same schema as current Drizzle)
- Config (JSONC parsing, multi-source merge, forward compatibility)
- Event bus (typed pub/sub, 56 event types)
- Permission system (rule evaluation, blocking ask/reply)
- MCP client (stdio/SSE/StreamableHTTP transports)
- ACP service (IDE integration via stdio JSON-RPC)

## Go Package Layout

```
tinycode-go/
├── cmd/
│   └── tinycode/
│       └── main.go                 # CLI entry, server startup
├── internal/
│   ├── server/                     # net/http server, routing, SSE, WebSocket
│   │   ├── server.go               # Listen, graceful shutdown (25s timeout)
│   │   ├── router.go               # Route registration (Go 1.22+ mux)
│   │   ├── middleware/             # CORS, compression, security headers
│   │   └── handler/               # Route handlers by domain
│   │       ├── session.go          # Session CRUD, prompt, abort, fork
│   │       ├── provider.go         # Provider/model listing
│   │       ├── permission.go       # Permission ask/reply
│   │       ├── mcp.go              # MCP server management
│   │       ├── config.go           # Config read/update
│   │       ├── file.go             # File operations
│   │       ├── event.go            # SSE event stream
│   │       └── pty.go              # PTY WebSocket
│   ├── session/                    # Core domain
│   │   ├── session.go              # Session CRUD, Info type
│   │   ├── processor.go            # LLM streaming loop, tool coordination
│   │   ├── compaction.go           # Context compaction/summarization
│   │   ├── retry.go                # Exponential backoff + 25% jitter
│   │   ├── prompt.go               # System prompt construction
│   │   ├── message.go              # Message/Part types
│   │   └── overflow.go             # Context window detection
│   ├── agent/                      # Agent definitions
│   │   ├── agent.go                # Registry, resolution, compact variants
│   │   ├── defaults/               # Embedded .md prompts (embed.FS)
│   │   └── permission.go           # Subagent permission scoping
│   ├── tool/                       # Tool implementations
│   │   ├── tool.go                 # Definition interface, registry
│   │   ├── registry.go             # Permission-filtered resolution
│   │   ├── read.go                 # File/image/PDF reading
│   │   ├── write.go                # File writing (BOM preservation)
│   │   ├── edit.go                 # 9-strategy fuzzy matching cascade
│   │   ├── shell.go                # Bash execution, destructive detection
│   │   ├── grep.go                 # ripgrep wrapper
│   │   ├── glob.go                 # File listing
│   │   ├── task.go                 # Subagent spawning (depth limit)
│   │   ├── webfetch.go             # HTML-to-markdown, PDF support
│   │   ├── websearch.go            # Exa/Parallel providers
│   │   ├── lsp.go                  # 9 LSP operations
│   │   ├── question.go             # Multi-question UI prompts
│   │   ├── todowrite.go            # Session task list
│   │   └── truncate.go             # Output truncation (2000 lines/50KB)
│   ├── provider/                   # LLM provider abstraction
│   │   ├── provider.go             # Registry, model resolution
│   │   ├── model.go                # Model type, capabilities, pricing
│   │   ├── discovery.go            # Background polling (30s, 2s probe timeout)
│   │   ├── ollama.go               # Auto-profiling, warmup, num_ctx
│   │   ├── openrouter.go           # Auto-discovery, cost tracking
│   │   ├── retry.go                # 30 error patterns, retryable classification
│   │   └── error.go                # Overflow detection (15 patterns)
│   ├── llm/                        # LLM streaming client
│   │   ├── client.go               # Interface, Request/Event types
│   │   ├── openai.go               # OpenAI-compatible SSE streaming
│   │   ├── stream.go               # SSE parser
│   │   └── event.go                # Event types (text-delta, tool-call, etc.)
│   ├── storage/                    # SQLite via sqlc
│   │   ├── db.go                   # Connection, WAL, pragmas, migrations
│   │   ├── queries/                # sqlc-generated code
│   │   └── migrations/             # SQL migration files (embed.FS)
│   ├── config/                     # Configuration
│   │   ├── config.go               # Type, multi-source loading/merging
│   │   ├── paths.go                # XDG-compliant path resolution
│   │   └── parse.go                # JSONC, variable substitution
│   ├── bus/                        # Event bus
│   │   ├── bus.go                  # Channel-based pub/sub (4096 buffer)
│   │   └── event.go                # 56 event type definitions
│   ├── permission/                 # Permission system
│   │   ├── permission.go           # Blocking ask/reply via channels
│   │   └── evaluate.go             # Wildcard rule matching
│   ├── mcp/                        # Model Context Protocol
│   │   ├── client.go               # MCP client, transport selection
│   │   ├── stdio.go                # Stdio transport
│   │   └── sse.go                  # SSE/StreamableHTTP transport
│   ├── acp/                        # Agent Client Protocol
│   │   ├── service.go              # Session management, prompt relay
│   │   └── transport.go            # Stdio JSON-RPC
│   └── id/                         # Prefixed ascending ID generation
│       └── id.go
├── pkg/
│   └── types/                      # Public API types (JSON tags for tygo)
│       ├── session.go
│       ├── provider.go
│       └── event.go
├── sql/
│   ├── schema.sql                  # Full schema definition
│   └── queries/                    # sqlc query definitions
│       ├── session.sql
│       ├── message.sql
│       ├── part.sql
│       ├── permission.sql
│       └── todo.sql
├── sqlc.yaml
├── go.mod
├── go.sum
└── Makefile
```

## Architectural Guidelines

### Standard Library First

Use `net/http` (Go 1.22+ mux with method patterns) for routing. `nhooyr.io/websocket` for WebSocket. SSE via `http.Flusher` (no library). `log/slog` for structured logging. `encoding/json` for serialization. `os/exec` for shell tool. `embed` for static assets and migrations.

Justified third-party dependencies:
- `modernc.org/sqlite` — pure Go SQLite (no CGO, cross-compilation)
- `nhooyr.io/websocket` — cleaner WebSocket API than gorilla
- `github.com/sqlc-dev/sqlc` — SQL-first code generation (build tool)
- `github.com/fsnotify/fsnotify` — config file watching
- `github.com/santhosh-tekuri/jsonschema` — tool parameter validation (0 deps)
- `github.com/modelcontextprotocol/go-sdk` — MCP client

### Concurrency Model

| TypeScript Pattern | Go Equivalent |
|---|---|
| Effect `Deferred` (blocking promise) | `chan struct{}` or typed channel |
| Effect PubSub (sliding 4096 buffer) | Buffered channel, non-blocking send, drop oldest |
| Effect `Scope` (resource cleanup) | `context.Context` + `defer` |
| Effect `Layer` (dependency injection) | Constructor injection, wire in `main.go` |
| `streamText()` streaming | `<-chan Event` from HTTP SSE parser |
| `Deferred` for permission ask/reply | Channel pair: ask sends, blocks on reply channel |
| Background title generation | `go func()` with context cancellation |
| Provider discovery polling | `time.Ticker` goroutine (30s interval) |
| Per-session serialization (Runner) | `sync.Mutex` per session, or channel-serialized command queue |
| Tool call concurrency | `errgroup.Group` with shared context |

### Error Handling

Go's `if err != nil` replaces Effect's typed error channels. Define sentinel errors for API boundaries:

```go
var (
    ErrNotFound         = errors.New("not found")
    ErrPermissionDenied = errors.New("permission denied")
    ErrContextOverflow  = errors.New("context overflow")
    ErrSessionBusy      = errors.New("session busy")
    ErrAborted          = errors.New("aborted")
    ErrDepthLimit       = errors.New("subagent depth limit exceeded")
)
```

Retry error classification: port the 30 regex patterns from `provider/error.ts` (network failures, timeouts, rate limits, server errors, response body patterns). Max 5 retries, exponential backoff (initial 2s, factor 2, max 30s), 25% jitter.

### Database (sqlc + SQLite)

Zero ORM. Write raw SQL, generate Go code with sqlc. Schema matches existing Drizzle schema (11 tables: project, session, message, part, todo, permission, plus indexes). SQLite pragmas: WAL mode, busy_timeout=5000, cache_size=-64000, foreign_keys=ON.

Driver: `modernc.org/sqlite` (pure Go) for cross-compilation. Switch to `mattn/go-sqlite3` (CGO) only if performance requires it.

### Tool Call JSON Repair

Port `sanitizeToolCallJson` from `session/llm.ts`: strip markdown fences (` ```json ... ``` `), fix trailing commas before `}` or `]`. This is critical — LLMs frequently produce malformed JSON in tool calls.

### Edit Tool Fuzzy Matching

The 9-strategy cascade (SimpleReplacer through ContextAwareReplacer with Levenshtein distance) is a core UX feature. LLMs rely on fuzzy matching for code edits. Port all 9 strategies — simplifying this causes visible regressions.

## Behavioral Contracts (must match exactly)

These implicit behaviors are not in any spec — they were discovered by reading source:

| Behavior | Value | Source |
|---|---|---|
| SSE heartbeat interval | 10 seconds | `server.ts` |
| Tool output truncation | 2000 lines / 50KB | `truncate.ts` |
| Compaction preserve budget | MIN 2K, MAX 15K tokens | `compaction.ts` |
| Compaction circuit breaker | Warning after 3+ compactions | `processor.ts` |
| Tool cleanup timeout | 250ms wait for pending tools | `processor.ts` |
| Delta event batching | 16ms | `processor.ts` |
| Provider probe timeout | 2s (5s for OpenRouter) | `local-discovery.ts` |
| Discovery poll interval | 30 seconds | `local-discovery.ts` |
| Retry initial delay | 2 seconds | `retry.ts` |
| Retry backoff factor | 2x with 25% jitter | `retry.ts` |
| Retry max delay | 30s (no headers), max-int32 (with headers) | `retry.ts` |
| Max retries | 5 | `provider.ts` |
| Header/chunk timeout | 5 minutes | `provider.ts` |
| Graceful shutdown | 25 second drain | `server.ts` |
| Subagent depth limit | 1 (default) | `config` |
| Edit tool semaphore | Per-file mutual exclusion | `edit.ts` |
| Permission rejection cascade | Reject all pending asks for session | `permission/index.ts` |
| Compact variant threshold | Model size <= 8B parameters | `agent.ts` |
| Cursor pagination | 50/page, newest-first, base64url cursor | `session.ts` |
| Ollama profile naming | `{model}-tc{N}k` | `ollama-profile.ts` |

## Migration Phases

### Phase 1: Foundation (Weeks 1-4)
**Packages:** `internal/storage`, `internal/config`, `internal/bus`, `internal/id`

These are leaf dependencies. Build and test independently.
- sqlc queries matching Drizzle schema, migration runner with embed.FS
- Config loading: JSONC parsing, multi-source merge (global, project, workspace), env var substitution, forward compatibility (ignore unknown fields)
- Event bus: channel-based pub/sub with 56 event types, 4096 sliding buffer
- ID generation: prefixed (`ses`, `msg`, `prt`), monotonically ascending

**Acceptance gate:** Config loads the same `config.json` format. DB reads/writes the same SQLite schema. Bus publishes and subscribes correctly under concurrent load.

### Phase 2: Core Domain (Weeks 5-10)
**Packages:** `internal/permission`, `internal/agent`, `internal/provider`, `internal/llm`

Middle tier — depends on Phase 1, consumed by session processor.
- Permission: wildcard rule evaluation, blocking ask/reply via channels, persistent approvals in SQLite
- Agent: embedded prompt loading (embed.FS), config overlay, compact variant selection for <=8B models
- Provider: Ollama/vLLM/LM Studio/OpenRouter discovery polling, Ollama auto-profiling (GPU detection, KV cache math, Modelfile generation), model warmup
- LLM client: OpenAI-compatible SSE streaming, tool call extraction, JSON repair, usage tracking

**Acceptance gate:** Provider discovery finds local Ollama models. LLM client streams a response with tool calls. Permission blocks and resumes on reply.

### Phase 3: Session Processor (Weeks 11-16)
**Packages:** `internal/session`, `internal/tool`

The heart of the system. Depends on everything from Phases 1-2.
- Session CRUD with full DB operations
- Processor loop: stream LLM events, coordinate concurrent tool calls via errgroup, handle abort, trigger compaction on overflow
- All 19 tool implementations including edit fuzzy matching, shell with destructive detection, file ops with BOM/CRLF preservation
- Compaction: lazy tail estimation, observation masking, deterministic file tracking, circuit breaker
- Retry: exponential backoff with 30 error patterns
- Subagent spawning with depth limiting

**Acceptance gate:** End-to-end test: create session, send prompt, receive streamed response with tool calls, verify tool side effects, verify compaction triggers correctly.

### Phase 4: HTTP Server (Weeks 17-20)
**Packages:** `internal/server`

Wrap the core domain for frontend consumption.
- All 114 route handlers matching the OpenAPI spec
- SSE event streaming (56 event types, 10s heartbeat)
- WebSocket for PTY connections
- Graceful shutdown with 25s drain
- OpenAPI spec generation matching `packages/sdk/openapi.json`

**Acceptance gate:** TypeScript TUI and web frontend connect to Go server and function identically. Run the existing test suite against the Go server.

### Phase 5: Integration Protocols (Weeks 21-24)
**Packages:** `internal/mcp`, `internal/acp`

Integration points — can run alongside TypeScript during transition.
- MCP client with stdio/SSE/StreamableHTTP transports, tool conversion
- ACP service with stdio JSON-RPC for IDE integration
- MCP OAuth flow (PKCE, callback server on port 19876, 5-minute timeout)

**Acceptance gate:** MCP servers connect and provide tools. VS Code extension works via ACP.

### Phase 6: Distribution (Weeks 25-28)
- Web UI static assets embedded via embed.FS
- Cross-platform build (linux/darwin amd64+arm64, windows amd64)
- Updated install.sh for Go binary distribution
- TypeScript SDK regenerated from Go server's OpenAPI spec
- CI/CD pipeline

**Acceptance gate:** Single binary serves the web UI, API, and all protocols. Install script works on all platforms.

## Reference Documents

- `plans/go-architecture.md` — full subsystem designs with Go interfaces and struct definitions
- `plans/go-requirements.md` — 114 endpoints, 19 tools, 56 events, acceptance criteria, edge cases
- `plans/go-critique.md` — risk registry, failure scenarios, kill criteria
- `packages/sdk/openapi.json` — API contract (18,400 lines, 114 endpoints)

## Kill Criteria

Abandon or pivot the migration if:
- Phase 3 (session processor) takes >2x estimated time, indicating the complexity is worse than assessed
- The LLM streaming + tool call lifecycle cannot be made reliable without an SDK abstraction layer
- Performance benchmarks show no meaningful improvement over TypeScript/Bun for I/O-bound workload
- The edit tool's fuzzy matching cascade cannot achieve equivalent success rates in Go
