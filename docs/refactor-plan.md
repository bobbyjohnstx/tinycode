# Tinycode Refactor Plan

> Transform the opencode fork into a self-contained, local-LLM-first coding assistant.
> Each phase leaves `bun dev` in a working state. No phase should break the TUI or API server.

---

## Phase 0: Remove Dead Packages and Root Infrastructure

**Goal:** Delete entire packages and root-level directories that tinycode will never use. These are pure deletions with no import chains into kept code.

**Why first:** Zero risk of breaking runtime code. Reduces the monorepo surface area dramatically, making subsequent phases easier to reason about. Establishes that the repo is "ours" now.

### Files/directories to delete

| Path | Reason |
|------|--------|
| `packages/enterprise/` | Cloud enterprise features |
| `packages/console/` | Cloud admin dashboard |
| `packages/stats/` | Cloud analytics dashboard |
| `packages/web/` | Documentation website (opencode.ai) |
| `packages/docs/` | API docs / OpenAPI spec |
| `packages/function/` | Cloudflare Workers (GitHub app auth, webhooks) |
| `packages/identity/` | Cloud identity service |
| `packages/containers/` | Docker container infrastructure for cloud |
| `packages/slack/` | Slack bot integration |
| `packages/storybook/` | UI Storybook dev tooling |
| `packages/extensions/` | VS Code extension |
| `infra/` | SST cloud infrastructure configs (`app.ts`, `console.ts`, `enterprise.ts`, `lake.ts`, `monitoring.ts`, `secret.ts`, `stage.ts`, `stats.ts`) |
| `sst.config.ts` | SST entry point |
| `sst-env.d.ts` | SST type declarations |
| `nix/` | Nix dev environment (`desktop.nix`, `hashes.json`, `node_modules.nix`, `opencode.nix`, `scripts/`) |
| `sdks/vscode/` | VS Code extension SDK |
| `perf/` | Cloud performance testing (`test-suite.md`) |
| `specs/` | API specifications (`project.md`, `storage/`, `v2/`) |

### Files to modify

**`package.json` (root)**
- Remove workspace entries for deleted packages: `packages/console/*`, `packages/stats/*`, `packages/slack`
- Remove scripts: `dev:console`, `dev:stats`, `dev:storybook`
- Remove dependencies: `@aws-sdk/client-s3`, `heap-snapshot-toolkit`
- Remove devDependencies: `@actions/artifact`, `sst`
- Remove any `patchedDependencies` entries for cloud-only SDK packages

**`turbo.json`**
- Remove build targets referencing deleted packages
- Keep only: `build`, `dev`, `typecheck`, `lint`, `test`

### Verification

```bash
bun install          # lockfile regenerates without deleted workspaces
bun dev              # TUI launches normally
bun run lint         # no broken imports from deleted packages
```

### Estimated effort: 1-2 hours

### Risk flags

- The `packages/console/*` workspace glob in root `package.json` (line 26) means any subdirectory of console is a workspace. Deleting the parent is clean.
- The `postinstall` script (`bun run --cwd packages/tinycode fix-node-pty`, line 18) is unaffected.

---

## Phase 1: Cut External Service Calls

**Goal:** Eliminate all runtime network calls to `opencode.ai`, `models.dev`, and `anomalyco` endpoints from the kept packages. After this phase, tinycode makes zero outbound requests unless the user explicitly configures a cloud LLM provider.

**Why second:** These are targeted edits to existing files -- no module-level deletions yet. Each edit is small and independently testable. This is the phase that makes the tool "local-first."

### Files to modify

**`packages/core/src/models-dev.ts` (line 140)**
- Change default source from `"https://models.dev"` to a bundled local file path
- Create `packages/core/src/models-local.json` as the default catalog (see "New files" below)
- Keep `OPENCODE_MODELS_URL` env var override for opt-in remote fetch
- Keep `OPENCODE_MODELS_PATH` env var override for custom local file

**`packages/tinycode/src/config/config.ts`**
- Line 429: Remove `data.$schema = "https://opencode.ai/config.json"` auto-injection
- Line 430: Remove the `text.replace(...)` that injects the schema URL into config files
- Lines 447-453: Remove the `globalConfigFile()` seeding block that writes `{ $schema: "https://opencode.ai/config.json" }` to new global config files
- Line 466: Remove `result["$schema"] = "https://opencode.ai/config.json"` in legacy TOML migration
- Line 579: Remove `remoteConfig.$schema = "https://opencode.ai/config.json"` in remote config loading
- Lines 146, 219: Update doc URL references from `opencode.ai/docs/...` to local docs or remove

**`packages/tinycode/src/cli/cmd/tui/config/tui-migrate.ts` (line 14)**
- Change `TUI_SCHEMA_URL = "https://opencode.ai/tui.json"` to a local schema reference or remove schema auto-injection entirely

**`packages/tinycode/src/server/shared/ui.ts`**
- Line 9: Remove `UI_UPSTREAM = new URL("https://app.opencode.ai")`
- Lines 78-108: Rewrite `serveUIEffect` to remove the upstream proxy fallback branch. When no embedded UI is found, return a 404 or a "build the web UI first" message instead of proxying to `app.opencode.ai`
- Lines 40-41: Remove `upstreamURL` helper function
- Lines 30-37: Remove `proxyResponseHeaders` helper function (only used by proxy path)

