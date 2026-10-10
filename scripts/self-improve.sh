#!/usr/bin/env bash
# self-improve.sh — run tinycode headlessly to pick up the next open issue,
# fix it, and open a PR. Loops until the issue backlog is exhausted.
#
# Design: https://github.com/bobbyjohnstx/tinycode/issues/35
#
# How it works
#   1. Writes a one-off config (autoApprove: true) into an isolated TINYCODE_CONFIG_DIR
#   2. Starts `tinycode serve` bound to 127.0.0.1 with token auth
#   3. For each round: pick the oldest actionable open issue, create a session,
#      prompt the agent to fix it and open a PR, poll until idle, archive.
#
# Safety: this runs an unattended agent with autoApprove. Refuses to start
# unless SI_ALLOW=1 is explicitly set.
#
# Usage
#   SI_ALLOW=1 scripts/self-improve.sh                  # up to $MAX_ITERATIONS rounds
#   SI_ALLOW=1 scripts/self-improve.sh --model ollama/qwen3
#   SI_ALLOW=1 scripts/self-improve.sh --label bug
#   SI_ALLOW=1 scripts/self-improve.sh 38               # one specific issue
#   SI_ALLOW=1 scripts/self-improve.sh --rounds 3
#   scripts/self-improve.sh --list-issues               # no server started
#
# Environment
#   SI_ALLOW=1                   required. Opt-in to the unattended agent loop.
#   TINYCODE                     binary path (default: ./dist/tinycode or PATH)
#   MAX_ITERATIONS               total rounds (default 8; --rounds wins)
#   ROUND_INACTIVITY_TIMEOUT_S   per-round no-progress budget (default 900)
#   SI_PORT                      server port (default 4096)
#   SI_LABEL_SKIP                comma-separated labels to skip
#                                (default: dup,help wanted,wontfix)
#   SI_SKIP_ISSUES               comma-separated issue numbers to skip
#                                (default: empty)

set -uo pipefail

PORT="${SI_PORT:-4096}"
D="$(pwd)"
MODEL_OVERRIDE=""
ROUNDS="${MAX_ITERATIONS:-8}"
LIST_ONLY=0
SPECIFIC_ISSUE=""
LABEL_FILTER="${SI_LABEL_FILTER:-}"
INACTIVITY_TIMEOUT="${ROUND_INACTIVITY_TIMEOUT_S:-900}"
SKIP_LABELS="${SI_LABEL_SKIP:-dup,help wanted,wontfix}"
SI_SKIP_ISSUES="${SI_SKIP_ISSUES:-}"
CFG_DIR="$(mktemp -d "${TMPDIR:-/tmp}/tinycode-si.XXXXXX")"
LOG_FILE="$CFG_DIR/serve.log"
SERVER_PID=""

# upstream repo: parent if this is a fork, else bobbyjohnstx/tinycode
SI_REPO="bobbyjohnstx/tinycode"
TARGET_REP="$(gh repo view --json parent --jq .parent.nameWithOwner 2>/dev/null)"
[[ -n "${TARGET_REP:-}" ]] || TARGET_REP="$SI_REPO"

cleanup() {
  trap - EXIT
  [[ -n "$SERVER_PID" ]] && kill "$SERVER_PID" 2>/dev/null
  wait 2>/dev/null
  rm -rf "$CFG_DIR"
}
trap cleanup EXIT

log() { printf '%s %s\n' "$(date +%H:%M:%S)" "$*"; }
fail() { log "FATAL: $*"; exit 1; }
api() { curl -sS --max-time 15 "$@"; }  # no auth — server uses TINYCODE_FORCE_NO_AUTH=1

usage() { sed -n '2,34p' "$0" | sed 's/^#\{1,2\} \{0,1\}//'; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --model) MODEL_OVERRIDE="${2:?--model needs a value}"; shift 2 ;;
    --label) LABEL_FILTER="$2"; shift ;;
    --rounds) ROUNDS="${2:?--rounds needs a number}"; shift ;;
    --list-issues) LIST_ONLY=1; shift ;;
    -h|--help) usage; exit 0 ;;
    -*) log "unknown flag: $1 (see --help)"; exit 1 ;;
    *) SPECIFIC_ISSUE="$1"; shift ;;
  esac
done

if [[ "$LIST_ONLY" == "1" ]]; then
  gh issue list -R "$TARGET_REP" --state open --limit 200 \
    --json number,title,labels \
    --jq '.[] | [(.number|tostring), (.labels|map(.name)|join(",")), .title] | @tsv' | sort -n
  exit 0
fi

