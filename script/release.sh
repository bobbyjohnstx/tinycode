#!/usr/bin/env bash
# Build and publish a release to Gitea.
# Usage: ./script/release.sh v1.0.0
#
# Requires: go, tar, curl
# Optional: goreleaser (if installed, uses it instead)

set -euo pipefail

VERSION="${1:?Usage: $0 <version> (e.g., v1.0.0)}"
GITEA_URL="${GITEA_URL:-http://localhost:3000}"
GITEA_OWNER="${GITEA_OWNER:-bjohns}"
GITEA_REPO="${GITEA_REPO:-tinycode}"
GITEA_TOKEN="${GITEA_TOKEN:-}"

if [ -z "$GITEA_TOKEN" ]; then
  # Try to read from tea config
  GITEA_TOKEN=$(grep -A5 "gitea-local" ~/Library/Application\ Support/tea/config.yml 2>/dev/null | grep token | awk '{print $2}' || true)
fi

if [ -z "$GITEA_TOKEN" ]; then
  echo "Error: GITEA_TOKEN not set and not found in tea config"
  exit 1
fi

# Check if goreleaser is available
if command -v goreleaser &>/dev/null; then
  echo "Using GoReleaser..."
  GITEA_TOKEN="$GITEA_TOKEN" goreleaser release \
    --clean \
    --config .goreleaser.yml
  exit 0
fi

echo "GoReleaser not found, building manually..."

COMMIT=$(git rev-parse --short HEAD)
DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}"
DIST="dist/release"
rm -rf "$DIST"
mkdir -p "$DIST"

PLATFORMS=(
  "darwin/amd64"
  "darwin/arm64"
  "linux/amd64"
  "linux/arm64"
)

echo "Building binaries..."
for platform in "${PLATFORMS[@]}"; do
  os="${platform%/*}"
  arch="${platform#*/}"
  output="tinycode"
  archive="tinycode-${os}-${arch}"

  echo "  Building ${archive}..."
  GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 \
    go build -ldflags "$LDFLAGS" -o "${DIST}/${output}" ./cmd/tinycode

  # Create tarball
  tar -czf "${DIST}/${archive}.tar.gz" -C "$DIST" "$output" \
    -C "$(pwd)" LICENSE README.md docs/quickstart.md
  rm "${DIST}/${output}"
done

# Generate checksums
echo "Generating checksums..."
(cd "$DIST" && shasum -a 256 *.tar.gz > checksums.txt)

echo ""
echo "Archives:"
ls -lh "$DIST"/*.tar.gz
echo ""
cat "$DIST/checksums.txt"

# Create git tag
echo ""
echo "Creating tag ${VERSION}..."
git tag -a "$VERSION" -m "Release ${VERSION}"
git push tinycode "$VERSION"

# Create Gitea release
echo ""
echo "Creating Gitea release..."
RELEASE_BODY=$(cat <<EOF
## tinycode ${VERSION}

### Installation

\`\`\`bash
# macOS (Apple Silicon)
curl -fsSL ${GITEA_URL}/${GITEA_OWNER}/${GITEA_REPO}/releases/download/${VERSION}/tinycode-darwin-arm64.tar.gz | tar xz
sudo mv tinycode /usr/local/bin/

# macOS (Intel)
curl -fsSL ${GITEA_URL}/${GITEA_OWNER}/${GITEA_REPO}/releases/download/${VERSION}/tinycode-darwin-amd64.tar.gz | tar xz
sudo mv tinycode /usr/local/bin/

# Linux (x86_64)
curl -fsSL ${GITEA_URL}/${GITEA_OWNER}/${GITEA_REPO}/releases/download/${VERSION}/tinycode-linux-amd64.tar.gz | tar xz
sudo mv tinycode /usr/local/bin/

# Linux (ARM64)
curl -fsSL ${GITEA_URL}/${GITEA_OWNER}/${GITEA_REPO}/releases/download/${VERSION}/tinycode-linux-arm64.tar.gz | tar xz
sudo mv tinycode /usr/local/bin/
\`\`\`

### Verify
\`\`\`bash
tinycode version
\`\`\`
EOF
)

RELEASE_ID=$(curl -s -X POST "${GITEA_URL}/api/v1/repos/${GITEA_OWNER}/${GITEA_REPO}/releases" \
  -H "Authorization: token ${GITEA_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "$(python3 -c "import json; print(json.dumps({'tag_name': '${VERSION}', 'name': '${VERSION}', 'body': '''${RELEASE_BODY}''', 'draft': False, 'prerelease': False}))")" \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")

echo "Release created: ID ${RELEASE_ID}"

# Upload assets
echo "Uploading assets..."
for file in "$DIST"/*.tar.gz "$DIST"/checksums.txt; do
  name=$(basename "$file")
  echo "  Uploading ${name}..."
  curl -s -X POST "${GITEA_URL}/api/v1/repos/${GITEA_OWNER}/${GITEA_REPO}/releases/${RELEASE_ID}/assets?name=${name}" \
    -H "Authorization: token ${GITEA_TOKEN}" \
    -H "Content-Type: application/octet-stream" \
    --data-binary "@${file}" > /dev/null
done

echo ""
echo "Release ${VERSION} published!"
echo "  ${GITEA_URL}/${GITEA_OWNER}/${GITEA_REPO}/releases/tag/${VERSION}"
echo ""
echo "Install on another machine:"
echo "  curl -fsSL ${GITEA_URL}/${GITEA_OWNER}/${GITEA_REPO}/releases/download/${VERSION}/tinycode-darwin-arm64.tar.gz | tar xz"
echo "  sudo mv tinycode /usr/local/bin/"