**`packages/tinycode/src/session/retry.ts`**
- Lines 9-11: Remove `GO_UPSELL_MESSAGE`, `GO_UPSELL_URL`, and `"free_tier_limit"` from `RetryReason`
- Lines 75-87: Remove the `FreeUsageLimitError` block that returns the "Subscribe to OpenCode Go" upsell
- Lines 88-119: Remove the `GoUsageLimitError` block that returns the workspace-specific rate limit message with `opencode.ai/workspace/` link
- Keep the generic retry/backoff logic (lines 26-65) and generic rate-limit detection (lines 121-151) intact

**`packages/tinycode/src/provider/provider.ts`**
- Lines 178-200: Remove the `opencode:` custom loader in the `custom()` function. This is the paid-model gate that checks `dep.auth(input.id)` and removes non-free models when the user has no account
- Lines 432, 443, 453, 464, 570, 856: Remove all `"HTTP-Referer": "https://opencode.ai/"` header injections. These are in provider-specific options blocks

**`packages/tinycode/src/cli/cmd/providers.ts`**
- Line 304: Remove "opencode auth provider" description
- Lines 325: Remove `/.well-known/opencode` fetch
- Lines 369, 396: Remove `opencode` ranking/recommendation entries
- Lines 463-464: Remove the "Create an api key at https://opencode.ai/auth" message
- Line 473: Update or remove the Cloudflare docs URL reference

**`packages/tinycode/src/mcp/oauth-provider.ts` (line 50)**
- Change `client_uri: "https://opencode.ai"` to `"https://github.com/anomalyco/tinycode"` or remove

### New files to create

**`packages/core/src/models-local.json`**
- Static JSON model catalog covering: OpenAI (gpt-4o, gpt-4.1, o3, o4-mini), Anthropic (claude-sonnet-4-5, claude-sonnet-4-6, opus-4), Google (gemini-2.5-pro, gemini-2.5-flash), and a placeholder for `openai-compatible` (Ollama/vLLM models)
- Include: model IDs, context window sizes, max output tokens, capability flags (vision, tool_use, thinking)
- This file ships with the binary; no network request at startup

### Verification

```bash
# Start with no network access to confirm no outbound calls
# (on macOS, use Little Snitch or `sudo pfctl` to block outbound)
bun dev              # TUI launches, shows local model catalog
bun dev serve        # API server starts on :4096
```

Grep verification:
```bash
grep -rn "opencode\.ai" packages/tinycode/src/ packages/core/src/ \
  --include="*.ts" | grep -v "node_modules" | grep -v "@opencode-ai/"
# Should return zero results (excluding npm scope references)
```

### Estimated effort: 4-6 hours

### Risk flags

- **models-dev.ts** is loaded at startup via `ModelsDev.defaultLayer` in `app-runtime.ts` (line 76). The change from remote fetch to local file must produce the same `Record<string, Provider>` shape or the provider system will fail silently. Test with `bun dev` and verify models appear in the TUI model picker.
- **retry.ts** exports `GO_UPSELL_MESSAGE` which may be referenced by other files. Grep for imports before removing.
- **provider.ts custom loaders**: The `opencode:` loader (lines 178-200) is registered in a `Record<string, CustomLoader>`. Simply deleting the key is safe; the provider system iterates over registered plugins and skips unknown custom loaders.
- **ui.ts proxy removal**: The TUI itself does not use the web UI proxy (it renders natively in the terminal). Only `bun dev web` is affected. After removal, `bun dev web` requires a pre-built `packages/app` or a running `packages/app` dev server.

---

## Phase 2: Remove Account/Auth Cloud System

**Goal:** Gut the OAuth device-code flow, account management, org membership, and remote config fetching. Replace with API-key-only auth.

**Why third:** The account system has tentacles into config loading, provider auth, the server HTTP API, the CLI, and the DB schema. This phase requires careful stubbing because `Account.Service` is wired into the Effect dependency graph via `app-runtime.ts`. We do this after Phase 1 so the "why" for each cloud call site is already neutralized.

### Dependency analysis

`Account.Service` is consumed by:
- `packages/tinycode/src/effect/app-runtime.ts` line 67: `Account.defaultLayer` in `AppLayer`
- `packages/tinycode/src/config/config.ts` line 386: `yield* Account.Service` for remote config
- `packages/tinycode/src/server/routes/instance/httpapi/handlers/experimental.ts` lines 25-69: account/org info endpoints
- `packages/tinycode/src/storage/schema.ts` line 1: `AccountTable`, `AccountStateTable`, `ControlAccountTable` DB exports

### Files to modify

**`packages/tinycode/src/account/account.ts`** -- Rewrite to no-op stub
- Keep the `Service` class and its type signature so that all existing `yield* Account.Service` calls compile
- Replace every method with a no-op or empty-result implementation:
  - `active()` -> `Option.none()`
  - `login()` -> `Effect.die("Account system removed")`
  - `logout()` -> `Effect.void`
  - `orgs()` / `orgsByAccount()` -> `Effect.succeed([])`
  - `accessToken()` -> `Effect.succeed(undefined)`
  - `remoteConfig()` -> `Effect.succeed(undefined)`
- Remove all HTTP client imports and OAuth logic (the entire 400+ line file reduces to ~50 lines)
- Keep `account.sql.ts` for now (DB migration safety -- tables exist in user databases)

**`packages/tinycode/src/account/repo.ts`** -- Simplify
- Keep `AccountRepo.Service` interface and `Service` class
- Stub all methods to return empty results

**`packages/tinycode/src/auth/index.ts`** -- Simplify to API-key-only
- Remove `OAuth` credential type support
- Keep `APIKey` and `WellKnown` types
- Remove token refresh logic
- Keep `auth.json` file reading for API keys

