# Tinycode Plugins vs MCP Servers: Architecture Analysis

**Date:** 2026-08-27
**Source:** Architect + Critic + Analyst review of tinycode-plugins monorepo
**Status:** Analysis complete, decision pending

## Executive Summary

The tinycode-plugins monorepo contains 25 Red Hat product integration plugins. An architect/critic/analyst review assessed whether these should be tinycode plugins, MCP servers, or both.

**Finding:** The plugin architecture is correct for ~8 plugins that use lifecycle hooks and context injection. The remaining ~17 are tool-only and could be MCP servers with zero capability loss. The recommendation is a dual-publish strategy, not a rewrite.

## Plugin Categorization

### Must Be Tinycode Plugins (4 plugins)

These use lifecycle hooks with no MCP equivalent:

| Plugin | Hooks Used | Why MCP Can't Work |
|--------|-----------|-------------------|
| `context-injection` | `session.start`, `system.transform`, `dispose` | Auto-injects cluster context into system prompt — zero tools, pure context |
| `oauth` | `auth`, `shell.env` | Interactive OAuth login flow with UI prompts, sets shell env for `oc` |
| `eda-events` | `session.start/end`, `tool.execute.after`, `dispose` | Session lifecycle telemetry to EDA endpoint — zero tools, purely observational |
| `experiment-tracker` | `session.start/end`, `system.transform`, `tool.execute.after`, `dispose`, `event` | MLflow experiment tracking across session lifecycle — zero tools |

### Benefit From Tinycode Hooks, Tools Portable (4 plugins)

| Plugin | Hooks Used | What Would Be Lost in MCP |
|--------|-----------|--------------------------|
| `obs-metrics` | `session.start`, `system.transform`, tools | Auto-injected alert summary |
| `rhacm` | `system.transform`, tools | Auto-injected cluster/violation summary |
| `dev-content` | `session.start`, `system.transform`, tools | Framework detection + context injection |
| `cluster-ops` | `shell.env`, tools | `OC_EDITOR=cat` (trivial) |

### Tool-Only, Zero Tinycode Dependency (17 plugins)

Could be MCP servers today with nothing lost:

- **Security:** rhacs, lightwell, container-linter
- **DevEx:** quay, rhdh, tekton (shell.env is trivial)
- **RHOAI:** mcp-bridge, eval-trustyai, mlflow-tools, model-serving, pipelines
- **Reference:** api-catalog, ecosystem-catalog, rhdp-provisioner
- **Automation:** aap-bridge (shell.env is trivial)
- **Observability:** obs-logging
- **Satellite:** lightspeed

## What Tinycode Plugins Provide That MCP Can't

### 1. Context Injection (the killer feature)

`experimental.chat.system.transform` — 5 plugins auto-inject cluster state, alerts, costs, and framework info into the system prompt every turn. MCP has `resources` but they're pull-based (user/LLM must explicitly request them), not auto-injected.

**Used by:** context-injection, obs-metrics, rhacm, dev-content, experiment-tracker

**Risk:** This hook is literally named "experimental" in tinycode's API. Building a product strategy on an experimental API is a calculated risk.

### 2. Session Lifecycle

`session.start`, `session.end`, `dispose` — plugins cache state at session start and clean up on exit. MCP servers are stateless per-call. No equivalent.

### 3. Interactive Confirmation (`ctx.ask()`)

8 plugins use `ctx.ask()` for mutating operations: `ocp_apply`, `ocp_gitops_sync`, `aap_launch_job`, `tekton_start_run`, `obs_alert_silence`, `acm_app_deploy`, `rhdp_provision_environment`, `ocp_apply` (YAML manifests).

MCP has no standard permission/confirmation mechanism — clients handle it at their own discretion.

### 4. Shared Auth Session

The `oauth` plugin authenticates `oc` once. All other plugins inherit the session in-process. MCP servers run as independent processes — each would need to authenticate independently.

### 5. Plugin Composition

Loading 10 plugins simultaneously with tools, context injections, and shell modifications merging automatically is something MCP can do for tools but not for system prompts or shell environments.

## What MCP Servers Would Provide

### 1. Broad Reach

Works with Claude Code, Claude Desktop, VS Code Copilot, Cursor, and any MCP client. Tinycode plugins only work in tinycode.

### 2. Industry Standard

MCP is backed by Anthropic, adopted by Microsoft (VS Code Copilot), supported by dozens of clients. Tinycode's plugin SDK is a single-vendor API.

### 3. MCP Resources

Structured data the LLM can pull on demand. The reference plugins (api-catalog, ecosystem-catalog, dev-content) are natural fits for resources rather than tools.

### 4. Independent Deployment

Each server versioned and deployed separately. Not tied to monorepo release cadence.

## Key Risks

### Lock-In

17 plugins are tinycode-locked for no technical reason. If Red Hat customers standardize on a non-tinycode LLM client (VS Code Copilot, Cursor, etc.), these plugins are stranded.

