# Changelog

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