**`packages/tinycode/src/config/config.ts` (line 386)**
- The `yield* Account.Service` call fetches remote config from the account server. With the account stub returning `undefined`, this becomes a no-op naturally. Optionally remove the dead code path for clarity.

**`packages/tinycode/src/server/routes/instance/httpapi/handlers/experimental.ts`**
- Lines 25-69: The account/org endpoints will return empty results from the stub. Optionally remove these handler registrations entirely.

**`packages/tinycode/src/effect/app-runtime.ts` (line 67)**
- `Account.defaultLayer` stays in `AppLayer` -- it now provides the stub. No change needed if the stub exports `defaultLayer`.

**`packages/tinycode/src/storage/schema.ts` (line 1)**
- Keep the `AccountTable`, `AccountStateTable`, `ControlAccountTable` exports for now. The tables exist in users' SQLite databases. Drizzle needs the schema to not crash on existing DBs. These become dead schema that can be removed in a future migration.

### Files/directories to delete

| Path | Reason |
|------|--------|
| `packages/tinycode/src/account/url.ts` | Server URL normalization for OAuth flows |
| `packages/tinycode/src/account/schema.ts` | OAuth schema types (DeviceCode, PollResult, etc.) -- only keep what the stub needs |
| `packages/core/src/account.ts` | Core account management against remote server |

### Verification

```bash
bun dev                           # TUI launches without account prompt
bun dev serve                     # API server starts
grep -rn "opencode.ai" packages/tinycode/src/account/   # zero results
bun typecheck --cwd packages/tinycode   # no type errors
```

### Estimated effort: 4-6 hours

### Risk flags

- **Effect layer graph**: `Account.defaultLayer` is composed into `AppLayer` via `Layer.mergeAll`. If the stub layer fails to provide the same `Service` tag, the entire runtime crashes at startup. Test immediately after stubbing.
- **DB schema**: Removing table exports from `storage/schema.ts` will cause Drizzle to drop tables on migration. Keep the exports; mark them as deprecated.
- **`json-migration.ts`** (lines 406-407): References `SessionShareTable` for migrating share data. This is touched in Phase 3 but the schema export must survive until then.

---

## Phase 3: Remove Cloud CLI Commands and Modules

**Goal:** Delete CLI commands and server-side modules that exist only for cloud platform integration.

**Why fourth:** After Phase 2 stubs the account system, these modules are dead code. But they still have import chains that must be severed cleanly in the CLI entry point and server routing.

### Files/directories to delete

| Path | Reason |
|------|--------|
| `packages/tinycode/src/cli/cmd/account.ts` | Console login/logout/switch commands |
| `packages/tinycode/src/cli/cmd/github.ts` | GitHub App integration, PR creation |
| `packages/tinycode/src/cli/cmd/stats.ts` | Stats dashboard command |
| `packages/tinycode/src/cli/cmd/upgrade.ts` | Self-upgrade from `opencode.ai/install` |
| `packages/tinycode/src/cli/cmd/pr.ts` | GitHub PR integration |
| `packages/tinycode/src/cli/cmd/acp.ts` | ACP CLI command |
| `packages/tinycode/src/installation/` | Self-upgrade logic (`index.ts`) |
| `packages/tinycode/src/acp/` | Agent Communication Protocol (`agent.ts`, `runtime.ts`, `session.ts`, `types.ts`) |
| `packages/tinycode/src/acp-next/` | ACP next-gen (12 files: `agent.ts`, `config-option.ts`, `content.ts`, `directory.ts`, `error.ts`, `event.ts`, `permission.ts`, `profile.ts`, `service.ts`, `session.ts`, `tool.ts`, `usage.ts`) |
| `packages/tinycode/src/share/` | Session sharing (`session.ts`, `share-next.ts`, `share.sql.ts`) |
| `packages/tinycode/src/sync/` | Multi-device sync (`index.ts`, `event.sql.ts`, `schema.ts`) |
| `packages/tinycode/src/control-plane/dev/` | Control plane dev utilities |
| `packages/core/src/github-copilot/` | GitHub Copilot auth and provider |

### Files to modify

**`packages/tinycode/src/index.ts`** -- Remove CLI command registrations
- Line 6: Remove `import { ConsoleCommand } from "./cli/cmd/account"`
- Line 9: Remove `import { UpgradeCommand } from "./cli/cmd/upgrade"`
- Line 13: Remove `import { Installation } from "./installation"`
- Line 20: Remove `import { StatsCommand } from "./cli/cmd/stats"`
- Line 22: Remove `import { GithubCommand } from "./cli/cmd/github"`
- Line 27: Remove `import { AcpCommand } from "./cli/cmd/acp"`
- Line 30: Remove `import { PrCommand } from "./cli/cmd/pr"`
- Remove corresponding `.command(...)` registrations in the yargs builder (search for each command name)
- The `Installation` import (line 13) is used for version checking at startup. Replace with a static version read from `package.json`.

**`packages/tinycode/src/effect/app-runtime.ts`** -- Remove cloud layer imports
- Line 47: Remove `import { Workspace } from "@/control-plane/workspace"`
- Line 51: Remove `import { Installation } from "@/installation"`
- Line 52: Remove `import { ShareNext } from "@/share/share-next"`
- Line 53: Remove `import { SessionShare } from "@/share/session"`
- Line 54: Remove `import { SyncEvent } from "@/sync"`
- Line 107: Remove `Workspace.defaultLayer` from `AppLayer`
- Line 111: Remove `Installation.defaultLayer` from `AppLayer`
- Line 112: Remove `ShareNext.defaultLayer` from `AppLayer`
- Line 113: Remove `SessionShare.defaultLayer` from `AppLayer`
- Line 114: Remove `SyncEvent.defaultLayer` from `AppLayer`
- Line 115: Remove `EventV2Bridge.defaultLayer` (depends on SyncEvent)

