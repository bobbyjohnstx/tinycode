#!/bin/sh
# test-plugins-container.sh — Disposable podman container for clean-room plugin testing.
#
# Builds linux/amd64 binaries locally, copies them into a disposable UBI9
# container, then verifies each plugin responds to the JSON-RPC initialize
# handshake. Tears down on exit.
#
# Usage:
#   ./script/test-plugins-container.sh              # test all plugins
#   ./script/test-plugins-container.sh ocp-oauth     # test one plugin
#   KEEP=1 ./script/test-plugins-container.sh       # keep container after run
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CONTAINER_NAME="tinycode-plugin-test-$$"
IMAGE="registry.access.redhat.com/ubi9/ubi:latest"
FILTER="${1:-}"

cleanup() {
  rm -rf "$BUILD_DIR"
  if [ "${KEEP:-}" = "1" ]; then
    echo ""
    echo "Container kept: $CONTAINER_NAME"
    echo "  $RUNTIME exec -it $CONTAINER_NAME /bin/bash"
    echo "  $RUNTIME rm -f $CONTAINER_NAME"
  else
    $RUNTIME rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
  fi
}

detect_runtime() {
  if command -v podman >/dev/null 2>&1; then
    echo "podman"
  elif command -v docker >/dev/null 2>&1; then
    echo "docker"
  else
    echo ""
  fi
}

RUNTIME=$(detect_runtime)
if [ -z "$RUNTIME" ]; then
  echo "Error: Neither podman nor docker found."
  exit 1
fi

# Discover plugin dirs
PLUGIN_NAMES=$(ls -d "$PROJECT_DIR"/cmd/plugin-* 2>/dev/null | xargs -I{} basename {})
if [ -n "$FILTER" ]; then
  PLUGIN_NAMES=$(echo "$PLUGIN_NAMES" | grep "plugin-${FILTER}$" || true)
  if [ -z "$PLUGIN_NAMES" ]; then
    echo "Error: No plugin matching '$FILTER'"
    exit 1
  fi
fi

PLUGIN_COUNT=$(echo "$PLUGIN_NAMES" | wc -l | tr -d ' ')

echo "==> Runtime: $RUNTIME"
echo "==> Plugins: $PLUGIN_COUNT"
echo ""

# Build locally for linux/amd64
# Use project-local dir — macOS /tmp isn't shared with the podman VM
BUILD_DIR="$PROJECT_DIR/.plugin-test-build"
rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"
trap cleanup EXIT

echo "==> Cross-compiling for linux/amd64..."
export GOOS=linux GOARCH=amd64 CGO_ENABLED=0

# Build main binary
printf "  %-45s" "tinycode"
if go build -ldflags "-s -w" -o "$BUILD_DIR/tinycode" "$PROJECT_DIR/cmd/tinycode" 2>/dev/null; then
  echo "ok"
else
  echo "FAIL"
  echo "Error: Main binary failed to build."
  exit 1
fi

# Build plugins
BUILT=0
BUILD_FAIL=0
BUILD_ERRORS=""
for plugin in $PLUGIN_NAMES; do
  printf "  %-45s" "$plugin"
  if go build -ldflags "-s -w" -o "$BUILD_DIR/$plugin" "$PROJECT_DIR/cmd/$plugin" 2>&1; then
    echo "ok"
    BUILT=$((BUILT + 1))
  else
    echo "FAIL"
    BUILD_FAIL=$((BUILD_FAIL + 1))
    BUILD_ERRORS="$BUILD_ERRORS\n  $plugin"
  fi
done

unset GOOS GOARCH CGO_ENABLED

echo ""
echo "==> Build: $BUILT ok, $BUILD_FAIL failed out of $PLUGIN_COUNT"

if [ "$BUILD_FAIL" -gt 0 ]; then
  echo "  Failed:$BUILD_ERRORS"
fi
echo ""

# Start disposable container
echo "==> Starting disposable UBI9 container..."
$RUNTIME run -d \
  --name "$CONTAINER_NAME" \
  -v "$BUILD_DIR:/binaries:z,ro" \
  "$IMAGE" \
  sleep infinity

run_in() {
  $RUNTIME exec "$CONTAINER_NAME" sh -c "$1"
}

# Copy binaries into container PATH
echo "==> Installing binaries into container..."
run_in "cp /binaries/* /usr/local/bin/ && chmod +x /usr/local/bin/tinycode /usr/local/bin/plugin-*"

# Install oc CLI (required by OpenShift plugins)
echo "==> Installing oc CLI..."
run_in "
  ARCH=\$(uname -m)
  case \$ARCH in
    x86_64)  OC_SUFFIX=linux ;;
    aarch64) OC_SUFFIX=linux-arm64 ;;
    *)       OC_SUFFIX=linux ;;
  esac
  OC_URL=\"https://mirror.openshift.com/pub/openshift-v4/clients/ocp/stable/openshift-client-\${OC_SUFFIX}.tar.gz\"
  curl -fsSL \"\$OC_URL\" | tar -C /usr/local/bin -xz oc kubectl 2>/dev/null && \
    echo \"  oc \$(oc version --client 2>/dev/null | head -1)\" || \
    echo '  WARNING: oc install failed (plugins needing oc will still pass handshake)'
"

# Test each plugin with JSON-RPC initialize handshake
echo "==> Testing plugin handshake..."
echo ""
PASS=0
FAIL=0
ERRORS=""

for plugin in $PLUGIN_NAMES; do
  printf "  %-45s" "$plugin"

  if ! run_in "test -f /usr/local/bin/$plugin" 2>/dev/null; then
    echo "SKIP (no binary)"
    continue
  fi

  RESULT=$(run_in "
    echo '{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{}}' | \
      timeout 10 /usr/local/bin/$plugin 2>/dev/null || true
  " 2>/dev/null)

  if echo "$RESULT" | grep -q '"jsonrpc"' && echo "$RESULT" | grep -q '"result"'; then
    HAS_ID=$(echo "$RESULT" | grep -c '"id"' || true)
    if [ "$HAS_ID" -gt 0 ]; then
      echo "ok"
      PASS=$((PASS + 1))
    else
      echo "FAIL (bad response)"
      FAIL=$((FAIL + 1))
      ERRORS="$ERRORS\n  $plugin: missing id in response"
    fi
  else
    echo "FAIL (no response)"
    FAIL=$((FAIL + 1))
    ERRORS="$ERRORS\n  $plugin: $(echo "$RESULT" | head -1)"
  fi
done

echo ""
echo "========================================"
echo "  Plugin Test Results"
echo "========================================"
echo "  Built:  $BUILT / $PLUGIN_COUNT"
echo "  Passed: $PASS / $BUILT"
echo "  Failed: $FAIL"
if [ -n "$ERRORS" ]; then
  echo ""
  echo "  Errors:"
  printf "$ERRORS\n"
fi
echo "========================================"
echo ""

if [ "$BUILD_FAIL" -gt 0 ] || [ "$FAIL" -gt 0 ]; then
  exit 1
fi

echo "All plugins built and responded to initialize handshake."
