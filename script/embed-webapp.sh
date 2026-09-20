#!/usr/bin/env sh
# Build the SolidJS web app and copy output into internal/static/dist/
# for embedding into the Go binary via embed.FS.
#
# Usage: ./script/embed-webapp.sh
#
# Prerequisites: node installed, deps via bun install or vendored in script/build-deps/

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

APP_DIR="$ROOT/packages/app"
STATIC_DIR="$ROOT/internal/static/dist"

if [ ! -d "$APP_DIR" ]; then
  echo "Error: packages/app not found at $APP_DIR"
  exit 1
fi

VENDORED="$ROOT/script/build-deps/node_modules"
WORKSPACE="$ROOT/node_modules"
if [ ! -d "$VENDORED" ] && [ ! -d "$WORKSPACE" ]; then
  echo "==> No deps found, running vendor-deps.sh..."
  "$ROOT/script/vendor-deps.sh"
fi

echo "==> Building SolidJS web app..."
node "$ROOT/script/build-webapp.mjs"

echo "==> Cleaning static dist..."
rm -rf "$STATIC_DIR"
mkdir -p "$STATIC_DIR"

echo "==> Copying build output (excluding sourcemaps)..."
cd "$APP_DIR/dist"
find . -type f -not -name '*.map' | while read -r file; do
  dir=$(dirname "$file")
  mkdir -p "$STATIC_DIR/$dir"
  cp "$file" "$STATIC_DIR/$file"
done

TOTAL=$(du -sh "$STATIC_DIR" | cut -f1)
COUNT=$(find "$STATIC_DIR" -type f | wc -l | tr -d ' ')
echo "==> Embedded $COUNT files ($TOTAL) into internal/static/dist/"
echo "==> Run 'make build' to compile the Go binary with the web UI."