**`packages/tinycode/src/effect/bootstrap-runtime.ts` (line 7)**
- Remove `import { ShareNext } from "@/share/share-next"` and its layer

**`packages/tinycode/src/event-v2-bridge.ts` (line 8)**
- Remove `import { SyncEvent } from "@/sync"` -- this entire file may be deletable if it only bridges sync events

**`packages/tinycode/src/server/routes/instance/httpapi/server.ts`**
- Lines 46-47: Remove `SessionShare` and `ShareNext` imports
- Line 51: Remove `SyncEvent` import
- Line 56: Remove `Workspace` import
- Line 84: Remove `syncHandlers` import
- Remove corresponding handler/route registrations

**`packages/tinycode/src/server/routes/instance/httpapi/api.ts`**
- Line 4: Remove `SyncEvent` import
- Line 19: Remove `SyncApi` import and route group

**`packages/tinycode/src/server/projectors.ts` (line 2)**
- Remove `SyncEvent` import

**`packages/tinycode/src/server/routes/instance/httpapi/handlers/sync.ts`** -- Delete entirely

**`packages/tinycode/src/server/routes/instance/httpapi/groups/sync.ts`** -- Delete entirely (if it exists)

**`packages/tinycode/src/server/routes/instance/httpapi/groups/workspace.ts`** -- Stub or delete

**`packages/tinycode/src/server/routes/instance/httpapi/middleware/workspace-routing.ts`**
- This file has deep integration with `Workspace`, `WorkspaceID`, `WorkspaceAdapterRuntime`. It needs to be stubbed to pass all requests through to the local instance without workspace routing. Replace with a pass-through middleware.

**`packages/tinycode/src/server/shared/fence.ts`**
- Lines 3-5: Remove imports from `@/sync/event.sql` and `@/control-plane/workspace`
- Stub or simplify the fence logic to work without workspace context

**`packages/tinycode/src/storage/schema.ts`**
- Line 4: Remove `export { SessionShareTable } from "../share/share.sql"` (after deleting share/)
- Line 5: Remove `export { WorkspaceTable } from "../control-plane/workspace.sql"` (keep the .sql.ts file itself for DB migration safety, just stop re-exporting)

**`packages/tinycode/src/storage/json-migration.ts`**
- Line 7: Remove `SessionShareTable` import
- Lines 406-407: Remove share migration logic

**`packages/tinycode/src/pty/ticket.ts` (line 3)**
- Remove `import { WorkspaceID } from "@/control-plane/schema"`
- Replace with a local type alias or inline string brand

**`packages/tinycode/src/plugin/index.ts`**
- Lines 29-30: Remove `import { registerAdapter }` and `WorkspaceAdapter` from control-plane
- Line 32: Remove `InstallationChannel` import if only used for cloud features

**`packages/tinycode/src/effect/instance-state.ts` (line 6)**
- Remove `import { WorkspaceContext } from "@/control-plane/workspace-context"`

**`packages/tinycode/src/effect/instance-ref.ts` (line 3)**
- Remove `import type { WorkspaceID } from "@/control-plane/schema"`

**`packages/tinycode/src/effect/bridge.ts` (lines 2-3)**
- Remove workspace-related imports

**`packages/tinycode/src/effect/run-service.ts` (line 5)**
- Remove `import { WorkspaceContext } from "@/control-plane/workspace-context"`

**`packages/tinycode/src/cli/upgrade.ts`**
- Lines 4-5: Remove Installation/InstallationVersion imports
- Stub or delete the upgrade check logic

**Control-plane directory: partial keep**
- Keep `packages/tinycode/src/control-plane/schema.ts` temporarily (exports `WorkspaceID` type used by multiple files)
- Keep `packages/tinycode/src/control-plane/workspace.sql.ts` (DB migration safety)
- Delete everything else in control-plane after stubbing all consumers

### Verification

```bash
bun typecheck --cwd packages/tinycode   # CRITICAL -- must pass with zero errors
bun dev                                  # TUI launches
bun dev serve                            # API server starts
bun dev web                              # Web UI loads (if packages/app is built)

# Verify no dead imports remain
grep -rn "from.*@/share\|from.*@/sync\|from.*@/acp\|from.*@/installation\|from.*@/control-plane" \
  packages/tinycode/src/ --include="*.ts" | grep -v ".sql.ts" | grep -v "schema.ts"
# Should return zero results
```

### Estimated effort: 8-12 hours

### Risk flags

- **This is the highest-risk phase.** The control-plane workspace system is deeply wired into the server routing middleware (`workspace-routing.ts`), the PTY ticket system, the Effect instance management, and the bridge/run-service layer. Each consumer must be carefully stubbed or the server will crash.
- **Workspace routing middleware** (`workspace-routing.ts`) is the single most complex file to modify. It determines how HTTP requests are routed to project instances. The workspace concept needs to be replaced with a simpler "single local instance" model. Plan for 2-3 hours on this file alone.
- **SyncEvent** is wired into the event bridge and server projectors. Removing it requires tracing every subscriber. Delete `event-v2-bridge.ts` and remove its layer from `app-runtime.ts` to clean-cut this.
- **DB tables**: `SessionShareTable`, `WorkspaceTable`, and sync `EventTable`/`EventSequenceTable` exist in user databases. Keep the `.sql.ts` files but stop using them. A future migration can drop these tables.
- **`InstallationVersion`** is imported by many files (provider.ts, plugin/loader.ts, mcp/index.ts, etc.) but comes from `@opencode-ai/core/installation/version` -- this is a version string utility, NOT the self-upgrade module. Do NOT delete the core version module. Only delete `packages/tinycode/src/installation/` (the self-upgrade logic).

