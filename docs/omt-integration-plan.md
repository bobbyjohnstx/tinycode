# oh-my-tiny Integration Plan

> Hybrid plugin + MCP integration of oh-my-tiny into tinycode.
> Each phase is independently deployable and leaves both projects working.

## Context

**tinycode** (`/Users/bjohns/projects/tinycode`) is a local-LLM-first AI coding assistant (Bun monorepo with Turborepo).
**oh-my-tiny** (`/Users/bjohns/projects/oh-my-tiny`) is an agent orchestration toolkit providing 25+ tools (state management, notepad, project memory, wiki, LSP, AST-grep, team runtime).

oh-my-tiny currently integrates via:
- A stdio MCP server (`node ./dist/mcp/server.js`) registered in config
- Two plugin files (`.opencode/plugins/context-injector.ts`, `.opencode/plugins/skill-detector.ts`)

This plan migrates to a first-class hybrid integration: a proper tinycode plugin that registers tools natively (faster, typed, no MCP serialization overhead) while keeping the MCP server for non-tinycode clients.

## Work Objectives

1. Make oh-my-tiny tools available in tinycode immediately (Phase 0)
2. Create a native plugin entry point for oh-my-tiny (Phase 1)
3. Wire context injection, skill detection, and permission hooks into the plugin (Phase 2)
4. Replace tmux-based team runtime with tinycode HTTP API (Phase 3)
5. Add a `tinycode omt` CLI subcommand for team coordination (Phase 4)
6. Deduplicate overlapping LSP tools (Phase 5)

## Guardrails

**Must Have:**
- Both projects remain independently functional after each phase
- Existing MCP server continues to work for non-tinycode clients
- All oh-my-tiny persistence stays file-based under `.omc/`
- Plugin uses `PluginInput.directory` (not `process.cwd()`) for all path resolution

**Must NOT Have:**
- No breaking changes to oh-my-tiny's MCP server interface
- No changes to tinycode's core server or session processor
- No new runtime dependencies added to tinycode core packages
- No hard coupling between tinycode and oh-my-tiny (oh-my-tiny remains an optional plugin)

---

## Phase 0: Register MCP Server (Immediate)

**Goal:** Make all 25+ oh-my-tiny tools available in tinycode right now with zero code changes.

**Effort:** ~15 minutes

### Changes

**File: `/Users/bjohns/projects/tinycode/.opencode/opencode.jsonc`**

Add oh-my-tiny as an MCP server in the existing `"mcp": {}` block:

```jsonc
"mcp": {
  "oh-my-tiny": {
    "type": "stdio",
    "command": "node",
    "args": ["../oh-my-tiny/dist/mcp/server.js"],
    "env": {}
  }
}
```

### Verification

1. Run `bun dev` from tinycode root
2. Confirm oh-my-tiny tools appear in the tool list (e.g., `notepad_read`, `wiki_query`, `state_read`)
3. Execute a tool (e.g., `notepad_read`) and verify it reads/writes to `.omc/` correctly
4. Confirm tinycode's built-in tools still work normally

### Acceptance Criteria

- [ ] oh-my-tiny MCP server connects on tinycode startup
- [ ] All 25+ tools are callable from tinycode sessions
- [ ] `.omc/` directory is created in the project root on first tool use
- [ ] No startup errors or crashes

---

## Phase 1: Plugin Entry Point

**Goal:** Create a native tinycode plugin in oh-my-tiny that registers all tools via the `tool()` hook, eliminating MCP serialization overhead for tinycode users.

**Effort:** ~1-2 days

### Changes

**New file: `/Users/bjohns/projects/oh-my-tiny/src/plugin/index.ts`**

Export a `Plugin` function conforming to the tinycode plugin API (`@opencode-ai/plugin`):

