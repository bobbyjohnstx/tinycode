#!/usr/bin/env bash
#
# Pre-release fresh-env smoke: new-user install path for plugins + LSP.
#
# Uses podman + a clean HOME, mounts locally built tinycode/plugins, drives
# doctor / plugin install / headless tool calls (mock LLM) / optional TUI tmux.
# Machine-readable results land in .fresh-env-results/ for a refinement loop.
#
# Prerequisites:
#   - podman
#   - go (cross-compiles linux binaries into dist/fresh-env/; also runs mock-llm)
#
# Usage:
#   ./script/test-fresh-env.sh
#   ./script/test-fresh-env.sh --skip-tui
#   ./script/test-fresh-env.sh --skip-build   # reuse existing dist/fresh-env/* if present
#
# Environment:
#   FRESH_ENV_IMAGE   Image name (default: tinycode-fresh-env)
#   FRESH_ENV_PLUGIN  Plugin to install (default: safety-net)

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

IMAGE="${FRESH_ENV_IMAGE:-tinycode-fresh-env}"
PLUGIN="${FRESH_ENV_PLUGIN:-safety-net}"
RESULTS_DIR="${ROOT}/.fresh-env-results"
SKIP_BUILD=0
SKIP_TUI=0

for arg in "$@"; do
  case "$arg" in
    --skip-build) SKIP_BUILD=1 ;;
    --skip-tui) SKIP_TUI=1 ;;
    -h|--help)
      sed -n '2,25p' "$0"
      exit 0
      ;;
  esac
done

if ! command -v podman >/dev/null 2>&1; then
  echo "podman not found — install podman to run fresh-env smoke" >&2
  exit 1
fi

mkdir -p "$RESULTS_DIR"
SUMMARY_JSON="$RESULTS_DIR/summary.json"
: >"$RESULTS_DIR/log.txt"
PASS=0
FAIL=0
RESULTS=()

log() { echo "$*" | tee -a "$RESULTS_DIR/log.txt"; }
record() {
  local status="$1" id="$2" detail="${3:-}"
  RESULTS+=("{\"id\":$(printf '%s' "$id" | jq -Rs .),\"status\":$(printf '%s' "$status" | jq -Rs .),\"detail\":$(printf '%s' "$detail" | jq -Rs .)}")
  if [ "$status" = "pass" ]; then
    PASS=$((PASS + 1))
    log "  PASS: $id"
  else
    FAIL=$((FAIL + 1))
    log "  FAIL: $id — $detail"
  fi
}

# Container runs Linux; host may be darwin — always cross-compile for the
# podman engine arch so mounted binaries are ELF, not Mach-O.
TARGET_OS=linux
TARGET_ARCH="$(podman info --format '{{.Host.Arch}}' 2>/dev/null || true)"
case "$TARGET_ARCH" in
  amd64|arm64) ;;
  x86_64) TARGET_ARCH=amd64 ;;
  aarch64|arm) TARGET_ARCH=arm64 ;;
  *)
    case "$(uname -m)" in
      x86_64|amd64) TARGET_ARCH=amd64 ;;
      *) TARGET_ARCH=arm64 ;;
    esac
    log "    warn: could not read podman arch; assuming $TARGET_ARCH"
    ;;
esac

FRESH_DIST="$ROOT/dist/fresh-env"
mkdir -p "$FRESH_DIST"
TINYCODE_BIN="$FRESH_DIST/tinycode-linux-${TARGET_ARCH}"
PLUGIN_SRC="cmd/plugin-${PLUGIN}"
PLUGIN_BIN="$FRESH_DIST/plugin-${PLUGIN}-linux-${TARGET_ARCH}"

if [ ! -d "$ROOT/$PLUGIN_SRC" ]; then
  echo "missing $PLUGIN_SRC — unknown plugin $PLUGIN" >&2
  exit 1
fi

# --- host build (linux binaries for the container) ---
if [ "$SKIP_BUILD" -eq 0 ] || [ ! -x "$TINYCODE_BIN" ] || [ ! -x "$PLUGIN_BIN" ]; then
  log "==> Cross-compiling tinycode + $PLUGIN for ${TARGET_OS}/${TARGET_ARCH}"
  CGO_ENABLED=0 GOOS="$TARGET_OS" GOARCH="$TARGET_ARCH" \
    go build -ldflags "-s -w" -o "$TINYCODE_BIN" ./cmd/tinycode
  CGO_ENABLED=0 GOOS="$TARGET_OS" GOARCH="$TARGET_ARCH" \
    go build -ldflags "-s -w" -o "$PLUGIN_BIN" "./$PLUGIN_SRC"
