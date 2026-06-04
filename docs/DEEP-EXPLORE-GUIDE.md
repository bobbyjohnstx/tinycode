# @deep-explore Usage Guide

`@deep-explore` is the LSP+AST-aware codebase search agent. It uses symbol lookup, reference
finding, and AST grep in addition to basic file search — making it better than `@explore`
for understanding code relationships and architecture.

## Steps Setting

The `steps` frontmatter field in `.opencode/agent/deep-explore.md` controls how many LLM
turns the agent gets. Each turn typically runs 1-3 tool calls (glob, grep, read, lsp_*).

### Model Lookup Table

| Model | Size | Context | Recommended steps | Notes |
|-------|------|---------|-------------------|-------|
| llama3.2 | 8B | ~8K | 30-35 | Context fills before steps exhaust; >35 produces incoherent output |
| qwen2.5 | 7B | ~8K | 30-35 | Same constraint as llama3.2 |
| gemma4:3b | 3B | ~4K | 20-25 | Very small context; keep steps low |
| gemma4:27b | 27B | ~32K | 50-60 | Can sustain longer explorations |
| gemma4:31b-cloud | 31B | ~32K | 50-60 | Good for architecture work |
| qwen3-14b (MaaS) | 14B | ~40K | 60-80 | Recommended for exhaustive audits |
| deepseek-r1-14b | 14B | ~32K | 60-70 | Good reasoning helps synthesis |
| llama-scout-17b | 17B | ~131K | 80-100 | Large context; can hold full exploration |

### Coverage Estimates (tinycode codebase, 38 src/ dirs)

| Steps | Approx. coverage | Time (local 8B) | Time (MaaS 14B) |
|-------|-----------------|------------------|------------------|
| 20 | ~21% (8 dirs) | 2-4 min | 1-2 min |
| 35 | ~40-50% | 5-8 min | 2-4 min |
| 50 | ~60-70% | 10-15 min | 4-6 min |
| 80 | ~80-85% | 20-30 min | 8-12 min |
| 100 | ~85-90% | 30-40 min | 12-18 min |

**Current setting**: `steps: 50` (good for MaaS 14B+; may hit context limits on local 8B models)

## Split Exploration Technique

For comprehensive architecture documentation on large codebases, split into two `@deep-explore`
sessions and combine with `@writer`. This prevents context overflow and produces better results
than a single long exploration.

### Pattern

**Session 1 — Structure scan:**
```
@deep-explore Map the full directory structure of this codebase. For every top-level
directory in src/, list what it contains and its purpose in one sentence. Be exhaustive —
cover all 38 directories, not just the obvious ones.
```

**Session 2 — Pattern scan:**
```
@deep-explore Identify the key design patterns in this codebase: how services are defined,
how dependency injection works, how errors are handled, how the data flow works from user
input to LLM to tool execution and back. Read actual code examples for each pattern.
```

**Synthesis:**
```
@writer Combine these two exploration results into a complete ARCHITECTURE.md:
[paste session 1 output]
---
[paste session 2 output]
```

### Why This Works

- Each session stays within the model's context budget
- Structure and patterns require different search strategies (breadth vs depth)
- The writer only needs to synthesize, not also explore — cleaner separation of concerns
- If one session produces bad output, you only need to re-run that half

## Known Accuracy Issues (gemma4, llama3.2)

Both models consistently produce these errors in architecture docs regardless of step count:

1. **Service naming**: Write `@tinycode/Foo` but actual code uses `@opencode/...` (legacy naming from upstream fork)
2. **`export namespace` claim**: Both claim it's avoided, but `src/plugin/loader.ts:14` uses it
3. **Missing sibling packages**: Never mention `packages/app`, `packages/core`, `packages/desktop`, etc.
4. **`src/storage/` vs `src/sync/`**: Confusion about which is the persistence layer (`src/storage/` is correct)

Use `/Users/bjohns/projects/tinycode/packages/tinycode/docs/ARCHITECTURE-REFERENCE.md` as
the verified ground truth to compare model output against.
