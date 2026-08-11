# Agent Client Protocol (ACP) Integration Guide

This guide covers building IDE integrations for tinycode using the Agent Client Protocol (ACP).

## What is ACP?

The Agent Client Protocol is a standardized JSON-RPC 2.0 protocol for communication between IDEs and AI coding agents. Originally developed by Zed Industries, ACP defines a bidirectional stdio-based protocol for:

- Session management (create, list, switch)
- Prompt streaming with real-time events
- Tool execution with permission requests
- Multi-turn conversations with context preservation

ACP enables tinycode to integrate with any editor that supports spawning a child process and communicating over stdin/stdout.

## Quick Start

Run tinycode in ACP mode:

```bash
tinycode acp --cwd /path/to/project
```

The process will:
1. Start listening on stdin for JSON-RPC requests
2. Write JSON-RPC responses and notifications to stdout
3. Create a session scoped to the specified working directory
4. Stream LLM responses and tool events back to the client

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
┌────────▼────────┐
│  tinycode acp   │
│   (ACP server)  │
└────────┬────────┘
         │ HTTP (REST)
         │ localhost:4096
         │
┌────────▼────────┐
│ tinycode server │
│  (session mgmt, │
│   LLM, tools)   │
└─────────────────┘
```

The `tinycode acp` command acts as a protocol adapter:
- Receives ACP requests via stdin
- Translates them to HTTP calls against the tinycode REST API
- Streams events back via stdout as ACP notifications

## Building an IDE Extension

### Installation

```bash
npm install @agentclientprotocol/sdk
```

### Basic Example

```typescript
import { ClientSideConnection, ndJsonStream } from '@agentclientprotocol/sdk'
import { spawn } from 'child_process'

// Spawn the ACP server
const proc = spawn('tinycode', ['acp', '--cwd', workspaceRoot], {
  stdio: ['pipe', 'pipe', 'pipe']
})

// Create the connection
const stream = ndJsonStream(proc.stdout, proc.stdin)
const conn = new ClientSideConnection(stream)

// Initialize
const initResult = await conn.initialize({
  protocolVersion: '0.1.0',
  capabilities: {
    supportsPermissions: true
  },
  clientInfo: {
    name: 'my-editor',
    version: '1.0.0'
  }
})

console.log('Connected to:', initResult.serverInfo)

// Create a session
const session = await conn.newSession({
  cwd: workspaceRoot
})

console.log('Session ID:', session.sessionId)

// Send a prompt
await conn.prompt({
  sessionId: session.sessionId,
  prompt: [
    { type: 'text', text: 'Explain this codebase' }
  ]
})
```

### Listening for Events

```typescript
conn.onNotification('sessionUpdate', (params) => {
  console.log('Session update:', params.sessionId)

  for (const event of params.events) {
    switch (event.type) {
      case 'message-part':
        if (event.part.type === 'text') {
          console.log('Text:', event.part.text)
        }
        break

      case 'tool-call-started':
        console.log('Tool started:', event.name)
        break

      case 'tool-call-completed':
        console.log('Tool completed:', event.name, event.status)
        break

      case 'request-permission':
        handlePermission(event)
        break
    }
  }
})
```

### Permission Handling

When tinycode requests permission to execute a tool, respond with:

```typescript
async function handlePermission(event) {
  // Show UI to user (dialog, quick-pick, etc.)
  const decision = await askUser(
    `Allow tinycode to use ${event.tool}?`,
    ['allow-once', 'allow-always', 'deny']
  )

  await conn.respondToPermissionRequest({
    sessionId: event.sessionId,
    requestId: event.requestId,
    decision
  })
}
```

## Supported Operations

| ACP Method             | Description                                      | tinycode Mapping       |
|------------------------|--------------------------------------------------|------------------------|
| `initialize`           | Establish connection and negotiate capabilities  | Returns server info    |
| `newSession`           | Create a new conversation session                | `POST /session`        |
| `listSessions`         | List all sessions                                | `GET /session`         |
| `getSession`           | Get session details                              | `GET /session/:id`     |
| `switchSession`        | Change active session                            | No-op (client-side)    |
| `prompt`               | Send a user prompt                               | `POST /session/:id/prompt` |
| `respondToPermissionRequest` | Respond to permission request           | Stored in-memory       |

## Event Handling

tinycode streams events via `sessionUpdate` notifications. Each notification contains an array of events:

### Event Types

**`message-part`** — A chunk of the LLM's response

```typescript
{
  type: 'message-part',
  part: {
    type: 'text',
    text: 'Here is the explanation...'
  }
}
```

**`tool-call-started`** — Tool execution begins

```typescript
{
  type: 'tool-call-started',
  name: 'Read',
  callId: 'call-abc123'
}
```

**`tool-call-completed`** — Tool execution finishes

```typescript
{
  type: 'tool-call-completed',
  name: 'Read',
  callId: 'call-abc123',
  status: 'success'
}
```

**`request-permission`** — Agent requests permission to run a tool

```typescript
{
  type: 'request-permission',
  sessionId: 'sess-xyz',
  requestId: 'req-123',
  tool: 'Bash',
  description: 'Run command: npm test'
}
```

### Streaming Responses

All text responses are streamed incrementally via `message-part` events. Accumulate them to build the full response:

```typescript
let fullResponse = ''

