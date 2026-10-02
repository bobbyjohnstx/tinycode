# tinycode Technical Specification

Version: 2.0
Date: 2026-09-11

This specification documents tinycode, the Go rewrite of [tinycode](https://github.com/bobbyjohnstx/tinycode). It serves as:

1. **Contributor reference** — how tinycode actually works, with exact constants, thresholds, and formulas
2. **Parity tracker** — what's been ported from the TypeScript original vs. what's missing
3. **Standalone spec** — tinycode on its own terms, including Go-only features

## Sections

| # | File | Section |
|---|------|---------|
| 0 | [00-overview.md](00-overview.md) | Overview and architecture |
| 1 | [01-interfaces.md](01-interfaces.md) | Interfaces: TUI, Web, CLI, ACP |
| 2 | [02-api-routes.md](02-api-routes.md) | HTTP API routes |
| 3 | [03-event-system.md](03-event-system.md) | Event system and SSE protocol |
| 4 | [04-session-management.md](04-session-management.md) | Session lifecycle and processing |
| 5 | [05-context-compaction.md](05-context-compaction.md) | Context compaction and pruning |
| 6 | [06-llm-providers.md](06-llm-providers.md) | LLM providers and discovery |
| 7 | [07-agents.md](07-agents.md) | Agent system |
| 8 | [08-tools.md](08-tools.md) | Built-in tools |
| 9 | [09-plugins.md](09-plugins.md) | Plugin system |
| 10 | [10-skills.md](10-skills.md) | Skill system |
| 11 | [11-mcp.md](11-mcp.md) | MCP client |
| 12 | [12-permissions.md](12-permissions.md) | Permission system |
| 13 | [13-configuration.md](13-configuration.md) | Configuration |
| 14 | [14-storage.md](14-storage.md) | Storage and database |
| 15 | [15-security.md](15-security.md) | Security |
| 16 | [16-not-implemented.md](16-not-implemented.md) | TS features not yet ported |
| 17 | [17-testing-strategy.md](17-testing-strategy.md) | Automated testing strategy |

## Relationship to TS spec

The TypeScript specification (`tc-spec.md`) documents the original tinycode 1.x implementation. This Go spec documents the 2.0 rewrite. Where behavior differs, this spec is authoritative for tinycode. The [not-implemented](16-not-implemented.md) section summarizes TS behaviors that have not been ported.
