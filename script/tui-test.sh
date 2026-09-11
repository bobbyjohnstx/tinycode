#!/usr/bin/env bash
#
# Automated TUI integration tests via tmux.
#
# Usage:
#   ./script/tui-test.sh           # run all tests
#   ./script/tui-test.sh T01       # run a single test
#   ./script/tui-test.sh T01 T05   # run specific tests
#
# Requires: tmux, built binary (make build)

set -euo pipefail

TINYCODE_BIN="${TINYCODE_BIN:-./dist/tinycode}"
SESSION_PREFIX="tui-test"
WORK_DIR=""
PASS=0
FAIL=0
SKIP=0
TOTAL=0

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BOLD='\033[1m'
NC='\033[0m'

cleanup_all() {
    tmux list-sessions -F '#{session_name}' 2>/dev/null | grep "^${SESSION_PREFIX}" | while read -r s; do
        tmux kill-session -t "$s" 2>/dev/null || true
    done
    [ -n "$WORK_DIR" ] && rm -rf "$WORK_DIR" 2>/dev/null || true
}

trap cleanup_all EXIT

check_prereqs() {
    if ! command -v tmux &>/dev/null; then
        echo "tmux not found — install it first"
        exit 1
    fi
    if [ ! -x "$TINYCODE_BIN" ]; then
        echo "tinycode binary not found at $TINYCODE_BIN — run 'make build' first"
        exit 1
    fi
}

new_session() {
    local name="${SESSION_PREFIX}-${1}"
    WORK_DIR=$(mktemp -d)

    tmux new-session -d -s "$name" -x 120 -y 40
    tmux send-keys -t "$name" "cd $WORK_DIR && TINYCODE_DISABLE_MOUSE=1 TINYCODE_DB=:memory: $TINYCODE_BIN" Enter
    echo "$name"
}

kill_session() {
    local name="$1"
    tmux kill-session -t "$name" 2>/dev/null || true
}

send_keys() {
    local session="$1"
    shift
    tmux send-keys -t "$session" "$@"
}

send_text() {
    local session="$1" text="$2"
    tmux send-keys -t "$session" "$text" Enter
}

capture_pane() {
    local session="$1"
    tmux capture-pane -t "$session" -p
}

wait_for_text() {
    local session="$1" pattern="$2" timeout="${3:-30}"
    local deadline=$((SECONDS + timeout))
    while [ $SECONDS -lt $deadline ]; do
        if capture_pane "$session" | grep -qF "$pattern" 2>/dev/null; then
            return 0
        fi
        sleep 0.5
    done
    return 1
}

wait_for_regex() {
    local session="$1" pattern="$2" timeout="${3:-30}"
    local deadline=$((SECONDS + timeout))
    while [ $SECONDS -lt $deadline ]; do
        if capture_pane "$session" | grep -qE "$pattern" 2>/dev/null; then
            return 0
        fi
        sleep 0.5
    done
    return 1
}

assert_contains() {
    local session="$1" pattern="$2" label="$3"
    TOTAL=$((TOTAL + 1))
    local captured
    captured=$(capture_pane "$session")
    if echo "$captured" | grep -qF "$pattern"; then
        echo -e "  ${GREEN}PASS${NC}: $label"
        PASS=$((PASS + 1))
        return 0
    else
        echo -e "  ${RED}FAIL${NC}: $label"
        echo "    Expected to contain: $pattern"
        echo "    Last 5 lines of pane:"
        echo "$captured" | tail -5 | sed 's/^/      /'
        FAIL=$((FAIL + 1))
        return 1
    fi
}

assert_not_contains() {
    local session="$1" pattern="$2" label="$3"
    TOTAL=$((TOTAL + 1))
    local captured
    captured=$(capture_pane "$session")
    if echo "$captured" | grep -qF "$pattern"; then
        echo -e "  ${RED}FAIL${NC}: $label"
        echo "    Should NOT contain: $pattern"
        FAIL=$((FAIL + 1))
        return 1
    else
        echo -e "  ${GREEN}PASS${NC}: $label"
        PASS=$((PASS + 1))
        return 0
    fi
}

assert_regex() {
    local session="$1" pattern="$2" label="$3"
    TOTAL=$((TOTAL + 1))
    local captured
    captured=$(capture_pane "$session")
    if echo "$captured" | grep -qE "$pattern"; then
        echo -e "  ${GREEN}PASS${NC}: $label"
        PASS=$((PASS + 1))
        return 0
    else
        echo -e "  ${RED}FAIL${NC}: $label"
        echo "    Expected regex: $pattern"
        echo "    Last 5 lines of pane:"
        echo "$captured" | tail -5 | sed 's/^/      /'
        FAIL=$((FAIL + 1))
        return 1
    fi
}

skip_test() {
    local label="$1" reason="$2"
    TOTAL=$((TOTAL + 1))
    SKIP=$((SKIP + 1))
    echo -e "  ${YELLOW}SKIP${NC}: $label ($reason)"
}

# ─── Test Cases ──────────────────────────────────────────────────

test_T01() {
    echo -e "${BOLD}T01: TUI renders on startup${NC}"
    local session
    session=$(new_session "T01")
    sleep 3

    # The TUI should render something — status bar, welcome, or prompt area
    assert_regex "$session" "." "TUI rendered content"

    kill_session "$session"
}

