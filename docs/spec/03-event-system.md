# 3. Event System

tinycode uses a multi-layer event system: an internal bus for inter-component communication, SSE for client streaming, and LLM streaming events for model responses.

## 3.1 Internal Event Bus

Package: `internal/bus/`

The bus is a publish-subscribe system with typed and wildcard subscriptions.

### Bus Architecture

```go
type Event struct {
    ID         string // ascending sortable ID (evt_*)
    Type       string // dot-separated event type
    Properties any    // event-specific payload
}
```

### Subscription Types

| Type | Method | Receives |
|------|--------|----------|
| Typed | `bus.Subscribe("topic")` | Events matching the exact topic |
| Wildcard | `bus.SubscribeAll()` | All events |

`Subscribe("*")` is a **typed** subscription for the literal topic `"*"` — it is **not** a wildcard. Use `SubscribeAll()` for all events.

After `Bus.Close()`, `Subscribe` / `SubscribeAll` return an already-closed subscription (no panic).

### Constants

| Constant | Value | Description |
|----------|-------|-------------|
| `defaultCapacity` | 4096 | Channel buffer size per subscription |
| `historySize` | 256 | Recent-event ring for SSE `Last-Event-ID` replay |

### Backpressure

When a subscription channel is full, the bus uses a **sliding buffer** strategy: it drops the oldest unread event(s) until the newest event can be enqueued. Slow consumers never block publishers. Drops are observable via `Bus.Drops()` and `slog.Debug`.

### Lock Order

Never hold `Subscription.mu` while taking `Bus.mu`. Publish copies the subscriber list under `RLock`, then delivers without the bus lock. Close collects subscribers under `Bus.mu`, unlocks, then closes channels.

### Bus Events

Events published by the server and session system:

| Event Type | Publisher | Properties |
|------------|-----------|------------|
| `server.connected` | Server | `{url}` |
| `session.created` | SessionManager | `{info: {id, ...}}` |
| `session.deleted` | SessionManager | `{sessionID}` |
| `session.message` | Processor | `{sessionID, message}` |
| `permission.asked` | PermService | `Request` struct |
| `permission.replied` | PermService | `{sessionID, requestID, reply}` |
| `session.status` | SessionManager | `{sessionID, status}` |
| `session.prompt` | SessionManager | `{sessionID, prompt}` |
| `session.error` | Processor | `{sessionID, error}` |
| `session.warning` | Processor | `{sessionID, message}` |
| `session.text.delta` | Processor | `{sessionID, text}` |
| `session.tool.begin` | Processor | `{sessionID, toolName, toolCallID}` |
| `session.tool.end` | Processor | `{sessionID, toolName, toolCallID, output}` |
| `session.step.start` | Processor | `{sessionID, iteration}` |
| `session.step.finish` | Processor | `{sessionID, iteration, usage}` |
| `session.compacted` | Processor | `{sessionID, preTokens, postTokens}` |
| `session.summarize` | SessionManager | `{sessionID}` |
| `session.revert` | SessionManager | `{sessionID}` |
| `session.reverted` | SessionManager | `{sessionID}` |
| `session.unreverted` | SessionManager | `{sessionID}` |
| `message.updated` | SessionManager | `{sessionID, info}` |
| `message.part.updated` | SessionManager | `{sessionID, messageID, part}` |
| `message.part.delta` | SessionManager | `{sessionID, messageID, partID, delta}` |
| `toast` | Various | `{message, level}` |
| `tool.execute.before` | ToolRegistry | `{sessionID, tool, args}` |
| `provider.discovered` | Discovery | `{providerID, modelCount}` |
| `provider.removed` | Discovery | `{providerID, providerName, reason, failures}` |
| `provider.reconnected` | Discovery | `{providerID, previous_failures}` |
| `provider.warmup.complete` | Discovery | `{modelID, toolCapable}` |
| `global.disposed` | Server | `{timestamp}` |
| `stream.gap` | SSE (reconnect) | `{lastSeen}` — emitted when `Last-Event-ID` is outside the history ring |

### Sync Hooks (Not Bus Events)

These are **synchronous callbacks**, not bus topics:

| Hook | Path | Purpose |
|------|------|---------|
| Permission ask interceptor | `permission.Service.SetAskInterceptor` | Plugins (e.g. safety-net) deny before UI ask |
| Shell env | `tool.Context.ShellEnvHook` | Mutate env before shell execution |
| Tool before/after | `tool.Context.BeforeHook` / `AfterHook` | Abort or transform tool execution |

## 3.2 Server-Sent Events (SSE)

SSE endpoints stream bus events to clients in real-time.

### SSE Endpoints

| Endpoint | Scope | Payload shape |
|----------|-------|---------------|
| `GET /global/event` | All bus events (`SubscribeAll`) | `{directory, payload: {id, type, properties}}` |
| `GET /event` | All bus events (alias) | `event:` type + `data:` properties |
| `GET /session/{id}/event` | Events with matching `properties.sessionID` | Same as `/event` |

