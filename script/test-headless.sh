#!/usr/bin/env bash
#
# Automated headless integration tests for tinycode run mode.
#
# Prerequisites:
#   - Ollama running with a model pulled (default: qwen3:8b)
#   - dist/tinycode built (make build)
#   - jq installed
#
# Usage:
#   ./script/test-headless.sh              # run all tests
#   ./script/test-headless.sh TC-H01       # run specific test
#   ./script/test-headless.sh --list       # list test cases
#
# Environment:
#   TINYCODE_TEST_MODEL   Override test model (default: ollama/qwen3:8b)
#   TINYCODE_TEST_TIMEOUT Override per-test timeout in seconds (default: 120)

set -euo pipefail

TINYCODE="${TINYCODE:-./dist/tinycode}"
MODEL="${TINYCODE_TEST_MODEL:-ollama/qwen3:8b}"
TIMEOUT="${TINYCODE_TEST_TIMEOUT:-120}"
TMPDIR_BASE=$(mktemp -d)
PASS=0
FAIL=0
SKIP=0
RESULTS=()

trap 'rm -rf "$TMPDIR_BASE"' EXIT

# Colors (disabled if not a terminal)
if [ -t 1 ]; then
    GREEN='\033[0;32m'
    RED='\033[0;31m'
    YELLOW='\033[0;33m'
    BOLD='\033[1m'
    NC='\033[0m'
else
    GREEN='' RED='' YELLOW='' BOLD='' NC=''
fi

log_pass() { echo -e "  ${GREEN}PASS${NC}"; }
log_fail() { echo -e "  ${RED}FAIL${NC}"; }
log_skip() { echo -e "  ${YELLOW}SKIP${NC}"; }

# --- Assertion helpers ---

assert_exit_zero() {
    local rc="$1" label="$2"
    if [ "$rc" -eq 0 ]; then return 0; fi
    echo "    assertion failed: $label (exit code $rc, expected 0)" >&2
    return 1
}

assert_exit_nonzero() {
    local rc="$1" label="$2"
    if [ "$rc" -ne 0 ]; then return 0; fi
    echo "    assertion failed: $label (exit code 0, expected non-zero)" >&2
    return 1
}

assert_output_contains() {
    local output="$1" needle="$2" label="$3"
    if echo "$output" | grep -qF "$needle"; then return 0; fi
    echo "    assertion failed: $label (output does not contain '$needle')" >&2
    return 1
}

assert_ndjson_has_type() {
    local file="$1" type="$2" label="$3"
    if jq -e "select(.type==\"$type\")" "$file" > /dev/null 2>&1; then return 0; fi
    echo "    assertion failed: $label (no NDJSON event with type '$type')" >&2
    return 1
}

assert_ndjson_type_count_le() {
    local file="$1" type="$2" max="$3" label="$4"
    local count
    count=$(jq -s "[.[] | select(.type==\"$type\")] | length" "$file" 2>/dev/null || echo 0)
    if [ "$count" -le "$max" ]; then return 0; fi
    echo "    assertion failed: $label (got $count events of type '$type', expected <= $max)" >&2
    return 1
}

# --- Test runner ---

run_test() {
    local id="$1" name="$2"
    shift 2
    printf "  %-12s %-40s" "$id" "$name"
    if "$@" 2>"$TMPDIR_BASE/stderr-$id.log"; then
        log_pass
        PASS=$((PASS + 1))
        RESULTS+=("PASS  $id  $name")
    else
        log_fail
        FAIL=$((FAIL + 1))
        RESULTS+=("FAIL  $id  $name")
        if [ -s "$TMPDIR_BASE/stderr-$id.log" ]; then
            echo "    stderr:" >&2
            head -5 "$TMPDIR_BASE/stderr-$id.log" | sed 's/^/      /' >&2
        fi
    fi
}

# --- Test implementations ---