```typescript
import type { Plugin } from "@opencode-ai/plugin"
import { tool } from "@opencode-ai/plugin"

export const server: Plugin = async (input) => {
  const directory = input.directory  // replaces process.cwd()
  // Initialize persistence with directory
  // ...
  return {
    tool: {
      omt_notepad_read: tool({ description: "...", args: {...}, execute: ... }),
      omt_notepad_write: tool({ description: "...", args: {...}, execute: ... }),
      omt_wiki_query: tool({ description: "...", args: {...}, execute: ... }),
      // ... all 25+ tools mapped from MCP handlers
    },
  }
}
```

Key implementation details:
- Import each tool handler from `src/mcp/server.ts` (refactor handlers into importable functions if currently inline)
- Use `input.directory` to initialize `persistence.ts` instead of `process.cwd()`
- Prefix tool names with `omt_` to avoid collisions with tinycode built-in tools
- Use `tool.schema` (zod) for args instead of MCP JSON Schema — map 1:1
- Return `ToolResult` (string or `{ title, output, metadata }`) from each execute function

**Modified file: `/Users/bjohns/projects/oh-my-tiny/src/mcp/server.ts`**

Refactor inline tool handlers into standalone exported functions so both the MCP server and the plugin can call them:

```typescript
// Before: handler logic inline in server.tool("notepad_read", ...)
// After: export async function notepadRead(args, directory) { ... }
```

This is the largest refactoring step. The MCP server.ts file is ~1388 lines with all handlers inline. Extract each handler into a function in a shared module (e.g., `src/tools/` directory with one file per tool group: `notepad.ts`, `wiki.ts`, `state.ts`, `lsp-tools.ts`, `ast-grep.ts`, `memory.ts`).

**Modified file: `/Users/bjohns/projects/oh-my-tiny/package.json`**

Add plugin export:

```json
{
  "exports": {
    "./server": "./dist/plugin/index.js"
  }
}
```

**Modified file: `/Users/bjohns/projects/tinycode/.opencode/opencode.jsonc`**

Replace MCP entry with plugin entry:

```jsonc
"plugin": ["../oh-my-tiny"]
```

Or install via CLI: `tinycode plugin ../oh-my-tiny`

### Verification

1. Build oh-my-tiny: `bun run build` (or equivalent)
2. Run `bun dev` from tinycode
3. Confirm `omt_*` tools appear in the tool list
4. Execute `omt_notepad_read` and verify output matches MCP version
5. Run a tool that writes (e.g., `omt_notepad_write_working`) and verify `.omc/notepad.md` is updated
6. Verify the standalone MCP server still works: `node dist/mcp/server.js` responds to MCP protocol

### Acceptance Criteria

- [ ] All 25+ tools registered via plugin with `omt_` prefix
- [ ] Tool args use zod schemas (not JSON Schema)
- [ ] All tools use `input.directory` for path resolution
- [ ] MCP server still works independently (no regressions)
- [ ] Plugin loads without errors on `bun dev`

---

## Phase 2: Plugin Hooks (Context Injection, Skill Detection, Permissions)

**Goal:** Move the functionality from `.opencode/plugins/context-injector.ts` and `.opencode/plugins/skill-detector.ts` into the main oh-my-tiny plugin, using proper tinycode hook APIs.

**Effort:** ~0.5-1 day

### Changes

**Modified file: `/Users/bjohns/projects/oh-my-tiny/src/plugin/index.ts`**

Add three hooks to the returned `Hooks` object:

#### 2a. Context Injection via `experimental.chat.system.transform`

```typescript
"experimental.chat.system.transform": async (input, output) => {
  // Read notepad priority section, project memory, active state
  // Append as system prompt sections
  const priority = await readNotepadPriority(directory)
  const memory = await readProjectMemory(directory)
  if (priority) output.system.push(`<omt-priority>\n${priority}\n</omt-priority>`)
  if (memory) output.system.push(`<omt-memory>\n${memory}\n</omt-memory>`)
}
```

#### 2b. Skill Detection via `command.execute.before`

