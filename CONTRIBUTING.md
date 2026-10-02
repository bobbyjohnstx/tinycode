# Contributing to tinycode

## Development Setup

1. Install [Go](https://go.dev/dl/) (1.27+, see `go.mod` for exact version)
2. Install `make` and `git`
3. Clone the repository
4. Run `make build` to verify setup

## Commands

```bash
# Build
make build                              # builds dist/tinycode

# Test
go test ./... -count=1                  # all tests (or: make test)
go test ./internal/tui/... -count=1     # single package

# Lint
go vet ./...                            # or: make lint

# Build + lint + test
make check
```

## Where to Start

Read [docs/architecture.md](docs/architecture.md) to understand how the codebase fits together.

Approachable contribution areas:

- **Agent definitions** -- add or improve agent `.md` files in `internal/agent/defaults/`
- **Skills** -- add skill definitions in `internal/skill/`
- **Plugin tools** -- standalone Go binaries using `pkg/plugin/` SDK (no internal access needed)
- **Core tools** -- tool implementations in `internal/tool/`
- **TUI components** -- bubbletea models in `internal/tui/`
- **Documentation** -- anything under `docs/`

There are two paths for adding tools. **Plugin tools** use `pkg/plugin/` and are the simplest way to contribute -- see [docs/adding-a-tool.md](docs/adding-a-tool.md). **Core tools** live in `internal/tool/` and have access to internal packages.

## Coding Standards

- Follow standard Go conventions (`gofmt`, `go vet`)
- Use `slog` for structured logging
- Prefer early returns over `else` blocks
- Keep functions focused and small; extract helpers when they name a real concept
- Error handling: return errors up the stack with `fmt.Errorf("context: %w", err)`. Do not swallow errors silently.
- Prefer value receivers for bubbletea `Update`/`View` methods (required by `tea.Model`)
- Use pointer receivers for methods that mutate state
- Test files live next to source: `foo.go` / `foo_test.go`
- Keep files under 800 lines
- Use table-driven tests where appropriate
- Test names: `TestFunctionName_ScenarioDescription`

See [AGENTS.md](AGENTS.md) for additional coding style rules.

## Commit Messages

Use conventional commit format:

```
<type>(<scope>): <description>
```

Types: `feat`, `fix`, `refactor`, `docs`, `test`, `chore`, `perf`, `ci`

Valid scopes: `tui`, `server`, `session`, `llm`, `provider`, `agent`, `tool`, `plugin`, `config`, `storage`, `bus`, `mcp`, `acp`, `permission`, `skill`, `command`, `project`, `vcs`

Examples:

```
feat(tool): add webfetch tool for HTTP GET requests
fix(tui): filter terminal OSC responses from textarea input
refactor(session): extract compaction logic into separate function
```

## Pull Request Process

Please open a [GitHub Issue](https://github.com/bobbyjohnstx/tinycode/issues) before submitting a PR for anything beyond a typo or documentation fix. Code PRs without a prior issue may be closed.

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-feature`)
3. Make your changes
4. Run `make check` (lint + tests) before committing
5. Use conventional commit messages (see above)
6. Push and open a PR against `main`
7. Describe the changes and include a test plan

## Adding Agents

Agents are markdown files in `internal/agent/defaults/`. Each agent has YAML frontmatter defining its mode, description, and tool permissions.

## Changelog

CHANGELOG.md is maintained by the project maintainer at release time. Contributors do not need to add changelog entries.

## Legal

No CLA or DCO is required to contribute.

## Questions?

Open a [GitHub Issue](https://github.com/bobbyjohnstx/tinycode/issues) for bugs or feature requests. For security vulnerabilities, see [SECURITY.md](SECURITY.md).
