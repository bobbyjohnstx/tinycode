# Agent Client Protocol (ACP) Integration Guide

This guide covers building IDE integrations for tinycode using the Agent Client Protocol (ACP).

## What is ACP?

The Agent Client Protocol is a standardized JSON-RPC 2.0 protocol for communication between IDEs and AI coding agents. Originally developed by Zed Industries, ACP defines a bidirectional stdio-based protocol for:

- Session management (create, list, load, fork, close)
- Prompt turns with streamed `session/update` notifications
- Tool execution with `session/request_permission` requests
- Multi-turn conversations with context preservation

ACP enables tinycode to integrate with any editor that supports spawning a child process and communicating over stdin/stdout.

## Quick Start

Run tinycode in ACP mode:

```bash
tinycode acp --cwd /path/to/project
```

The process will:
1. Boot an embedded tinycode server (ephemeral localhost port) with providers, tools, permissions, agents, plugins, and MCP
2. Listen on stdin for JSON-RPC requests using official ACP wire methods (`session/new`, `session/prompt`, …)
3. Stream LLM responses and tool events as `session/update` notifications on stdout
4. Ask the IDE for tool permissions via `session/request_permission` requests

Flags:

| Flag | Description |
|------|-------------|
| `--cwd <dir>` | Working directory for new sessions (also accepts `-cwd`) |
| `--model` / `-m` | Default model (`provider/model`) |
| `--safe-mode` | Skip plugins, MCP, and user agents |
| Common TUI flags | `--title`, `--append-system-prompt`, `--max-tokens`, etc. |

## Architecture

```
┌─────────────────┐
│  IDE Extension  │
│   (VS Code,     │
│   Zed, etc.)    │
└────────┬────────┘
         │ stdio (NDJSON)
         │ JSON-RPC 2.0
         │
┌────────▼──────────────────────────────┐
│  tinycode acp                         │
│  ┌────────────┐  ┌─────────────────┐  │
│  │ ACP Service│──│ SessionManager  │  │
│  │ + EventRelay│  │ (in-process)    │  │
│  └────────────┘  └────────┬────────┘  │
│                           │           │
│              embedded HTTP server     │
│              (port 0 / ephemeral)     │
└───────────────────────────────────────┘
```

Go `tinycode acp` boots the same core stack as TUI/serve (providers, tools, permissions, SessionManager, plugins, MCP) and speaks ACP over stdio. Session operations prefer in-process `SessionManager` calls; the ephemeral HTTP listener exists so the full server lifecycle (hooks, discovery, etc.) runs even when the IDE never opens an HTTP client.

> Note: The legacy TypeScript `packages/tinycode` ACP path started HTTP + SDK separately. The Go binary is the supported ACP entrypoint.

## Building an IDE Extension

### Installation

```bash
npm install @agentclientprotocol/sdk
```

### Basic Example

```typescript
import { ClientSideConnection, ndJsonStream } from '@agentclientprotocol/sdk'
import { spawn } from 'child_process'

const proc = spawn('tinycode', ['acp', '--cwd', workspaceRoot], {
  stdio: ['pipe', 'pipe', 'pipe']
})

const stream = ndJsonStream(proc.stdout, proc.stdin)

const conn = new ClientSideConnection((agent) => ({
  async sessionUpdate(params) {
    // Handle streamed chunks / tool calls
    const update = params.update
    if (update.sessionUpdate === 'agent_message_chunk') {
      console.log(update.content?.text)
    }
  },
  async requestPermission(params) {
    // Present options to the user; return selected optionId
    return {
      outcome: { outcome: 'selected', optionId: 'allow_once' }
    }
  }
}), stream)

const initResult = await conn.initialize({
  protocolVersion: 1,
  clientCapabilities: {},
  clientInfo: { name: 'my-editor', version: '1.0.0' }
})

console.log('Connected to:', initResult.agentInfo)

const session = await conn.newSession({ cwd: workspaceRoot })
console.log('Session ID:', session.sessionId)

const result = await conn.prompt({
  sessionId: session.sessionId,
  prompt: [{ type: 'text', text: 'Explain this codebase' }]
})
console.log('stopReason:', result.stopReason) // "end_turn" | "cancelled"
```

### Listening for Session Updates

Official ACP notifications use method `session/update`:

