# Contributing to tinycode

## Development Setup

1. Install [Bun](https://bun.sh) (v1.3+)
2. Clone the repository
3. Run `bun install`
4. Run `bun dev` to start in development mode

## Commands

- `bun dev` — Run TUI in development mode
- `bun run lint` — Lint with oxlint
- `cd packages/tinycode && bun typecheck` — Type check
- `cd packages/tinycode && bun test` — Run tests

## Pull Request Process

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-feature`)
3. Make your changes
4. Run `bun run lint` and `cd packages/tinycode && bun test` before committing
5. Use conventional commit messages: `feat:`, `fix:`, `refactor:`, `docs:`, `test:`, `chore:`
6. Push and open a PR against `main`

## Coding Standards

- TypeScript with Effect framework for server-side code
- SolidJS for TUI and web UI
- See [AGENTS.md](AGENTS.md) for detailed coding style rules
- Keep files under 800 lines
- Prefer immutable patterns for shared state

## Adding Agents

Agents are markdown files in `packages/tinycode/src/agent/defaults/`. See [docs/agent-prompt-tiers.md](docs/agent-prompt-tiers.md) for the dual-tier system.

## Adding Skills

Skills are `SKILL.md` files in `packages/tinycode/src/skill/defaults/`. Each skill is a subdirectory with a SKILL.md containing YAML frontmatter.

## Tests

Run tests from a package directory, never from root:
```bash
cd packages/tinycode && bun test --timeout 30000
```

## Questions?

Open a [GitHub Issue](https://github.com/bobbyjohnstx/tinycode/issues) for bugs or feature requests.
