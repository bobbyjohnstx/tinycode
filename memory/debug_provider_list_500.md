---
name: debug-provider-list-500
description: Active debugging session — provider.list returns HTTP 500, root cause not yet captured
metadata:
  type: project
---

## Status: Provider.list returns HTTP 500

The tinycode refactor is complete through all 8 phases. The server starts cleanly. The TUI launches but fails with:

```
Error: 1 of 5 requests failed: provider.list: Unexpected server error. Check server logs for details.
```

## What we know

- **Server starts fine** — no startup errors, migration runs, DB init completes
- **`bun typecheck` passes** on all 3 packages (tinycode, core, app)  
- **Config** at `~/.config/tinycode/config.json` has `model: "ollama/qwen2.5:latest"` with ollama provider configured
- **Ollama is running** on default port 11434
- **`GET /provider` returns HTTP 500** `{"name":"UnknownError","data":{"message":"Unexpected server error...","ref":"err_..."}}`
- **Error middleware** is in `packages/tinycode/src/server/routes/instance/httpapi/middleware/error.ts` — it logs to `log.error("failed", {ref, error, cause})` then returns 500
- **Log goes to stderr** before `log.init()` is called (serve mode may not call init)
- **No log files created** yet at `~/.local/state/tinycode/log/`

## Root cause candidates

1. **`Provider.fromModelsDevProvider()`** — converts models.dev catalog format to internal Provider.Info format. The models-local.json was just fixed (removed `"providers"` wrapper, fixed field names: `tool_call` not `tool`, `limit.context` not top-level `context`, added `release_date` and `reasoning`). This might still fail if the conversion function doesn't handle the bundled format.

2. **`LocalDiscovery.Service`** — Phase 6 added this to the provider layer. The `local-discovery.ts` at `packages/tinycode/src/provider/local-discovery.ts` runs Ollama/vLLM probe on startup. If it throws during layer init, the provider layer fails. But server starts fine, so init is probably OK. Could fail during request handling.

3. **`Provider.Service.list()`** — the main provider listing function was heavily modified in Phase 4 (trimmed from 30 custom loaders to 2). Something in the list() call might throw.

## Next debugging step

Capture the actual error message from stderr. The error middleware logs the full cause before returning 500. Need to start the server, capture stderr, hit `/provider`, and read the error:

```bash
/Users/bjohns/.bun/bin/bun run --cwd packages/tinycode --conditions=browser src/index.ts serve > /tmp/tc.out 2>/tmp/tc.err &
sleep 10
PORT=$(grep -o '127.0.0.1:[0-9]*' /tmp/tc.out /tmp/tc.err | head -1 | sed 's/.*://')
python3 -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:$PORT/provider', timeout=5)" 2>/dev/null || true
sleep 1
kill %1
cat /tmp/tc.err | tail -100
```

## Key files involved

- `packages/tinycode/src/server/routes/instance/httpapi/handlers/provider.ts` — the list handler
- `packages/tinycode/src/provider/provider.ts` — Provider.Service, Provider.fromModelsDevProvider, Provider.list()
- `packages/tinycode/src/provider/local-discovery.ts` — LocalDiscovery.Service (new, Phase 6)
- `packages/core/src/models-dev.ts` — ModelsDev.Service, reads models-local.json
- `packages/core/src/models-local.json` — bundled model catalog (just fixed)

**Why to apply:** Debugging the provider.list 500 error in tinycode.