---

## Phase 4: Slim Provider Plugins

**Goal:** Reduce provider plugins from 32 to 5. Remove 27 unused provider plugin files and their SDK dependencies.

**Why fifth:** Provider plugins are self-contained -- each is a single file that registers models and SDK configuration. Removing them is mechanical and low-risk, but we do it after the cloud code is gone so we can also remove the `opencode` provider plugin that depends on the account system.

### Files to delete from `packages/core/src/plugin/provider/`

| File | Provider |
|------|----------|
| `alibaba.ts` | Alibaba Cloud |
| `amazon-bedrock.ts` | AWS Bedrock |
| `azure.ts` | Azure OpenAI (exports `AzurePlugin` and `AzureCognitiveServicesPlugin`) |
| `cerebras.ts` | Cerebras |
| `cloudflare-ai-gateway.ts` | Cloudflare AI Gateway |
| `cloudflare-workers-ai.ts` | Cloudflare Workers AI |
| `cohere.ts` | Cohere |
| `deepinfra.ts` | DeepInfra |
| `gateway.ts` | AI SDK Gateway |
| `github-copilot.ts` | GitHub Copilot |
| `gitlab.ts` | GitLab |
| `google-vertex.ts` | Google Vertex (exports `GoogleVertexPlugin` and `GoogleVertexAnthropicPlugin`) |
| `groq.ts` | Groq |
| `kilo.ts` | Kilo |
| `llmgateway.ts` | LLM Gateway |
| `mistral.ts` | Mistral |
| `nvidia.ts` | Nvidia |
| `opencode.ts` | Proprietary opencode managed provider |
| `openrouter.ts` | OpenRouter |
| `perplexity.ts` | Perplexity |
| `sap-ai-core.ts` | SAP AI Core |
| `togetherai.ts` | Together AI |
| `venice.ts` | Venice |
| `vercel.ts` | Vercel AI |
| `xai.ts` | xAI/Grok |
| `zenmux.ts` | Zenmux |

**Keep:**
- `openai.ts`
- `openai-compatible.ts` (Ollama, vLLM, any OpenAI-compatible endpoint)
- `anthropic.ts`
- `google.ts`
- `dynamic.ts` (user-defined providers via config)

### Files to modify

**`packages/core/src/plugin/provider/index.ts`** -- Rewrite
- Remove all 27 deleted plugin imports (lines 1-31 currently)
- Trim the `ProviderPlugins` array from 32 entries down to 5:
  ```typescript
  export const ProviderPlugins = [
    AnthropicPlugin,
    GooglePlugin,
    OpenAICompatiblePlugin,
    OpenAIPlugin,
    DynamicProviderPlugin,
  ]
  ```

**`packages/tinycode/src/provider/provider.ts`** -- Trim BUNDLED_PROVIDERS
- Lines 109-135: Remove entries from `BUNDLED_PROVIDERS` that correspond to deleted providers:
  - Remove: `@ai-sdk/amazon-bedrock`, `@ai-sdk/azure`, `@ai-sdk/google-vertex`, `@ai-sdk/google-vertex/anthropic`, `@openrouter/ai-sdk-provider`, `@ai-sdk/xai`, `@ai-sdk/mistral`, `@ai-sdk/groq`, `@ai-sdk/deepinfra`, `@ai-sdk/cerebras`, `@ai-sdk/cohere`, `@ai-sdk/gateway`, `@ai-sdk/togetherai`, `@ai-sdk/perplexity`, `@ai-sdk/vercel`, `@ai-sdk/alibaba`, `gitlab-ai-provider`, `@ai-sdk/github-copilot`, `venice-ai-sdk-provider`
  - Keep: `@ai-sdk/anthropic`, `@ai-sdk/google`, `@ai-sdk/openai`, `@ai-sdk/openai-compatible`
- Lines 167-265: Trim the `custom()` function to remove custom loaders for deleted providers (`opencode`, `xai`, `github-copilot`, and any others referencing deleted plugins)
- Lines 159-165: Remove `selectAzureLanguageModel` helper (only used by Azure)

**`packages/tinycode/src/provider/provider.ts`** -- Clean up Vertex helper
- Lines 96-103: Remove `getVertexAnthropicBaseURL` function (only used by Vertex Anthropic provider)

**`packages/tinycode/package.json`** -- Remove SDK dependencies
- Remove: `@ai-sdk/amazon-bedrock`, `@ai-sdk/azure`, `@ai-sdk/google-vertex`, `@openrouter/ai-sdk-provider`, `@ai-sdk/xai`, `@ai-sdk/mistral`, `@ai-sdk/groq`, `@ai-sdk/deepinfra`, `@ai-sdk/cerebras`, `@ai-sdk/cohere`, `@ai-sdk/gateway`, `@ai-sdk/togetherai`, `@ai-sdk/perplexity`, `@ai-sdk/vercel`, `@ai-sdk/alibaba`, `gitlab-ai-provider`, `venice-ai-sdk-provider`