fi

if [ ! -x "$TINYCODE_BIN" ]; then
  echo "missing $TINYCODE_BIN — re-run without --skip-build" >&2
  exit 1
fi
if [ ! -x "$PLUGIN_BIN" ]; then
  echo "missing $PLUGIN_BIN — re-run without --skip-build" >&2
  exit 1
fi
# Sanity: refuse to mount a non-ELF host binary into Linux
if command -v file >/dev/null 2>&1; then
  if ! file "$TINYCODE_BIN" | grep -qiE 'ELF|executable'; then
    echo "refusing to mount non-Linux binary: $(file "$TINYCODE_BIN")" >&2
    exit 1
  fi
fi

log "==> Building fresh-env image ($IMAGE)"
podman build -f Containerfile.fresh-env -t "$IMAGE" . >>"$RESULTS_DIR/log.txt" 2>&1

# --- mock LLM on host (reachable via host.containers.internal / host gateway) ---
SCENARIO="$RESULTS_DIR/scenario.json"
cat >"$SCENARIO" <<EOF
[
  {
    "tool_calls": [
      {
        "name": "lsp_diagnostics",
        "arguments": "{\"file_path\":\"/work/project/hello.go\"}"
      }
    ]
  },
  {
    "content": "LSP diagnostics looked fine for the smoke test."
  }
]
EOF

