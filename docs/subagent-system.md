# Subagent System Design

Native goroutine-based multi-agent orchestration — tinycode-go's primary differentiator from the TypeScript original.

## Build Agent Delegation

The default **build** agent handles simple tasks inline but delegates complex work to specialized subagents via the `task` tool:

- **executor** — implementation work (code changes, refactors, applying fixes)
- **architect** — design decisions, API design, system-level trade-offs
- **critic** — multi-perspective quality review with gap analysis and pre-mortem

The build agent's prompt (in `internal/agent/defaults/build.txt`) contains delegation rules specifying when to use each subagent and how to write specific task prompts for them.

## Architecture

```
User prompt ("/swarm run tests on 3 packages")
    │
    ▼
┌─────────────────────────────────────────────┐
│  TUI / Web UI                               │
│  expandSlashCommand() injects SWARM prefix  │
│  with delegation rules and examples         │
└──────────────┬──────────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────────┐
│  Session Processor (parent)                 │
│  Sends expanded prompt to LLM              │
│  LLM returns N "task" tool calls           │
│  executeTools() runs them in parallel       │
└──────────────┬──────────────────────────────┘
               │  N parallel goroutines
               ▼
┌──────────┐ ┌──────────┐ ┌──────────┐
│ task tool │ │ task tool │ │ task tool │
│ executor  │ │ executor  │ │ executor  │
│ -A        │ │ -B        │ │ -C        │
└─────┬────┘ └─────┬────┘ └─────┬────┘
      │            │            │
      ▼            ▼            ▼
┌──────────┐ ┌──────────┐ ┌──────────┐
│ Processor │ │ Processor │ │ Processor │
│ (child)   │ │ (child)   │ │ (child)   │
│ own LLM   │ │ own LLM   │ │ own LLM   │
│ own tools  │ │ own tools  │ │ own tools  │
└──────────┘ └──────────┘ └──────────┘
      │            │            │
      ▼            ▼            ▼
  Results collected by parent, synthesized into report
```

## Key Components

### Task Tool (`internal/tool/task.go`)

The LLM-callable tool that spawns subagents. Two modes:

- **Foreground** (default): Blocks the calling goroutine until the subagent completes. Returns the subagent's text response directly. Used by `/swarm`.
- **Background**: Wraps execution in `JobManager.Start()`, returns a job ID immediately. LLM polls with `task_id` to check status.

Parameters: `description`, `prompt`, `subagent_type` (agent name), `task_id` (for polling), `background` (bool).

No permission required — the task tool is a coordinator. Subagents' own tool calls trigger their own permission checks.

### SubagentRunner (`internal/tool/tool.go`)

A callback on `tool.Context` that decouples the tool layer from the server layer:

```go
type SubagentRunnerFunc func(ctx context.Context, parentSessionID string, parentDepth int, prompt, agent, directory string) (string, error)
```

Set in `cmd/tinycode/serve.go` and `tui.go` after server creation. The closure calls `Server.RunSubagent()` which delegates to `SessionManager.RunSubagent()`.

### RunSubagent (`internal/server/session_subagent.go`)

Creates and runs a child session processor:

1. Resolves the model from the parent session's DB record, or falls back to the first connected provider
2. Loads the agent's system prompt (e.g., executor, explore, critic)
3. Creates a tool registry copy with incremented `SubagentDepth`
4. Creates a `session.Processor` with a 5-iteration max and 2-minute timeout
5. Runs `proc.Process(ctx, prompt)` in the calling goroutine
6. Extracts assistant text from the result messages
7. Returns the text to the parent

Each subagent gets a distinct label: `executor-A`, `executor-B`, `explore-C` (sequential counter with letter suffix). Session IDs are `parentID:executor-A`.

### Slash Command Expansion (`internal/tui/run.go`)

`/swarm` and `/work-loop` are expanded into instructed prompts before sending to the LLM:

- **`/swarm <task>`** → Injects SWARM mode instructions telling the LLM to delegate ALL work via the task tool, make multiple task calls in a single response for parallelism, and synthesize results after all complete.
- **`/work-loop <task>`** → Injects the read-act-verify-repeat protocol for iterative task completion.

## Safety Guards

### Depth Limit

Subagents can spawn nested subagents, but depth is bounded:

- `maxSubagentDepth = 5` (const in task.go)
- Each child gets `parentDepth + 1` via `Registry.WithDepth()`
- The depth check at `executeTask()` rejects spawning when `tc.SubagentDepth >= maxSubagentDepth`

### Concurrent Limit

- `maxConcurrentSubagents = 8` (const in task.go)
- Atomic counter (`SubagentCount *atomic.Int32`) on `tool.Context`, shared across all copies
- Incremented before spawn, decremented on completion (foreground: defer, background: job goroutine)
- Rejects with clear error when limit reached

### Session Budget

- Initial budget: 20 subagent spawns per session
- Atomic counter (`SubagentBudget *atomic.Int32`) on `tool.Context`, shared across all copies
- Decremented before spawn, never restored
- Prevents unbounded cost from repeated `/swarm` calls

### Iteration & Timeout

- Subagent processors limited to `maxIterations = 5` (const in session_subagent.go)
- 2-minute `context.WithTimeout` wraps the processor run
- Prevents small models from running away with endless tool call loops

### Panic Recovery

Three layers of defense:

1. **`RunSubagent()`** — named return + `defer/recover` catches panics from the processor
2. **`executeTools()`** — each tool goroutine has `defer/recover`, converts panic to error tool result
3. **`callSubagentRunner()`** — wrapper in task.go catches panics from the runner itself

### Permission & Directory Boundary

- Task tool has `Permission: ""` — no permission prompt for delegation itself
- Subagent processors receive `Perms` in their ProcessorConfig
- `checkExternalDirectory()` enforces path boundaries — subagents can only access files within the project directory
- Subagent tool calls (bash, read, edit) trigger their own permission checks via the shared `permission.Service`

### File Mutex Safety

- `ClearFileMutexes()` removed from per-prompt defer — was racing with concurrent subagents
- Cleanup now happens only in `SessionManager.Shutdown()` where no concurrent operations remain
- File-level mutual exclusion works correctly across parallel subagents

### Background Job Lifecycle

- `JobManager.Start()` derives context from parent — jobs cancel when session aborts
- `JobManager.Shutdown()` cancels all running jobs — called from `SessionManager.Shutdown()`
- Background jobs don't survive server shutdown

## Event Routing

- Subagent events use synthetic session IDs (`parentID:executor-A`)
- SSE filters skip events with `:` in session ID — prevents client confusion
- Event bridge forwards subagent events to parent session with label prefix for UI visibility

## Implementation Waves

### Wave 1: Critical Safety (Done)
- #220: SubagentDepth propagation via `WithDepth()` + depth check enforcement
- #221: ClearFileMutexes race fix — moved to Shutdown()
- #222: Panic recovery at 3 layers (RunSubagent, tool goroutines, callSubagentRunner)
- #232: Removed dead ProcessorConfig.SubagentDepth field

### Wave 2: Spawn Controls (Done)
- #225: Concurrent subagent limit (max 8) via atomic semaphore
- #237: Per-session spawn budget (20) via atomic decrement
- #226: Perms wired to ProcessorConfig — directory boundary enforcement
- #224: Background job context from parent + Shutdown() method

### Wave 3: Event Routing & UX (In Progress)
- #227: Forward subagent events to parent session with label
- #231: Filter synthetic IDs from SSE stream
- #223: Fix pendingPrompt to store expanded text on first use

### Wave 4: Server-side Commands & Job Lifecycle
- #228: Move expandSlashCommand to server layer (Web UI support)
- #229: Job eviction after TTL
- #239: Background job completion notification via bus event

### Wave 5: Subagent Quality
- #230: Subagents get MCP tools
- #233: Inherit parent's LLM params (temperature, topP, maxTokens)
- #238: Aggregate subagent token usage to parent session
- #236: Default agent changed from "build" to "executor"

### Wave 6: Polish
- #234: Non-wrapping subagent label counter
- #235: Reorder task_id check before prompt emptiness check

## Why Go

The TypeScript version required tmux to run multiple agents — each needed its own Node.js process. In Go:

- Subagents are goroutines (~4KB each), not processes
- Shared memory with proper synchronization (atomic counters, mutexes)
- Context cancellation propagates through the entire subagent tree
- Single binary, single process, no IPC serialization
- 50 parallel subagents add negligible memory overhead