# ---------- preflight ----------
TINYCODE="${TINYCODE:-}"
if [[ -z "$TINYCODE" && -x "$D/dist/tinycode" ]]; then TINYCODE="$D/dist/tinycode"
elif [[ -z "$TINYCODE" ]]; then TINYCODE="$(command -v tinycode 2>/dev/null || true)"; fi
[[ -n "$TINYCODE" && -x "$TINYCODE" ]] || fail "tinycode binary not found. Build it (make build) or set TINYCODE=/path"
command -v curl >/dev/null || fail "curl not installed"
command -v gh    >/dev/null || fail "gh not installed"
command -v jq    >/dev/null || fail "jq not installed"
gh auth status   >/dev/null 2>&1 || fail "gh not authenticated — run: gh auth login"
command -v git   >/dev/null || fail "git not installed"
git config user.email >/dev/null 2>&1 || fail "git user.email not set"

if [[ -z "${SI_ALLOW:-}" ]]; then
  echo "refusing to start: this script runs an agent unattended with autoApprove"
  echo "against issues on $TARGET_REP. Read Issue 31.3 in that repo first."
  echo "If you know what you are doing, re-run with: SI_ALLOW=1 $(basename "$0") $*"
  exit 2
fi

BASE="http://127.0.0.1:$PORT"

log "workdir       : $D"
log "upstream repo : $TARGET_REP"
log "model         : ${MODEL_OVERRIDE:-<configured default>}"
log "rounds        : $ROUNDS   (no-progress budget ${INACTIVITY_TIMEOUT}s/round)"
log "label filter  : ${LABEL_FILTER:-<any>}   skip: $SKIP_LABELS"
log "log file      : $CFG_DIR"

# ---------- server start ----------
cat > "$CFG_DIR/tinycode.json" <<'EOF'
{
  "autoApprove": true
}
EOF

MODEL_ARGS=()
[[ -n "$MODEL_OVERRIDE" ]] && MODEL_ARGS=(--model "$MODEL_OVERRIDE")

