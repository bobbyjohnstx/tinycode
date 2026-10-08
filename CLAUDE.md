# CLAUDE.md

Shared project rules for coding agents. Keep this file in sync with `AGENTS.md`, except the Agent Delegation section at the end.

## Commands

```bash
# Build
make build                      # builds dist/tinycode

# Run
./dist/tinycode                 # TUI mode
./dist/tinycode <directory>     # TUI against a different directory
./dist/tinycode serve           # headless API proxy
./dist/tinycode acp             # Agent Client Protocol (IDE integration, stdio)

# Tests
go test ./... -count=1          # all tests (or: make test)
go test ./internal/tui/... -count=1   # single package

# Lint
make lint                       # go vet + staticcheck (make test-race for -race)

# Embed web app into binary (requires packages/app built first)
make embed-webapp
```

## Commits and PR Titles

Use conventional commit-style messages and PR titles: `type(scope): summary`.

Valid types are `feat`, `fix`, `docs`, `chore`, `refactor`, and `test`. Scopes: `tui`, `server`, `llm`, `session`, `provider`, `config`, `tool`, `plugin`, `acp`, `permission`, `mcp`, `vcs`, `bus`, `storage`, `agent`, `skill`, `command`, `project`.

## Style Guide

- Follow standard Go conventions (`gofmt`, `go vet`)
- Prefer early returns over `else` blocks
- Keep functions focused and small; extract helpers when they name a real concept
- Use `slog` for structured logging (already used throughout)
- Error handling: return errors up the stack with `fmt.Errorf("context: %w", err)`. Don't swallow errors silently.
- Prefer value receivers for bubbletea `Update`/`View` methods (required by the `tea.Model` interface)
- Use pointer receivers for methods that mutate state (e.g. `resize()`, `setFocus()`)
- Test files live next to source: `foo.go` / `foo_test.go`

## Testing

- Use table-driven tests where appropriate
- Test names: `TestFunctionName_ScenarioDescription`
- Run from repo root: `go test ./...` or target a package: `go test ./internal/tui/...`

## Pitfalls

Things that break silently if you guess wrong.

- **`earlyinit` import**: `cmd/tinycode/main.go` must keep the blank import `_ "github.com/bobbyjohnstx/tinycode/internal/earlyinit"`, and that import must appear before any TUI package import. It sets lipgloss dark-background defaults before TUI packages initialize. Removing it, or importing a TUI package ahead of it, breaks all TUI colors with no error. It currently follows the standard-library imports. It is not `_ "internal/earlyinit"`.
- **Plugin JSON-RPC field is `"args"`, not `"arguments"`**: `pkg/plugin/protocol.go` uses `json:"args"`. Sending `"arguments"` silently zero-values the struct — the tool runs with empty params.
- **`defaultAutoContinueMax = 0` is intentional**: The agent does not auto-continue by default. Setting this to a positive number enables unsupervised agent loops that burn tokens and run tools without consent.
- **`SetWorking(true)` return value must be propagated**: `StatusBar.SetWorking()` returns a `tea.Cmd` that drives the spinner tick chain. Discarding it freezes the spinner — the UI appears hung.
- **Config file 3-name fallback**: Config loading tries `tinycode.jsonc` → `tinycode.json` → `config.json` in each directory. Only checking one name silently ignores user config.
- **`internal/static/dist/` is committed, not gitignored**: The `//go:embed dist/*` directive in `internal/static/static.go` requires these files at compile time. Deleting them as build artifacts breaks `go build`.
- **`LSPConfig` accepts both forms**: `"lsp": true` (boolean shorthand) and `"lsp": { "enabled": true, ... }` (struct). Always expecting an object breaks boolean-shorthand users.
- **SQLite is pure Go (`modernc.org/sqlite`), not cgo**: No C toolchain needed. Setting `CGO_ENABLED=1` or switching to `mattn/go-sqlite3` breaks cross-compilation.
- **Project config walks UP the directory tree**: Config files in nested directories merge with parent configs (innermost wins). Only checking the project root ignores monorepo nested configs.
- **Plugin system is JSON-RPC over stdin/stdout**: Plugins are standalone Go binaries spawned as child processes, not HTTP services. `pkg/plugin/` is the public SDK.
- **`packages/` is the web UI build**: Not the Go runtime. `make embed-webapp` builds `packages/app` (with `sdk`, `ui`, `plugin`, and `tinycode`). `packages/tinycode` is six helpers under `src/core/util` that the web UI imports. The old TypeScript CLI, server, session, and PTY code is gone, along with `packages/desktop`, `packages/vscode-extension`, `packages/llm`, `packages/http-recorder`, `packages/effect-drizzle-sqlite`, and `packages/script`.
- **`safego.Go()` must wrap all goroutine launches**: Bare `go func()` skips panic recovery — a panic in the goroutine crashes the process with no log. `internal/safego` adds `recover()` + `slog.Error`.
- **Plugin tool timeout is 30s, hook timeout is 5s**: `internal/plugin/manager.go` constants. Plugins that exceed these are killed. Changing them affects all plugins and shutdown timing.
- **`config.share` defaults to `"disabled"`**: Not a working share/publish feature in Go — no `/session/{id}/share` routes; web PTY is unsupported (`PTY_SUPPORTED=false`). Do not advertise share or the in-browser terminal as working.
- **Manual summarize works**: `POST /session/{id}/summarize` and TUI `/compact` run `Processor.Compact` and return `200` + `{"compacted": bool}` — not `501`.
- **First-run is doctor + `/connect`**: Use `tinycode doctor` (diagnose) and TUI `/connect` (configure models). There is no `tinycode setup`. Legacy `/tc-doctor` bash skill is obsolete.
- **Skills paths are plural**: `skills/`. Project agents live at `.tinycode/agent/*.md`.

## Agent Delegation

Use specialized agents instead of doing everything inline:

- **`debugger`** — Finding and diagnosing bugs. Root-cause analysis, race conditions, stack traces.
- **`executor`** — Writing and editing code. Implementation work, refactors, applying fixes.
- **`architect`** — Designing solutions. Architecture decisions, API design, system-level trade-offs.