```typescript
"command.execute.before": async (input, output) => {
  // Detect skill keywords in command text
  // If matched, inject skill prompt into parts
  const skill = detectSkill(input.command, input.arguments)
  if (skill) {
    output.parts.push({ type: "text", text: skill.prompt })
  }
}
```

#### 2c. Permission Auto-Approve via `permission.ask`

```typescript
"permission.ask": async (input, output) => {
  // Auto-approve omt_* tool calls (they're all safe file ops under .omc/)
  if (input.tool?.startsWith("omt_")) {
    output.status = "allow"
  }
}
```

**Deleted files (after migration verified):**
- `/Users/bjohns/projects/tinycode/.opencode/plugins/context-injector.ts`
- `/Users/bjohns/projects/tinycode/.opencode/plugins/skill-detector.ts`

### Verification

1. Start tinycode with the plugin
2. Verify system prompt includes omt priority/memory sections (check via debug/log output)
3. Type a skill-triggering command and verify skill prompt is injected
4. Call an `omt_*` tool and verify no permission prompt appears
5. Call a non-omt tool (e.g., `shell`) and verify permission prompt still appears

### Acceptance Criteria

- [ ] Context injection adds notepad/memory to system prompt on each message
- [ ] Skill detection intercepts commands and injects correct skill prompts
- [ ] `omt_*` tools are auto-approved without user permission prompt
- [ ] Non-omt tools are unaffected (permissions work normally)
- [ ] Old plugin files removed from `.opencode/plugins/`

---

## Phase 3: Team Runtime via HTTP API

**Goal:** Replace tmux `sendKeys()` + file-based IPC with tinycode's HTTP API for team worker orchestration. Workers become tinycode sessions instead of CLI processes in tmux panes.

**Effort:** ~2-3 days (highest risk phase)

### Changes

**New file: `/Users/bjohns/projects/oh-my-tiny/src/team/tinycode-runtime.ts`**

A new runtime implementation that uses the tinycode SDK:

```typescript
import { createOpencodeClient } from "@opencode-ai/sdk"

export class TinycodeRuntime {
  private client: ReturnType<typeof createOpencodeClient>

  constructor(serverUrl: string) {
    this.client = createOpencodeClient({ baseUrl: serverUrl })
  }

  async spawnWorker(config: WorkerConfig): Promise<WorkerHandle> {
    // Create a new session via API
    const session = await this.client.session.create({
      body: { /* agent config, system prompt */ }
    })
    return { sessionId: session.data.id, /* ... */ }
  }

  async assignTask(worker: WorkerHandle, task: Task): Promise<void> {
    // Send prompt via HTTP API instead of tmux sendKeys
    await this.client.session.promptAsync({
      path: { sessionID: worker.sessionId },
      body: { parts: [{ type: "text", text: task.prompt }] }
    })
  }

  async subscribeToResults(worker: WorkerHandle): AsyncIterable<TaskResult> {
    // Subscribe to SSE event stream instead of polling outbox files
    // Listen for message.updated events on the worker's session
  }
}
```

**Modified file: `/Users/bjohns/projects/oh-my-tiny/src/team/runtime.ts`**

Lines ~93-115 (worker spawn): Add a conditional branch:

```typescript
if (this.mode === "tinycode") {
  return this.tinycodeRuntime.spawnWorker(config)
} else {
  // existing tmux spawn logic
}
```

Lines ~140-163 (task assignment): Add a conditional branch:

```typescript
if (this.mode === "tinycode") {
  await this.tinycodeRuntime.assignTask(worker, task)
} else {
  // existing sendKeys logic
}
```

**Modified file: `/Users/bjohns/projects/oh-my-tiny/src/team/coordinator.ts`**

Accept `--server-url` flag to enable tinycode mode:

```typescript
// If --server-url is provided, use TinycodeRuntime
// Otherwise, fall back to tmux runtime
const runtime = serverUrl
  ? new TinycodeRuntime(serverUrl)
  : new TmuxRuntime()
```

**Modified file: `/Users/bjohns/projects/oh-my-tiny/src/team/inbox-outbox.ts`**

