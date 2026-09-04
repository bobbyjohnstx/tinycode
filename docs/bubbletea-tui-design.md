# Bubbletea TUI Architecture Design

## Overview

Go bubbletea-based TUI replacing the TypeScript SolidJS/opentui TUI (~29K lines). The TUI is a client of the Go HTTP API server (same as the web UI), not an embedded server.

## Component Hierarchy

14 bubbletea models composed in a root `App` model:

```
App (root)
├── ChatView        — scrollable message list with viewport
├── PromptInput     — multi-line textarea + slash autocomplete + metadata bar
├── StatusBar       — cwd, model info, working indicator, hints
├── Sidebar         — session tree, agent/model info (toggleable)
├── CommandPalette  — fuzzy search overlay (ctrl+p)
├── Dialog          — modal dialogs (model picker, session list, agent list)
│   ├── ModelDialog
│   ├── SessionDialog
│   └── AgentDialog
├── PermissionPrompt — allow/always/reject inline
├── QuestionPrompt   — inline questions from agent
├── Toast           — ephemeral notifications
└── Spinner         — working state animation
```

## Shared State

```go
type AppState struct {
    Route         Route
    ActiveSession string

    // Server-synced
    Sessions      []session.Info
    Messages      map[string][]MessageView
    Parts         map[string][]PartView
    Providers     []provider.Info
    Agents        []AgentInfo
    Skills        []SkillInfo
    Commands      []CommandInfo
    Permissions   map[string][]PermissionRequest
    SessionStatus map[string]SessionStatus

    // Local UI state
    CurrentAgent  string
    CurrentModel  ModelSelection
    SidebarOpen   bool
}
```

## Focus System

```go
type FocusTarget int
const (
    FocusPrompt FocusTarget = iota
    FocusPalette
    FocusDialog
    FocusSidebar
    FocusPermission
)
```

## SSE-to-Bubbletea Bridge

SSE events from `GET /event` are mapped to typed bubbletea messages:

| SSE Event Type | Bubbletea Msg |
|----------------|---------------|
| `message.part.delta` | `MessagePartDeltaMsg` |
| `message.updated` | `MessageUpdatedMsg` |
| `message.part.updated` | `MessagePartUpdatedMsg` |
| `session.status` | `SessionStatusMsg` |
| `session.created` | `SessionCreatedMsg` |
| `session.deleted` | `SessionDeletedMsg` |

SSE client runs in a goroutine, feeds events into `tea.Cmd` chain:

```go
func listenSSE(client *api.Client) tea.Cmd {
    return func() tea.Msg {
        events, err := client.SubscribeEvents(ctx)
        if err != nil {
            return SSEDisconnectedMsg{Err: err}
        }
        evt := <-events
        return mapSSEToMsg(evt)
    }
}

func waitForSSE(events <-chan ServerEvent) tea.Cmd {
    return func() tea.Msg {
        evt, ok := <-events
        if !ok {
            return SSEDisconnectedMsg{}
        }
        return mapSSEToMsg(evt)
    }
}
```

## Key Bindings

| Key | Action |
|-----|--------|
| `ctrl+p` | Command palette |
| `ctrl+c` / `ctrl+d` | Exit (empty prompt) / Clear (with text) |
| `escape` | Interrupt session (double-press aborts) |
| `tab` / `shift+tab` | Cycle agents |
| `f2` / `shift+f2` | Cycle recent models |
| `enter` | Submit prompt |
| `shift+enter` / `alt+enter` | Newline in prompt |
| `pgup` / `pgdown` | Scroll messages |
| `ctrl+x b` | Toggle sidebar |
| `ctrl+x o` | Session list |
| `ctrl+x n` | New session |
| `ctrl+x m` | Model list |
| `ctrl+x a` | Agent list |
| `ctrl+x u` | Undo |
| `ctrl+x r` | Redo |

Leader key (`ctrl+x`) requires a state machine: 500ms timeout after press, next key interpreted as command.

## Layout

```
+----------------------------------------------------------+
|  [scrollable chat area]                   | [sidebar]     |
|                                           | Session Tree  |
|  > User message                           | Agent info    |
|    ▸ Read file.go                         | Model info    |
|    Assistant response in markdown...      |               |
|    $ shell command                        |               |
|    ← Edit file.go [diff]                  |               |
+----------------------------------------------------------+
| [permission prompt — replaces prompt when active]        |
+----------------------------------------------------------+
| ╽ prompt text input area                                 |
| ╹ Build · qwen3:8b ollama                               |
|   /cwd/path                tab agents  ctrl+p commands   |
+----------------------------------------------------------+
```

## Rendering Pipeline

- **Markdown**: glamour for terminal rendering with syntax highlighting
- **Streaming**: Split on double newlines, render complete blocks through glamour, leave trailing incomplete block as raw text
- **Tool rendering**: Per-tool dispatch (Shell, Read, Write, Edit, Glob, Grep, WebFetch, Task) with inline (single-line) and block (bordered box) modes
- **Sticky scroll**: Auto-scroll to bottom on new content unless user manually scrolled up

## File Structure

```
internal/tui/
    app.go                  // Root App model
    state.go                // AppState shared state
    msg.go                  // All Msg type definitions
    keys.go                 // KeyMap and DefaultKeyMap()
    layout.go               // View composition
    styles.go               // lipgloss styles and theme
    cmd.go                  // Shared Cmd constructors
    chat.go                 // ChatView: scrollable message list
    chat_render.go          // Message and part rendering
    prompt.go               // PromptInput: multi-line textarea
    prompt_autocomplete.go  // Slash command and file autocomplete
    prompt_history.go       // Prompt history ring buffer
    statusbar.go            // StatusBar
    sidebar.go              // Sidebar: session tree
    palette.go              // CommandPalette: fuzzy search
    dialog.go               // Dialog: model/session/agent pickers
    dialog_model.go         // Model selection dialog
    dialog_session.go       // Session list dialog
    dialog_agent.go         // Agent selection dialog
    permission.go           // PermissionPrompt
    question.go             // QuestionPrompt
    toast.go                // Toast notifications
    spinner.go              // Spinner animation
    api/
        client.go           // HTTP client
        sse.go              // SSE subscription + reconnect
        types.go            // Request/response types
        session.go          // Session CRUD
        provider.go         // Provider/model methods
        permission.go       // Permission reply
    render/
        markdown.go         // glamour-based markdown
        tool.go             // Tool-specific renderers
        diff.go             // Unified diff rendering
        part.go             // Part type dispatch
```

## Dependencies

```
github.com/charmbracelet/bubbletea      // TUI framework
github.com/charmbracelet/lipgloss       // Styling
github.com/charmbracelet/bubbles        // Standard components (viewport, textarea, spinner, list)
github.com/charmbracelet/glamour        // Markdown rendering
github.com/muesli/reflow                // Text wrapping
github.com/atotto/clipboard             // Clipboard access
```

## Phased Implementation

### Phase 1 — Core (functional chat)
App, ChatView, PromptInput, SSE client, basic message rendering, session create/list/switch.

### Phase 2 — Features
Tool rendering per-type, permission prompts, command palette, model/agent selectors, sidebar, toast notifications.

### Phase 3 — Polish
Streaming markdown with glamour, diff rendering, session tree, frecency, prompt history/stash, shell mode, autocomplete.

### Phase 4 — Deferred
Plugin system, workspace support, diff viewer, which-key panel, themes.

## Key Design Decision

The TS TUI uses ~15 nested SolidJS context providers for reactive state. The Go version flattens this into a single `*AppState` struct mutated in `Update` and read in `View` — simpler, but requires disciplined mutation only through the message path.
