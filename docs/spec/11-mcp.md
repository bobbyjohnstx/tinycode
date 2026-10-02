# 11. Model Context Protocol (MCP)

Package: `internal/mcp/`

MCP provides a standardized protocol for connecting to external tool servers. tinycode acts as an MCP client, connecting to one or more MCP servers that expose tools via JSON-RPC 2.0.

## 11.1 Service

```go
type Service struct {
    mu      sync.RWMutex
    servers map[string]*serverConn
    bus     *bus.Bus
}
```

`NewService(bus)` creates the service. The MCP service manages concurrent connections to multiple MCP servers, each identified by a unique name from the configuration map.

### Server Connection

```go
type serverConn struct {
    name         string
    config       config.MCPConfig
    transport    Transport
    status       Status
    err          string
    tools        []MCPTool
    cancel       context.CancelFunc
    ctx          context.Context
    reconnecting bool
}
```

### Connection States

| Status | Value | Description |
|--------|-------|-------------|
| `StatusDisconnected` | `"disconnected"` | Not connected |
| `StatusConnecting` | `"connecting"` | Initial connection in progress |
| `StatusConnected` | `"connected"` | Active and available |
| `StatusReconnecting` | `"reconnecting"` | Lost connection, attempting to restore |
| `StatusError` | `"error"` | Failed, not retrying |

### Data Types

```go
type ServerStatus struct {
    Name      string `json:"name"`
    Status    Status `json:"status"`
    Error     string `json:"error,omitempty"`
    ToolCount int    `json:"toolCount"`
}

type MCPTool struct {
    Name        string          `json:"name"`
    Description string          `json:"description,omitempty"`
    InputSchema json.RawMessage `json:"inputSchema,omitempty"`
}
```

## 11.2 Configuration

MCP servers are configured in `config.json` under the `mcp` key:

```json
{
  "mcp": {
    "my-server": {
      "command": "npx",
      "args": ["-y", "@my/mcp-server"],
      "env": { "API_KEY": "$MY_API_KEY" }
    },
    "remote-sse": {
      "url": "https://mcp.example.com/sse",
      "transport": "sse",
      "headers": { "Authorization": "Bearer $TOKEN" }
    },
    "remote-streamable": {
      "url": "https://mcp.example.com/mcp",
      "transport": "streamable-http"
    },
    "array-command": {
      "command": ["node", "server.js", "--port", "3000"]
    },
    "oauth-server": {
      "url": "https://mcp.example.com/sse",
      "oauth": {
        "client_id": "my-app",
        "auth_url": "https://auth.example.com/authorize",
        "token_url": "https://auth.example.com/token",
        "scopes": ["tools:read", "tools:execute"]
      }
    }
  }
}
```

### MCPConfig Fields

| Field | Type | JSON | Description |
|-------|------|------|-------------|
| `Command` | `string` | `command` | Stdio transport: command to run. Accepts string or string array in JSON (custom `UnmarshalJSON`). When array, first element is command, rest become `Args`. |
| `Args` | `[]string` | `-` | Command arguments. Populated automatically when `command` is a JSON array. |
| `Env` | `map[string]string` | `env` | Environment variables passed to the subprocess |
| `URL` | `string` | `url` | Endpoint URL for SSE or streamable-http transports |
| `Transport` | `string` | `transport` | Explicit transport type: `"stdio"`, `"sse"`, or `"streamable-http"` |
| `Headers` | `map[string]string` | `headers` | HTTP headers for remote transports |
| `OAuth` | `*MCPOAuthConfig` | `oauth` | OAuth configuration for authenticated servers |

Source: `internal/config/config.go`

### MCPOAuthConfig Fields

| Field | Type | JSON | Description |
|-------|------|------|-------------|
| `ClientID` | `string` | `client_id` | OAuth client identifier |
| `AuthURL` | `string` | `auth_url` | Authorization endpoint URL |
| `TokenURL` | `string` | `token_url` | Token exchange endpoint URL |
| `Scopes` | `[]string` | `scopes` | Requested OAuth scopes |
| `CallbackURL` | `string` | `callback_url` | Override for the local callback URL (default: `http://127.0.0.1:19876/mcp/oauth/callback`) |

