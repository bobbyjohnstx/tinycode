# Bubbletea TUI Architecture Design

## Overview

Go bubbletea-based TUI replacing the TypeScript SolidJS/opentui TUI (~29K lines). The TUI is a client of the Go HTTP API server (same as the web UI), not an embedded server.

## Component Hierarchy

14 bubbletea models composed in a root `App` model:

```
App (root)
├── ChatView        — scrollable message list with viewport, expandable thought blocks
├── PromptInput     — multi-line textarea + /ask agent autocomplete + metadata bar
├── StatusBar       — cwd, model/provider info, spinner animation, hints
├── WelcomeView     — centered ASCII art + getting-started hints (no-messages state)
├── Sidebar         — session tree, agent/model info (toggleable)
├── CommandPalette  — fuzzy search overlay (ctrl+p)
├── Dialog          — modal dialogs
│   ├── ModelDialog     — two-step: provider list → model list with search filter
│   ├── SessionDialog   — session list picker
│   └── AgentDialog     — agent list picker
├── PermissionPrompt — allow/always/reject with tool-specific context
├── Toast           — ephemeral notifications
└── LeaderState     — ctrl+x state machine (500ms timeout)
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
    CurrentAgent       string
    CurrentModel       ModelSelection
    SidebarOpen        bool
    Connected          bool
    PendingModelDialog bool   // bridges async provider fetch with dialog display
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
| `permission.requested` | `PermissionRequestedMsg` |

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
| `escape` | Interrupt session / close dialog |
| `tab` / `shift+tab` | Cycle agents |
| `T` | Toggle thought/reasoning block expansion |
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
- **Reasoning blocks**: Render as `+ Thought` (collapsed) or `- Thought` (expanded) with duration. `T` key toggles all, `ToggleThoughtMsg{PartID}` toggles one. Expanded content is word-wrapped and styled with `styleReasoningLabel`.
- **Welcome screen**: Centered ASCII art logo + getting-started hints. Shown when `ActiveSession` is empty or session has no messages.
- **Sticky scroll**: Auto-scroll to bottom on new content unless user manually scrolled up

## File Structure

```
internal/tui/
    app.go                  // Root App model, overlay routing, leader dispatch
    run.go                  // connectedApp wrapper: API client, SSE, prompt submission
    state.go                // AppState shared state, PermissionRequest, PartView
    msg.go                  // All Msg type definitions
    keys.go                 // KeyMap and DefaultKeyMap()
    leader.go               // Leader key (ctrl+x) state machine
    layout.go               // View composition, calculateLayout()
    styles.go               // lipgloss styles and Theme
    cmd.go                  // Shared Cmd constructors (fetch*, send*, create*)
    chat.go                 // ChatView: scrollable message list, thought toggle
    chat_render.go          // Message and part rendering (reasoning, tool calls)
    prompt.go               // PromptInput: multi-line textarea, startup guard
    prompt_autocomplete.go  // Slash command + /ask agent autocomplete
    prompt_history.go       // Prompt history ring buffer
    statusbar.go            // StatusBar: model/provider, spinner tick chain
    welcome.go              // WelcomeView: ASCII art + getting-started hints
    sidebar.go              // Sidebar: session tree
    palette.go              // CommandPalette: fuzzy search
    dialog_model.go         // Two-step provider→model dialog with search filter
    dialog_session.go       // Session list dialog
    dialog_agent.go         // Agent selection dialog
    permission.go           // PermissionPrompt: allow/always/reject with context
    toast.go                // Toast notifications
    api/
        client.go           // HTTP client
        sse.go              // SSE subscription + reconnect
        types.go            // Request/response types
    render/
        markdown.go         // glamour-based markdown
        tool.go             // Tool-specific renderers
        diff.go             // Unified diff rendering
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

## Implementation Status

All core phases are implemented:

- **Core**: App, ChatView, PromptInput, SSE client, message rendering, session CRUD
- **Features**: Tool rendering, permission prompts (with context), command palette, two-step model/agent selectors, sidebar, toast, /ask autocomplete
- **Polish**: Streaming markdown, diff rendering, session tree, prompt history, welcome screen, expandable thought blocks, spinner animation, /connect with search filter

### Remaining
Plugin system (Go binary plugins are built but UI integration is partial), workspace support, diff viewer, which-key panel, themes.

## Key Design Decisions

- **State model**: The TS TUI uses ~15 nested SolidJS context providers for reactive state. The Go version flattens this into a single `*AppState` struct mutated in `Update` and read in `View` — simpler, but requires disciplined mutation only through the message path.
- **Async dialog pattern**: Dialogs that need fresh server data (like `/connect`) use a pending flag + refresh message + loaded handler pattern: set `PendingModelDialog=true`, emit `ProvidersRefreshMsg`, then open the dialog when `ProvidersLoadedMsg` arrives.
- **Config compatibility**: On macOS, config is loaded from both `~/Library/Application Support/tinycode/` and `~/.config/tinycode/` so that config files from the TS version are found automatically.

## Testing

### Unit tests

```bash
go test ./internal/tui/... -count=1
```

### Side-by-side TUI comparison

`script/tui-compare.sh` runs TS tinycode and Go tinycode in a split tmux session for visual comparison. See the script for commands (`launch`, `send`, `capture`, `diff`, `kill`).
