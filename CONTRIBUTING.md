# Contributing to tinycode

## Development Setup

1. Install [Bun](https://bun.sh) (v1.3+)
2. Clone the repository
3. Run `bun install`
4. Run `bun dev` to start in development mode

## Commands

- `bun dev` — Run TUI in development mode
- `bun run lint` — Lint with oxlint
- `bun typecheck` — Type check all packages (runs via Turborepo from repo root)
- `cd packages/tinycode && bun test` — Run tests

## Where to Start

Read [docs/architecture.md](docs/architecture.md) first to understand how the codebase fits together.

Approachable contribution areas:

- **Agent definitions** -- add or improve agent `.md` files in `packages/tinycode/src/agent/defaults/`
- **Skills** -- add skill definitions in `packages/tinycode/src/skill/defaults/`
- **Plugin tools** -- create tools via the `@tinycode/plugin` SDK (no Effect knowledge needed)
- **Documentation** -- improve anything under `docs/`

There are two paths for adding tools. **Plugin tools** use `@tinycode/plugin` and are the easiest way to contribute -- see [docs/adding-a-tool.md](docs/adding-a-tool.md). **Core tools** live in `packages/tinycode/src/tool/` and require familiarity with the [Effect](https://effect.website) framework.

> **Note:** `bun test` cannot be run from the repo root. Always `cd` into a package first (e.g., `cd packages/tinycode && bun test`).

## Pull Request Process

Please open a [GitHub Issue](https://github.com/bobbyjohnstx/tinycode/issues) before submitting a PR for anything beyond a typo or documentation fix. Code PRs without a prior issue may be closed.

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-feature`)
3. Make your changes
4. Run `bun run lint`, `bun typecheck`, and `cd packages/tinycode && bun test` before committing
5. Use conventional commit messages: `feat:`, `fix:`, `refactor:`, `docs:`, `test:`, `chore:`
6. Push and open a PR against `main`

CI runs three gates on every PR: **lint**, **typecheck**, and **test** (see `.github/workflows/ci.yml`). Some tests (TUI, httpapi-listen, help) are excluded in CI.

## Coding Standards

- TypeScript with Effect framework for server-side code
- SolidJS for TUI and web UI
- See [AGENTS.md](AGENTS.md) for detailed coding style rules
- Keep files under 800 lines
- Prefer immutable patterns for shared state

## Adding Agents

Agents are markdown files in `packages/tinycode/src/agent/defaults/`. See [docs/internal/agent-prompt-tiers.md](docs/internal/agent-prompt-tiers.md) for the dual-tier system.

## Adding Skills

Skills are `SKILL.md` files in `packages/tinycode/src/skill/defaults/`. Each skill is a subdirectory with a SKILL.md containing YAML frontmatter.

## Tests

Run tests from a package directory, never from root:
```bash
cd packages/tinycode && bun test --timeout 30000
```

## Changelog

CHANGELOG.md is maintained by the project maintainer at release time. Contributors do not need to add changelog entries.

## Legal

No CLA or DCO is required to contribute.

## Questions?

Open a [GitHub Issue](https://github.com/bobbyjohnstx/tinycode/issues) for bugs or feature requests. For security vulnerabilities, see [SECURITY.md](SECURITY.md).