## 11.3 Transport Interface

```go
type Transport interface {
    Connect(ctx context.Context) error
    ListTools(ctx context.Context) ([]MCPTool, error)
    CallTool(ctx context.Context, name string, args json.RawMessage) (string, error)
    Close() error
}
```

All three transport implementations share a common JSON-RPC 2.0 wire protocol. Each supports:
- **`Connect`** -- establishes the connection, performs MCP `initialize` handshake (protocol version `2024-11-05`), sends `notifications/initialized`
- **`ListTools`** -- paginates via `tools/list` with cursor-based iteration (`nextCursor`)
- **`CallTool`** -- sends `tools/call`, extracts `text`-type content blocks from response, returns concatenated output
- **`Close`** -- tears down the connection

Source: `transport.go`

### Transport Resolution

`resolveTransport(cfg)` selects the transport type using this precedence:

| Priority | Condition | Result |
|----------|-----------|--------|
| 1 | `cfg.Transport` is set | Use that value directly |
| 2 | `cfg.Command` is non-empty | `"stdio"` |
| 3 | `cfg.URL` is non-empty | `"sse"` |
| 4 | Default | `"stdio"` |

### Transport Validation

`createTransport(cfg)` validates requirements per transport:

| Transport | Required Field | Error if Missing |
|-----------|---------------|-----------------|
| `stdio` | `Command` | `"stdio transport requires command"` |
| `sse` | `URL` | `"SSE transport requires url"` |
| `streamable-http` | `URL` | `"streamable-http transport requires url"` |

## 11.4 Stdio Transport

File: `stdio.go`

Spawns a subprocess and communicates via newline-delimited JSON-RPC over stdin/stdout.

### Process Management

- Command executed via `exec.CommandContext` with parent environment inherited
- Custom `env` map appended to `os.Environ()`
- Stderr discarded (`io.Discard`)
- Buffered reader: 1 MB buffer for stdout
- Message channel: buffered (64 messages)

### Read Loop

A goroutine (`readLoop`) continuously reads newline-delimited JSON from stdout:

- **Responses** (non-zero `ID`): dispatched to the `messages` channel for `roundTrip` to consume
- **Notifications** (zero `ID`, non-empty `Method`): dispatched to `onNotification` callback in a goroutine
- **Disconnect detection**: when read returns error and context is not cancelled, triggers `onDisconnect` callback

### Request/Response

`roundTrip(req)` sends a JSON-RPC request and waits for the matching response by `ID`. Timeout defaults to 60 seconds (`defaultStdioTimeout`), configurable via `Timeout` field.

### JSON-RPC Types

```go
type jsonrpcRequest struct {
    JSONRPC string `json:"jsonrpc"`
    ID      int64  `json:"id,omitempty"`
    Method  string `json:"method"`
    Params  any    `json:"params,omitempty"`
}

type jsonrpcResponse struct {
    JSONRPC string          `json:"jsonrpc"`
    ID      int64           `json:"id,omitempty"`
    Method  string          `json:"method,omitempty"`
    Result  json.RawMessage `json:"result,omitempty"`
    Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
}
```

### Cleanup

`Close()` kills the subprocess and closes stdin. Process is killed via `Process.Kill()` followed by `Wait()`.

## 11.5 SSE Transport

File: `sse.go`

HTTP Server-Sent Events for server-to-client messages, HTTP POST for client-to-server requests.

### Connection Flow

1. HTTP GET to the configured URL with `Accept: text/event-stream` header
2. Custom headers applied from config
3. SSE stream reader goroutine (`readSSEStream`) starts
4. Waits for the `endpoint` event that provides the POST messages URL
5. Connection timeout: 30 seconds (`sseConnectTimeout`)

### SSE Event Types

| Event Type | Handling |
|------------|----------|
| `endpoint` | Extracts messages URL (resolves relative URLs against the base). Signals connection ready. |
| `message` | Parses JSON-RPC response. Routes by ID to pending request channels. Notifications (ID=0, non-empty Method) dispatched to `onNotification`. |

### Request Handling

