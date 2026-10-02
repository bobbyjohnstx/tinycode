# Structural Review: tinycode

## 1. Project Layout and Conventions

### 1.1 Standard Go Layout
The project follows standard Go conventions:
- `cmd/` — Binaries (entry points)
- `internal/` — Private packages (core logic, not importable from outside)
- `pkg/plugin/` — Public Go plugin SDK (importable by external consumers)

This separation is clean and follows the "internal" convention correctly. The `pkg/plugin/` SDK is intentionally public so plugin authors can build against it.

### 1.2 File Organization
- Source files are organized by concern within each package directory
- Test files live alongside source: `foo.go` / `foo_test.go` (Go convention)
- Total Go lines across `internal/`: ~46,838 lines

### 1.3 Package Naming
All internal packages use lowercase names matching Go conventions: `tui`, `server`, `session`, `llm`, `provider`, etc. The module path is `github.com/bobbyjohnstx/tinycode`.

---

## 2. Dependency Graph

### 2.1 Core Dependencies (High-Level)

```
                    ┌─────────────┐
                    │   tui/      │  ← Terminal UI (bubbletea)
                    └──────┬──────┘
                           │ uses
              ┌────────────┼────────────┐
              ▼            ▼            ▼
        ┌───────────┐  ┌──────────┐  ┌────────────┐
        │ session/  │  │ server/  │  │ llm/       │
        └─────┬─────┘  └──────────┘  └─────┬──────┘
              │                              │
              ▼                              ▼
        ┌───────────┐                ┌────────────┐
        │ storage/  │                │ provider/  │
        └───────────┘                └─────┬──────┘
                                           │ uses
              ┌────────────────────────────┼─────────┐
              ▼                            ▼         ▼
        ┌──────────┐               ┌───────────┐  ┌──────┐
        │ bus/     │               │ config/   │  │tool/ │
        └──────────┘               └───────────┘  └──────┘
```

### 2.2 Detailed Package Dependencies

| Package | Depends On | Notes |
|---------|-----------|-------|
| `acp/` | `bus`, `session` | Agent Client Protocol transport |
| `agent/` | `permission` | Agent definitions, prompt loading |
| `bus/` | `id` | Event bus for inter-component communication |
| `command/` | `frontmatter` | Slash command discovery from markdown files |
| `config/` | *(none)* | Config file parsing (JSONC), standalone utility |
| `earlyinit/` | *(none)* | Package-init side effects (runs before other imports) |
| `frontmatter/` | *(none)* | YAML-like frontmatter parser for markdown |
| `id/` | *(none)* | Sortable ID generation with typed prefixes |
| `llm/` | *(none)* | LLM client abstraction (OpenAI-compatible streaming) |
| `lsp/` | `tool` | LSP client for code intelligence |
| `mcp/` | `bus`, `config`, `tool` | Model Context Protocol client |
| `permission/` | `bus`, `id` | Tool permission prompting and rules |
| `plugin/` | `config`, `id` | Plugin lifecycle management (spawns subprocesses) |
| `project/` | *(none)* | Project metadata, VCS detection, worktree paths |
| `provider/` | `bus` | Provider auto-discovery (Ollama, vLLM, LM Studio) |
| `redhat/` | *(none)* | Red Hat shared library (OcClient, APIClient, etc.) |
| `server/` | `agent`, `bus`, `config`, `mcp`, `permission`, `plugin`, `provider`, `session`, `tool` | HTTP server (chi router), REST + SSE endpoints |
| `session/` | `id`, `llm`, `provider`, `permission` | Session lifecycle, processor loop, LLM coordination |
| `skill/` | `frontmatter` | Skill discovery and loading |
| `static/` | *(none)* | Embedded web app file server with SPA fallback |
| `storage/` | *(none)* | SQLite via modernc.org/sqlite, migrations |
| `tool/` | `bus`, `id`, `llm`, `permission`, `skill`, `session` | Tool implementations (file ops, shell, grep, glob) |
| `tui/` | `session`, `tui/api`, `tui/render` | Terminal UI (bubbletea, Elm architecture) |
| `vcs/` | *(none)* | Git operations |

### 2.3 Dependency Analysis

**Strengths:**
- `config/`, `id/`, `frontmatter/`, `storage/` are leaf packages with no internal dependencies — they can be tested in isolation
- `llm/` is a leaf package (no internal deps) — critical for testing since it's the most complex
- `bus/` is a leaf package — essential infrastructure tested independently

