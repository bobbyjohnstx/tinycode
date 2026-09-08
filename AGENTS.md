## Commits and PR Titles

Use conventional commit-style messages and PR titles: `type(scope): summary`.

Valid types are `feat`, `fix`, `docs`, `chore`, `refactor`, and `test`. Scopes: `tui`, `server`, `llm`, `session`, `provider`, `config`, `tool`, `plugin`, `acp`.

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
