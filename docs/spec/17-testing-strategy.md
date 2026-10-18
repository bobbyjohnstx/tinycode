# 17. Automated Testing Strategy

This document defines the automated testing approach for tinycode, combining unit tests, headless run-mode integration tests, and tmux-based TUI tests.

## 17.1 Testing Tiers

### Tier 1: Unit Tests (Go test)

Standard `go test ./... -count=1`. Fast, no external dependencies.

**Coverage targets:**
- Permission evaluation (rule matching, wildcard, cascade)
- Config parsing (JSONC, merge semantics, env vars)
- LLM event parsing (SSE, tool-call JSON repair)
- Agent registry (loading, compact selection, frontmatter)
- Provider retry logic (backoff calculation, retryable patterns)
- ID generation (prefix, ordering)
- Compaction (token estimation, observation masking, truncation)
- Frontmatter parsing
- Skill discovery and parameter substitution
- Command discovery and deduplication

**Existing test files:** `permission_test.go`, `overlay_test.go`, `prompt_autocomplete_test.go`, `run_test.go`, `app_test.go`, `flags_test.go`, `server_test.go`, `sse_test.go`, `auth_test.go`, `middleware_test.go`, `respond_test.go`, `pending_store_test.go`, `handler_plugin_test.go`, `integration_test.go`, `db_test.go`.

### Tier 2: Headless Integration Tests (run mode)

Uses `tinycode run` with NDJSON output and JSON permission protocol for end-to-end testing without a TUI. Requires an LLM (Ollama recommended for local testing).

**What it tests:**
- Full prompt → LLM → tool execution → response pipeline
- Provider discovery and model resolution
- Permission system (config rules, JSON protocol, skip-permissions)
- Session persistence (create, continue, message storage)
- Tool execution (shell, read, write, edit, grep, glob)
- Multi-turn conversation flow
- Context compaction under load
- NDJSON event stream format and completeness
- Agent switching and agent-specific permissions
- MCP tool bridging (when MCP servers are configured)
- Plugin hook dispatch

**Advantages over tmux:**
- Deterministic (no timing-dependent pane capture)
- Scriptable (stdin/stdout JSON protocol)
- Parseable output (NDJSON events)
- No terminal emulator required (CI-friendly)
- Permission responses can be programmatic

**Interactive JSON permission replies** (`--permissions json` with stdin `permission_reply` lines) are covered in `cmd/tinycode/run_test.go` (`TestJSONPermissionProtocol_EndToEnd`). Integration tests in `test/integration/permission_test.go` cover config allow/deny and JSON mode with pre-approved rules.

### Tier 3: TUI Integration Tests (tmux)

Uses `script/tui-compare.sh` and the `qa-tester` agent for interactive TUI testing. Tests visual rendering, keyboard shortcuts, and user interaction flows that headless mode cannot cover.

**What it tests:**
- TUI startup and initial render
- Keyboard shortcuts (leader key, Tab cycling, Ctrl+P palette)
- Prompt input and submission
- Chat scroll and message rendering
- Sidebar toggle and session tree
- Model/agent switching dialogs
- Permission prompt rendering and interaction
- Thought block expand/collapse
- Toast notifications
- Status bar content
- Diff viewer navigation
- Welcome screen

## 17.2 Headless Test Harness

### Architecture

```
test-harness.sh
  ├── starts Ollama (if not running)
  ├── builds dist/tinycode
  ├── runs test cases via tinycode run --format json
  ├── parses NDJSON output
  ├── asserts on event types, content, exit codes
  └── reports results
```

### Test Case Format

Each test case is a shell function that:
1. Runs `tinycode run` with specific flags and prompt
2. Captures stdout (NDJSON events) and stderr
3. Parses events with `jq`
4. Asserts on expected behavior
5. Returns 0 (pass) or 1 (fail)

### Core Test Cases

#### TC-H01: Basic Prompt Response
```bash
# Verify: prompt goes in, text comes out
echo "What is 2+2? Answer with just the number." | \
  tinycode run --format json --dangerously-skip-permissions \
    -m ollama/qwen3:8b
# Assert: at least one {"type":"text"} event exists
# Assert: exit code 0
```

