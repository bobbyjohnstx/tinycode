# AGENTS.md (TUI)

Go terminal UI built with [bubbletea](https://github.com/charmbracelet/bubbletea) (Elm-style `Model` / `Update` / `View`). One package. `app.go` is the root model. `run.go` is `connectedApp`.

`api/` is HTTP/SSE. `render/` is markdown and tool drawing.

## prompt

`prompt.go`, `prompt_*.go`

## chat

`chat.go`, `chat_render.go`, `chat_search.go`

## welcome

`welcome.go`

## sidebar

`sidebar.go`

## status

`statusbar.go`

## palette

`palette.go`, `palette_files.go`, `palette_sessions.go`. Full-screen.

## permission

`permission.go`. Full-screen, before other overlays.

## dialogs

`dialog_*.go` only.

## commands

`commands.go`, `app_commands.go`, `run_handlers_commands.go`

## Commands

- Test: `go test ./internal/tui/... -count=1`
- Lint: `make lint` (from repo root)

Shared project rules, pitfalls, and commit conventions live in the repo root [AGENTS.md](../../AGENTS.md).
