# Changelog

## [Unreleased]

## [1.19.0] — 2026-08-24

### Added
- Plugin SDK V2 hooks: `tool.execute.before`, `tool.execute.after`, `tool.definition`, `chat.params`, `chat.headers`, `command.execute.before`, `experimental.compaction.autocontinue` surfaced to external plugins
- Plugin SDK `ToolContext` gains `progress()`, `messages()`, and `sessionInfo()` methods
- Plugin SDK config schema validation and API versioning (`engines.tinycode-plugin`)
- Plugin test utilities exported from `tinycode-plugin/test` (`createTestHarness`, `createMockToolContext`, `createMockPluginInput`)
- Plugin SDK published to npm as `tinycode-plugin` and `tinycode-sdk` (unscoped packages)
- `tinycode plugin-init` CLI command for scaffolding new plugins
- Plugin template repository (`bobbyjohnstx/tinycode-plugin-template`)
- Reference plugin example (`examples/tinycode-plugin-hello`)
- LM Studio auto-discovery as a local provider
- CI expanded to typecheck all packages and test 5 additional packages (app, ui, llm, effect-drizzle-sqlite, http-recorder)
- docs/adding-a-tool.md — guide for plugin and core tool development
- docs/deployment.md — extracted from README for focused deployment guidance
- patches/README.md — rationale for all 7 active dependency patches
- "Where to Start" contributor on-ramp in CONTRIBUTING.md

### Changed
- omt tools converted from `plainTool()` to `tool()` with zod schemas
- Plugin hook triggers wrapped with error boundaries to prevent single-plugin failures from crashing sessions
- README restructured: npm/homebrew primary install, curl deprecated, Documentation section added, deployment content extracted, hardware expectations added
- Roadmap version scheme updated from v0.x to v1.x to match actual releases
- Internal agent-generated docs moved to docs/internal/

### Fixed
- LICENSE now includes upstream SST/opencode copyright notice (MIT compliance)
- Branch contradiction: CONTRIBUTING.md aligned to `dev` branch (was incorrectly referencing `main`)
- Contradictory install instructions resolved between README and getting-started.md
- "No telemetry" claim qualified to "no telemetry by default" with Sentry disclosure for desktop app
- Plugin development docs updated: all imports use npm name `tinycode-plugin`, ToolContext documented with all methods
- CI double-v in registry commit tags
- Dead workspace API code removed from plugin system

## [1.18.0] — 2026-08-10

### Added
- Subagent depth limit (`subagent_depth` config, default: 1) — prevents infinite recursion by limiting how deep subagents can spawn further subagents

### Changed
- Expanded retry error patterns — `retryable()` now uses regex-based matching for ~30 scenarios including network failures, timeouts, provider overloads, rate limits, and server errors
- OpenAI header timeout increased from 10s to 300s (5 minutes) to support reasoning models (o1, o3) that can take minutes to produce the first SSE event
- Config parsing now silently ignores unknown fields for forward compatibility and shared configs
- Session message ordering uses `time.created` timestamps instead of ID-based comparison for chronological correctness

### Fixed
- Stream error preservation in `@ai-sdk/openai-compatible` — patched to preserve full error objects during streaming (not just `.message`)
- MCP SSE reconnect loop — patched `@modelcontextprotocol/sdk` to recognize JSON-RPC error responses, preventing infinite reconnection loops
- Message boundary detection for imported sessions where IDs may not be monotonically increasing
- Truncation cleanup uses filesystem mtime instead of ID-embedded timestamps — fixes incorrect retention calculations
- Grep results preserve user-provided symlink paths instead of resolving them to physical paths
- ACP mode drains SSE events before returning end-of-turn — prevents clients from missing final tool results
- CI: configure git identity for AUR and Homebrew tap commits in publish script

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