test_h01_basic_prompt() {
    local out="$TMPDIR_BASE/h01.ndjson"
    local rc=0
    echo "What is 2+2? Answer with just the number." | \
      timeout "$TIMEOUT" "$TINYCODE" run --format json \
        --dangerously-skip-permissions -m "$MODEL" > "$out" 2>/dev/null || rc=$?
    assert_exit_zero "$rc" "exit code" && \
    assert_ndjson_has_type "$out" "text" "text event exists" && \
    assert_ndjson_has_type "$out" "step_start" "step_start event exists"
}

test_h02_event_types() {
    local out="$TMPDIR_BASE/h02.ndjson"
    local rc=0
    timeout "$TIMEOUT" "$TINYCODE" run --format json \
      --dangerously-skip-permissions -m "$MODEL" \
      "List the files in the current directory using the shell tool. Just run ls." \
      > "$out" 2>/dev/null || rc=$?
    assert_exit_zero "$rc" "exit code" && \
    assert_ndjson_has_type "$out" "step_start" "step_start" && \
    assert_ndjson_has_type "$out" "step_finish" "step_finish" && \
    assert_ndjson_has_type "$out" "text" "text" && \
    assert_ndjson_has_type "$out" "tool_begin" "tool_begin" && \
    assert_ndjson_has_type "$out" "tool_end" "tool_end"
}

test_h03_tool_execution() {
    local out="$TMPDIR_BASE/h03.ndjson"
    local rc=0
    timeout "$TIMEOUT" "$TINYCODE" run --format json \
      --dangerously-skip-permissions -m "$MODEL" \
      "Run this exact shell command: echo hello-tc-test-h03" \
      > "$out" 2>/dev/null || rc=$?
    assert_exit_zero "$rc" "exit code" && \
    assert_ndjson_has_type "$out" "tool_end" "tool executed"
}

test_h04_permission_deny() {
    local out="$TMPDIR_BASE/h04.ndjson"
    local rc=0
    echo "Run the shell command: echo test" | \
      timeout "$TIMEOUT" "$TINYCODE" run --format json \
        -m "$MODEL" --max-iterations 2 \
      > "$out" 2>/dev/null || rc=$?
    # Without --dangerously-skip-permissions, tools should be auto-denied
    local tool_end_count
    tool_end_count=$(jq -s '[.[] | select(.type=="tool_end")] | length' "$out" 2>/dev/null || echo 0)
    if [ "$tool_end_count" -eq 0 ]; then return 0; fi
    echo "    assertion failed: tool_end events found ($tool_end_count), expected 0 (tools should be denied)" >&2
    return 1
}

test_h06_multi_turn() {
    local out="$TMPDIR_BASE/h06.ndjson"
    local rc=0
    printf '{"type":"prompt","text":"Say the word hello"}\n{"type":"exit"}\n' | \
      timeout "$TIMEOUT" "$TINYCODE" run --format json \
        --dangerously-skip-permissions --multi-turn -m "$MODEL" \
      > "$out" 2>/dev/null || rc=$?
    assert_exit_zero "$rc" "exit code" && \
    assert_ndjson_has_type "$out" "step_start" "step_start" && \
    assert_ndjson_has_type "$out" "ready" "ready signal between turns"
}

test_h07_max_iterations() {
    local out="$TMPDIR_BASE/h07.ndjson"
    local rc=0
    echo "Use the shell tool to run 'echo iteration' five separate times in a row" | \
      timeout "$TIMEOUT" "$TINYCODE" run --format json \
        --dangerously-skip-permissions --max-iterations 2 -m "$MODEL" \
      > "$out" 2>/dev/null || rc=$?
    assert_ndjson_type_count_le "$out" "step_start" 2 "max 2 iterations"
}

test_h09_agent_selection() {
    local out="$TMPDIR_BASE/h09.ndjson"
    local rc=0
    timeout "$TIMEOUT" "$TINYCODE" run --format json \
      --dangerously-skip-permissions -m "$MODEL" --agent architect \
      "Describe the architecture of a hello world program in one sentence" \
      > "$out" 2>/dev/null || rc=$?
    assert_exit_zero "$rc" "exit code" && \
    assert_ndjson_has_type "$out" "text" "text output exists"
}