#### TC-H02: NDJSON Event Types
```bash
# Verify: all 8 event types can be emitted
tinycode run --format json --dangerously-skip-permissions \
  -m ollama/qwen3:8b \
  "List the files in the current directory using the shell tool"
# Assert: events include step_start, text, tool_begin, tool_end, step_finish
# Assert: tool_begin has toolName="shell"
```

#### TC-H03: Tool Execution
```bash
# Verify: shell tool executes and returns output
echo '{"type":"prompt","text":"Run: echo hello-from-tinycode"}' | \
  tinycode run --format json --dangerously-skip-permissions \
    -m ollama/qwen3:8b
# Assert: tool_end event contains output with "hello-from-tinycode"
```

#### TC-H04: Permission Deny (Default)
```bash
# Verify: without --dangerously-skip-permissions, tools are denied
echo "Run the shell command: echo test" | \
  tinycode run --format json -m ollama/qwen3:8b \
    --max-iterations 3
# Assert: no tool_end events (tools denied by default)
```

#### TC-H05: JSON Permission Protocol
```bash
# Verify: permission requests emitted, replies accepted
# Use a co-process or named pipe for bidirectional communication
mkfifo /tmp/tc-perm-test
(
  # Read NDJSON, find permission request, send approval
  while IFS= read -r line; do
    type=$(echo "$line" | jq -r '.type // empty')
    if [ "$type" = "permission" ]; then
      id=$(echo "$line" | jq -r '.id')
      echo "{\"type\":\"permission_reply\",\"id\":\"$id\",\"reply\":\"once\"}"
    fi
  done < /tmp/tc-perm-test
) | tinycode run --format json --permissions json \
    -m ollama/qwen3:8b \
    "Run: echo permission-granted" > /tmp/tc-perm-test
# Assert: tool_end event exists (permission was granted)
rm /tmp/tc-perm-test
```

#### TC-H06: Multi-Turn Conversation
```bash
# Verify: multi-turn mode processes multiple prompts
printf '{"type":"prompt","text":"Remember the word: banana"}\n{"type":"prompt","text":"What word did I ask you to remember?"}\n{"type":"exit"}\n' | \
  tinycode run --format json --dangerously-skip-permissions \
    --multi-turn -m ollama/qwen3:8b
# Assert: two step_start events (one per turn)
# Assert: ready signals between turns
# Assert: second turn text contains "banana"
```

#### TC-H07: Max Iterations Cap
```bash
# Verify: processor stops after max-iterations
echo "Keep using the shell tool to list files, do it 10 times" | \
  tinycode run --format json --dangerously-skip-permissions \
    --max-iterations 2 -m ollama/qwen3:8b
# Assert: at most 2 step_start events
```

#### TC-H08: Session Continue
```bash
# Verify: session continuation works
# First turn
SESSION_ID=$(tinycode run --format json --dangerously-skip-permissions \
  -m ollama/qwen3:8b \
  "Remember: the secret code is 42" 2>&1 | \
  jq -r 'select(.type=="step_start") | .sessionID' | head -1)

# Second turn (continue)
echo "What is the secret code?" | \
  tinycode run --format json --dangerously-skip-permissions \
    -m ollama/qwen3:8b -c
# Assert: text output contains "42"
```

#### TC-H09: Agent Selection
```bash
# Verify: --agent flag selects the right agent
echo "What agent are you?" | \
  tinycode run --format json --dangerously-skip-permissions \
    -m ollama/qwen3:8b --agent architect
# Assert: exit code 0
# Assert: text output exists
```

#### TC-H10: Config Permission Rules
```bash
# Verify: config-level permission rules work
# Create temp config that allows shell
TMPDIR=$(mktemp -d)
cat > "$TMPDIR/config.json" << 'EOF'
{
  "model": "ollama/qwen3:8b",
  "permission": {
    "allow": ["shell *"]
  }
}
EOF
TINYCODE_CONFIG_DIR="$TMPDIR" \
  echo "Run: echo config-allowed" | \
  tinycode run --format json -m ollama/qwen3:8b
# Assert: tool_end event with "config-allowed"
rm -rf "$TMPDIR"
```