`sendRequest(ctx, req)` posts JSON-RPC to the messages URL, registers a pending channel for the request ID, and waits for the response via SSE. Call timeout: 60 seconds (`sseCallTimeout`).

### Disconnect Handling

When the SSE stream closes:
1. All pending request channels receive an error response (`"SSE connection closed"`)
2. Pending map is cleared
3. If context is not cancelled, `onDisconnect` callback fires

## 11.6 Streamable HTTP Transport

File: `streamable.go`

Stateless HTTP POST for all communication. Simpler than SSE -- no persistent connection.

### Session Management

- Server may return `Mcp-Session-Id` header on any response
- Client persists the session ID and sends it on subsequent requests via `Mcp-Session-Id` header
- Call timeout: 60 seconds (`streamableCallTimeout`)

### HTTP Status Handling

| Status | Behavior |
|--------|----------|
| `200 OK` | Decode JSON-RPC response from body |
| `202 Accepted` | Return empty response (used for notifications like `notifications/initialized`) |
| `401 Unauthorized` | Return `UnauthorizedError`, trigger `onDisconnect` callback |
| Other | Return error with status code |

### UnauthorizedError

```go
type UnauthorizedError struct {
    StatusCode int
}
```

Implements `error`. Used by callers (via `errors.As`) to detect auth failures and trigger re-authentication flows.

## 11.7 Service Lifecycle

### Configure

`Configure(ctx, mcpConfigs)` reconciles the running server set with the desired configuration:

1. **Lock** -- acquires write lock
2. **Remove** -- servers absent from the new config are collected for stopping
3. **Add** -- servers present in the new config but not running are created as `serverConn` with `StatusDisconnected`
4. **Unlock**
5. **Stop removed** -- calls `stopServer()` on each removed connection (cancel context, close transport)
6. **Connect new** -- launches `connectServer()` in a goroutine per new server

### Connect Server

`connectServer(ctx, name)` performs the full connection sequence:

1. Set status to `StatusConnecting`, publish status event
2. Create transport via `createTransport(cfg)`, set disconnect/notification callbacks
3. Call `transport.Connect(ctx)` with a derived cancellable context
4. Call `transport.ListTools(ctx)` to discover available tools
5. Store transport, tools, context; set status to `StatusConnected`
6. Publish status event

On any failure, status transitions to `StatusError` with the error message.

### Transport Callbacks

`setTransportCallbacks(transport, ctx, name)` configures two callbacks on the transport via type switch:

| Callback | Trigger | Action |
|----------|---------|--------|
| `onDisconnect` | Transport detects connection loss | Launches `reconnectServer()` in a goroutine |
| `onNotification` | Server sends `notifications/tools/list_changed` | Calls `refreshTools()` to re-list tools |

### Stop Server

`stopServer(conn)` cancels the connection context and closes the transport.

### Restart

`Restart(ctx, name)` stops the named server, resets its state to `StatusDisconnected`, and launches a new `connectServer()` goroutine. Returns error if server name is unknown.

### Close

`Close()` stops all servers and clears the server map.

## 11.8 Reconnection

File: `reconnect.go`

When a transport detects disconnection, `reconnectServer()` attempts to restore the connection with exponential backoff.

### Parameters

| Constant | Value | Description |
|----------|-------|-------------|
| `reconnectBaseDelay` | 1 second | Initial backoff delay |
| `reconnectMaxDelay` | 30 seconds | Maximum backoff delay |
| `reconnectJitter` | 0.25 (25%) | Jitter factor applied to each delay |
| `maxReconnectAttempts` | 10 | Maximum number of reconnection attempts |

### Backoff Formula

```
delay = min(baseDelay * 2^attempt, maxDelay)
jitter = delay * 0.25 * random(-1, 1)
finalDelay = delay + jitter
```

### Reconnection Flow

1. **Guard** -- if server is already reconnecting (`reconnecting` flag), return immediately
2. **Mark** -- set `reconnecting = true`, status to `StatusReconnecting`
3. **Loop** -- up to `maxReconnectAttempts` times:
   a. Compute backoff delay via `backoffDelay(attempt)`
   b. Publish `mcp.reconnecting` bus event with server name and attempt number
   c. Wait for delay (or context cancellation)
   d. Stop existing transport, clear connection state
   e. Call `connectServer()` to attempt fresh connection
   f. If connected, publish `mcp.reconnected` event and return
