# Running the Model Compatibility Benchmark

This guide covers everything you need to run the tinycode LLM benchmark on any machine.

## Prerequisites

| Dependency | Version | Check |
|------------|---------|-------|
| Bun | 1.3+ | `bun --version` |
| Ollama | 0.30+ | `ollama --version` |
| Git | any | `git --version` |

Ollama must be running before you start: `ollama serve` (or the Ollama desktop app).

## Setup

```bash
git clone https://github.com/bobbyjohnstx/tinycode.git
cd tinycode
bun install
```

### Pull models

The benchmark does **not** pull models for you. Pull each model you want to test:

```bash
ollama pull qwen3.5:9b
ollama pull north-mini-code-1.0
ollama pull gemma4:12b
ollama pull qwen2.5:latest
ollama pull llama3.2:latest
# ... etc
```

Verify with `ollama list`. The benchmark checks this list at startup and exits if any requested model is missing.

## CLI Usage

```bash
bun benchmark --models "model1,model2" [options]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--models` | **(required)** | Comma-separated Ollama model tags |
| `--agent` | `build` | Agent persona (`build`, `debugger`, `executor`, etc.) |
| `--tasks` | `1,2,3,4,5` | Comma-separated task IDs to run |
| `--runs` | `10` | Runs per model/task combination |
| `--timeout` | `300` | Timeout in seconds per task |
| `--help` | — | Print usage |

### Examples

```bash
# Quick smoke test — one model, one task, one run
bun benchmark --models "qwen3.5:9b" --tasks 2 --runs 1 --timeout 120

# Full suite for a single model
bun benchmark --models "qwen3.5:9b" --runs 1 --timeout 600

# Compare multiple models
bun benchmark --models "qwen3.5:9b,gemma4:12b,llama3.1:8b" --runs 1 --timeout 600

# Test with a different agent persona
bun benchmark --models "qwen3.5:9b" --agent debugger --tasks 5 --runs 3

# Cloud ceiling reference (requires API key in tinycode config)
bun benchmark --models "anthropic/claude-sonnet-4-20250514" --runs 1
```

## Model Warmup

Before Task 1 for each model, the benchmark sends a warmup request to Ollama (`/api/generate` with `keep_alive: "10m"`). This pre-loads the model into GPU memory so the cold-load penalty doesn't inflate Task 1's duration. The warmup time is printed but not counted in any task score.

## The Five Benchmark Tasks

Each task uses a fixture project (a small TypeScript project with intentional bugs) that is copied to a fresh temp directory, git-initialized, and cleaned up after each run. The model gets the prompt, has full tool access, and is scored 0–3 per task (max 15 total).

### Task 1: Tool-Calling Diagnostic

**Prompt:** `Find the function "divide" in this project, read its implementation, and add a @throws JSDoc comment above the function describing when it would throw (e.g., division by zero). Do not fix the function, just document it.`

**Tests:** Discovery tools (grep/glob/read) + modification tools (edit/write).

**Scoring:**
- 3/3 — `@throws` added, discovery tools used, modification tools used
- 2/3 — Tools used but `@throws` not in file
- 1/3 — No tool calls (chat-only response)

### Task 2: Fix Failing Test

**Prompt:** `Run "bun test test/math.test.ts". One test fails. Fix the source code so all tests pass. Do not modify the test file.`

**Tests:** Shell execution to discover failure, source code editing to fix it. The bug: `divide()` returns `Infinity` for division by zero instead of throwing.

**Scoring:**
- 3/3 — Tests pass, test file unmodified, shell tool used
- 2/3 — Shell tool used but tests still fail
- 1/3 — No tool calls (text advice only)

### Task 3: Add Input Validation

**Prompt:** `Add input validation to the "processUser" function in src/validate.ts. It should throw an Error if: name is not a string, age is not a number, age is less than 0, or age is greater than 150. Keep the existing return statement for valid inputs.`

**Tests:** Logic and type checking. Verification dynamically imports the modified file and runs 5 test cases (valid input, bad name type, bad age type, negative age, age > 150).

**Scoring:**
- 3/3 — All 5 cases pass, tools used
- 2/3 — 3–4 cases pass, tools used
- 1/3 — Fewer than 3 cases pass or no tool calls

### Task 4: Multi-File Rename (hardest)

**Prompt:** `Rename the function "multiply" to "product" in src/math.ts and update all references across the project so that everything still works. Run the tests to confirm.`

**Tests:** Cross-file grep and coordinated multi-file editing. Requires renaming in both `src/math.ts` and `test/math.test.ts`.

**Scoring:**
- 3/3 — All "multiply" references removed, "product" in 2+ places, tests pass
- 2/3 — "multiply" removed but tests fail or incomplete replacement
- 1/3 — No tool calls

### Task 5: Debug from Stack Trace

**Prompt:** `Running "bun test test/format.test.ts" produces: TypeError: Cannot read properties of undefined (reading 'toUpperCase'). Find and fix the root cause. The bug is NOT in format.ts -- trace the error to its source.`

**Tests:** Cross-file debugging. The error is in `format.ts` but the root cause is in `user.ts` (returns `undefined` displayName for empty string input).

**Scoring:**
- 3/3 — Tests pass AND fix is in `src/user.ts` (correct root cause)
- 2/3 — Tests pass but fix is in `src/format.ts` (symptom fix)
- 1/3 — No tool calls or tests still fail

## Tier Definitions

| Tier | Score | Meaning |
|------|-------|---------|
| Full Agentic | 12–15 | Reliable tool calling, suitable for production workflows |
| Limited | 8–11 | Inconsistent tool usage, may require manual intervention |
| Chat Only | 4–7 | Minimal tool usage, primarily text-based responses |
| Not Recommended | 0–3 | Unreliable or non-functional |

## Output

### JSON reports

Written to `packages/tinycode/results/benchmark-<timestamp>.json`. Includes hardware info (CPU, cores, RAM), Ollama version, all per-run results with duration, tool call counts, and scores.

### Markdown report

Overwrites `docs/model-compatibility.md` with a formatted results table. If you want to preserve previous results, back up the file before running.

## Hardware Notes

Results from the reference run on Apple M1 Pro 32GB:

| RAM | Recommended Model | Score | Avg Time/Task |
|-----|-------------------|-------|---------------|
| 32GB | qwen3.5:9b | 14/15 | ~5.5 min |
| 32GB | north-mini-code-1.0 (MoE, 3B active) | 12/15 | ~5.5 min |
| 16GB | qwen2.5:latest (7B) | 9/15 | ~2 min |
| 8GB | llama3.2:latest (3B) | 6/15 | ~2.5 min |

Key constraints:
- Dense models >12B are too slow on 32GB (KV cache competes with model weights for RAM)
- MoE models (small active parameter count) run faster but may sacrifice accuracy
- Increase `--timeout` to 600+ for large or dense models

## Troubleshooting

| Problem | Fix |
|---------|-----|
| "Model not found" at startup | Run `ollama pull <model>` first |
| Tasks timing out | Increase `--timeout` (try 600 or 900) |
| Ollama connection refused | Start Ollama: `ollama serve` or launch the desktop app |
| All tasks score 1/3 | Model lacks tool-calling training — this is fundamental, not a config issue |
| Score 5/15 with 0 tool calls | Same as above — Granite, CodeLlama, DeepSeek-R1 distilled all hit this ceiling |