**`packages/tinycode/src/cli/cmd/providers.ts`**
- Remove provider-specific help text and configuration guidance for deleted providers (Bedrock, Azure, Cloudflare, etc.)

### Verification

```bash
bun install                              # lockfile shrinks significantly
bun typecheck --cwd packages/tinycode    # must pass
bun typecheck --cwd packages/core        # must pass
bun dev                                  # TUI launches, model picker shows remaining providers
```

### Estimated effort: 3-4 hours

### Risk flags

- **Dynamic imports in `BUNDLED_PROVIDERS`**: These are lazy `import()` calls, so deleted SDKs won't crash at startup. But the corresponding entries in the map must be removed or they'll throw `MODULE_NOT_FOUND` when a user tries to configure a deleted provider.
- **transform.ts (1389 lines)**: This file has model-specific transforms for ALL providers. Removing provider plugins does NOT require removing their transforms from this file -- dead transforms are harmless and can be cleaned up later. Attempting to clean transform.ts in this phase risks breaking working providers.
- **Plugin system**: Users can still add removed providers back via `@ai-sdk/*` packages and the `dynamic` provider plugin. The plugin interface is preserved.

---

## Phase 5: Replace ghostty-web with xterm.js; Remove Sentry

**Goal:** Replace the private `ghostty-web` terminal widget dependency with open-source `xterm.js`, and remove Sentry error reporting from the web UI.

**Why sixth:** This phase touches `packages/app`, which is independent from the core server changes in Phases 1-4. Doing it after the core refactor means we're working with a stable backend. The ghostty-web replacement is the most user-visible change and needs careful testing.

### ghostty-web replacement

**`packages/app/package.json` (line 69)**
- Remove: `"ghostty-web": "github:anomalyco/ghostty-web#main"`
- Add: `"@xterm/xterm": "^5.x"`, `"@xterm/addon-fit": "^0.x"`, `"@xterm/addon-webgl": "^0.x"`

**`packages/app/src/components/terminal.tsx`**
- Line 6: Change `import type { FitAddon, Ghostty, Terminal as Term } from "ghostty-web"` to xterm.js equivalents
- Lines 31-36: Replace `shared` Promise that loads `ghostty-web` module with xterm.js loading
- Lines 188+: Replace all `Ghostty` references with xterm.js `Terminal` instantiation
- Lines 349-369: Replace the ghostty init block with xterm.js `Terminal` constructor + `FitAddon` + `WebglAddon`
- The xterm.js API is similar (both expose `Terminal` with `write()`, `onData()`, `onResize()`, `open(container)`) but the initialization differs:
  - ghostty: `Ghostty.load()` then `new Terminal({ ghostty })` 
  - xterm: `new Terminal(options)` then `term.open(container)`

**`packages/app/src/addons/serialize.ts`**
- Line 4: Update comment from "Port of xterm.js addon-serialize for ghostty-web" 
- Line 16: Change `import type { ITerminalAddon, ITerminalCore, IBufferRange } from "ghostty-web"` to xterm.js types
- This file is a serialization addon -- the interface it depends on (`ITerminalAddon`, `ITerminalCore`, `IBufferRange`) maps closely to xterm.js equivalents

**`packages/app/src/addons/serialize.test.ts`**
- Lines 2, 5-8, 24: Replace ghostty-web `Terminal` and `Ghostty` with xterm.js equivalents

**`packages/app/src/i18n/*.ts`**
- Multiple i18n files reference `"session.header.open.app.ghostty": "Ghostty"` -- keep as-is (Ghostty is a terminal app name, not the ghostty-web library)

**`packages/app/src/components/session/session-header.tsx` (line 40)**
- The `"ghostty"` reference is the Ghostty terminal application (for "open in Ghostty" feature), NOT the ghostty-web library. Keep as-is.

### Sentry removal

**`packages/app/package.json`**
- Remove: `"@sentry/solid"` from dependencies
- Remove: `"@sentry/vite-plugin"` from devDependencies

**`packages/app/src/app.tsx` (line 2, line 169)**
- Remove `import * as Sentry from "@sentry/solid"`
- Remove `Sentry.captureException(error)` call

**`packages/app/src/entry.tsx` (line 3, lines 138+)**
- Remove `import * as Sentry from "@sentry/solid"`
- Remove the `Sentry.init({...})` block

**`packages/app/src/pages/error.tsx` (line 2, lines 312-320)**
- Remove `import * as Sentry from "@sentry/solid"`
- Remove `Sentry.isEnabled` conditional and `Sentry.captureException` call

**`packages/app/vite.config.ts`** (if it references Sentry plugin)
- Remove `sentryVitePlugin` import and configuration

### Verification

```bash
bun install                              # no more ghostty-web private repo fetch
bun run --cwd packages/app dev           # web UI dev server starts
bun dev web                              # full stack with web UI
# Open browser, navigate to a session, verify:
# - Terminal renders in the web UI
# - Terminal resizes correctly
# - Terminal input/output works
# - No Sentry errors in console
bun test --cwd packages/app              # serialize tests pass with xterm.js
```

### Estimated effort: 6-8 hours

### Risk flags

- **ghostty-web API surface**: The xterm.js API is close but not identical. Key differences:
  - ghostty-web requires `Ghostty.load()` (WASM initialization) before creating terminals. xterm.js has no equivalent -- terminals are created directly.
  - ghostty-web's `Terminal` constructor takes a `ghostty` instance. xterm.js's `Terminal` constructor takes an options object.
  - Buffer access APIs may differ (the serialize addon accesses internal buffer structures).