#### TC-H11: Error Handling
```bash
# Verify: invalid model produces clear error
echo "hello" | \
  tinycode run --format json -m invalid/nonexistent 2>&1
# Assert: exit code non-zero
# Assert: stderr contains "error"
```

#### TC-H12: Stdin Pipe Input
```bash
# Verify: piped stdin works as prompt
echo "What is 1+1? Answer with just the number." | \
  tinycode run --dangerously-skip-permissions -m ollama/qwen3:8b
# Assert: output contains "2"
# Assert: exit code 0
```

## 17.3 TUI Test Cases (tmux)

These tests use the `qa-tester` agent with tmux. They verify visual behavior that headless mode cannot cover.

#### TC-T01: Startup and Welcome Screen
```
Launch tinycode in tmux, capture pane after 3s.
Assert: welcome message or prompt area is rendered.
Assert: status bar shows model name.
Assert: no crash or error text.
```

#### TC-T02: Prompt Input and Submission
```
Send "hello" + Enter via tmux send-keys.
Wait for response (poll for assistant text in pane).
Assert: user message "hello" appears in chat.
Assert: assistant response appears below.
```

#### TC-T03: Leader Key — Sidebar Toggle
```
Send Ctrl+X then b.
Capture pane.
Assert: sidebar is visible (session tree or separator character).
Send Ctrl+X then b again.
Assert: sidebar is hidden.
```

#### TC-T04: Tab Agent Cycling
```
Press Tab.
Capture status bar.
Assert: agent name changes from "build" to next agent.
Press Shift+Tab.
Assert: agent name changes back.
```

#### TC-T05: Command Palette (Ctrl+P)
```
Send Ctrl+P.
Capture pane.
Assert: palette overlay is visible with command list.
Send Escape.
Assert: palette is dismissed.
```

#### TC-T06: Escape Interrupt
```
Submit a long prompt, then immediately send Escape.
Capture pane.
Assert: "interrupted" or similar message appears.
Assert: prompt becomes active again.
```

#### TC-T07: PgUp/PgDown Scroll
```
Submit several prompts to fill the chat area.
Send PgUp.
Capture pane.
Assert: earlier messages are visible.
Send PgDown.
Assert: latest messages are visible.
```

#### TC-T08: Model Switch (leader+m)
```
Send Ctrl+X then m.
Capture pane.
Assert: model list dialog is visible.
Send Escape.
Assert: dialog dismissed.
```

#### TC-T09: Permission Prompt
```
Submit a prompt that triggers a shell tool.
Wait for permission prompt.
Capture pane.
Assert: permission dialog shows tool name and command.
Send "a" (Allow) or "y".
Assert: tool executes and output appears.
```

#### TC-T10: Thought Block Toggle
```
Submit a prompt that triggers reasoning (if model supports it).
Look for "+/- Thought" in output.
Send "T" key.
Assert: thought blocks toggle visibility.
```

## 17.4 Test Runner Script

### `script/test-headless.sh`

