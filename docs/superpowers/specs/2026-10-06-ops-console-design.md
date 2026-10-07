# Ops Console Design (`tinycode serve`)

**Date:** 2026-10-06  
**Status:** Approved (serve-only)  
**Issue epic:** Gitea #637–#642

## Intent

Give `tinycode serve` a thin, server-rendered browser surface for health and inventory — the same ops questions users already ask via CLI (`status`, `doctor`, models/providers/agents/sessions/plugins). Chat / agent loops stay on TUI, `tinycode run`, ACP, and **`tinycode web` (Solid SPA)**. Phone/remote UI (C) is out of scope.

## Goals

- Usable ops console after `tinycode serve` (authenticated)
- Pages map to CLI: status, doctor-ish diagnostics, models, providers, agents, sessions, plugins
- Same auth cookie / HTML recovery behavior as #636 (`?auth_token=…` URL printed in the serve log)
- Honest docs: console ≠ SPA; no PTY/share; `tinycode web` unchanged

## Non-goals

- Chat, streaming prompts, permissions UX, swarm, PTY, share
- Mobile-first phone shell
- Replacing or demoting the Solid SPA on `tinycode web`

## Architecture

- Server-rendered Go `html/template` under `internal/server/console`
- Registered on the mux **only when `ServeWebUI` is false** (`tinycode serve`)
- `tinycode web` (`ServeWebUI=true`) keeps the embedded SPA path unchanged
- Routes pass through existing `TokenAuth` (`AuthModeAPI`)
- Data from in-process deps (no second HTTP client)

## Pages (MVP)

| Path | CLI analogue | Actions |
|------|----------------|---------|
| `/` | (home) | Nav + short health summary |
| `/status` | `tinycode status` | Health, version, project path |
| `/doctor` | `tinycode doctor` | Structured checks (config, providers, agents, plugins, MCP, LSP) |
| `/providers` | `tinycode providers` | List providers |
| `/models` | `tinycode models` | Flattened model list |
| `/agents` | `tinycode agent` | List agents |
| `/sessions` | `tinycode session` | List; delete with confirm POST |
| `/plugins` | `tinycode plugin list` | List loaded plugins |

## Success criteria

- Authenticated browser on `tinycode serve` gets console home
- Unauthenticated browser gets HTML recovery mentioning ops console + `auth_token` URL
- Serve log prints `ops console` URL with `?auth_token=…` (do not auto-open browser)
- `tinycode web` still serves the Solid SPA
- Tests for route auth + page render smoke tests
