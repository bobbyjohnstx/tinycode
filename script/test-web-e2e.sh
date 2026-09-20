#!/usr/bin/env bash
#
# Playwright-based E2E tests for the tinycode web UI.
#
# These tests exercise the full browser → Go server → LLM → SSE → browser
# round trip using headless Chromium. They require a running tinycode web
# server and a configured LLM provider.
#
# Prerequisites:
#   - Node.js 18+
#   - npx playwright install chromium       (one-time browser download)
#   - tinycode web server running           (./dist/tinycode web)
#   - At least one LLM provider configured  (ollama, openrouter, etc.)
#
# Usage:
#   ./script/test-web-e2e.sh                        # run all tests
#   ./script/test-web-e2e.sh test-hello-roundtrip   # run specific test file
#   ./script/test-web-e2e.sh --list                 # list test files
#
# Environment:
#   TINYCODE_WEB_TOKEN       Auth token (auto-reads from data dir if unset)
#   TINYCODE_WEB_PORT        Server port (default: 4096)
#   TINYCODE_WEB_HOST        Server host (default: 127.0.0.1)
#   E2E_PROJECT_DIR          Directory to use for test sessions (default: cwd)
#   E2E_SCREENSHOT_DIR       Where to save screenshots (default: /tmp)
#   E2E_FILTER               Only run tests whose name contains this string
#
# How to add a new test:
#   1. Create script/e2e/test-<name>.mjs
#   2. Import helpers from ./helpers.mjs
#   3. Use runTest() for each assertion, printSummary() at the end
#   4. See test-hello-roundtrip.mjs for the pattern
#
# Key patterns for writing tests:
#   - Create sessions via API first (createSession), then open in browser.
#     This avoids the project picker and localStorage setup complexity.
#   - Use waitUntil: 'domcontentloaded' (NOT 'networkidle') because the
#     SSE event stream keeps the connection open indefinitely.
#   - The SPA uses contenteditable divs, not <textarea>. Use findInput()
#     which checks both.
#   - Auth is passed via ?auth_token=<base64(tinycode:token)> URL param.
#     The SPA extracts it, stores in localStorage, and strips from URL.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
E2E_DIR="$SCRIPT_DIR/e2e"

# Colors
if [ -t 1 ]; then
    BOLD='\033[1m' GREEN='\033[0;32m' RED='\033[0;31m' NC='\033[0m'
else
    BOLD='' GREEN='' RED='' NC=''
fi

# --- Prerequisites ---

check_prereqs() {
    local ok=true

    if ! command -v node &>/dev/null; then
        echo "Error: Node.js is required (brew install node)" >&2
        ok=false
    fi

    if ! node -e "require('playwright')" 2>/dev/null; then
        echo "Error: playwright not installed." >&2
        echo "  Run: npm install -g playwright && npx playwright install chromium" >&2
        ok=false
    fi

    local host="${TINYCODE_WEB_HOST:-127.0.0.1}"
    local port="${TINYCODE_WEB_PORT:-4096}"
    if ! curl -sf "http://${host}:${port}/health" >/dev/null 2>&1; then
        if ! curl -sf "http://${host}:${port}/" >/dev/null 2>&1; then
            echo "Warning: tinycode web server not detected at ${host}:${port}" >&2
            echo "  Start it with: ./dist/tinycode web" >&2
        fi
    fi

    if [ "$ok" = false ]; then exit 1; fi
}

# --- Main ---

main() {
    echo -e "${BOLD}tinycode web UI E2E tests (Playwright)${NC}"
    echo "======================================"

    if [ "${1:-}" = "--list" ]; then
        for f in "$E2E_DIR"/test-*.mjs; do
            [ -f "$f" ] && echo "  $(basename "$f" .mjs)"
        done
        exit 0
    fi

    check_prereqs

    local filter="${1:-}"
    local exit_code=0

    if [ -n "$filter" ]; then
        local test_file="$E2E_DIR/test-${filter}.mjs"
        if [ ! -f "$test_file" ]; then
            test_file="$E2E_DIR/${filter}.mjs"
        fi
        if [ ! -f "$test_file" ]; then
            echo "Error: test file not found: $filter" >&2
            echo "  Available: $(ls "$E2E_DIR"/test-*.mjs 2>/dev/null | xargs -I{} basename {} .mjs | tr '\n' ' ')" >&2
            exit 1
        fi
        echo "  Running: $(basename "$test_file")"
        echo ""
        node "$test_file" || exit_code=$?
    else
        for test_file in "$E2E_DIR"/test-*.mjs; do
            [ -f "$test_file" ] || continue
            echo ""
            echo "  Running: $(basename "$test_file")"
            echo ""
            node "$test_file" || exit_code=$?
        done
    fi

    exit $exit_code
}

main "$@"