```bash
#!/usr/bin/env bash
# Automated headless integration tests for tinycode run mode.
#
# Prerequisites:
#   - Ollama running with qwen3:8b pulled
#   - dist/tinycode built (make build)
#
# Usage:
#   ./script/test-headless.sh              # run all tests
#   ./script/test-headless.sh TC-H01       # run specific test
#   ./script/test-headless.sh --list       # list test cases

set -euo pipefail

TINYCODE="./dist/tinycode"
MODEL="ollama/qwen3:8b"
TIMEOUT=120  # seconds per test
PASS=0
FAIL=0
SKIP=0
RESULTS=()

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m'

assert_exit_code() {
    local expected="$1" actual="$2" label="$3"
    if [ "$actual" -eq "$expected" ]; then
        return 0
    fi
    echo "  FAIL: $label (expected exit $expected, got $actual)"
    return 1
}

assert_contains() {
    local haystack="$1" needle="$2" label="$3"
    if echo "$haystack" | grep -q "$needle"; then
        return 0
    fi
    echo "  FAIL: $label (output does not contain '$needle')"
    return 1
}

assert_json_has_type() {
    local output="$1" type="$2" label="$3"
    if echo "$output" | jq -e "select(.type==\"$type\")" > /dev/null 2>&1; then
        return 0
    fi
    echo "  FAIL: $label (no event with type '$type')"
    return 1
}

run_test() {
    local name="$1"
    shift
    echo -n "  $name ... "
    if "$@"; then
        echo -e "${GREEN}PASS${NC}"
        PASS=$((PASS + 1))
        RESULTS+=("PASS $name")
    else
        echo -e "${RED}FAIL${NC}"
        FAIL=$((FAIL + 1))
        RESULTS+=("FAIL $name")
    fi
}

# --- Test implementations ---

test_h01_basic_prompt() {
    local output
    output=$(echo "What is 2+2? Answer with just the number." | \
      timeout "$TIMEOUT" "$TINYCODE" run --format json \
        --dangerously-skip-permissions -m "$MODEL" 2>/dev/null)
    assert_json_has_type "$output" "text" "text event exists"
}

test_h02_event_types() {
    local output
    output=$(timeout "$TIMEOUT" "$TINYCODE" run --format json \
      --dangerously-skip-permissions -m "$MODEL" \
      "List files in the current directory using the shell tool" 2>/dev/null)
    assert_json_has_type "$output" "step_start" "step_start" && \
    assert_json_has_type "$output" "step_finish" "step_finish"
}

test_h06_multi_turn() {
    local output
    output=$(printf '{"type":"prompt","text":"Say hello"}\n{"type":"exit"}\n' | \
      timeout "$TIMEOUT" "$TINYCODE" run --format json \
        --dangerously-skip-permissions --multi-turn -m "$MODEL" 2>/dev/null)
    assert_json_has_type "$output" "step_start" "step_start" && \
    assert_json_has_type "$output" "ready" "ready signal"
}

test_h07_max_iterations() {
    local output count
    output=$(echo "Use the shell tool to run 'echo test' five separate times" | \
      timeout "$TIMEOUT" "$TINYCODE" run --format json \
        --dangerously-skip-permissions --max-iterations 2 -m "$MODEL" 2>/dev/null)
    count=$(echo "$output" | jq -s '[.[] | select(.type=="step_start")] | length')
    if [ "$count" -le 2 ]; then
        return 0
    fi
    echo "  FAIL: max-iterations not respected (got $count step_starts, expected <=2)"
    return 1
}

test_h11_invalid_model() {
    local rc=0
    echo "hello" | timeout 30 "$TINYCODE" run -m invalid/nonexistent 2>/dev/null || rc=$?
    if [ "$rc" -ne 0 ]; then
        return 0
    fi
    echo "  FAIL: expected non-zero exit for invalid model"
    return 1
}

test_h12_stdin_pipe() {
    local output
    output=$(echo "Say the word 'pineapple'" | \
      timeout "$TIMEOUT" "$TINYCODE" run \
        --dangerously-skip-permissions -m "$MODEL" 2>/dev/null)
    assert_contains "$output" "pineapple" "output contains pineapple"
}

# --- Main ---

check_prereqs() {
    if ! command -v jq &> /dev/null; then
        echo "Error: jq is required" >&2
        exit 1
    fi
    if [ ! -x "$TINYCODE" ]; then
        echo "Error: $TINYCODE not found. Run 'make build' first." >&2
        exit 1
    fi
    if ! curl -s http://localhost:11434/api/version > /dev/null 2>&1; then
        echo "Warning: Ollama not detected at localhost:11434" >&2
        echo "  Some tests may fail without a running LLM." >&2
    fi
}

main() {
    echo "tinycode headless integration tests"
    echo "===================================="
    check_prereqs

    if [ "${1:-}" = "--list" ]; then
        echo "TC-H01  Basic prompt response"
        echo "TC-H02  NDJSON event types"
        echo "TC-H06  Multi-turn conversation"
        echo "TC-H07  Max iterations cap"
        echo "TC-H11  Invalid model error"
        echo "TC-H12  Stdin pipe input"
        exit 0
    fi

    local filter="${1:-}"

    [ -z "$filter" ] || [ "$filter" = "TC-H01" ] && run_test "TC-H01 Basic prompt" test_h01_basic_prompt
    [ -z "$filter" ] || [ "$filter" = "TC-H02" ] && run_test "TC-H02 Event types" test_h02_event_types
    [ -z "$filter" ] || [ "$filter" = "TC-H06" ] && run_test "TC-H06 Multi-turn" test_h06_multi_turn
    [ -z "$filter" ] || [ "$filter" = "TC-H07" ] && run_test "TC-H07 Max iterations" test_h07_max_iterations
    [ -z "$filter" ] || [ "$filter" = "TC-H11" ] && run_test "TC-H11 Invalid model" test_h11_invalid_model
    [ -z "$filter" ] || [ "$filter" = "TC-H12" ] && run_test "TC-H12 Stdin pipe" test_h12_stdin_pipe

    echo ""
    echo "===================================="
    echo -e "Results: ${GREEN}$PASS passed${NC}, ${RED}$FAIL failed${NC}, ${YELLOW}$SKIP skipped${NC}"
    echo ""
    for r in "${RESULTS[@]}"; do
        echo "  $r"
    done

    [ "$FAIL" -eq 0 ]
}

main "$@"
```

