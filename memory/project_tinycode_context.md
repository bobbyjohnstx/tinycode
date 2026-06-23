---
name: tinycode-project-context
description: Tinycode project goals, origin, and refactor direction — local LLM focus, slimmed from opencode
metadata:
  type: project
---

Tinycode is a fork of opencode being refactored into a slim, local-LLM-first AI coding assistant.

**Why:** opencode is tightly coupled to opencode.ai cloud services, a private `anomalyco/ghostty-web` dependency, SST infrastructure, an OAuth account system, and 30+ provider plugins. Tinycode strips all of that out.

**Hard constraints:**

- Primary: local LLM inference via Ollama (`localhost:11434`) and vLLM (`localhost:8000`)
- OK: OpenAI, Anthropic, Google via API key only
- Forbidden: any traffic to opencode.ai, anomalyco, or opencode-specific servers
- Goal: single binary, zero cloud dependencies, runs air-gapped

**Companion:** oh-my-tiny at `../oh-my-tiny/` is the agent orchestration layer (replaces opencode's ACP).

**Architecture docs (produced by architect + writer agents):**

- `docs/architecture-current.md` — current state as forked from opencode
- `docs/architecture-future.md` — future tinycode blueprint
- `docs/refactor-plan.md` — step-by-step refactor plan (from planner)

**Key removal targets:**

- `packages/enterprise`, `console`, `stats`, `web`, `function`, `identity`, `containers`, `slack`, `storybook`, `extensions`
- `src/account/`, `src/control-plane/`, `src/share/`, `src/sync/`
- `src/cli/cmd/account.ts`, `github.ts`, `stats.ts`, `upgrade.ts`, `pr.ts`
- `infra/`, `sst.config.ts`, `nix/`
- 27 of 32 provider plugins (keep: openai, openai-compatible, anthropic, google, dynamic)

**How to apply:** When suggesting code changes, prioritize local LLM compatibility and zero cloud dependencies. Config path will move from `~/.config/opencode/` to `~/.config/tinycode/`.
