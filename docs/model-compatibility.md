# Model Compatibility

This benchmark validates local LLM compatibility with tinycode's tool-calling system.

## Metadata

- **Last verified:** July 9, 2026
- **Hardware:**  (0 cores)
- **Memory:** 
- **Ollama:** 

> **Staleness notice:** Results are considered current for 90 days. This report will be stale after October 7, 2026.

## Results

| Model | Total | Tier | T1: Tool Diagnostic | T2: Fix Test | T3: Validation | T4: Rename | T5: Debug |
|-------|-------|------|-------|-------|-------|-------|-------|
| qwen3.5:9b | 0.0/3 | Not Recommended | 0.0/3 (60s) | 0.0/3 (0s) | 0.0/3 (0s) | 0.0/3 (0s) | 0.0/3 (0s) |

## Tier Definitions

- **Full Agentic (12-15):** Reliable tool calling, suitable for production workflows
- **Limited (8-11):** Inconsistent tool usage, may require manual intervention
- **Chat Only (4-7):** Minimal tool usage, primarily text-based responses
- **Not Recommended (0-3):** Unreliable or non-functional

## Task Descriptions

1. **Tool Diagnostic:** Find function, read it, add JSDoc (tests grep, read, edit)
2. **Fix Test:** Run tests, fix source code (tests shell, edit)
3. **Validation:** Add input validation (tests logic, type checking)
4. **Rename:** Multi-file rename with test verification (tests grep, edit)
5. **Debug:** Trace stack trace to root cause (tests debugging, cross-file)

## Notes

- All tasks are machine-verified with deterministic pass/fail criteria
- Each model is tested 10 times per task for statistical significance
- Scores are averaged across runs (0-3 points per task)
- Cloud models (Claude, GLM-5.2, etc.) can be included as ceiling references — label as "reference" not "recommended"
- To run the benchmark yourself: `bun benchmark --models "qwen3:14b,llama3.1:8b" --runs 10`
- See `packages/tinycode/benchmark/run.ts --help` for all options

## Running the Benchmark

```bash
# Full local benchmark (7 models, ~3 hours)
bun benchmark --models "qwen3:14b,qwen3.5:9b,llama3.1:8b,mistral-nemo:12b,phi4:14b,codestral:22b,qwen3:3b" --runs 10

# Quick test (1 model, 1 task, ~2 min)
bun benchmark --models "qwen3:14b" --tasks 2 --runs 1

# Cloud ceiling reference
bun benchmark --models "anthropic/claude-sonnet-4-20250514" --runs 3
```

Prerequisites: models must be pulled in Ollama first (`ollama pull qwen3:14b`). Cloud models require API keys configured in tinycode.