test_h11_invalid_model() {
    local rc=0
    echo "hello" | \
      timeout 30 "$TINYCODE" run -m invalid/nonexistent \
      > /dev/null 2>&1 || rc=$?
    assert_exit_nonzero "$rc" "invalid model should fail"
}

test_h12_stdin_pipe() {
    local out="$TMPDIR_BASE/h12.txt"
    local rc=0
    echo "Say the word pineapple and nothing else" | \
      timeout "$TIMEOUT" "$TINYCODE" run \
        --dangerously-skip-permissions -m "$MODEL" \
      > "$out" 2>/dev/null || rc=$?
    assert_exit_zero "$rc" "exit code" && \
    assert_output_contains "$(cat "$out")" "pineapple" "output contains pineapple"
}

# --- Prerequisites ---

check_prereqs() {
    local ok=true

    if ! command -v jq &> /dev/null; then
        echo "Error: jq is required (brew install jq)" >&2
        ok=false
    fi

    if [ ! -x "$TINYCODE" ]; then
        echo "Error: $TINYCODE not found. Run 'make build' first." >&2
        ok=false
    fi

    if ! curl -sf http://localhost:11434/api/version > /dev/null 2>&1; then
        echo "Warning: Ollama not detected at localhost:11434" >&2
        echo "  Tests requiring an LLM will fail." >&2
    fi

    if [ "$ok" = false ]; then exit 1; fi
}

# --- Main ---

main() {
    echo -e "${BOLD}tinycode headless integration tests${NC}"
    echo "===================================="
    echo "  Binary:  $TINYCODE"
    echo "  Model:   $MODEL"
    echo "  Timeout: ${TIMEOUT}s per test"
    echo ""

    if [ "${1:-}" = "--list" ]; then
        echo "TC-H01  Basic prompt response"
        echo "TC-H02  NDJSON event types"
        echo "TC-H03  Tool execution"
        echo "TC-H04  Permission deny (default)"
        echo "TC-H06  Multi-turn conversation"
        echo "TC-H07  Max iterations cap"
        echo "TC-H09  Agent selection"
        echo "TC-H11  Invalid model error"
        echo "TC-H12  Stdin pipe input"
        exit 0
    fi

    check_prereqs

    local filter="${1:-}"

    [ -z "$filter" ] || [ "$filter" = "TC-H01" ] && run_test "TC-H01" "Basic prompt response" test_h01_basic_prompt
    [ -z "$filter" ] || [ "$filter" = "TC-H02" ] && run_test "TC-H02" "NDJSON event types" test_h02_event_types
    [ -z "$filter" ] || [ "$filter" = "TC-H03" ] && run_test "TC-H03" "Tool execution" test_h03_tool_execution
    [ -z "$filter" ] || [ "$filter" = "TC-H04" ] && run_test "TC-H04" "Permission deny" test_h04_permission_deny
    [ -z "$filter" ] || [ "$filter" = "TC-H06" ] && run_test "TC-H06" "Multi-turn conversation" test_h06_multi_turn
    [ -z "$filter" ] || [ "$filter" = "TC-H07" ] && run_test "TC-H07" "Max iterations cap" test_h07_max_iterations
    [ -z "$filter" ] || [ "$filter" = "TC-H09" ] && run_test "TC-H09" "Agent selection" test_h09_agent_selection
    [ -z "$filter" ] || [ "$filter" = "TC-H11" ] && run_test "TC-H11" "Invalid model error" test_h11_invalid_model
    [ -z "$filter" ] || [ "$filter" = "TC-H12" ] && run_test "TC-H12" "Stdin pipe input" test_h12_stdin_pipe

    echo ""
    echo "===================================="
    echo -e "  ${GREEN}$PASS passed${NC}  ${RED}$FAIL failed${NC}  ${YELLOW}$SKIP skipped${NC}"
    echo ""
    for r in "${RESULTS[@]}"; do
        echo "  $r"
    done
    echo ""

    [ "$FAIL" -eq 0 ]
}

main "$@"
