# tinycode Roadmap

Vision and current status of tinycode 2.0 development.

## Completed -- Go 2.0 Rewrite

The Go rewrite (2.0) replaces the TypeScript 1.x codebase with a standalone Go binary.

### Core
- **Standalone binary** -- Single Go binary embeds HTTP server, TUI, session management, LLM client, and tool execution. No separate server process, no TypeScript runtime required.
- **Bubbletea TUI** -- Terminal interface rebuilt with [bubbletea](https://github.com/charmbracelet/bubbletea) (Elm architecture). Session tree sidebar, conversation history, model switching, command palette, leader-key navigation.
- **Embedded HTTP server** -- standard library `net/http` with REST and SSE endpoints. Ephemeral port, no port 4096 dependency.
- **Provider abstraction** -- Ollama, OpenAI-compatible, OpenRouter auto-discovery with capability detection.
- **Session management** -- SQLite-backed persistence, session tree hierarchy, context compaction.
- **ACP mode** -- Agent Client Protocol for IDE integration (stdio transport).
- **Headless mode** -- `tinycode serve` for API-only deployments.

### Plugins
- **30 plugin binaries** -- curated in `internal/plugin/registry.go`. notify, code-review, handoff, and context-pruning are in-process builtins, not separate binaries.
- **Go plugin SDK** -- `pkg/plugin/` with JSON-RPC protocol, lifecycle hooks, and tool definitions.
- **Plugin wire protocol alignment** -- Server-side and SDK protocols unified.
- **Red Hat plugin conversion** -- OpenShift, Ansible, RHOAI, and platform plugins ported from TypeScript, sharing `internal/redhat/` and a containerized test harness.

### Run Mode (Non-Interactive CLI)
- **Single prompt** -- `tinycode run "prompt"` processes a prompt and exits
- **Multi-turn** -- `--multi-turn` loops on stdin for conversational flows
- **NDJSON output** -- `--format json` emits structured events (text, tool_begin, tool_end, reasoning, step_start, step_finish, warning, compacted)
- **JSON permission protocol** -- `--permissions json` enables programmatic permission handling via stdin/stdout
- **Iteration budget** -- `--max-iterations` caps LLM round-trips per prompt
- **Config permission rules** -- `permission.allow`/`permission.deny` in config.json loaded at runtime

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

- **LSP polish** -- Broader language coverage and tighter post-edit diagnostics wiring (`lsp_*` tools are already functional)

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
