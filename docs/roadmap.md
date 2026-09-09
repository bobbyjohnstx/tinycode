# tinycode Roadmap

Vision and current status of tinycode 2.0 development.

## Completed -- Go 2.0 Rewrite

The Go rewrite (2.0) replaces the TypeScript 1.x codebase with a standalone Go binary.

### Core
- **Standalone binary** -- Single Go binary embeds HTTP server, TUI, session management, LLM client, and tool execution. No separate server process, no TypeScript runtime required.
- **Bubbletea TUI** -- Terminal interface rebuilt with [bubbletea](https://github.com/charmbracelet/bubbletea) (Elm architecture). Session tree sidebar, conversation history, model switching, command palette, leader-key navigation.
- **Embedded HTTP server** -- net/http + chi router with REST and SSE endpoints. Ephemeral port, no port 4096 dependency.
- **Provider abstraction** -- Ollama, OpenAI-compatible, OpenRouter auto-discovery with capability detection.
- **Session management** -- SQLite-backed persistence, session tree hierarchy, context compaction.
- **ACP mode** -- Agent Client Protocol for IDE integration (stdio transport).
- **Headless mode** -- `tinycode serve` for API-only deployments.

### Plugins
- **12 Go plugins converted** -- cluster-ops, code-review, command-inject, context-pruning, handoff, log-sanitizer, notify, pilot, safety-net, snippets, telemetry, web-search.
- **Go plugin SDK** -- `pkg/plugin/` with JSON-RPC protocol, lifecycle hooks, and tool definitions.
- **Plugin wire protocol alignment** -- Server-side and SDK protocols unified.

### Tools
- File operations (read, write, edit)
- Shell execution
- Grep and glob for codebase search
- MCP client for external tool servers

### Agents
- All built-in agents ported (architect, debugger, executor, planner, code-reviewer, etc.)
- Per-agent tool permissions via frontmatter
- Agent prompt tiers for small models

---

## Current -- Documentation and Polish

Active work in this phase:

- **Documentation overhaul** -- Updating all docs for the Go 2.0 architecture
- **Sidebar functionality fixes** -- Session tree sidebar behavior and rendering
- **Bug fixes** -- Ongoing stability improvements

---

## Planned

Near-term work under consideration:

- **Additional plugin conversions** -- Red Hat-specific plugins not yet ported from TypeScript
- **LSP integration** -- Stub exists in the codebase; not yet functional

---

## Deferred

These items are on the radar but not actively planned:

- **Wiki / oh-my-tiny** -- The TypeScript wiki and oh-my-tiny plugin system. May be revisited once the Go core stabilizes.

---

## Non-Goals

- **Browser automation** -- tinycode focuses on CLI and APIs, not UI testing
- **Mobile app** -- Primary interface is terminal, not phone
- **Proprietary cloud service** -- tinycode is self-hosted, not SaaS
- **LLM training** -- tinycode uses models, it does not train them

---

## How to Help

tinycode is open source. Contributions welcome:

- **Bug reports:** [GitHub Issues](https://github.com/bobbyjohnstx/tinycode/issues)
- **Feature requests:** [GitHub Discussions](https://github.com/bobbyjohnstx/tinycode/discussions)
- **Code contributions:** Fork, create a branch, submit a PR
- **Documentation:** Help improve guides and examples

---

## Tracking Progress

- **Issues:** [GitHub Issues](https://github.com/bobbyjohnstx/tinycode/issues)
- **CHANGELOG:** [CHANGELOG.md](../CHANGELOG.md)
- **Discussions:** [GitHub Discussions](https://github.com/bobbyjohnstx/tinycode/discussions)
