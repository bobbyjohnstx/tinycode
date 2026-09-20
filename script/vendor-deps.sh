#!/usr/bin/env sh
# Install build dependencies for the esbuild-based SPA build.
# Run once after cloning; re-run if script/build-deps/package.json changes.
#
# Usage: ./script/vendor-deps.sh
#
# Prerequisites: node and npm installed
# After running: `node script/build-webapp.mjs` works without bun

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DEPS_DIR="$SCRIPT_DIR/build-deps"

if [ ! -f "$DEPS_DIR/package.json" ]; then
  echo "Error: $DEPS_DIR/package.json not found"
  exit 1
fi

echo "==> Installing build dependencies..."
npm install --prefix "$DEPS_DIR" --no-audit --no-fund

TOTAL=$(du -sh "$DEPS_DIR/node_modules" | cut -f1)
COUNT=$(ls "$DEPS_DIR/node_modules" | wc -l | tr -d ' ')
echo "==> Installed $COUNT packages ($TOTAL)"
echo "==> Run 'node script/build-webapp.mjs' to build the SPA"