Add an alternative IPC channel that uses SSE events instead of file polling. The existing file-based IPC remains for tmux mode.

### Verification

1. Start tinycode server: `bun dev serve`
2. Run team coordinator with `--server-url http://localhost:4096`
3. Assign a simple task to a worker
4. Verify worker session is created via API (check `GET /session`)
5. Verify task prompt is sent via `promptAsync`
6. Verify results are received via SSE stream
7. Verify tmux mode still works without `--server-url`
8. Test worker failure/timeout handling

### Acceptance Criteria

- [ ] Workers are created as tinycode sessions (not tmux panes) when `--server-url` is provided
- [ ] Tasks are sent via `POST /session/:id/prompt_async`
- [ ] Results are received via SSE event stream (no file polling)
- [ ] Tmux fallback mode is preserved and unchanged
- [ ] Worker lifecycle (create, assign, complete, cleanup) works end-to-end
- [ ] Error handling: worker crash, timeout, server disconnect

---

## Phase 4: `tinycode omt` CLI Subcommand

**Goal:** Add a `tinycode omt` subcommand that delegates to oh-my-tiny's team coordinator, automatically passing the tinycode server URL.

**Effort:** ~0.5-1 day

### Changes

**New file: `/Users/bjohns/projects/tinycode/packages/tinycode/src/cli/cmd/omt.ts`**

```typescript
import { effectCmd } from "../effect-cmd"
import { Effect } from "effect"

export const OmtCommand = effectCmd({
  command: "omt <subcommand>",
  describe: "oh-my-tiny team orchestration",
  builder: (yargs) =>
    yargs
      .positional("subcommand", {
        type: "string",
        describe: "team subcommand (run, status, cancel)",
      })
      .option("task", {
        type: "string",
        describe: "task description for team",
      })
      .option("workers", {
        type: "number",
        default: 3,
        describe: "number of workers",
      }),
  handler: Effect.fn("Cli.omt")(function* (args) {
    // Get server URL from running instance
    // Delegate to oh-my-tiny coordinator with --server-url
    // Stream output back to terminal
  }),
})
```

**Modified file: `/Users/bjohns/projects/tinycode/packages/tinycode/src/cli/cmd/cmd.ts`**

Register the new `omt` subcommand alongside existing commands.

### Verification

1. Run `bun dev omt run --task "implement feature X" --workers 2`
2. Verify it starts the tinycode server if not already running
3. Verify it delegates to oh-my-tiny coordinator with correct server URL
4. Verify team output streams to terminal
5. Verify `bun dev omt status` shows active workers
6. Verify `bun dev omt cancel` stops all workers

### Acceptance Criteria

- [ ] `tinycode omt run` starts team orchestration via HTTP API
- [ ] Server URL is automatically resolved (no manual `--server-url` needed)
- [ ] Subcommands: `run`, `status`, `cancel`
- [ ] Output streams to terminal in real time
- [ ] Graceful shutdown on Ctrl+C

---

## Phase 5: LSP Tool Deduplication

**Goal:** Avoid duplicate LSP tools. tinycode has a built-in `lsp` tool (9 operations: goToDefinition, findReferences, hover, documentSymbol, workspaceSymbol, goToImplementation, prepareCallHierarchy, incomingCalls, outgoingCalls). oh-my-tiny has separate LSP tools via its MCP server (`lsp_goto_definition`, `lsp_find_references`, `lsp_hover`, `lsp_document_symbols`, `lsp_workspace_symbols`, `lsp_diagnostics`, `lsp_diagnostics_directory`, `lsp_code_actions`, `lsp_code_action_resolve`, `lsp_rename`, `lsp_prepare_rename`).

**Effort:** ~0.5 day

### Changes

**Modified file: `/Users/bjohns/projects/oh-my-tiny/src/plugin/index.ts`**

Skip registering oh-my-tiny LSP tools that overlap with tinycode built-ins. Only register the ones tinycode does NOT have:

