# Open Questions

## Agent Prompt Tiers - 2026-06-18

- [ ] Should compact `permission` fully replace the default permission ruleset, or merge with it? — Merge is safer (compact inherits base permissions, only overrides specific rules), but replace is simpler to reason about. Recommend merge for safety.
- [ ] Should `config.json` user overrides of `prompt:` apply to both tiers or only default? — If a user sets a custom prompt via config, it currently replaces the agent prompt. When compact tier is active, should the user override still win? Recommend yes (user intent is explicit).
- [ ] Should native agents (build, general, explore) eventually get hardcoded compact prompts, or is filesystem-only sufficient? — Filesystem override works today. Hardcoded compact prompts are a future enhancement if needed.

## Swarm Web UI - 2026-06-23

- [x] Should `pty.open` auto-open the terminal panel, or only add the tab? — **Decision: auto-open** (set as active tab, triggers panel open).
- [x] Should closing the swarm terminal tab also kill the tmux session? — **Decision: no** (closing detaches; swarm self-terminates when all workers complete naturally).
- [x] Should auto-reattach on WebSocket reconnect be in scope? — **Decision: yes, in scope** (step 4). On reconnect, check if tmux session still exists, create new PTY to reattach, emit `pty.open`.