- **Serialize addon**: This is the highest-risk file in the replacement. It accesses terminal internals (`ITerminalCore`, buffer cells). If xterm.js's internal buffer API differs, this addon needs a deeper rewrite.
- **WebGL addon**: xterm.js supports a WebGL renderer via `@xterm/addon-webgl`. This should be used for performance parity with ghostty-web's GPU-accelerated rendering.
- **CSS**: ghostty-web may bundle its own styles. xterm.js requires importing `@xterm/xterm/css/xterm.css`. Ensure this is added to the build pipeline.

---

## Phase 6: Add Local LLM Discovery Module

**Goal:** Auto-detect Ollama and vLLM instances running on localhost or LAN and register them as providers.

**Why seventh:** This is new functionality that builds on top of the cleaned-up provider system. All prior phases removed cloud dependencies; this phase adds the local-first replacement.

### New files to create

**`packages/tinycode/src/provider/local-discovery.ts`**

Core logic:
1. On startup and every 30 seconds, probe known endpoints:
   - Ollama: `GET http://localhost:11434/api/tags` -- returns `{ models: [{ name, size, ... }] }`
   - vLLM: `GET http://localhost:8000/v1/models` -- returns OpenAI-compatible `{ data: [{ id, ... }] }`
2. For each discovered model, register as an `openai-compatible` provider entry:
   - Provider ID: `ollama` or `vllm`
   - Base URL: the server's URL + `/v1` suffix for Ollama
   - Model ID: the model name from the API response
3. Emit bus events when models are discovered or lost
4. Use existing `bonjour-service` dependency (already in the project) for optional mDNS discovery of LAN servers

Implementation notes:
- Implement as an Effect service with `defaultLayer` 
- Probe endpoints with short timeouts (2 seconds) to avoid blocking startup
- Cache results to avoid re-probing on every provider resolution
- Expose `TINYCODE_OLLAMA_HOST` and `TINYCODE_VLLM_HOST` env vars for custom endpoints

### Files to modify

**`packages/tinycode/src/effect/app-runtime.ts`**
- Add `LocalDiscovery.defaultLayer` to `AppLayer`

**`packages/tinycode/src/provider/provider.ts`**
- Wire `LocalDiscovery` results into the model catalog population logic
- When a local model is discovered, create a provider entry with `npm: "@ai-sdk/openai-compatible"` and the server's base URL

### Verification

```bash
# Start Ollama with a model
ollama serve &
ollama pull llama3.2

bun dev              # TUI launches, Ollama models appear in model picker
# Select an Ollama model, send a message, verify response streams back

# Test without Ollama running
killall ollama
bun dev              # TUI launches normally, no Ollama models shown, no errors
```

### Estimated effort: 4-6 hours

### Risk flags

- **Startup latency**: The 2-second probe timeout adds up if both Ollama and vLLM are probed sequentially. Probe in parallel using `Effect.all` with `{ concurrency: "unbounded" }`.
- **Provider registration timing**: Models discovered after the TUI renders its initial model list need to trigger a UI refresh. Use the existing bus event system to notify the TUI of new models.
- **Ollama API compatibility**: Ollama's `/api/tags` response format is not OpenAI-compatible. The discovery module must map Ollama model names to the format expected by `@ai-sdk/openai-compatible`. Ollama's OpenAI-compatible endpoint is at `/v1/chat/completions`, so the base URL should be `http://localhost:11434/v1`.

---

## Phase 7: Config Path Migration and Default Config

**Goal:** Migrate config paths from `~/.config/opencode/` to `~/.config/tinycode/`, set up default configuration for local LLMs, and bundle a local model catalog.

**Why last:** This is a user-facing change that affects where config files are read from. Doing it last means all the code changes are complete and we can set sensible defaults for the new tool. It also avoids config path confusion during development of earlier phases.

### Files to modify

**`packages/core/src/global.ts` (line 9)**
- Change `const app = "opencode"` to `const app = "tinycode"`
- This cascades to all XDG paths: `~/.local/share/tinycode/`, `~/.cache/tinycode/`, `~/.config/tinycode/`, `~/.local/state/tinycode/`

**`packages/core/src/flag/flag.ts`**
- Rename env var prefixes from `OPENCODE_*` to `TINYCODE_*` (keep `OPENCODE_*` as fallbacks for migration)
- Add: `TINYCODE_OLLAMA_HOST`, `TINYCODE_VLLM_HOST`

**`packages/tinycode/src/config/config.ts`**
- Update `ConfigPaths.files()` call to search for `tinycode.json`/`tinycode.jsonc` in addition to (or instead of) `opencode.json`/`opencode.jsonc`
- Add migration logic: if `~/.config/opencode/config.json` exists but `~/.config/tinycode/config.json` does not, copy the old config (with a log message)

**`packages/core/src/models-dev.ts`**
- Update cached file name from `models.json` to use the tinycode cache directory (automatic from global.ts change)

### New files to create

**`packages/tinycode/src/config/migration.ts`**
- One-time migration from `~/.config/opencode/` to `~/.config/tinycode/`:
  - Copy `config.json`, `auth.json`, `tui.json` if they exist
  - Do NOT delete the old files (user may have opencode installed alongside)
  - Log a message: "Migrated config from ~/.config/opencode/ to ~/.config/tinycode/"
- Run at startup, before config loading

### Default config