MOCK_LOG="$RESULTS_DIR/mock-llm.log"
MOCK_PID=""
cleanup() {
  if [ -n "$MOCK_PID" ] && kill -0 "$MOCK_PID" 2>/dev/null; then
    kill "$MOCK_PID" 2>/dev/null || true
    wait "$MOCK_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

log "==> Starting mock LLM (0.0.0.0 so podman host-gateway can reach it)"
(
  cd "$ROOT"
  # Bind all interfaces — 127.0.0.1 is unreachable from the container VM/network ns.
  go run ./script/mock-llm -addr 0.0.0.0:18080 -script "$SCENARIO"
) >"$MOCK_LOG" 2>&1 &
MOCK_PID=$!
sleep 0.5
if ! kill -0 "$MOCK_PID" 2>/dev/null; then
  record fail "mock-llm-start" "mock LLM exited early; see $MOCK_LOG"
  exit 1
fi
# Wait for listen line
for _ in $(seq 1 20); do
  if grep -q 'MOCK_LLM_URL=' "$MOCK_LOG" 2>/dev/null; then
    break
  fi
  sleep 0.2
done
MOCK_URL=$(grep -oE 'MOCK_LLM_URL=http://[^[:space:]]+' "$MOCK_LOG" | head -1 | cut -d= -f2-)
if [ -z "$MOCK_URL" ]; then
  MOCK_URL="http://0.0.0.0:18080"
fi
log "    mock at $MOCK_URL"

# Host gateway for containers (podman)
HOST_GW="${FRESH_ENV_HOST_GW:-host.containers.internal}"
MOCK_IN_CONTAINER="http://${HOST_GW}:18080"

# --- run inside clean container ---
# Use bash -c (not -lc): login shells source /etc/profile and overwrite PATH,
# which drops /go/bin where the image installs gopls — lsp_* then fails silently.
CONTAINER_PATH="/go/bin:/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin"
run_in_container() {
  podman run --rm \
    --add-host=host.containers.internal:host-gateway \
    -v "$TINYCODE_BIN:/usr/local/bin/tinycode:ro" \
    -v "$PLUGIN_BIN:/opt/plugins/$PLUGIN:ro" \
    -v "$RESULTS_DIR:/results:rw" \
    -e HOME=/home/newuser \
    -e TINYCODE_CONFIG_DIR=/home/newuser/.config/tinycode \
    -e TINYCODE_DATA_DIR=/home/newuser/.local/share/tinycode \
    -e TINYCODE_DB=/home/newuser/.local/share/tinycode/tinycode.db \
    -e TINYCODE_DISABLE_MOUSE=1 \
    -e TINYCODE_VLLM_HOST="$MOCK_IN_CONTAINER" \
    -e OLLAMA_HOST=http://127.0.0.1:1 \
    -e PATH="$CONTAINER_PATH" \
    "$IMAGE" \
    bash -c "export PATH=\"$CONTAINER_PATH\"; $*"
}

log "==> TC1: doctor on empty home (new-user tips)"
DOC1=$(run_in_container '
  mkdir -p "$HOME" /work/project
  cd /work/project
  tinycode doctor 2>&1
' || true)
echo "$DOC1" >"$RESULTS_DIR/doctor-empty.txt"
if echo "$DOC1" | grep -q 'plugin install'; then
  record pass "doctor-empty-plugin-tip"
else
  record fail "doctor-empty-plugin-tip" "doctor did not tip plugin install"
fi

log "==> TC2: plugin install --from (local pre-release binary)"
INSTALL_OUT=$(run_in_container '
  mkdir -p "$HOME" "$TINYCODE_CONFIG_DIR/plugins" "$TINYCODE_DATA_DIR" /work/project
  tinycode plugin install '"$PLUGIN"' --from /opt/plugins/'"$PLUGIN"' 2>&1
  test -x "$TINYCODE_CONFIG_DIR/plugins/'"$PLUGIN"'" && echo INSTALL_OK
' || true)
echo "$INSTALL_OUT" >"$RESULTS_DIR/plugin-install.txt"
if echo "$INSTALL_OUT" | grep -q INSTALL_OK; then
  record pass "plugin-install-from"
else
  record fail "plugin-install-from" "$INSTALL_OUT"
fi

log "==> TC3: enable plugin + doctor resolvable"
DOC2=$(run_in_container '
  mkdir -p "$HOME" "$TINYCODE_CONFIG_DIR/plugins" "$TINYCODE_DATA_DIR" /work/project
  tinycode plugin install '"$PLUGIN"' --from /opt/plugins/'"$PLUGIN"' >/dev/null
  cat > "$TINYCODE_CONFIG_DIR/config.json" <<JSON
{
  "disabled_providers": ["ollama", "openrouter", "lmstudio", "maas"],
  "plugins": ["'"$PLUGIN"'"],
  "model": "vllm/test-model",
  "provider": {
    "vllm": {
      "options": { "baseURL": "'"$MOCK_IN_CONTAINER"'/v1" }
    }
  }
}
JSON
  cd /work/project
  # Go fixture so gopls is relevant
  printf "module example.com/smoke\n\ngo 1.22\n" > go.mod
  printf "package main\n\nfunc main() {\n\tx := 1\n\t_ = x\n}\n" > hello.go
  tinycode doctor 2>&1
' || true)
echo "$DOC2" >"$RESULTS_DIR/doctor-enabled.txt"
if echo "$DOC2" | grep -qE '1 configured, 1 installed|Plugins: 1 configured, 1 installed'; then
  record pass "doctor-plugin-resolvable"
else
  record fail "doctor-plugin-resolvable" "expected installed summary"
fi
# Require gopls found (not merely mentioned in an install tip).
if echo "$DOC2" | grep -qE 'Dev tool: gopls \(Go LSP\)$' \
  && ! echo "$DOC2" | grep -q 'Dev tool: gopls (Go LSP) -- not found'; then
  record pass "doctor-gopls-relevant"
else
  record fail "doctor-gopls-relevant" "expected gopls ready with go.mod (check PATH includes /go/bin)"
fi

log "==> TC4: headless run forces lsp_diagnostics (mock LLM) — silent failure detector"
RUN_OUT=$(run_in_container '
  mkdir -p "$HOME" "$TINYCODE_CONFIG_DIR/plugins" "$TINYCODE_DATA_DIR" /work/project
  tinycode plugin install '"$PLUGIN"' --from /opt/plugins/'"$PLUGIN"' >/dev/null
  cat > "$TINYCODE_CONFIG_DIR/config.json" <<JSON
{
  "disabled_providers": ["ollama", "openrouter", "lmstudio", "maas"],
  "plugins": ["'"$PLUGIN"'"],
  "model": "vllm/test-model",
  "provider": {
    "vllm": {
      "options": { "baseURL": "'"$MOCK_IN_CONTAINER"'/v1" }
    }
  }
}
JSON
  cd /work/project
  printf "module example.com/smoke\n\ngo 1.22\n" > go.mod
  printf "package main\n\nfunc main() {\n\tx := 1\n\t_ = x\n}\n" > hello.go
  tinycode run --format json --dangerously-skip-permissions \
    -m vllm/test-model \
    "Please check diagnostics on hello.go" 2>/results/run.stderr | tee /results/run.ndjson
' || true)

if [ ! -f "$RESULTS_DIR/run.ndjson" ] || [ ! -s "$RESULTS_DIR/run.ndjson" ]; then
  record fail "headless-lsp-tool-path" "no run.ndjson or empty; see run.stderr + mock-llm.log"
elif grep -qE '"toolName":"lsp_diagnostics"' "$RESULTS_DIR/run.ndjson" 2>/dev/null; then
  # Tool path exercised — also fail if gopls was invisible (the silent failure we care about).
  if grep -q 'No LSP server available' "$RESULTS_DIR/run.ndjson" 2>/dev/null; then
    record fail "headless-lsp-tool-path" "lsp_diagnostics ran but no server (gopls not on PATH)"
  else
    record pass "headless-lsp-tool-path"
  fi
else
  detail="NDJSON lacked lsp_diagnostics tool events"
  if grep -q 'step_finish' "$RESULTS_DIR/run.ndjson" 2>/dev/null; then
    detail="$detail (got step_finish but no tool — mock may not be streaming SSE)"
  fi
  record fail "headless-lsp-tool-path" "$detail"
fi

if [ "$SKIP_TUI" -eq 0 ]; then
  log "==> TC5: TUI smoke (tmux) — welcome + typed prompt"
  TUI_OUT=$(run_in_container '
    mkdir -p "$HOME" "$TINYCODE_CONFIG_DIR" "$TINYCODE_DATA_DIR" /work/project
    cat > "$TINYCODE_CONFIG_DIR/config.json" <<JSON
{
  "disabled_providers": ["ollama", "openrouter", "lmstudio", "maas"],
  "model": "vllm/test-model",
  "provider": {
    "vllm": {
      "options": { "baseURL": "'"$MOCK_IN_CONTAINER"'/v1" }
    }
  }
}
JSON
    cd /work/project
    printf "module example.com/smoke\n\ngo 1.22\n" > go.mod
    SESSION=fresh-tui
    tmux new-session -d -s "$SESSION" -x 120 -y 40 \
      "TINYCODE_DISABLE_MOUSE=1 tinycode 2>/results/tui.stderr"
    sleep 2
    tmux capture-pane -t "$SESSION" -p > /results/tui-pane1.txt || true
    tmux send-keys -t "$SESSION" "hello from fresh-env" Enter
    sleep 3
    tmux capture-pane -t "$SESSION" -p > /results/tui-pane2.txt || true
    tmux kill-session -t "$SESSION" 2>/dev/null || true
    if grep -qE "hello from fresh-env|tinycode|Model|Agent" /results/tui-pane1.txt /results/tui-pane2.txt 2>/dev/null; then
      echo TUI_OK
    else
      echo TUI_FAIL
      cat /results/tui-pane1.txt /results/tui-pane2.txt 2>/dev/null | tail -40
    fi
  ' || true)
  echo "$TUI_OUT" >"$RESULTS_DIR/tui.txt"
  if echo "$TUI_OUT" | grep -q TUI_OK; then
    record pass "tui-welcome-and-type"
  else
    record fail "tui-welcome-and-type" "pane did not show expected UI/text"
  fi
else
  log "==> TC5: TUI skipped (--skip-tui)"
fi

# --- write summary ---
{
  echo "{"
  echo "  \"pass\": $PASS,"
  echo "  \"fail\": $FAIL,"
  echo "  \"plugin\": $(printf '%s' "$PLUGIN" | jq -Rs .),"
  echo "  \"results\": ["
  if [ ${#RESULTS[@]} -gt 0 ]; then
    printf '    %s\n' "${RESULTS[0]}"
    for ((i = 1; i < ${#RESULTS[@]}; i++)); do
      printf '    ,%s\n' "${RESULTS[$i]}"
    done
  fi
  echo "  ]"
  echo "}"
} >"$SUMMARY_JSON"

log ""
log "==> Summary: $PASS pass, $FAIL fail"
log "    results: $SUMMARY_JSON"
log "    logs:    $RESULTS_DIR/"

if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
exit 0