**Concerns:**
- **`tool/` has 6 internal dependencies**: `bus`, `id`, `llm`, `permission`, `skill`, `session`. This is the most coupled package and could be hard to test in isolation.
- **`server/` has 9 internal dependencies**: the most coupled package, making integration testing essential.
- **`session/` has 4 internal dependencies**: `id`, `llm`, `provider`, `permission`. The session processor loop is complex and tightly coupled to these.
- **`mcp/` has 3 internal dependencies**: `bus`, `config`, `tool`. The MCP client depends on the tool system.

**No Circular Dependencies Detected** — The dependency graph is a DAG (directed acyclic graph).

---

## 3. Architecture Patterns

### 3.1 Bubbletea Elm Architecture (`internal/tui/`)
- **Immutable state**: `Update()` returns a new model, never mutates in place
- **Value receivers for `Update`/`View`**: Required by the `tea.Model` interface
- **Pointer receivers for state-mutating methods**: e.g., `resize()`, `setFocus()`
- **`tea.Cmd` for async work**: API calls, SSE connections, spinner animation tick chains
- **`tea.Msg` for event dispatch**: Messages flow from components to the root `App` model
- **Root `App` composes sub-components**: `chat`, `prompt`, `status`, `sidebar`, dialogs, palette, permission prompt, toast

**Example pattern from `app.go`:**
```go
type App struct {
    chat       ChatView
    prompt     PromptInput
    status     StatusBar
    // ... other components
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.WindowSizeMsg:
        // ...
    }
}

func (a App) View() string {
    return fmt.Sprintf("%s\n%s", a.chat.View(), a.prompt.View())
}
```

### 3.2 ConnectedApp Wrapper (`internal/tui/run.go`)
- Wraps the pure TUI `App` with an API client (`tui/api.Client`)
- Handles prompt submission, session creation, SSE event mapping, permission replies
- The inner `App` is pure UI state; the wrapper adds server communication

### 3.3 SSE Event Flow
```
Server (SSE) → tui/api.Client.Subscribe() → channel of ServerEvent
                                                      ↓
                                              waitForSSE() converts to tea.Cmd
                                                              ↓
                                              mapSSEToMsg() maps events to tea.Msg
                                                              ↓
                                                          App.Update() handles messages
```

### 3.4 Plugin System Architecture
- **Plugins are standalone Go binaries** in `cmd/plugin-*/` (36 total)
- **Communication**: JSON-RPC 2.0 over stdin/stdout
- **Plugin manager** (`internal/plugin/manager.go`): Spawns processes, performs initialize handshake, dispatches hooks and tool calls
- **Hook system** (`internal/plugin/hook.go`): Plugins register hooks (session lifecycle, permission, tool execution)
- **Tool registration**: Plugins expose tools via `pkg/plugin.ToolManifest`

**Wire protocol:**
```go
// Plugin → Manager: JSON-RPC request
{ "jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {...} }

// Manager → Plugin: JSON-RPC response
{ "jsonrpc": "2.0", "id": 1, "result": {...} }

// Manager → Plugin: Tool call
{ "jsonrpc": "2.0", "id": 2, "method": "tool_call", "params": {
    "name": "...", "args": {...}, "context": {"sessionId":"...","directory":"..."}
} }

// Plugin → Manager: Tool result
{ "jsonrpc": "2.0", "id": 2, "result": {
    "content": "...", "isError": false
} }
```

### 3.5 Event Bus (`internal/bus/`)
- Central event bus for inter-component communication
- Used by: `acp`, `agent`, `mcp`, `permission`, `plugin`, `provider`, `session`, `tool`
- Provides a pub/sub mechanism for decoupling components

### 3.6 Session Processor Loop (`internal/session/`)
- Orchestrates the LLM conversation: prompt → LLM call → tool execution → response
- `processor_loop.go`: Main loop coordinating the flow
- `processor_llm.go`: LLM call coordination
- `processor_tools.go`: Tool execution after LLM response
- `processor_validation.go`: Input validation before processing

### 3.7 Provider Auto-Discovery (`internal/provider/`)
- Probes local LLM providers at startup:
  - Ollama on `127.0.0.1:11434`
  - LM Studio on `127.0.0.1:1234`
  - vLLM (requires env var)
- Uses `bus/` for event-driven discovery updates

---

## 4. Error Handling Patterns

### 4.1 Consistent Wrapping Pattern
The project consistently uses `fmt.Errorf` with `%w` for error wrapping:

```go
// Good patterns observed:
return nil, fmt.Errorf("marshaling request: %w", err)
return nil, fmt.Errorf("creating request: %w", err)
return nil, fmt.Errorf("sending request: %w", err)
return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
return nil, fmt.Errorf("reading config %s: %w", path, err)
return nil, fmt.Errorf("parsing JSONC: %w", err)
```