### The mcp-bridge Irony

The `rhoai/mcp-bridge` plugin wraps an MCP server as a tinycode plugin. Tinycode already supports MCP natively (confirmed by `packages/tinycode/src/config/mcp.ts`). This adds an unnecessary indirection layer that loses MCP's native tool discovery. Should be replaced by direct MCP config.

### Auth Sharing Blocked for MCP

The `openshift/oauth` plugin creates an authenticated `oc` session that other plugins inherit in-process. MCP servers run as independent processes and can't share this session. Converting `oc`-dependent plugins to MCP is architecturally blocked until auth sharing is solved.

### BunShell Dependency

The shared `OcClient` depends on `PluginInput["$"]` (Bun's shell). MCP servers would need `child_process`-based execution. The shared library would need abstraction first.

## Recommendation: Dual-Publish Strategy

### Don't rewrite. Dual-publish the tool-only plugins.

The tool signatures (`{description, args, execute}`) are already protocol-agnostic. A thin adapter layer could expose them as MCP servers without touching the implementations.

### Implementation Path

1. **Decide the strategic intent first.** "Should Red Hat tool integrations be available outside tinycode?" is a product decision. If no, the MCP discussion is moot.

2. **Pilot with `container-linter`.** Zero external dependencies, pure functions (content in, lint results out). Lowest-risk candidate for dual-publishing.

3. **Abstract the shared library.** `OcClient` must work without BunShell before any `oc`-dependent plugin can be an MCP server.

4. **Resolve auth sharing.** Without solving how MCP servers share an authenticated `oc` session, converting `oc`-dependent plugins is blocked.

5. **Investigate runtime bridging.** Tinycode already has MCP server infrastructure. A runtime feature to auto-expose plugin tools as MCP endpoints would eliminate dual-publishing entirely.

6. **Remove `mcp-bridge`.** Configure the RHOAI MCP server directly in tinycode's native MCP config.

### What Not To Do

- Don't rewrite the 4 lifecycle-heavy plugins as MCP servers — they are genuinely impossible to build that way
- Don't generalize the mcp-bridge pattern (wrapping MCP as tinycode plugin) — it adds latency and loses schema information
- Don't invest in dual-publishing before validating demand from non-tinycode users

## Decision Criteria

The answer depends on one question: **Is there demand for Red Hat tool integrations outside tinycode?**

- **If yes** → dual-publish tool-only plugins as MCP servers, starting with `container-linter` pilot
- **If no** → keep current architecture, revisit when MCP demand materializes
- **Regardless** → remove `mcp-bridge` plugin, replace with direct MCP config

## Open Questions

- [ ] Is the `experimental.chat.system.transform` API stable? If tinycode renames or removes it, 5 plugins break.
- [ ] Can tinycode's runtime auto-expose plugin tools as MCP endpoints? If yes, dual-publishing is unnecessary.
- [ ] Are there Red Hat customers requesting standalone MCP servers for OpenShift/RHACS/AAP today?
- [ ] How should `ctx.ask()` permission flows translate to MCP? Options: (a) remove confirmation, (b) use MCP sampling, (c) exclude destructive tools from MCP surface.
- [ ] What is the latency overhead of MCP JSON-RPC vs in-process plugin calls? Needs measurement.
- [ ] Should the 3 lifecycle-only plugins (context-injection, experiment-tracker, eda-events) remain tinycode-exclusive by design?

## Appendix: Hook Usage Matrix

| Plugin | tool | shell.env | session.start | session.end | system.transform | auth | tool.execute.after | dispose | event |
|--------|------|-----------|---------------|-------------|-----------------|------|-------------------|---------|-------|
| context-injection | | | X | | X | | | X | |
| oauth | | X | | | | X | | | |
| eda-events | | | X | X | | | X | X | |
| experiment-tracker | | | X | X | X | | X | X | X |
| obs-metrics | X | | X | | X | | | | |
| rhacm | X | | | | X | | | | |
| dev-content | X | | X | | X | | | | |
| cluster-ops | X | X | | | | | | | |
| aap-bridge | X | X | | | | | | | |
| tekton | X | X | | | | | | | |
| container-linter | X | | | | | | X | | |
| quay | X | | | | | | | | |
| rhdh | X | | | | | | | | |
| obs-logging | X | | | | | | | | |
| rhacs | X | | | | | | | | |
| lightwell | X | | | | | | | | |
| api-catalog | X | | | | | | | | |
| ecosystem-catalog | X | | | | | | | | |
| rhdp-provisioner | X | | | | | | | | |
| eval-trustyai | X | | | | | | | | |
| mcp-bridge | X | | | | | | | | |
| mlflow-tools | X | | | | | | | | |
| model-serving | X | | | | | | | | |
| pipelines | X | | | | | | | | |
| satellite/lightspeed | X | | | | | | | | |
