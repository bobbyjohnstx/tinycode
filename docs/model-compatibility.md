# Model Compatibility

This benchmark validates local LLM compatibility with tinycode's tool-calling system.

## Metadata

- **Last verified:** July 10, 2026
- **Hardware:** Apple M1 Pro (10 cores)
- **Memory:** 32 GB
- **Ollama:** 0.30.10

> **Staleness notice:** Results are considered current for 90 days. This report will be stale after October 8, 2026.

## Results

| Model | Size (Active) | Total | Tier | T1: Tool Diagnostic | T2: Fix Test | T3: Validation | T4: Rename | T5: Debug |
|-------|--------------|-------|------|-----|-----|-----|-----|-----|
| qwen3.5:9b | 9B | 14/15 | Full Agentic | 3/3 (366s) | 3/3 (221s) | 3/3 (324s) | 2/3 (455s) | 3/3 (332s) |
| north-mini-code-1.0 | 30B (3B) | 12/15 | Full Agentic | 3/3 (169s) | 3/3 (150s) | 3/3 (297s) | 0/3 (600s) | 3/3 (478s) |
| gemma4:12b | 12B | 11/15 | Limited | 3/3 (256s) | 3/3 (257s) | 0/3 (305s) | 2/3 (498s) | 3/3 (313s) |
| gemma4:26b | 26B (3.8B) | 9/15 | Limited | 3/3 (194s) | 3/3 (162s) | 3/3 (272s) | 0/3 (722s) | 0/3 (1035s) |
| qwen2.5:latest | 7B | 9/15 | Limited | 2/3 (90s) | 3/3 (138s) | 3/3 (124s) | 0/3 (130s) | 1/3 (94s) |
| lfm2:24b | 24B (2B) | 8/15 | Limited | 2/3 (66s) | 2/3 (40s) | 3/3 (84s) | 0/3 (45s) | 1/3 (39s) |
| qwen3.6:27b | 27B | 6/15 | Chat Only | 3/3 (510s) | 3/3 (562s) | 0/3 (600s) | 0/3 (1198s) | 0/3 (600s) |
| llama3.2:latest | 3B | 6/15 | Chat Only | 2/3 (42s) | 2/3 (41s) | 1/3 (37s) | 0/3 (600s) | 1/3 (38s) |
| llama3.1:8b | 8B | 6/15 | Chat Only | 2/3 (91s) | 2/3 (103s) | 1/3 (102s) | 0/3 (87s) | 1/3 (75s) |
| granite4.1:8b | 8B | 5/15 | Chat Only | 1/3 (3s) | 1/3 (1s) | 1/3 (1s) | 1/3 (1s) | 1/3 (1s) |
| granite3.1-dense:latest | 8B | 5/15 | Chat Only | 1/3 (130s) | 1/3 (96s) | 1/3 (111s) | 1/3 (99s) | 1/3 (97s) |
| codellama:latest | 7B | 5/15 | Chat Only | 1/3 (51s) | 1/3 (22s) | 1/3 (36s) | 1/3 (44s) | 1/3 (30s) |
| deepseek-r1:latest | 8B | 5/15 | Not Recommended | 1/3 (255s) | 1/3 (398s) | 1/3 (242s) | 1/3 (263s) | 1/3 (97s) |

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

## Key Findings

### Tool calling is the dividing line

Models either have tool-calling capability trained in or they don't. Size matters less than training:
- **Qwen family** (3.5, 2.5) and **Gemma4** have strong tool-calling training
- **Granite** (3.1 and 4.1), **CodeLlama**, and **DeepSeek-R1** (8B distilled) produce zero tool calls regardless of version
- **north-mini-code-1.0** is purpose-built for agentic coding and shows it

### MoE models: fast but constrained

MoE (Mixture of Experts) models with small active parameter counts run fast on consumer hardware:
- **north-mini-code-1.0** (3B active): Full Agentic tier, very persistent (retries on failures)
- **lfm2** (2B active): Fastest model tested (~55s avg), but limited accuracy
- **gemma4:26b** (3.8B active): Smart but too slow on complex tasks (T4/T5 timeout)

### Dense models >12B are too slow for 32GB

qwen3.6:27b aces the tasks it finishes but times out on 3 of 5. On M1 Pro 32GB, dense models above ~12B leave insufficient RAM for KV cache, causing inference to crawl.

### Recommended models by hardware

| RAM | Recommended | Score | Notes |
|-----|------------|-------|-------|
| 32GB | qwen3.5:9b | 14/15 | Best overall on consumer hardware |
| 32GB | north-mini-code-1.0 | 12/15 | MoE alternative, persistent on hard tasks |
| 16GB | qwen2.5:latest | 9/15 | Fast, solid validation/testing capability |
| 8GB | llama3.2:latest | 6/15 | Floor model, partial tool calling |

## Notes

- All tasks are machine-verified with deterministic pass/fail criteria
- Each model tested once per task (single run) — results may vary between runs
- Timeout is 600 seconds per task
- Durations include full end-to-end time (inference + tool execution)
- Cloud models (Claude, GPT-4, etc.) are not included in this benchmark
- Models scoring 5/15 with zero tool calls may work for chat-only usage but cannot perform agentic coding tasks