### Tier 4: Fresh-env Smoke (container)

Validates the new-user install path in a clean container — no host config, no database, no prior sessions. Catches issues invisible to developer-machine tests: missing PATH entries, Mach-O binaries in Linux containers, mock LLM protocol mismatches.

**Script:** `script/test-fresh-env.sh`

**What it tests:**
- `tinycode doctor` on empty HOME (plugin tips, GOPATH/bin hint, LSP summary)
- Plugin install from local binary (`--from`) and from GitHub release assets
- Doctor configured-vs-installed plugin resolve
- Headless `tinycode run` against mock LLM with scripted tool_calls (lsp_diagnostics via gopls)
- Optional TUI smoke via tmux (prompt chrome, typed input, history count)
- Cross-compilation (Linux ELF binaries for container, not host Mach-O)

**Mock LLM:** `script/mock-llm/main.go` — Go binary serving SSE on `0.0.0.0:18080`. Returns scripted tool_calls so silent no-ops fail the run.

**Container engine:** podman or docker (`FRESH_ENV_ENGINE`). Uses host networking (`FRESH_ENV_NETWORK=host`) in CI for mock access.

**Test cases:**

| ID | Check | Requires |
|----|-------|----------|
| TC1 | Doctor empty HOME — plugin tips | Container only |
| TC2 | Plugin install from local binary | Cross-compiled plugin |
| TC3 | Doctor configured + gopls relevance | go.mod fixture |
| TC4 | Headless run — mock LLM forces lsp_diagnostics | Mock LLM + gopls |
| TC5 | TUI prompt chrome + typed input | tmux (skip with `--skip-tui`) |
| TC6 | Plugin install from GitHub release | Internet (`--release-install`) |
| TC7 | GOPATH/bin PATH gap detection | Go installed, gopls in GOPATH |

**CI wiring:**
- GitHub: `.github/workflows/fresh-env.yml` (PR/push path filters + weekly cron Sunday 06:00 UTC)
- Gitea: `fresh-env` job in `.gitea/workflows/ci.yml`
- Both run `--skip-tui --release-install` with `FRESH_ENV_ENGINE=docker`

**Results:** `.fresh-env-results/summary.json` + doctor dumps + NDJSON captures

**Key design decisions:**
- Container uses `bash -c` (not `-lc`) so `/etc/profile` doesn't wipe PATH and hide `/go/bin/gopls`
- TUI sends keys via a mounted script file (avoids quote-breaking in `docker exec`)
- Mock LLM must speak SSE — tinycode always streams, plain JSON responses silently fail
- Release install runs in an empty `/work/project` so install can't fall back to building from source

## 17.5 qa-tester Agent

Port the TS `qa-tester` agent to Go as `internal/agent/defaults/qa-tester.md`. The agent is designed for tmux-based interactive testing and produces structured QA test reports.

See the agent definition for the full prompt, constraints, and output format.

## 17.6 CI Integration

### Tier 1 (every PR)
```yaml
- name: Unit tests
  run: go test ./... -count=1 -race -timeout 5m
```