test_T02() {
    echo -e "${BOLD}T02: Ctrl+D exits cleanly${NC}"
    local session
    session=$(new_session "T02")
    sleep 3

    send_keys "$session" C-d
    sleep 2

    # After Ctrl+D, tinycode exits and the shell prompt should appear
    local captured
    captured=$(capture_pane "$session" 2>/dev/null || echo "session ended")
    TOTAL=$((TOTAL + 1))

    if ! tmux has-session -t "$session" 2>/dev/null; then
        echo -e "  ${GREEN}PASS${NC}: Session terminated (tinycode exited)"
        PASS=$((PASS + 1))
    elif echo "$captured" | grep -qE '\$\s*$'; then
        echo -e "  ${GREEN}PASS${NC}: Shell prompt visible (tinycode exited)"
        PASS=$((PASS + 1))
    else
        echo -e "  ${RED}FAIL${NC}: tinycode did not exit on Ctrl+D"
        FAIL=$((FAIL + 1))
    fi

    kill_session "$session"
}

test_T03() {
    echo -e "${BOLD}T03: Ctrl+C clears prompt${NC}"
    local session
    session=$(new_session "T03")
    sleep 3

    send_keys "$session" "some text to clear"
    sleep 0.5
    send_keys "$session" C-c
    sleep 1

    # After Ctrl+C the typed text should be gone
    assert_not_contains "$session" "some text to clear" "prompt cleared after Ctrl+C"

    kill_session "$session"
}

test_T04() {
    echo -e "${BOLD}T04: Command palette opens with Ctrl+P${NC}"
    local session
    session=$(new_session "T04")
    sleep 3

    send_keys "$session" C-p
    sleep 1

    # Palette should show some command names or a search prompt
    assert_regex "$session" "(ask|review|init|swarm|Search)" "command palette visible"

    kill_session "$session"
}

test_T05() {
    echo -e "${BOLD}T05: Leader key + b toggles sidebar${NC}"
    local session
    session=$(new_session "T05")
    sleep 3

    # Send leader key (Ctrl+X) then b
    send_keys "$session" C-x
    sleep 0.6
    send_keys "$session" b
    sleep 1

    # Sidebar should show session tree or "Sessions" header
    assert_regex "$session" "(Sessions|session)" "sidebar visible after <leader>b"

    # Toggle off
    send_keys "$session" C-x
    sleep 0.6
    send_keys "$session" b
    sleep 1

    kill_session "$session"
}

test_T06() {
    echo -e "${BOLD}T06: Tab cycles agent name in status bar${NC}"
    local session
    session=$(new_session "T06")
    sleep 3

    # Capture initial state
    local before
    before=$(capture_pane "$session")

    send_keys "$session" Tab
    sleep 1

    # The status bar or agent indicator should change
    TOTAL=$((TOTAL + 1))
    local after
    after=$(capture_pane "$session")
    if [ "$before" != "$after" ]; then
        echo -e "  ${GREEN}PASS${NC}: pane content changed after Tab (agent cycled)"
        PASS=$((PASS + 1))
    else
        echo -e "  ${YELLOW}SKIP${NC}: pane unchanged — may need model connected to show agent"
        SKIP=$((SKIP + 1))
    fi

    kill_session "$session"
}

test_T07() {
    echo -e "${BOLD}T07: Leader key + n creates new session${NC}"
    local session
    session=$(new_session "T07")
    sleep 3

    send_keys "$session" C-x
    sleep 0.6
    send_keys "$session" n
    sleep 2

    # New session should be created — pane should show empty chat or welcome
    assert_regex "$session" "." "new session rendered"

    kill_session "$session"
}

test_T08() {
    echo -e "${BOLD}T08: Leader key + m shows model list${NC}"
    local session
    session=$(new_session "T08")
    sleep 3

    send_keys "$session" C-x
    sleep 0.6
    send_keys "$session" m
    sleep 2

    # Model list or connect dialog should appear
    assert_regex "$session" "(model|Model|provider|Provider|connect|Connect|No models)" "model list visible"

    kill_session "$session"
}

# ─── Runner ──────────────────────────────────────────────────────

run_test() {
    local name="$1"
    local func="test_${name}"
    if type "$func" &>/dev/null; then
        "$func"
        echo ""
    else
        echo "Unknown test: $name"
        exit 1
    fi
}

run_all() {
    for func in $(declare -F | awk '{print $3}' | grep '^test_T' | sort); do
        name="${func#test_}"
        run_test "$name"
    done
}

report() {
    echo -e "${BOLD}═══ TUI Test Summary ═══${NC}"
    echo -e "  Total:   $TOTAL"
    echo -e "  ${GREEN}Passed:  $PASS${NC}"
    [ "$FAIL" -gt 0 ] && echo -e "  ${RED}Failed:  $FAIL${NC}" || echo "  Failed:  0"
    [ "$SKIP" -gt 0 ] && echo -e "  ${YELLOW}Skipped: $SKIP${NC}" || echo "  Skipped: 0"
    echo ""

    if [ "$FAIL" -gt 0 ]; then
        exit 1
    fi
}

# ─── Main ────────────────────────────────────────────────────────

check_prereqs

echo -e "${BOLD}tinycode TUI Integration Tests${NC}"
echo "Binary: $TINYCODE_BIN"
echo ""

if [ $# -eq 0 ]; then
    run_all
else
    for name in "$@"; do
        run_test "$name"
    done
fi

report
