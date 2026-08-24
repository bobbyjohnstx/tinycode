# Implementation Findings Summary

This document summarizes the findings from the current pending changes in
`packages/tinycode`. It is intentionally limited to the OpenRouter discovery
work and the native oh-my-tiny (`omt`) integration represented by the current
worktree diff.

## Executive summary

The changes address two related integration gaps:

1. OpenRouter is now represented as a discoverable provider when
   `OPENROUTER_API_KEY` is present.
2. `omt` tools and persistent project context are moved into the core runtime,
   removing the internal plugin indirection.
3. Several tool argument schemas are corrected so enum values are represented
   as flat arrays rather than nested arrays.

The package passes TypeScript type checking. A live OpenRouter probe was not
run because it requires an API key and network access; provider discovery and
model metadata therefore remain runtime items to verify in an environment with
valid credentials.

## Finding 1: OpenRouter discovery is now environment-gated

**Evidence:** `src/provider/local-discovery.ts:587-629`

The discovery loop reads `OPENROUTER_API_KEY` and only schedules an OpenRouter
probe when the variable is set. The probe runs alongside the existing local,
LAN, and Kubernetes probes rather than blocking startup independently. The
request has a five-second timeout and converts failures into a `null` result,
so an unavailable or invalid OpenRouter endpoint does not fail the complete
provider-discovery cycle.

**Impact:** Users with an OpenRouter key can receive provider metadata
automatically; users without one incur no OpenRouter request.

## Finding 2: OpenRouter models are filtered to tool-capable models

**Evidence:** `src/provider/local-discovery.ts:563-584`

The `/models` response is schema-decoded before use. Entries are excluded when
they have an empty ID, a `:free` suffix, a `:beta` suffix, or do not advertise
the `tools` supported parameter. If no eligible model remains, the provider is
not added to the discovered-provider set.

**Impact:** The TUI model list should contain models that can support tinycode's
tool-oriented workflow, while free and beta variants are intentionally omitted.

## Finding 3: OpenRouter model metadata is translated into tinycode capabilities

**Evidence:** `src/provider/local-discovery.ts:89-142`

The provider builder maps OpenRouter metadata into tinycode's provider model
shape, including:

- provider and model identifiers;
- context and output limits;
- prompt and completion pricing converted to per-million-token values;
- temperature, reasoning, and tool-call support;
- text, audio, image, video, file, and PDF input capabilities; and
- the OpenRouter AI SDK package identifier.

Missing context metadata falls back to `0`, and missing maximum-output metadata
falls back to 20% of the context length, capped at 16,384 tokens. Missing price
metadata is represented as zero cost.

**Follow-up:** These fallbacks should be checked against real `/models`
responses, especially for models whose provider-specific limits differ from
the advertised context length.

## Finding 4: OpenRouter is exposed in the provider-selection UI

**Evidence:** `src/cli/cmd/tui/component/dialog-provider.tsx:25-76` and
`src/core/models-local.json:134-141`

OpenRouter has been added to provider priority ordering and is labeled as a
cloud provider requiring an API key. The provider catalog declares
`OPENROUTER_API_KEY` as its environment variable and associates the
`@openrouter/ai-sdk-provider` package.

**Impact:** A discovered OpenRouter provider can be displayed and selected
through the existing provider dialog instead of requiring a separate provider
path.

## Finding 5: `omt` tools are registered directly by the core tool registry

**Evidence:** `src/tool/registry.ts:220-223`

The registry now calls `createOmtTools(ctx.directory)` and converts each
returned definition through the same `fromPlugin` adapter used by other custom
tools. The former internal `OmtPlugin` registration was removed from
`src/plugin/index.ts`, and `src/plugin/omt.ts` was deleted.

**Impact:** `omt` tools remain available through the normal tool registry while
avoiding an extra plugin lifecycle and registration layer. Tool IDs are now
registered directly from the `omt` tool definitions.

## Finding 6: Persistent `omt` context is injected into system prompts

**Evidence:** `src/session/llm/request.ts:75-97` and
`src/agent/agent.ts:441-460`

The runtime reads the directory-scoped notepad priority section and project
memory. Non-empty values are wrapped in explicit XML-like context tags:

- `<omt-priority-context>`
- `<omt-project-memory>`

Read and serialization errors are swallowed so missing or malformed optional
context does not abort a request. Empty notepad and empty project-memory values
are not injected.

**Important observation:** The same context-injection logic exists in both the
agent generation path and the LLM request preparation path. The current diff
does not establish whether those paths can be used together for one request.
If they can, the same context could be appended twice. This should be checked
with a request-level integration test or a captured system prompt before the
change is considered complete.

## Finding 7: `omt` enum schemas are corrected

**Evidence:** `src/omt/tools.ts:203,302,352,400-480`

Tool argument definitions now use flat enum arrays. The affected values include
notepad sections, project-memory sections and priorities, wiki categories, and
wiki confidence levels.

**Impact:** Consumers receiving these definitions should see the intended set
of string choices rather than a single nested array value. This improves schema
compatibility with model tool-call generation and validation.

## Verification status

The following command was run from `packages/tinycode` and exited successfully:

```bash
bun typecheck
```

No live OpenRouter discovery was performed because it requires a valid
`OPENROUTER_API_KEY` and external network access. No runtime TUI session or
request-capture test was performed, so the potential duplicate `omt` context
injection remains an open verification item.

## Recommended next verification

1. Run discovery with a valid OpenRouter key and confirm that at least one
   eligible tool-capable model appears in the provider dialog.
2. Capture a prepared request containing non-empty `.tinycode` `omt` data and
   confirm each context block appears exactly once.
3. Exercise representative `omt` tools and inspect their generated argument
   schemas to confirm enum values are flat arrays at runtime.