### Tier 2 (nightly / pre-release)
```yaml
- name: Headless integration tests
  run: |
    make build
    ollama pull qwen3:8b
    ./script/test-headless.sh
```

### Tier 3 (manual / pre-release)
```
Run qa-tester agent with specific TUI test cases.
Requires interactive terminal (not CI).
```

## 17.7 Test Coverage Targets

As of v2.4.1 (October 2026). 36 of 66 packages are at or above 80%.

| Package | Actual | Target | Notes |
|---------|--------|--------|-------|
| `internal/agent` | 90.3% | 90% | At target |
| `internal/lsp` | 84.8% | 80% | At target |
| `internal/tool` | 82.3% | 80% | At target |
| `internal/tui` | 80.5% | 80% | Crossed in Oct 2026 sprint (~1500 new test lines) |
| `internal/session` | 80%+ | 80% | At target |
| `internal/storage` | 77.8% | 80% | Near-miss, next candidate |
| `cmd/tinycode` | 13.7% | — | CLI wiring + doctor output; 80% not a meaningful target |

The 80% target applies to packages with real logic. CLI entry points (`cmd/tinycode`) and pure rendering code are exempt — testing exact print format has diminishing returns.

## 17.8 Test Data and Fixtures

### Mock LLM Server

`script/mock-llm/main.go` is a standalone Go binary that speaks the OpenAI-compatible `/v1/chat/completions` SSE protocol. It serves scripted responses including tool_calls, enabling deterministic integration tests without a real LLM or GPU.

**Key design constraints:**
- Must speak **SSE** — tinycode always streams; plain JSON responses silently fail
- Must bind `0.0.0.0` — container tests access via host gateway or host networking
- Tool_calls in responses must be syntactically valid — the model's tool-call JSON repair expects well-formed function_call objects

**Usage:**
```bash
go run ./script/mock-llm &   # starts on :18080
# Point tinycode at it:
TINYCODE_OPENROUTER_HOST=http://localhost:18080 tinycode run ...
```

The mock is used by the fresh-env harness (Tier 4) and can be used for local development testing of tool-call flows without burning LLM tokens.

### Fake LSP Server

`internal/lsp/fake_server_test.go` contains a Python-based minimal LSP server for unit tests. It supports configurable delays (`FAKE_LSP_INIT_DELAY`, `FAKE_LSP_DIAG_DELAY`), init hang (`FAKE_LSP_INIT_HANG`), and post-init hang (`FAKE_LSP_HANG_AFTER_INIT`) for testing ConnectTimeout, diagnostics caching, and reconnection.

### Fixture Files

Test fixtures for config parsing, permission rules, and agent definitions live adjacent to their test files (not in a separate `testdata/` tree) to maintain locality.

## 17.9 CI Pipeline Summary

| Tier | Trigger | Script/Command | Requires | Time |
|------|---------|---------------|----------|------|
| 1 Unit | Every push/PR | `go test ./... -count=1` | Go | ~30s |
| 1 Lint | Every push/PR | `make lint` | Go, staticcheck | ~15s |
| 2 Headless | Pre-release / manual | `script/test-headless.sh` | Ollama + model | ~5min |
| 3 TUI | Pre-release / manual | `script/tui-test.sh` | tmux, optionally Ollama | ~3min |
| 4 Fresh-env | PR/push + weekly cron | `script/test-fresh-env.sh --skip-tui` | docker/podman, Go | ~5min |
| — Plugins | Pre-release / manual | `script/test-plugins.sh` | Internet | ~2min |

### Workflow Files

| File | Platform | Runs |
|------|----------|------|
| `.github/workflows/go-ci.yml` | GitHub | Tier 1 on every push/PR |
| `.github/workflows/go-release.yml` | GitHub | Build + release on tag push |
| `.github/workflows/fresh-env.yml` | GitHub | Tier 4 on PR/push (path-filtered) + weekly cron |
| `.gitea/workflows/ci.yml` | Gitea | Tier 1 + TUI + fresh-env |

---

Prev: [16-not-implemented.md](16-not-implemented.md) | [Back to Index](README.md)