### 4.2 Error Propagation
- Errors are returned up the stack with context, not swallowed silently
- The pattern `return nil, fmt.Errorf("context: %w", err)` is used throughout
- HTTP errors include status codes for debugging

### 4.3 Timeout Handling
- Plugin hook timeout: 5 seconds (`hookTimeout = 5 * time.Second`)
- Plugin kill timeout: 3 seconds (`killTimeout = 3 * time.Second`)
- LLM chunk read timeout: configurable per provider

### 4.4 Areas for Improvement
- Some error messages could be more descriptive (e.g., `"HTTP %d: %s"` doesn't include the URL)
- The `internal/tui/run.go` error wrapping `"TUI error: %w"` is too generic — should include context about what operation failed

---

## 5. Testing Strategy

### 5.1 Test Distribution
Tests are organized alongside source files (`foo.go` / `foo_test.go`).

**Tested packages:**
- `internal/tui/`: Multiple test files (run_test.go, prompt_autocomplete_test.go)
- `internal/session/`: 4 test files (job_test.go, processor_test.go, prompt_test.go, session_test.go)
- `internal/server/`: 4 test files (handler_plugin_test.go, integration_test.go, pending_store_test.go, respond_test.go, server_test.go, sse_test.go)
- `internal/llm/`: 4 test files (anthropic_test.go, event_test.go, llm_test.go, openai_test.go, repair_test.go)
- `internal/plugin/`: 6 test files (builtin_test.go, config_test.go, hook_test.go, manager_test.go, registry_test.go, resolve_test.go)
- `internal/tui/api/`: Tests for SSE and API client

### 5.2 Test Patterns
- **Table-driven tests**: Used where appropriate (e.g., `TestFunctionName_ScenarioDescription`)
- **Integration tests**: `server/integration_test.go` tests end-to-end flows
- **Unit tests for handlers**: Individual handler functions tested in isolation

### 5.3 Coverage Concerns
- **`internal/static/`**: No tests — embedded web app file server
- **`internal/frontmatter/`**: No tests — YAML-like frontmatter parser
- **`internal/id/`**: No tests — ID generation with typed prefixes
- **`internal/project/`**: No tests — Project metadata and VCS detection
- **`internal/vcs/`**: No tests — Git operations
- **`internal/redhat/`**: No tests — Large shared library (OcClient, APIClient, etc.)
- **`internal/earlyinit/`**: No tests — Package-init side effects

---

## 6. Build System (`Makefile`)

### 6.1 Targets
| Target | Description |
|--------|-------------|
| `build` | Build for current platform → `dist/tinycode` |
| `build-plugins` | Build all 36 plugins for current platform → `dist/plugins/` |
| `build-full` | Build tinycode + all plugins for current platform |
| `build-all` | Cross-compile for 5 platforms (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64) |
| `build-all-plugins` | Cross-compile all plugins for all platforms (25 binaries) |
| `build-all-full` | Cross-compile everything for all platforms (26 binaries) |
| `package` | Create release archives with tinycode + all plugins for all platforms |
| `test` | Run all tests: `go test ./... -count=1` |
| `vet` / `lint` | Run `go vet ./...` |
| `check` | Run vet + tests |
| `embed-webapp` | Build SolidJS web app and embed into Go binary via `script/embed-webapp.sh` |
| `clean` | Remove build artifacts (`dist/`, run `go clean`) |

### 6.2 Build Configuration
- **LDFLAGS**: Strips symbols (`-s -w`), embeds version/commit/date via `-X main.version=...`
- **Version**: Derived from `git describe --tags --always --dirty` (falls back to "dev")
- **Platforms**: 5 platforms supported for cross-compilation

### 6.3 Build Complexity
- **26 binaries total**: 1 main binary + 36 plugins (but some platforms may not have all plugins)
- **Cross-compilation**: 5 platforms × (1 + 36) = up to 185 binaries for `build-all-full`
- **No caching**: Each build recompiles from scratch — could benefit from `go build -o` with incremental builds or a build cache

---

## 7. Plugin SDK (`pkg/plugin/`)

### 7.1 Protocol Design
The plugin SDK defines:
- **`InitializeParams`**: Version, directory, options sent during handshake
- **`InitializeResult`**: Plugin ID, tools, hooks registered
- **`ToolManifest`**: Name, description, input schema for tool registration
- **`ToolCallParams`**: Name, args, context (sessionId, directory)
- **`ToolCallResult`**: Content, isError flag

### 7.2 Hook System
Plugins can register hooks for:
- Session lifecycle events (created, updated, etc.)
- Permission decisions (allow/reject tool calls)
- Tool execution hooks

### 7.3 Strengths
- Clean separation between protocol types and implementation
- Tests alongside source files
- JSON-RPC 2.0 standard for interoperability

### 7.4 Concerns
- The wire format types in `internal/plugin/` (e.g., `toolCallParams`, `initializeParams`) are duplicated from `pkg/plugin/` types. This creates a potential drift risk if the SDK changes without updating internal types.

---

## 8. Key Architectural Decisions

### 8.1 Single Binary Architecture
- All components (TUI, server, sessions, LLM client, tools) run in a single process
- Pros: No IPC overhead, simpler deployment, no separate server process to manage
- Cons: Crash of one component takes down everything; harder to scale

### 8.2 SQLite for Storage
- Uses `modernc.org/sqlite` (pure Go, no CGO) — good for portability
- Migrations in `sql/` directory with sqlc.yaml configuration
- Pros: No external dependencies, works on all platforms
- Cons: Not designed for high-concurrency writes; single-process architecture mitigates this

### 8.3 Bubbletea for TUI
- Elm architecture with immutable state — good functional programming patterns
- Pros: Clean separation of concerns, testable components
- Cons: Bubbletea has a learning curve; some advanced features may be limited

### 8.4 Provider Auto-Discovery
- Probes providers at startup rather than requiring explicit configuration
- Pros: User-friendly, works out-of-the-box with local providers
- Cons: Adds startup latency; probes may fail on slow networks

### 8.5 Red Hat Library (`internal/redhat/`)
- Substantial shared library with multiple clients (OcClient, APIClient, ConsoleAuthClient, PromQLClient)
- Pros: Reusable across plugins; consistent API
- Cons: Large codebase that's hard to test and maintain

---

## 9. Strengths of the Architecture

1. **Clean separation of concerns**: TUI, server, session, LLM client are distinct packages
2. **Standard Go conventions**: `cmd/`, `internal/`, `pkg/` layout; test files alongside source
3. **Event-driven communication**: Bus-based inter-component messaging reduces coupling
4. **Plugin system design**: Standalone binaries with JSON-RPC is a solid pattern for extensibility
5. **Error handling consistency**: `fmt.Errorf` with `%w` wrapping throughout
6. **Immutable TUI state**: Elm architecture makes reasoning about UI state easier
7. **Provider auto-discovery**: Good UX for local-first workflow
8. **Pure Go SQLite**: No CGO dependency, works on all platforms

---

## 10. Areas for Improvement

### 10.1 High Priority
1. **Test coverage gaps**: `static/`, `frontmatter/`, `id/`, `project/`, `vcs/`, and `redhat/` have no tests
2. **`tool/` package coupling**: 6 internal dependencies make it hard to test in isolation
3. **Error message quality**: Some error messages are too generic (e.g., `"TUI error: %w"`)

### 10.2 Medium Priority
4. **Plugin wire format duplication**: `internal/plugin/` has duplicate types from `pkg/plugin/`; consider using the SDK types directly
5. **Build system complexity**: 26+ binaries with no caching; consider `go build` flags or a build cache
6. **`server/` package size**: 9 internal dependencies and many handler files; consider splitting into sub-packages
7. **`session/` package complexity**: 4 internal dependencies and multiple processor files; consider extracting common logic

### 10.3 Low Priority
8. **Cross-compilation overhead**: Building for all platforms × all plugins is expensive; consider on-demand builds
9. **Documentation**: Architecture review exists (`docs/architecture-review.md`) but could be more detailed
10. **`earlyinit/` package**: Runs before other imports but has no tests; fragile if dependencies change

---

## 11. Summary

The project follows solid Go conventions with a clean separation of concerns. The single-binary architecture is well-suited for the use case (local-first AI coding assistant). The plugin system design with JSON-RPC over stdin/stdout is a robust pattern for extensibility. Error handling is consistent, and the Bubbletea Elm architecture provides good functional programming patterns for the TUI.

Key risks are:
- **Test coverage gaps** in utility packages (no tests for `static/`, `frontmatter/`, `id/`, etc.)
- **Tight coupling** in `tool/` (6 deps) and `server/` (9 deps) packages
- **Plugin wire format duplication** between `internal/plugin/` and `pkg/plugin/`

The architecture is well-structured for a project of this complexity, but the large number of plugins (36) and the substantial `redhat/` library suggest that future growth may benefit from more modular package splitting.