# The server reads its auth token from $DATADIR/web_token (not an env var).
# Use TINYCODE_FORCE_NO_AUTH=1 — safe here because we bind to 127.0.0.1 only.
(
  cd "$D" || exit 1
  export TINYCODE_CONFIG_DIR="$CFG_DIR"
  export TINYCODE_FORCE_NO_AUTH=1
  if [[ ${#MODEL_ARGS[@]} -gt 0 ]]; then
    exec "$TINYCODE" serve "${MODEL_ARGS[@]}" </dev/null >>"$LOG_FILE" 2>&1
  else
    exec "$TINYCODE" serve </dev/null >>"$LOG_FILE" 2>&1
  fi
) &
SERVER_PID=$!

ok=""
for i in $(seq 1 30); do
  if curl -sS --max-time 2 "$BASE/global/health" >/dev/null 2>&1; then ok=1; break; fi
  if ! kill -0 "$SERVER_PID" 2>/dev/null; then
    log "server exited early — last log lines:"; tail -5 "$LOG_FILE"; exit 1
  fi
  sleep 1
done
[[ -n "$ok" ]] || { log "server failed to become healthy on $BASE — see $LOG_FILE"; exit 1; }
log "server up: $BASE  (pid $SERVER_PID)"

# ---------- API helpers ----------
create_session() {
  local payload
  payload=$(jq -nc --arg c "self-improve #$1 round $2" '{title:$c}')
  api -X POST "$BASE/session" -H 'Content-Type: application/json' -d "$payload"
}

send_prompt() {
  local sid="$1"
  api -X POST "$BASE/session/$sid/prompt_async" -H 'Content-Type: application/json' \
    -d "$(jq -nc --arg t "$2" '{parts:[{type:"text",text:$t}]}')" >/dev/null
}

assistant_message_count() {
  api "$BASE/session/$1/message" \
    | jq '[.[] | select(.info.role == "assistant")] | length' 2>/dev/null \
    || echo 0
}

# The /session/status map lists only in-flight sessions; when the entry
# disappears the prompt has finished. Success = an assistant message exists.
wait_idle() {
  local sid="$1" last=$SECONDS
  while true; do
    if api "$BASE/session/status" 2>/dev/null | jq -e --arg s "$sid" '.[$s]' >/dev/null; then
      last=$SECONDS
      sleep 3
      continue
    fi
    if (( SECONDS - last > INACTIVITY_TIMEOUT )); then
      log "no completion for ${INACTIVITY_TIMEOUT}s — aborting round"
      api -X POST "$BASE/session/$sid/abort" >/dev/null
      return 1
    fi
    sleep 2
    if (( $(assistant_message_count "$sid") > 0 )); then
      log "session $sid → done"
      return 0
    fi
    log "session $sid ended with no assistant output (check $LOG_FILE)"
    return 1
  done
}

# ---------- main loop ----------
skip_jq=$(printf '%s' "$SKIP_LABELS" | jq -Rcs 'split(",") | map(gsub("^\\s+|\\s+$";""))')
SEEN_JSON='[]'   # issue numbers already attempted this run
SI_SKIP_JSON=$(printf '%s' "$SI_SKIP_ISSUES" | jq -Rcs 'split(",") | map(gsub("^\\s+|\\s+$";"")) | map(select(length > 0) | tonumber)')
pick_pickable() {
  gh issue list -R "$TARGET_REP" --state open --limit 200 \
    ${1:+--label "$1"} \
    --json number,title,labels \
  | jq --argjson skip "$skip_jq" --argjson seen "$SEEN_JSON" --argjson si "$SI_SKIP_JSON" '
      [ .[]
        | select(.number as $n | (($seen + $si) | index($n)) | not)
        | select((.labels | map(.name) | map(ascii_downcase)) as $l
               | (($l - $skip) | length) == ($l | length))
      ] | sort_by(.number) | first'
}
for round in $(seq 1 "$ROUNDS"); do
  log "══════════ round $round / $ROUNDS ══════════"

  if [[ -n "$SPECIFIC_ISSUE" ]]; then
    picked=$(gh issue view "$SPECIFIC_ISSUE" -R "$TARGET_REP" --json number,title --jq '{number, title}')
  else
    if [[ -n "$LABEL_FILTER" ]]; then
      picked=$(pick_pickable "$LABEL_FILTER")
    else
      picked=$(pick_pickable)
    fi
  fi

  if [[ -z "$picked" || "$picked" == "null" ]]; then
    log "no actionable issues left — done."
    break
  fi

  num=$(echo "$picked" | jq -r .number)
  title=$(echo "$picked" | jq -r '.title // ""')
  log "round $round: issue #$num — $title"
  # Remember this issue so the next round (even after a failed or
  # "won't fix" outcome) moves to the next one instead of re-picking it.
  SEEN_JSON=$(jq -cn --argjson s "$SEEN_JSON" --argjson n "$num" '$s + [$n')

  created=$(create_session "$num" "$round")
  sid=$(echo "$created" | jq -r '.id // empty' 2>/dev/null)
  if [[ -z "$sid" ]]; then
    log "session create failed: $created"
  else
    read -r -d '' PROMPT <<PROMPT || true
You are running unattended in a headless tinycode session, fixing issue #$num in $TARGET_REP.
Working directory: $D

Issue: #$num — $title

Follow this sequence exactly, using your tools (bash, read, write, edit, task, skill).

1. Read the issue fully:
   gh issue view $num -R $TARGET_REP --json title,body,labels,comments

2. Judge actionability. If the issue is:
   - already fixed (check recent commits / blame),
   - an upstream model or provider bug outside this codebase,
   - a feature request with no clear spec, or needs design discussion,
   then explain why in a comment and STOP:
   gh issue comment $num -R $TARGET_REP --body '<a short, evidence-backed reason>'
   (Do not branch, commit, or open a PR.)

3. Otherwise implement the fix:
   - Read the relevant source first. This is a Go project (bubbletea TUI,
     pure-Go SQLite, lipgloss). Match surrounding style.
   - Keep the change minimal and targeted. No new abstractions for one-off logic.
   - For multi-file or architectural work, spawn a subagent (architect for
     design, executor for implementation).
   - Do not touch packages/ — that is legacy TypeScript, not the Go runtime.
   - Never disable auth checks or weaken tests just to make them pass.

4. Verify before committing:
   go build ./...
   go vet ./...
   go test ./... -count=1
   Fix anything you broke before committing.

5. Commit and open a PR (branch from main, not canary):
   git checkout -b fix-$num-<slug>
   git add <specific files>
   git commit -m 'fix(scope): <imperative summary>'
   git push -u origin fix-$num-<slug>
   gh pr create -R $TARGET_REP --base main --head origin:fix-$num-<slug> \\
     --title 'fix(scope): <same as commit>' \\
     --body "Fixes #$num.\\n\\n<what was wrong, what changed, why minimal. Commands run.>"
   gh issue comment $num -R $TARGET_REP --body "Fix in #<PR-number>. <one line on what changed>."

6. If gh pr create fails for external reasons (permissions, branch protection),
   post the exact error as an issue comment and STOP.

If you get stuck, the acceptable outcome is an honest issue comment — a commented
issue beats a broken PR. Do not silently give up.
PROMPT

    send_prompt "$sid" "$PROMPT"
    log "prompt sent"

    if wait_idle "$sid"; then
      pr=$(gh pr list -R "$TARGET_REP" --state open --limit 10 \
        --json number,title --jq --arg n "#$num" \
        '.[] | select(.title | ascii_downcase | contains($n | ascii_downcase))
         | "\(#): \(.title)"' | head -1)
      log "round $round result: ${pr:-<no matching PR — check issue comment>}"
    else
      log "round $round gave up — details in $LOG_FILE and the session record"
    fi
    api -X POST "$BASE/session/$sid/archive" >/dev/null
  fi

  if [[ -n "$SPECIFIC_ISSUE" ]]; then
    log "single-issue mode — stopping after round $round"
    break
  fi
done

log "self-improve run finished."
exit 0
