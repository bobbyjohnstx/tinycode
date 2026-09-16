#!/usr/bin/env sh
# Regenerate vendored build dependencies for the web UI.
# Run this when script/build-deps/package.json changes.
#
# Prerequisites: npm, node
# Installs deps for: darwin-arm64, darwin-x64, linux-x64, linux-arm64

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DEPS_DIR="$SCRIPT_DIR/build-deps"

echo "==> Installing npm dependencies..."
cd "$DEPS_DIR"
rm -rf node_modules package-lock.json
npm install

echo "==> Installing cross-platform binaries..."
npm install --force \
  @esbuild/darwin-x64@0.25.12 \
  @esbuild/linux-x64@0.25.12 \
  @esbuild/linux-arm64@0.25.12 \
  lightningcss-darwin-x64@1.32.0 \
  lightningcss-linux-x64-gnu@1.32.0 \
  lightningcss-linux-arm64-gnu@1.32.0 \
  @tailwindcss/oxide-darwin-x64@4.3.3 \
  @tailwindcss/oxide-linux-x64-gnu@4.3.3 \
  @tailwindcss/oxide-linux-arm64-gnu@4.3.3

echo "==> Trimming unnecessary files..."
cd node_modules
find . -name "*.md" -not -name "LICENSE*" -delete 2>/dev/null || true
find . -name "CHANGELOG*" -delete 2>/dev/null || true
find . -name "*.map" -delete 2>/dev/null || true
find . -name "*.d.ts" -delete 2>/dev/null || true
find . -name "*.d.mts" -delete 2>/dev/null || true
find . -name ".npmignore" -delete 2>/dev/null || true
find . -name ".eslintrc*" -delete 2>/dev/null || true
find . -name "tsconfig*" -delete 2>/dev/null || true
find . -type d -name "__tests__" -exec rm -rf {} + 2>/dev/null || true
find . -type d -name "test" -exec rm -rf {} + 2>/dev/null || true
find . -type d -name "tests" -exec rm -rf {} + 2>/dev/null || true
find . -type d -name ".github" -exec rm -rf {} + 2>/dev/null || true

TOTAL=$(du -sh . | cut -f1)
COUNT=$(ls -d */ 2>/dev/null | wc -l | tr -d ' ')
echo "==> Vendored $COUNT packages ($TOTAL)"
echo "==> Commit script/build-deps/node_modules/ to complete vendoring."