4. **Exhaust** -- if all attempts fail, set status to `StatusError` with message, publish status

### Tool Refresh

`refreshTools(name)` re-lists tools from a connected server and replaces the stored tool list. Triggered by `notifications/tools/list_changed` notifications. Publishes a status event after refresh.

## 11.9 Tool Bridging

MCP tools are converted to the internal `tool.Def` format for use by the LLM tool-calling loop.

### Tool ID Format

```
mcp__{serverName}__{toolName}
```

Example: server `"websearch"` with tool `"search"` becomes `mcp__websearch__search`.

### Conversion

`convertMCPTool(serverName, tool, transport)` creates a `tool.Def`:

| Field | Value |
|-------|-------|
| `ID` | `mcp__{serverName}__{toolName}` |
| `Description` | From `MCPTool.Description` |
| `Parameters` | From `MCPTool.InputSchema` (parsed to `map[string]any`). Falls back to `{"type": "object", "properties": {}}` if schema is nil or empty. |
| `Permission` | `"mcp"` |
| `Execute` | Calls `transport.CallTool(ctx, toolName, args)`. On transport error, returns `ExecuteResult{Output: err.Error(), IsError: true}` (does not propagate Go error). |

### Tool Registration

`Tools(ctx)` returns a `map[string]*tool.Def` of all tools from connected servers. Only servers with `StatusConnected` are included. The session manager calls this during prompt processing to register MCP tools alongside built-in tools.

## 11.10 OAuth Support

File: `oauth.go`

PKCE-based OAuth 2.0 authorization code flow for MCP servers requiring authentication.

### Flow Steps

1. **`StartAuth(ctx, cfg)`** -- generates cryptographic state (32 bytes, hex) and PKCE code verifier (32 bytes, base64url). Computes S256 code challenge. Starts a local callback HTTP server if not already running. Saves OAuth state (verifier + state) to disk keyed by `client_id`. Returns the full authorization URL.

2. **`WaitForCallback(ctx, state)`** -- registers a channel for the given state string and waits for the OAuth callback (or timeout at 5 minutes, or context cancellation). Returns the authorization code.

3. **`ExchangeCode(ctx, cfg, code)`** -- POSTs to the token URL with `grant_type=authorization_code`, the code, redirect URI, client ID, and PKCE code verifier (loaded from saved state). Parses the token response and saves tokens to disk. Returns `OAuthTokens`.

### Callback Server

- Listens on `127.0.0.1:19876` by default (configurable via `callback_url`)
- Serves `GET /mcp/oauth/callback`
- Matches the `state` parameter against pending flows
- Returns HTML success/error pages

### Token Storage

Tokens are persisted to `$XDG_DATA_HOME/tinycode/mcp-auth.json` (default: `~/.local/share/tinycode/mcp-auth.json`). File is written atomically via temp file + rename with `0600` permissions. Keyed by `client_id`.

```go
type OAuthTokens struct {
    AccessToken  string `json:"accessToken"`
    RefreshToken string `json:"refreshToken,omitempty"`
    ExpiresAt    int64  `json:"expiresAt,omitempty"`
}
```

## 11.11 Bus Events

Three bus events are published for MCP status changes:

| Event | Properties | Description |
|-------|-----------|-------------|
| `mcp.status` | `{server: ServerStatus}` | Published on any connection state change (connecting, connected, error, etc.) |
| `mcp.reconnecting` | `{server: string, attempt: int}` | Published before each reconnection attempt |
| `mcp.reconnected` | `{server: string}` | Published on successful reconnection |

## 11.12 Status API

`Status(ctx)` returns `map[string]ServerStatus` with the current state of all configured servers:

```json
[
  {"name": "my-server", "status": "connected", "toolCount": 5},
  {"name": "remote-server", "status": "error", "error": "connection refused", "toolCount": 0}
]
```

Exposed via `GET /mcp/status` on the HTTP server.

---

Prev: [10-skills.md](10-skills.md) | Next: [12-permissions.md](12-permissions.md)
