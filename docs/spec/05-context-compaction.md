# 5. Context Compaction

Package: `internal/session/compaction.go`

Context compaction summarizes conversation history when the token count approaches the model's context limit, allowing sessions to continue indefinitely.

## 5.1 Compaction Config

```go
type CompactionConfig struct {
    MaskObservations bool // replace old tool outputs with "[output masked]"
    MinPreserve      int  // minimum tokens to keep uncompacted (2000)
    MaxPreserve      int  // maximum tokens to keep uncompacted (15000)
    MaxMessages      int  // trigger compaction when message count exceeds this (80)
}
```

### Constants

| Constant | Value | Description |
|----------|-------|-------------|
| `MinPreserveRecentTokens` | 2,000 | Floor for preserve boundary |
| `MaxPreserveRecentTokens` | 15,000 | Ceiling for preserve boundary |
| `defaultMaxMessages` | 80 | Message count trigger |
| `compactionCircuitBreakerThreshold` | 3 | Max compaction attempts before giving up |
| `maxTextChars` | 2,000 | Truncation limit for text parts in compaction prompt |
| `maxToolArgsChars` | 500 | Truncation limit for tool args in compaction prompt |
| `maxToolOutputChars` | 2,000 | Truncation limit for tool output in compaction prompt |
| `preserveRecentOutputs` | 5 | Tool results preserved when masking observations |

## 5.2 Compaction Trigger

The processor calls `checkCompaction()` after each LLM step. Compaction triggers when:

1. The message count exceeds `MaxMessages` (default 80), OR
2. The estimated token count approaches the model's context limit

A **circuit breaker** prevents infinite compaction loops — after `compactionCircuitBreakerThreshold` (3) compactions within a single `Process()` call, no further compaction attempts are made.

## 5.3 Observation Masking

When `MaskObservations` is true (default), older tool outputs are replaced with `"[output masked for compaction]"` before building the compaction prompt. The most recent `preserveRecentOutputs` (5) tool results are preserved verbatim.

This reduces the compaction prompt size while keeping recent context intact.

## 5.4 Compaction Prompt

The compaction prompt is built by `buildCompactionPrompt()`:

```xml
<task>
Summarize the conversation so far...
(9-section summary template)
</task>

<conversation>
<prior-summary>
(previous summary, if any)
</prior-summary>

<user>
[text content, truncated to 2000 chars]
[tool_call: name(args truncated to 500 chars)]
</user>
<assistant>
[text content, truncated to 2000 chars]
[tool_result: name = output truncated to 2000 chars]
[reasoning: content truncated to 2000 chars]
</assistant>
</conversation>

<read-files>
/path/to/file1.go
/path/to/file2.go
</read-files>

<modified-files>
/path/to/changed.go
</modified-files>
```

### Summary Sections

The compaction prompt requests these sections:

1. **Primary Request and Intent** — User's ultimate goal
2. **Key Technical Concepts** — Architecture, patterns, decisions
3. **Files and Code Sections** — Paths, functions, code discussed/modified
4. **Errors and fixes** — Problems and solutions
5. **Problem Solving** — Key decisions and rationale
6. **All user messages** — Every user instruction, in order
7. **Pending Tasks** — Remaining work
8. **Current Work** — What was being worked on at compaction time
9. **Optional Next Step** — Most logical next action

### File Tracking

`trackFiles()` scans messages for file paths:

| Tool | Tracked As |
|------|-----------|
| `read` | Read file (from `file_path` arg) |
| `write`, `edit`, `patch`, `apply_patch` | Modified file (from `file_path`/`path`/`file` arg) |
| `shell`, `bash` | Both (regex extraction from command string) |

## 5.5 Token Estimation

`LazyEstimator` provides cheap token estimates without a tokenizer:

```go
type LazyEstimator struct {
    tokensPerChar float64 // default: 0.25 (~4 chars per token)
}
```

### Preserve Boundary

`FindPreserveBoundary()` walks messages from newest to oldest, spending up to `budgetTokens` (clamped to `[MinPreserve, MaxPreserve]`). Returns the index where the "preserve" zone starts — messages before this index are candidates for compaction.

## 5.6 Compaction Result

```go
type CompactionResult struct {
    Summary       string   // LLM-generated summary
    ReadFiles     []string // files that were read during the session
    ModifiedFiles []string // files that were modified
    PreTokens     int      // estimated tokens before compaction
    PostTokens    int      // estimated tokens after compaction
    CompactionNum int      // 1-indexed compaction count for this session
}
```

After compaction, the processor replaces old messages with the summary, preserving only the "preserve boundary" tail of recent messages. The prior summary is passed to subsequent compactions for incremental refinement.
