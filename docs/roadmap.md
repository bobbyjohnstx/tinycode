# tinycode Roadmap

Vision and current status of tinycode development.

## Current (v1.x) — Foundation

What's available now (v1.19.0+):

### Core
- **TUI** — Full-featured terminal interface with session hierarchy
- **HTTP API** — REST + SSE/WebSocket server for remote access
- **Web UI** — SolidJS + TailwindCSS browser interface
- **Desktop app** — Electron wrapper with system tray, auto-updates, global hotkey
- **Provider abstraction** — Support for Ollama, vLLM, LM Studio, ramalama, OpenAI-compatible endpoints, cloud providers (Anthropic, OpenAI, Google, OpenRouter)
- **Ollama auto-profiling** — GPU-aware `num_ctx` baked into derived Modelfile profiles, with configurable `max_num_ctx` cap
- **Session tree** — Parent-child session relationships with tree sidebar visualization
- **Plugin system** — SDK published as `tinycode-plugin` on npm, with lifecycle hooks, test utilities, and plugin marketplace
- **ACP mode** — Agent Client Protocol for IDE integration (VS Code extension, stdio transport)
- **Auto-continue** — Automatic continuation loop with tiered tool injection for small local models

### Agents
- All 20+ built-in agents (architect, debugger, executor, planner, code-reviewer, test-engineer, etc.)
- Two execution modes: **build** (full access) and **plan** (read-only)
- **Per-agent tool permissions** — Frontmatter-declared permission blocks scope which tools each agent can access
- Agent prompt tiers for small models (< 8B params)

### Tools
- File operations (read, write, edit with unified diff)
- Shell execution (bash, zsh, etc.)
- Grep and glob for codebase search
- LSP integration for code intelligence
- MCP client for connecting servers (web search, etc.)
- oh-my-tiny plugin (notepad, wiki, state management, AST grep)
- Wiki skill for guided knowledge persistence

### Storage
- SQLite-backed session persistence
- Session export to HTML (self-contained) and JSON
- Context compaction with deterministic file tracking and observation masking

### Deployment
- Local development (TUI, server, web)
- Headless API server for remote access
- Docker container image with vLLM auto-discovery
- Kubernetes operator for OpenShift/K8s (separate repo: tinycode-operator)

---

## Next (v1.x) — Enhancement & Polish

Planned improvements for upcoming releases.

### UX & Workflow
- **Prompt templates** — Save and reuse common prompts with variables
- **Plugin keybindings** — Custom hotkeys for plugin tools
- **JSON-RPC mode** — Alternative to REST API for editor integrations (Zed, JetBrains via ACP)
- **Session search** — Full-text search across all sessions
- **Diff improvements** — Better inline diff viewer with blame/history integration
- **Which-key enhancements** — Better discovery of less-used features

### Model & Provider
- **Streaming optimizations** — Better token streaming for slower connections
- **Provider fallback** — Auto-retry with fallback provider if one fails
- **Model variants** — Save and switch between model quantizations (e.g., Q4 vs Q5)

### Agent & Skill Expansion
- **New agents:** DevOps engineer, Data engineer, Security engineer personas
- **Enhanced skills:** Better `/trace` with visualization, `/verify` with confidence scoring
- **Custom agent builder** — UI to create custom agents without code

### Storage & Performance
- **Session analytics** — Track token usage, session duration, tool invocation patterns
- **Database optimizations** — Query performance improvements for large session counts
- **Incremental sync** — Sync only changed messages when pulling remote sessions

---

## Future (v2.0+) — Ecosystem & Scale

Longer-term vision.

### Multi-Agent Orchestration
- **Swarm mode** — Coordinate multiple agents on a task (e.g., planner + architect + executor in sequence)
- **Tool discovery** — Agents auto-discover available tools and adapt
- **Cross-session learning** — Learn from solutions in other sessions

### Enterprise Features
- **Team workspaces** — Shared sessions and projects with access controls
- **Audit logging** — Full history of tool invocations for compliance
- **SAML/OIDC auth** — Enterprise authentication for remote deployments
- **Fine-grained RBAC** — Control which agents and tools each user can access

### Plugin Ecosystem
- **Custom tools** — Plugins can define new tools (not just instructions)
- **Tool composition** — Chain tool outputs as inputs to other tools

### Advanced Reasoning
- **Multi-step planning** — Break complex goals into sub-goals with automatic verification
- **Hypothesis testing** — Agents propose and test hypotheses
- **Confidence scoring** — Agents rate confidence in their answers

### Broader Integration
- **IDE plugins** — Native VSCode, Zed, JetBrains extensions (not just ACP)
- **Git integration** — Agents understand branch history and diffs deeply
- **Documentation generation** — Auto-generate API docs, architecture diagrams, runbooks
- **Monitoring integration** — Agents can query logs, metrics, traces

---

## Non-Goals

Things we won't do:

- **Browser automation** — tinycode focuses on CLI and APIs, not UI testing
- **Mobile app** — Primary interface is terminal/desktop, not phone
- **Voice interface** — No speech recognition or TTS
- **Proprietary cloud service** — tinycode is self-hosted, not SaaS
- **Closed-source components** — All code is open source (MIT)
- **Ad-hoc fine-tuning** — We don't offer model fine-tuning as a service
- **LLM training** — tinycode doesn't train models, only uses them

---

## How to Help

tinycode is open source. Contributions welcome:

- **Bug reports:** [GitHub Issues](https://github.com/bobbyjohnstx/tinycode/issues)
- **Feature requests:** [GitHub Discussions](https://github.com/bobbyjohnstx/tinycode/discussions)
- **Code contributions:** Fork, create a branch, submit a PR
- **Documentation:** Help improve guides and examples
- **Testing:** Use tinycode, report edge cases

---

## Release Cadence

- **v1.x:** Regular releases with new features and improvements
- **v2.0:** Ecosystem & scale milestone (multi-agent orchestration, enterprise features)

Release frequency is driven by community feedback and contributions.

---

## Tracking Progress

- **Issues:** [GitHub Issues](https://github.com/bobbyjohnstx/tinycode/issues) — bugs, enhancements, tasks
- **Projects:** [GitHub Projects](https://github.com/orgs/bobbyjohnstx/projects) — organized roadmap
- **CHANGELOG:** [CHANGELOG.md](../CHANGELOG.md) — release notes and breaking changes
- **Discussions:** [GitHub Discussions](https://github.com/bobbyjohnstx/tinycode/discussions) — feature discussion and feedback
