# Changelog

## [Unreleased]

## [1.17.1] — 2026-08-10

### Added
- OpenRouter as full-fledged provider with auto-discovery — set `OPENROUTER_API_KEY` to discover 300+ models with capability detection, pricing, and cost tracking
- OpenRouter in `/connect` TUI dialog for API key entry
- Tool-call failure tracking with warning toast after 3+ consecutive failures (#69)
- Auto-compact on model switch to smaller context window (#74)
- Model size field in config for custom/finetuned models (#75)
- Context warning for models with <8K context window (#77)

### Changed
- omt tools and system prompt injection moved from plugin to native registration
- Skip tool injection for models with `capabilities.toolcall=false` to save context tokens (#69)
- Sanitize markdown fences and trailing commas from malformed tool-call JSON (#69)

### Fixed
- oh-my-tiny tool schemas: nested `enum` arrays broke OpenAI strict JSON Schema validation
- Surface friendly error on ECONNREFUSED instead of stack trace (#76)

## [1.17.0] — 2026-08-07

### Added
- Unified command palette (`Ctrl+P`) searches across commands, agents, sessions, and skills with frecency ranking (#59)
- Subagent output rendered inline in parent session as collapsible preview (#87)
- Leader-key session navigation using vim-style hjkl (`<leader>h/j/k/l`) — replaces bare arrow keys (#88)
- OpenAPI spec and Scalar docs UI served at `/api/docs`
- First 5 Minutes quick-start section in user guide
- Comprehensive user manual and deployment verification guide

### Changed
- Session list moved from `<leader>l` to `<leader>o` (freed for sibling navigation)
- Code block concealment moved from `<leader>h` to `<leader>;` (freed for sibling navigation)
- System prompts: agent prompts compose with base instead of replacing
- System prompts: tool infrastructure separated from personality

### Fixed
- Command palette crash causing unrecoverable stuck/dimmed TUI state (#86)
- Non-interactive run mode (`bun dev run`) — SSE events now fully delivered before exit (#84)
- Docker multi-platform build — `TARGETARCH` moved to global scope in Dockerfile (#83)
- `modelSizeB` regex handles decimal model sizes like "3.5b" (#80)
- ErrorComponent double-fault from invalid `new URL("")` constructor
- Dialog-scoped ErrorBoundary for graceful dialog error recovery
- Release workflow: added buildx setup for multi-platform Docker builds
- File path whitespace trimming in read/edit/write tools

## [0.2.1] — 2026-07-06

### Added
- Agent Client Protocol (ACP) support for IDE integration (`tinycode acp` command, stdio transport)
- Reference VS Code extension with editor context injection and agent command routing
- ACP integration developer guide (`docs/acp-integration.md`)
- Session export to self-contained HTML files (`tinycode export --format html <session-id>`)
- Session tree sidebar toggleable in TUI with ASCII visualization (`<leader>b`)
- Plugin lifecycle hooks: `session.start`, `session.end`, `session.switch`, `session.model.change`
- Mock LLM provider for deterministic testing of session logic
- Compaction: deterministic file-operation tracking with XML blocks carried across chains
- Compaction: observation masking replaces old tool outputs with placeholders before summarization
- Compaction: conversation serialized to text format for summarization (prevents model continuation)
- Compaction: circuit breaker warns after 3+ compactions, suggesting new session approach
- Compaction: structured telemetry logging (pre/post token counts, model, timing, compaction number)
- Compaction: summary size capped at min(4096, 10% of usable context)
- Compaction: increased preserve_recent_tokens defaults (MIN 4K, MAX 20K tokens)
- Desktop app (Electron) with system tray integration, global hotkey (Cmd/Ctrl+Shift+T), auto-updates
- Desktop app features: persistent zoom level, window state, macOS dock badge, theme sync, platform-specific menus
- Desktop security: Content Security Policy headers, navigation origin validation, URL scheme validation
- Skill: `tc-doctor` for diagnosing tinycode configuration and environment issues
- Skill: `ai-slop-cleaner` for regression-safe code cleanup with deletion-first workflow
- Skill: comprehensive verification workflow with evidence ladder (`/verify`)
- Agent: `cluster-admin` persona for Kubernetes/OpenShift operations with `oc` CLI cheatsheet
- Documentation: comprehensive build guide for tinycode, container, and operator
- Documentation: positioning document (`why-tinycode`) covering sovereign AI and small-model design
- Documentation: troubleshooting, getting-started, cheatsheet, and SUPPORT files
- Documentation: OC CLI cheatsheet for cluster-admin agent users
- CI: Daily dependency audit workflow
- CI: Lint, typecheck, and test workflows with Playwright E2E tests
- AI SDK v7 migration with improved provider handling and streaming support
- Electron 42 upgrade (Chromium 148) with supply chain hardening

### Changed
- Compact prompt cutoff lowered from ≤9B to ≤8B parameters for more efficient small-model instructions
- Agent `personas` clarified: read-only intent vs hard permission enforcement (plan mode enforces)
- Provider filtering (`enabled_providers`, `disabled_providers`) now applies to locally-discovered providers

### Security
- Config API secret redaction (redact API keys, passwords in `/global/config` response)
- Enforce authentication on non-loopback network bind (prevent unsecured remote access)
- LLM retry cap (20 attempts) to prevent infinite retries
- Agent step limit (200 default) to prevent runaway loops
- RPC timeout (30 seconds) for LLM calls
- Ripgrep result limit (200 results) to prevent denial of service
- Timing-safe password comparison for auth tokens
- Command injection prevention via `execFileSync` (no shell interpretation)
- Security headers middleware for HTTP responses
- Remove .npmrc from git tracking, add to .gitignore for credential safety
- Electron: `setWindowOpenHandler` prevents uncontrolled new windows
- Electron: `shell.openExternal` URL scheme validation (http/https/mailto only)

### Fixed
- Provider filter applies to locally-discovered providers (Ollama, vLLM)
- Compaction: toMessage() replaced with session.findMessage() for robust message lookup
- Compaction: file part serialization in tracking, masking, and serialization logic (3 instances)
- Removed dead omt LSP stub tools
- vLLM provider health check for `tc-doctor` skill
- Qwen3 thinking mode disabled on vLLM to prevent output budget loops
- Test assertion updates and slow test (>30s) management
- WebSocket listener shutdown race condition (close before stop)
- TypeScript migration: resolved 292+ type errors, re-enabled CI typecheck
- AI SDK adapter buffer and instruction rules in mock provider
- Snapshot updates for help output and agent defaults
- SDK import paths and TUI/CLI type errors
- Plugin default exports and test skips for pre-existing failures

### Deprecated
- `@` reference syntax (still works for files) — use `/ask <agent>` for agent invocation

## [0.1.0] — 2026-06-26

Initial public release.

### Added
- Terminal UI (TUI) with session tree sidebar, conversation history, and model switching
- Web UI (SolidJS + TailwindCSS) for browser-based access
- Desktop app (Electron) for macOS, Windows, and Linux
- 24 built-in agents with compact variants for models ≤9B parameters
- 9 built-in skills (debug, trace, verify, deepinit, mcp-setup, tc-doctor, remember, claude-api, run)
- 5 bundled rules (coding-style, testing, security, git-workflow, performance)
- Local LLM support — native Ollama and vLLM auto-discovery with Kubernetes integration
- Cloud provider support — Anthropic, OpenAI, Google, OpenRouter
- MCP client integration for external tool servers
- Plugin system (@tinycode/plugin) with lifecycle hooks
- oh-my-tiny built-in plugin (notepad, wiki, state management, AST grep)
- Mock LLM provider for deterministic testing
- Session export to JSON and self-contained HTML
- Effect-based HTTP API server with SSE/WebSocket streaming
- `/ask <agent>` command for agent invocation
- Swarm tool for multi-agent orchestration
- Setup wizard with onboarding flow
- Model capability auto-detection from Ollama and vLLM metadata
- Reasoning model support (parse `<think>` blocks from vLLM)
- Daily dependency audit CI workflow
- Lint, typecheck, and test CI workflow
- Playwright E2E tests

### Security
- Timing-safe password comparison
- Command injection prevention (execFileSync)
- Security headers middleware
- Auth token deprecation warning for query parameter usage
- Config permission hardening (0600)