| oh-my-tiny tool | tinycode built-in | Action |
|---|---|---|
| `lsp_goto_definition` | `lsp` (goToDefinition) | **Skip** |
| `lsp_find_references` | `lsp` (findReferences) | **Skip** |
| `lsp_hover` | `lsp` (hover) | **Skip** |
| `lsp_document_symbols` | `lsp` (documentSymbol) | **Skip** |
| `lsp_workspace_symbols` | `lsp` (workspaceSymbol) | **Skip** |
| `lsp_diagnostics` | -- | **Keep** (unique) |
| `lsp_diagnostics_directory` | -- | **Keep** (unique) |
| `lsp_code_actions` | -- | **Keep** (unique) |
| `lsp_code_action_resolve` | -- | **Keep** (unique) |
| `lsp_rename` | -- | **Keep** (unique) |
| `lsp_prepare_rename` | -- | **Keep** (unique) |

```typescript
// In plugin/index.ts, conditionally exclude overlapping tools:
const SKIP_IN_PLUGIN = new Set([
  "lsp_goto_definition",
  "lsp_find_references",
  "lsp_hover",
  "lsp_document_symbols",
  "lsp_workspace_symbols",
])

const tools = Object.fromEntries(
  allTools.filter(([name]) => !SKIP_IN_PLUGIN.has(name))
)
```

The MCP server keeps all tools (non-tinycode clients may need them).

### Verification

1. Start tinycode with the plugin
2. Verify only unique omt LSP tools appear (diagnostics, code_actions, rename, prepare_rename)
3. Verify tinycode's built-in `lsp` tool still works for overlapping operations
4. Verify the omt-unique tools work correctly (e.g., `omt_lsp_diagnostics`)
5. Verify MCP server still exposes all LSP tools

### Acceptance Criteria

- [ ] No duplicate LSP operations in tinycode tool list
- [ ] 5 overlapping tools skipped in plugin mode
- [ ] 6 unique tools registered with `omt_` prefix
- [ ] MCP server unaffected (all tools still available)
- [ ] Built-in tinycode `lsp` tool works for all 9 operations

---

## Phase Summary

| Phase | Description | Effort | Risk | Dependencies |
|-------|-------------|--------|------|-------------|
| 0 | Register MCP server | 15 min | Very Low | None |
| 1 | Plugin entry point | 1-2 days | Low | Phase 0 (for verification) |
| 2 | Plugin hooks | 0.5-1 day | Low | Phase 1 |
| 3 | Team runtime via HTTP | 2-3 days | Medium-High | Phase 1 |
| 4 | `tinycode omt` CLI | 0.5-1 day | Low | Phase 3 |
| 5 | LSP deduplication | 0.5 day | Low | Phase 1 |

**Total estimated effort:** 5-8 days

**Recommended execution order:** 0 -> 1 -> 2 -> 5 -> 3 -> 4

Phase 5 (LSP dedup) can be done right after Phase 1 since it only requires the plugin entry point. Phase 3 (team runtime) is the most complex and can be deferred if the priority is getting tools working natively first.

## Open Questions

- Should the `omt_` prefix be configurable or hardcoded? A prefix avoids name collisions but adds verbosity.
- Should Phase 3 support running workers on remote tinycode instances (not just localhost)?
- How should team worker sessions be cleaned up? Auto-delete on completion, or keep for inspection?
- Should Phase 2 context injection load wiki content in addition to notepad/memory, or is that too much system prompt bloat?
- For Phase 4, should `tinycode omt` auto-start the server if not running, or require `tinycode serve` to be running separately?

## Success Criteria

1. All oh-my-tiny tools are available in tinycode without MCP serialization overhead
2. Context injection and skill detection work via plugin hooks (no separate plugin files)
3. Team runtime can orchestrate workers via tinycode HTTP API (no tmux dependency)
4. Both projects remain independently functional
5. MCP server continues to work for non-tinycode clients