### SSE Message Format

`/event` and `/session/{id}/event`:

```
event: <event-type>
data: <json-properties>
id: <event-id>

```

`/global/event` wraps each bus event:

```
id: <event-id>
data: {"directory":"...","payload":{"id":"...","type":"...","properties":...}}

```

### Reconnect / Last-Event-ID

Clients may send `Last-Event-ID` on reconnect. The bus keeps a ring of the last 256 events:

- If the ID is found → replay events after it, then live-subscribe
- If missing / too old → emit `stream.gap` with `{lastSeen}` then live-subscribe

### SSE Heartbeat

The server sends a heartbeat comment every 10 seconds (`sseHeartbeatInterval = 10s`) to keep the connection alive:

```
: heartbeat

```

### SSE Headers

```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
X-Accel-Buffering: no
```

### TUI SSE Integration

The TUI (`internal/tui/run.go`) subscribes to SSE via `api.Client.Subscribe()`, which returns a `<-chan ServerEvent`. The `waitForSSE()` function converts channel reads into `tea.Cmd` chains. Events are mapped to TUI messages via `mapSSEToMsg()`. The client sends `Last-Event-ID` on reconnect.

## 3.3 LLM Streaming Events

Package: `internal/llm/event.go`

Events emitted by the LLM client during streaming responses:

| EventType | Constant | Fields | Description |
|-----------|----------|--------|-------------|
| `text-delta` | `EventTextDelta` | `Text` | Incremental text content |
| `reasoning-delta` | `EventReasoningDelta` | `Text` | Incremental reasoning/thinking content |
| `tool-call-begin` | `EventToolCallBegin` | `ToolCallID`, `ToolName` | Start of a tool call |
| `tool-call-delta` | `EventToolCallDelta` | `ToolCallID`, `ToolCallArgs` | Incremental tool call arguments |
| `tool-call-end` | `EventToolCallEnd` | `ToolCallID`, `ToolName`, `ToolCallArgs` | Complete tool call with full arguments |
| `finish` | `EventFinish` | `FinishReason`, `Usage` | Stream complete |
| `error` | `EventError` | `Error` | Stream error |

### Tool Call Accumulation

Tool call arguments arrive as incremental deltas. The client accumulates them per tool call index (`toolCallAccum`):

1. `tool-call-begin` — emitted when a new tool call index appears
2. `tool-call-delta` — arguments appended to accumulator
3. `tool-call-end` — emitted on `finish_reason`, with full accumulated arguments

### JSON Repair

When the accumulated arguments are not valid JSON (common with small models), the client attempts repair via `RepairToolCallJSON()`. If repair fails, the tool call is redirected to the `invalid` tool with the original name and args in the error payload.

### Streaming Timeouts

| Constant | Value | Description |
|----------|-------|-------------|
| `headerTimeout` | 5 minutes | Time to receive HTTP response headers |
| `chunkTimeout` | 5 minutes | Time between SSE chunks before timeout |

### Usage Tracking

```go
type Usage struct {
    PromptTokens     int
    CompletionTokens int
    TotalTokens      int
}
```

Usage may arrive either in the final `finish` event's `Usage` field, or in a separate usage-only chunk (no choices, just `usage` object) — both paths are handled.

## 3.4 Run Mode NDJSON Events

When running in headless mode with `--format json`, the run mode emits NDJSON (newline-delimited JSON) events to stdout:

| Event Type | Fields | Description |
|------------|--------|-------------|
| `text` | `content` | Assistant text output |
| `reasoning` | `content` | Reasoning/thinking content |
| `tool_begin` | `toolName`, `toolCallID` | Tool call starting |
| `tool_call_end` | `toolName`, `toolCallID`, `toolArgs` | LLM finished streaming tool-call args |
| `tool_end` | `toolName`, `toolCallID`, `output`, `isError` | Tool execution complete |
| `step_start` | `iteration` | Processor loop iteration starting |
| `step_finish` | `iteration`, `usage` | Processor loop iteration complete |
| `warning` | `message` | Warning (e.g., tool failures) |
| `compacted` | `preTokens`, `postTokens` | Context compaction occurred |

### NDJSON Example

```json
{"type":"step_start","iteration":1}
{"type":"text","content":"I'll fix the bug."}
{"type":"tool_begin","toolName":"read","toolCallID":"call_1"}
{"type":"tool_end","toolName":"read","toolCallID":"call_1","output":"package main...","isError":false}
{"type":"step_finish","iteration":1,"usage":{"input":450,"output":120}}
```

## 3.5 Multi-Turn Ready Signal

In `--multi-turn` mode with `--format json`, a ready signal is emitted between turns:

```json
{"type":"ready"}
```

This tells the controlling process that tinycode is ready for the next prompt or command.
