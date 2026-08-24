# Patched Dependencies

These patches are applied automatically by Bun via the `patchedDependencies` field in the root `package.json`.

## Active Patches

| Package | Patch | What it fixes | Remove when |
|---------|-------|---------------|-------------|
| `@ai-sdk/openai-compatible@3.0.1` | Passes the full error object instead of just `error.message` during streaming | Error details (code, type) are preserved for retry logic | Fixed upstream in `@ai-sdk/openai-compatible` |
| `@modelcontextprotocol/sdk@1.27.1` | Treats JSON-RPC error responses as valid responses in the SSE reconnect logic | Prevents infinite SSE reconnection loops when MCP servers return errors | Fixed upstream in `@modelcontextprotocol/sdk` |
| `@npmcli/agent@4.0.0` | Calls `.toString()` on the proxy URL before passing it as an option | Fixes proxy URL handling when the value is a URL object instead of a string | Fixed upstream in `@npmcli/agent` |
| `@silvia-odwyer/photon-node@0.3.4` | Decouples wasm binding registration from `module.exports` and adds `__OPENCODE_PHOTON_WASM_PATH` global for custom wasm location | Allows photon-node to work inside Bun compiled binaries where the wasm asset path differs | Unlikely to be fixed upstream; keep indefinitely |
| `@standard-community/standard-openapi@0.2.9` | Handles external `$ref` URIs (containing `://`) by stripping them instead of crashing during OpenAPI schema conversion | Fixes crash when JSON schemas contain external references | Fixed upstream in `@standard-community/standard-openapi` |
| `solid-js@1.9.10` | Backports solid-js#2046 -- sets committed value on first computation during transitions | Fixes stale values when transitions complete | Included in solid-js >= 1.10 |
| `virtua@0.49.1` | Adds `measure()` method to Virtualizer handle, clamps range indexes to valid bounds, and filters out-of-range `keepMounted` indexes | Fixes crashes from out-of-bounds access and adds synchronous measurement API for the TUI | Fixed upstream in virtua or no longer needed after upgrade |

## Potentially Stale Patches

These patch files exist in the `patches/` directory but are **not** referenced in `patchedDependencies` in `package.json`. They may be leftover from previous versions:

- **`@ai-sdk/xai@3.0.82.patch`** -- Adds PDF file upload support (`input_file` content part) to the xAI Responses API adapter. Not applied; may have been superseded by a version upgrade to a newer `@ai-sdk/xai`.
- **`gcp-metadata@8.1.2.patch`** -- Suppresses `AggregateError` warning from `isAvailable()` when running outside GCP. Not applied; may have been superseded by a version upgrade.

## Other Files

- **`install-korean-ime-fix.sh`** -- Not a dependency patch. A standalone install script for a Korean IME input fix (see [issue #14371](https://github.com/anomalyco/opencode/issues/14371)).