```json
{
  "jsonrpc": "2.0",
  "method": "session/update",
  "params": {
    "sessionId": "ses_…",
    "update": {
      "sessionUpdate": "agent_message_chunk",
      "content": { "type": "text", "text": "Hello…" }
    }
  }
}
```

Common `sessionUpdate` values:

| Value | Meaning |
|-------|---------|
| `agent_message_chunk` | Assistant text |
| `agent_thought_chunk` | Reasoning/thought text |
| `user_message_chunk` | Replayed user text (loadSession) |
| `tool_call` | Tool invocation started |
| `tool_call_update` | Tool status change |

### Permission Handling

When a tool needs approval, the agent sends a **request** (not a notification):

- Method: `session/request_permission`
- Client must reply with `{ outcome: { outcome: "selected", optionId: "allow_once" | "allow_always" | "reject_once" } }`
- On timeout (120s) or disconnect, tinycode auto-denies

## Supported Operations

| Wire method | CamelCase alias | Status |
|-------------|-----------------|--------|
| `initialize` | — | Supported |
| `authenticate` | — | Supported (no-op) |
| `session/new` | `newSession` | Supported |
| `session/load` | `loadSession` | Supported (replays text history) |
| `session/list` | `listSessions` | Supported |
| `session/resume` | `resumeSession` | Supported (no history replay) |
| `session/close` | `closeSession` | Supported |
| `session/fork` | `forkSession` | Supported (copies messages) |
| `session/prompt` | `prompt` | Supported — waits until idle, returns `stopReason` |
| `session/cancel` | `cancel` | Supported — aborts active run |
| `session/set_mode` | `setSessionMode` | Supported — maps to agent name |
| `session/set_model` | `setSessionModel` | Supported — `provider/model` |
| `session/set_config_option` | `setSessionConfigOption` | Accepted (no-op) |

### Capabilities (honest)

During `initialize`, tinycode advertises:

- `loadSession: true`
- `sessionCapabilities`: `close`, `fork`, `list`, `resume`
- `promptCapabilities.image: false` (not yet supported)
- `promptCapabilities.embeddedContext: false` (not yet supported)

## End-of-Turn Handling

`session/prompt` blocks until the SessionManager reports idle (or cancel), then returns:

```json
{ "stopReason": "end_turn" }
```

or `{ "stopReason": "cancelled" }` if the client sent `session/cancel`.

Prompt params accept either official `prompt: ContentBlock[]` or legacy `content: ContentBlock[]`.

## Session Management

### Creating Sessions

```typescript
const session = await conn.newSession({
  cwd: '/path/to/project'
})
```

New sessions use agent `build` and the default model from config/discovery (same heuristics as TUI).

### Listing / Loading / Forking

```typescript
const { sessions } = await conn.listSessions({ cwd: workspaceRoot })
await conn.loadSession({ sessionId, cwd: workspaceRoot }) // replays history via session/update
const forked = await conn.unstable_forkSession({ sessionId })
```

## Error Handling

All ACP methods return promises that reject on error. Handle JSON-RPC error codes (`-32602` invalid params, `-32603` internal, `-32601` method not found).

## Editor Status

| Editor | Status |
|--------|--------|
| VS Code | Spawn `tinycode acp --cwd …` from the editor. There is no VS Code extension in this repository. |
| Zed | Use official ACP agent spawn pointing at `tinycode acp --cwd …` |
| JetBrains | Not packaged yet; same stdio protocol applies |

## Troubleshooting

### Connection Failed

- Verify `which tinycode` (Go binary from `make build` → `dist/tinycode`)
- Ensure `--cwd` points to a valid directory
- Check stderr for bootstrap/discovery errors

### No Response to Prompts

- Confirm a model is available (`tinycode models`) or pass `--model provider/model`
- Read process stderr — missing providers surface there
- `session/prompt` should eventually return a `stopReason`; if it hangs, check permission UI

### Permission Requests Hang

- Client must implement `requestPermission` and always respond (deny on cancel)
- Auto-deny kicks in after 120s

## Go implementation

ACP is implemented in Go under `internal/acp/` (`service.go`, `transport.go`, `adapter.go`, `event.go`) — not a stub. Entry point: `tinycode acp`.

## Specification

The full ACP specification is available at [agentclientprotocol.com](https://agentclientprotocol.com).

## Next Steps

- Explore [@agentclientprotocol/sdk](https://www.npmjs.com/package/@agentclientprotocol/sdk)
)