When no config file exists, tinycode should create a default `~/.config/tinycode/config.json`:

```json
{
  "provider": {
    "ollama": {
      "npm": "@ai-sdk/openai-compatible",
      "options": {
        "baseURL": "http://localhost:11434/v1",
        "name": "ollama"
      }
    },
    "vllm": {
      "npm": "@ai-sdk/openai-compatible",
      "options": {
        "baseURL": "http://localhost:8000/v1",
        "name": "vllm"
      }
    }
  }
}
```

### Verification

```bash
# Clean slate test
rm -rf ~/.config/tinycode/
bun dev                                  # creates ~/.config/tinycode/ with default config
ls ~/.config/tinycode/config.json        # exists with default providers
cat ~/.config/tinycode/config.json       # shows ollama/vllm defaults

# Migration test
mkdir -p ~/.config/opencode/
echo '{"model":"anthropic/claude-sonnet-4-5"}' > ~/.config/opencode/config.json
rm -rf ~/.config/tinycode/
bun dev                                  # migrates config, launches with anthropic model
cat ~/.config/tinycode/config.json       # shows migrated config

# Env var test
TINYCODE_OLLAMA_HOST=http://192.168.1.100:11434 bun dev   # uses custom Ollama host
```

### Estimated effort: 3-4 hours

### Risk flags

- **`const app = "opencode"` in global.ts**: This is used to construct ALL XDG paths. Changing it affects data, cache, config, state, tmp, log, and repos directories. Existing user data in `~/.local/share/opencode/` (SQLite databases, session data) will NOT be found at the new path. Add a data migration for SQLite database files too, or symlink the old data directory.
- **Environment variable rename**: Many env vars use the `OPENCODE_` prefix (grep shows 20+ in `flag.ts`). Changing all of them at once is a breaking change. Keep `OPENCODE_*` as deprecated fallbacks: check `TINYCODE_*` first, fall back to `OPENCODE_*`.
- **Project-level config**: Local `.opencode/config.json` files in project directories should still be read (for backward compatibility with existing projects). Add `.tinycode/config.json` as the preferred path.

---

## Definition of Done

The refactor is complete when ALL of the following are true:

### Zero Cloud Dependencies

- [ ] `grep -rn "opencode\.ai" packages/tinycode/src/ packages/core/src/ --include="*.ts"` returns zero results (excluding `@opencode-ai/` npm scope in imports)
- [ ] `grep -rn "models\.dev" packages/core/src/ --include="*.ts"` returns zero results (the bundled catalog replaces the remote fetch)
- [ ] `grep -rn "anomalyco" packages/ --include="*.json"` returns zero results (ghostty-web removed)
- [ ] No runtime HTTP requests are made on startup when no cloud API keys are configured
- [ ] No Sentry telemetry is initialized or sent

### Functional Requirements

- [ ] `bun dev` launches the TUI and connects to a local Ollama instance (if running)
- [ ] `bun dev serve` starts the headless API server on port 4096
- [ ] `bun dev web` serves the web UI with the xterm.js terminal widget
- [ ] Model picker shows locally discovered Ollama/vLLM models
- [ ] Cloud providers (OpenAI, Anthropic, Google) work when API keys are provided
- [ ] `bun typecheck --cwd packages/tinycode` passes with zero errors
- [ ] `bun typecheck --cwd packages/core` passes with zero errors
- [ ] `bun run lint` passes
- [ ] `bun test --cwd packages/tinycode` passes (existing tests that don't test deleted features)
- [ ] Config is read from `~/.config/tinycode/`
- [ ] Old config at `~/.config/opencode/` is migrated on first run

### Code Health

- [ ] No dead imports remain (all `import` statements resolve to existing files)
- [ ] No orphaned files remain in `packages/tinycode/src/` (every `.ts` file is reachable from an import chain starting at `index.ts` or `app-runtime.ts`)
- [ ] `packages/core/src/plugin/provider/index.ts` exports exactly 5 providers
- [ ] `BUNDLED_PROVIDERS` in `provider.ts` has exactly 4 entries
- [ ] SQLite schema still includes deprecated tables (no data loss for existing users)
- [ ] Root `package.json` workspaces only reference kept packages

### Package Count

- [ ] `ls packages/ | wc -l` returns 10 or fewer (from current 22+)
- [ ] Kept: `tinycode`, `core`, `app`, `desktop`, `llm`, `ui`, `plugin`, `effect-drizzle-sqlite`, `sdk/js`, `http-recorder`, `script`
- [ ] Removed: `enterprise`, `console`, `stats`, `web`, `docs`, `function`, `identity`, `containers`, `slack`, `storybook`, `extensions`

---

## Effort Summary

| Phase | Description | Estimated Hours | Risk Level |
|-------|-------------|-----------------|------------|
| 0 | Remove dead packages and root infra | 1-2 | LOW |
| 1 | Cut external service calls | 4-6 | MEDIUM |
| 2 | Remove account/auth cloud system | 4-6 | MEDIUM |
| 3 | Remove cloud CLI commands and modules | 8-12 | HIGH |
| 4 | Slim provider plugins | 3-4 | LOW |
| 5 | Replace ghostty-web, remove Sentry | 6-8 | MEDIUM |
| 6 | Add local LLM discovery | 4-6 | LOW |
| 7 | Config path migration | 3-4 | MEDIUM |
| **Total** | | **33-48 hours** | |

Phase 3 is the critical path. Budget extra time for the workspace routing middleware rewrite and testing the Effect layer graph after removing 5+ service layers from `AppLayer`.
