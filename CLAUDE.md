# CLAUDE.md

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
go vet ./...                    # or: make lint

# Embed web app into binary (requires packages/app built first)
make embed-webapp
```

## Pitfalls

Things that break silently if you guess wrong.

- **`earlyinit` import order**: The blank import `_ "internal/earlyinit"` in `cmd/tinycode/main.go` MUST be the first import. It sets lipgloss dark-background defaults before any TUI package initializes. Moving or removing it breaks all TUI colors with no error.
- **Plugin JSON-RPC field is `"args"`, not `"arguments"`**: `pkg/plugin/protocol.go` uses `json:"args"`. Sending `"arguments"` silently zero-values the struct — the tool runs with empty params.
- **`defaultAutoContinueMax = 0` is intentional**: The agent does not auto-continue by default. Setting this to a positive number enables unsupervised agent loops that burn tokens and run tools without consent.
- **`SetWorking(true)` return value must be propagated**: `StatusBar.SetWorking()` returns a `tea.Cmd` that drives the spinner tick chain. Discarding it freezes the spinner — the UI appears hung.
- **Config file 3-name fallback**: Config loading tries `tinycode.jsonc` → `tinycode.json` → `config.json` in each directory. Only checking one name silently ignores user config.
- **`internal/static/dist/` is committed, not gitignored**: The `//go:embed dist/*` directive in `internal/static/static.go` requires these files at compile time. Deleting them as build artifacts breaks `go build`.
- **`LSPConfig` accepts both forms**: `"lsp": true` (boolean shorthand) and `"lsp": { "enabled": true, ... }` (struct). Always expecting an object breaks boolean-shorthand users.
- **SQLite is pure Go (`modernc.org/sqlite`), not cgo**: No C toolchain needed. Setting `CGO_ENABLED=1` or switching to `mattn/go-sqlite3` breaks cross-compilation.
- **Project config walks UP the directory tree**: Config files in nested directories merge with parent configs (innermost wins). Only checking the project root ignores monorepo nested configs.
- **Plugin system is JSON-RPC over stdin/stdout**: Plugins are standalone Go binaries spawned as child processes, not HTTP services. `pkg/plugin/` is the public SDK.
- **`packages/` is legacy TypeScript**: Not Go code. The web app in `packages/app` can be embedded via `make embed-webapp`, but the directory is from the original TypeScript repo.
- **`safego.Go()` must wrap all goroutine launches**: Bare `go func()` skips panic recovery — a panic in the goroutine crashes the process with no log. `internal/safego` adds `recover()` + `slog.Error`.
- **Plugin tool timeout is 30s, hook timeout is 5s**: `internal/plugin/manager.go` constants. Plugins that exceed these are killed. Changing them affects all plugins and shutdown timing.

## Agent Delegation

Use specialized agents instead of doing everything inline:

- **`debugger`** — Finding and diagnosing bugs. Root-cause analysis, race conditions, stack traces.
- **`executor`** — Writing and editing code. Implementation work, refactors, applying fixes.
- **`architect`** — Designing solutions. Architecture decisions, API design, system-level trade-offs.
