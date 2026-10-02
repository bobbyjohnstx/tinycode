#!/usr/bin/env bash
#
# Side-by-side TUI comparison: TS tinycode (left) vs Go tinycode (right).
#
# Usage:
#   ./script/tui-compare.sh                    # launch both, interactive
#   ./script/tui-compare.sh capture            # launch, wait, capture pane text
#   ./script/tui-compare.sh send "hello world" # send keystrokes to both panes
#
# Requires: tmux
#
# The session name is "tui-compare". Attach manually with:
#   tmux attach -t tui-compare

set -euo pipefail

SESSION="tui-compare"
TS_BIN="/Users/bjohns/projects/tinycode/packages/tinycode/dist/tinycode-darwin-arm64/bin/tinycode"
GO_BIN="/Users/bjohns/projects/tinycode/dist/tinycode"
WORK_DIR="/tmp/tui-compare-workdir"
CAPTURE_DIR="/tmp/tui-compare-captures"

mkdir -p "$WORK_DIR" "$CAPTURE_DIR"

cmd_launch() {
    if tmux has-session -t "$SESSION" 2>/dev/null; then
        echo "Session '$SESSION' already exists. Kill it first:"
        echo "  tmux kill-session -t $SESSION"
        exit 1
    fi

    # Create session with TS tinycode on the left
    tmux new-session -d -s "$SESSION" -x 200 -y 45
    tmux send-keys -t "$SESSION" "cd $WORK_DIR && $TS_BIN" Enter

    # Split right pane for Go tinycode
    tmux split-window -h -t "$SESSION"
    tmux send-keys -t "$SESSION" "cd $WORK_DIR && $GO_BIN" Enter

    # Label panes
    tmux select-pane -t "$SESSION:0.0" -T "TS tinycode"
    tmux select-pane -t "$SESSION:0.1" -T "Go tinycode"

    # Equal split
    tmux select-layout -t "$SESSION" even-horizontal

    echo "Launched tmux session: $SESSION"
    echo "  Left pane:  TS tinycode"
    echo "  Right pane: Go tinycode"
    echo ""
    echo "Commands:"
    echo "  tmux attach -t $SESSION              # view side-by-side"
    echo "  ./script/tui-compare.sh send 'text'  # send input to both"
    echo "  ./script/tui-compare.sh capture       # save pane contents"
    echo "  ./script/tui-compare.sh diff          # capture + diff"
    echo "  ./script/tui-compare.sh kill          # tear down"
}

cmd_send() {
    local input="${1:-}"
    if [ -z "$input" ]; then
        echo "Usage: $0 send 'text to send'"
        exit 1
    fi
    tmux send-keys -t "$SESSION:0.0" "$input" Enter
    tmux send-keys -t "$SESSION:0.1" "$input" Enter
    echo "Sent to both panes: $input"
}

cmd_sendkeys() {
    local keys="${1:-}"
    if [ -z "$keys" ]; then
        echo "Usage: $0 sendkeys 'C-c' (tmux key name)"
        exit 1
    fi
    tmux send-keys -t "$SESSION:0.0" "$keys"
    tmux send-keys -t "$SESSION:0.1" "$keys"
    echo "Sent key to both panes: $keys"
}

cmd_capture() {
    local ts="$CAPTURE_DIR/ts-$(date +%s).txt"
    local go="$CAPTURE_DIR/go-$(date +%s).txt"

    tmux capture-pane -t "$SESSION:0.0" -p > "$ts"
    tmux capture-pane -t "$SESSION:0.1" -p > "$go"

    echo "Captured:"
    echo "  TS: $ts"
    echo "  Go: $go"
}

cmd_diff() {
    local ts="$CAPTURE_DIR/ts-latest.txt"
    local go="$CAPTURE_DIR/go-latest.txt"

    tmux capture-pane -t "$SESSION:0.0" -p > "$ts"
    tmux capture-pane -t "$SESSION:0.1" -p > "$go"

    echo "=== TS tinycode ==="
    cat "$ts"
    echo ""
    echo "=== Go tinycode ==="
    cat "$go"
    echo ""
    echo "=== Differences ==="
    diff --color=auto -u "$ts" "$go" || true
}

cmd_kill() {
    tmux kill-session -t "$SESSION" 2>/dev/null && echo "Killed session: $SESSION" || echo "No session to kill"
}

case "${1:-launch}" in
    launch)   cmd_launch ;;
    send)     cmd_send "${2:-}" ;;
    sendkeys) cmd_sendkeys "${2:-}" ;;
    capture)  cmd_capture ;;
    diff)     cmd_diff ;;
    kill)     cmd_kill ;;
    *)
        echo "Usage: $0 {launch|send|sendkeys|capture|diff|kill}"
        exit 1
        ;;
esac