conn.onNotification('sessionUpdate', (params) => {
  for (const event of params.events) {
    if (event.type === 'message-part' && event.part.type === 'text') {
      fullResponse += event.part.text
      updateUI(fullResponse)
    }
  }
})
```

## Permission System

tinycode requires explicit permission before executing tools that modify files or run shell commands. The IDE must handle `request-permission` events and respond with one of:

- **`allow-once`** — Grant permission for this single operation
- **`allow-always`** — Add the tool/command to the allowlist (stored in tinycode config)
- **`deny`** — Reject the operation

Example flow:

1. User sends prompt: "Run the tests"
2. tinycode decides to call the `Bash` tool
3. ACP server emits `request-permission` event
4. IDE shows permission dialog to user
5. User selects "Allow Once"
6. IDE calls `respondToPermissionRequest` with decision
7. tinycode executes the tool
8. ACP server emits `tool-call-completed` event

## Session Management

### Creating Sessions

```typescript
const session = await conn.newSession({
  cwd: '/path/to/project',
  agent: 'executor',  // optional: default agent
  model: 'ollama/llama3.2'  // optional: override model
})
```

### Listing Sessions

```typescript
const sessions = await conn.listSessions()
for (const session of sessions) {
  console.log(`${session.sessionId}: ${session.title}`)
}
```

### Switching Sessions

```typescript
await conn.switchSession({
  sessionId: 'sess-abc123'
})
```

## Error Handling

All ACP methods return promises that reject on error. Handle errors appropriately:

```typescript
try {
  await conn.prompt({
    sessionId: session.sessionId,
    prompt: [{ type: 'text', text: 'Hello' }]
  })
} catch (error) {
  if (error.code === -32602) {
    // Invalid params
    console.error('Invalid request:', error.message)
  } else if (error.code === -32603) {
    // Internal error
    console.error('Server error:', error.message)
  } else {
    console.error('Unexpected error:', error)
  }
}
```

## Advanced: Multi-Modal Prompts

Send images or other content types in prompts:

```typescript
await conn.prompt({
  sessionId: session.sessionId,
  prompt: [
    { type: 'text', text: 'Explain this screenshot:' },
    {
      type: 'image',
      source: {
        type: 'base64',
        mediaType: 'image/png',
        data: base64ImageData
      }
    }
  ]
})
```

## End-of-Turn Handling

ACP clients need to know when the agent has finished processing a prompt. tinycode uses SSE event draining internally to ensure all tool results and message parts are delivered before the `prompt()` call resolves.

The `prompt()` method returns only after:
1. The LLM finishes generating
2. All tool executions complete
3. All `sessionUpdate` notifications for the turn have been emitted

This means clients can safely treat the `prompt()` promise resolution as the end-of-turn signal — no polling or timeout required.

```typescript
// prompt() resolves only after all events are delivered
await conn.prompt({
  sessionId: session.sessionId,
  prompt: [{ type: 'text', text: 'Run the tests and report results' }]
})
// At this point, all tool-call-completed events have been emitted
console.log('Turn complete — all results delivered')
```

For non-interactive integrations (CI pipelines, batch processing), this guarantees that reading the session messages after `prompt()` returns will include the complete response.

## Troubleshooting

### Connection Failed

**Symptom:** `spawn` fails or process exits immediately

**Causes:**
- `tinycode` binary not found in PATH
- Binary missing execute permission
- Wrong working directory

**Solutions:**
- Verify `which tinycode` or provide absolute path
- Run `chmod +x /path/to/tinycode`
- Check that `--cwd` points to a valid directory

### No Response to Prompts

**Symptom:** `prompt()` succeeds but no `sessionUpdate` events arrive

**Causes:**
- Session was closed or invalid
- Server crashed (check stderr)
- Network issue (if using remote server)

**Solutions:**
- Call `getSession()` to verify session is active
- Read from `proc.stderr` and log errors
- Ensure tinycode server is running (`lsof -i :4096`)

### Permission Requests Hang

**Symptom:** Tool execution pauses indefinitely

**Cause:** IDE didn't respond to `request-permission` event

**Solution:** Ensure you have a listener for `request-permission` and always call `respondToPermissionRequest`, even if the user cancels (send `deny`).

### High Latency

**Symptom:** Responses are slow

**Causes:**
- Large codebase (many files to index)
- Slow LLM backend
- Cold start (first request after spawn)

**Solutions:**
- Use `.tineignore` to exclude node_modules, build artifacts
- Configure a faster model for quick responses
- Keep the ACP process alive across multiple prompts

## Reference Implementation

See `packages/vscode-extension/` for a complete VS Code extension that demonstrates:

- Spawning the ACP server as a child process
- Creating sessions on workspace open
- Registering a chat participant
- Streaming responses to the chat UI
- Handling permission requests with VS Code quick-picks
- Logging to the output channel

## Specification

The full ACP specification is available at [agentclientprotocol.com](https://agentclientprotocol.com).

## Next Steps

- Read the [VS Code extension README](../packages/vscode-extension/README.md) for a working example
- Explore the ACP SDK documentation at [@agentclientprotocol/sdk](https://www.npmjs.com/package/@agentclientprotocol/sdk)
- Join the tinycode community to share your integration or ask questions
