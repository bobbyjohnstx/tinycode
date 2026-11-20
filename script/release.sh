#!/usr/bin/env bash
# release.sh — Full release pipeline for tinycode.
#
# Builds, tags, publishes to Gitea and GitHub, updates the Homebrew tap,
# and verifies the install. Enforces ordering so checksums never drift.
#
# Usage:
#   ./script/release.sh v2.11.0           # full release
#   ./script/release.sh v2.11.0 --dry-run # build + verify only, no publish
#   ./script/release.sh --check           # pre-flight checks only
#
# Environment:
#   GITEA_URL       Gitea server  (default: http://localhost:3000)
#   GITEA_OWNER     Gitea org     (default: bjohns)
#   GITEA_REPO      Gitea repo    (default: tinycode)
#   GITEA_TOKEN     Gitea API token (reads tea config as fallback)
#   GITHUB_REPO     GitHub repo   (default: bobbyjohnstx/tinycode)
#   HOMEBREW_TAP    Tap repo path (default: ../homebrew-tap)
#   SKIP_GITEA      Skip Gitea publish (default: false)
#   SKIP_GITHUB     Skip GitHub publish (default: false)
#   SKIP_HOMEBREW   Skip Homebrew tap update (default: false)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
GITEA_URL="${GITEA_URL:-http://localhost:3000}"
GITEA_OWNER="${GITEA_OWNER:-bjohns}"
GITEA_REPO="${GITEA_REPO:-tinycode}"
GITEA_TOKEN="${GITEA_TOKEN:-}"
GITHUB_REPO="${GITHUB_REPO:-bobbyjohnstx/tinycode}"
HOMEBREW_TAP="${HOMEBREW_TAP:-${PROJECT_DIR}/../homebrew-tap}"
SKIP_GITEA="${SKIP_GITEA:-false}"
SKIP_GITHUB="${SKIP_GITHUB:-false}"
SKIP_HOMEBREW="${SKIP_HOMEBREW:-false}"
DRY_RUN=false

PLATFORMS=(
  "darwin/amd64"
  "darwin/arm64"
  "linux/amd64"
  "linux/arm64"
)

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BOLD='\033[1m'
NC='\033[0m'

step() { echo -e "\n${BOLD}==> $1${NC}"; }
ok()   { echo -e "  ${GREEN}✓${NC} $1"; }
warn() { echo -e "  ${YELLOW}!${NC} $1"; }
fail() { echo -e "  ${RED}✗${NC} $1"; exit 1; }

# --- Argument parsing ---

VERSION=""
for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=true ;;
    --check)
      # Pre-flight only
      VERSION="check"
      ;;
    v*) VERSION="$arg" ;;
    *) echo "Unknown argument: $arg"; exit 1 ;;
  esac
done

if [ -z "$VERSION" ]; then
  echo "Usage: $0 <version> [--dry-run|--check]"
  echo "  e.g., $0 v2.11.0"
  exit 1
fi

# --- Pre-flight checks ---

step "Pre-flight checks"

# Clean working tree
if [ -n "$(git -C "$PROJECT_DIR" status --porcelain)" ]; then
  fail "Working tree is dirty. Commit or stash changes first."
fi
ok "Working tree clean"

# Tests pass
step "Running tests"
if ! (cd "$PROJECT_DIR" && go test ./... -count=1 -timeout 120s > /tmp/release-test-output.txt 2>&1); then
  echo ""
  tail -20 /tmp/release-test-output.txt
  fail "Tests failed. Fix before releasing."
fi
ok "All tests pass"

# Lint
step "Running lint"
if ! (cd "$PROJECT_DIR" && make lint > /tmp/release-lint-output.txt 2>&1); then
  echo ""
  tail -10 /tmp/release-lint-output.txt
  warn "Lint warnings (non-blocking)"
else
  ok "Lint clean"
fi

# Changelog has entry for this version
if [ "$VERSION" != "check" ]; then
  step "Checking docs"
  changelog_version="${VERSION#v}"
  if ! grep -q "\[${changelog_version}\]" "$PROJECT_DIR/CHANGELOG.md"; then
    fail "CHANGELOG.md has no entry for [${changelog_version}]. Add one before releasing."
  fi
  ok "CHANGELOG.md has entry for ${changelog_version}"

  # Check CHANGELOG [Unreleased] section is empty or minimal
  unreleased=$(sed -n '/## \[Unreleased\]/,/## \[/p' "$PROJECT_DIR/CHANGELOG.md" | grep -c "^-" || true)
  if [ "$unreleased" -gt 0 ]; then
    warn "CHANGELOG.md [Unreleased] has $unreleased items — should these be under [${changelog_version}]?"
  fi
fi

# Gitea token
if [ "$SKIP_GITEA" != "true" ]; then
  if [ -z "$GITEA_TOKEN" ]; then
    GITEA_TOKEN=$(grep -A5 "gitea-local" ~/Library/Application\ Support/tea/config.yml 2>/dev/null | grep token | awk '{print $2}' || true)
  fi
  if [ -z "$GITEA_TOKEN" ]; then
    warn "GITEA_TOKEN not set — Gitea publish will be skipped"
    SKIP_GITEA=true
  else
    ok "Gitea token found"
  fi
fi

# GitHub CLI
if [ "$SKIP_GITHUB" != "true" ]; then
  if ! command -v gh &>/dev/null; then
    warn "gh CLI not found — GitHub publish will be skipped"
    SKIP_GITHUB=true
  elif ! gh auth status &>/dev/null 2>&1; then
    warn "gh not authenticated — GitHub publish will be skipped"
    SKIP_GITHUB=true
  else
    ok "GitHub CLI authenticated"
  fi
fi

# Homebrew tap
if [ "$SKIP_HOMEBREW" != "true" ]; then
  if [ ! -d "$HOMEBREW_TAP" ]; then
    warn "Homebrew tap not found at $HOMEBREW_TAP — will be skipped"
    SKIP_HOMEBREW=true
  elif [ ! -f "$HOMEBREW_TAP/tinycode.rb" ]; then
    warn "No tinycode.rb in $HOMEBREW_TAP — will be skipped"
    SKIP_HOMEBREW=true
  else
    ok "Homebrew tap found at $HOMEBREW_TAP"
  fi
fi

if [ "$VERSION" = "check" ]; then
  echo ""
  echo "Pre-flight checks passed."
  exit 0
fi

# --- Build ---

step "Building binaries for ${VERSION}"

COMMIT=$(git -C "$PROJECT_DIR" rev-parse --short HEAD)
DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}"
DIST="${PROJECT_DIR}/dist/release"
rm -rf "$DIST"
mkdir -p "$DIST"

for platform in "${PLATFORMS[@]}"; do
  os="${platform%/*}"
  arch="${platform#*/}"
  archive="tinycode-${os}-${arch}"
  stage="${DIST}/${archive}"
  mkdir -p "$stage"

  echo "  Building ${archive}..."
  GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 \
    go build -ldflags "$LDFLAGS" -o "${stage}/tinycode" ./cmd/tinycode

  # Create tarball (binary inside a named directory)
  tar -czf "${DIST}/${archive}.tar.gz" -C "$DIST" "${archive}"
  rm -rf "$stage"
done

ok "4 platform binaries built"

# --- Build plugins ---

step "Building plugins"
if (cd "$PROJECT_DIR" && make package-plugins > /tmp/release-plugins.txt 2>&1); then
  # Copy plugin archives to release dir
  cp "$PROJECT_DIR"/dist/release/tinycode-plugin-*.tar.gz "$DIST/" 2>/dev/null || true
  cp "$PROJECT_DIR"/dist/release/tinycode-plugins-*.tar.gz "$DIST/" 2>/dev/null || true
  plugin_count=$(ls "$DIST"/tinycode-plugin-*.tar.gz 2>/dev/null | wc -l | tr -d ' ')
  ok "${plugin_count} plugin archives built"
else
  warn "Plugin build failed (non-blocking) — see /tmp/release-plugins.txt"
fi

# --- Checksums ---

step "Generating checksums"
(cd "$DIST" && shasum -a 256 *.tar.gz > checksums-sha256.txt)
ok "checksums-sha256.txt generated"
echo ""
grep "tinycode-darwin\|tinycode-linux" "$DIST/checksums-sha256.txt" | while read -r line; do
  echo "  $line"
done

# --- Verify binary ---

step "Verifying binary"
local_arch=$(uname -m)
local_os=$(uname -s | tr '[:upper:]' '[:lower:]')
if [ "$local_arch" = "arm64" ] || [ "$local_arch" = "aarch64" ]; then
  local_arch="arm64"
elif [ "$local_arch" = "x86_64" ]; then
  local_arch="amd64"
fi
test_tarball="${DIST}/tinycode-${local_os}-${local_arch}.tar.gz"
if [ -f "$test_tarball" ]; then
  test_dir=$(mktemp -d)
  tar xzf "$test_tarball" -C "$test_dir"
  actual_version=$("$test_dir"/tinycode-*/tinycode version 2>&1 | head -1)
  rm -rf "$test_dir"
  if echo "$actual_version" | grep -q "$VERSION"; then
    ok "Binary reports: $actual_version"
  else
    fail "Binary version mismatch: $actual_version (expected $VERSION)"
  fi
fi

if $DRY_RUN; then
  echo ""
  echo -e "${YELLOW}Dry run — skipping tag, publish, and tap update.${NC}"
  echo "Release artifacts in: $DIST"
  exit 0
fi

# --- Tag ---

step "Creating tag ${VERSION}"
if git -C "$PROJECT_DIR" rev-parse "$VERSION" &>/dev/null; then
  existing_commit=$(git -C "$PROJECT_DIR" rev-list -n 1 "$VERSION")
  current_commit=$(git -C "$PROJECT_DIR" rev-parse HEAD)
  if [ "$existing_commit" = "$current_commit" ]; then
    ok "Tag ${VERSION} already exists at HEAD"
  else
    fail "Tag ${VERSION} exists but points to ${existing_commit:0:8}, not HEAD (${current_commit:0:8}). Delete or bump version."
  fi
else
  git -C "$PROJECT_DIR" tag -a "$VERSION" -m "Release ${VERSION}"
  ok "Tag created"
fi

# Push tag to all remotes
for remote in $(git -C "$PROJECT_DIR" remote); do
  git -C "$PROJECT_DIR" push "$remote" "$VERSION" 2>/dev/null && ok "Tag pushed to $remote" || warn "Failed to push tag to $remote"
done

# --- Publish to Gitea ---

if [ "$SKIP_GITEA" != "true" ]; then
  step "Publishing to Gitea"

  RELEASE_ID=$(curl -s -X POST "${GITEA_URL}/api/v1/repos/${GITEA_OWNER}/${GITEA_REPO}/releases" \
    -H "Authorization: token ${GITEA_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "$(python3 -Ic "import json; print(json.dumps({'tag_name': '${VERSION}', 'name': '${VERSION}', 'body': 'Release ${VERSION}', 'draft': False, 'prerelease': False}))")" \
    | python3 -Ic "import sys,json; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || echo "")

  if [ -n "$RELEASE_ID" ] && [ "$RELEASE_ID" != "" ]; then
    ok "Gitea release created (ID: $RELEASE_ID)"
    for file in "$DIST"/*.tar.gz "$DIST"/checksums-sha256.txt; do
      name=$(basename "$file")
      curl -s -X POST "${GITEA_URL}/api/v1/repos/${GITEA_OWNER}/${GITEA_REPO}/releases/${RELEASE_ID}/assets?name=${name}" \
        -H "Authorization: token ${GITEA_TOKEN}" \
        -H "Content-Type: application/octet-stream" \
        --data-binary "@${file}" > /dev/null
    done
    ok "Assets uploaded to Gitea"
  else
    warn "Gitea release creation failed — skipping"
  fi
fi

# --- Publish to GitHub ---

if [ "$SKIP_GITHUB" != "true" ]; then
  step "Publishing to GitHub"

  # Create release
  if gh release view "$VERSION" --repo "$GITHUB_REPO" &>/dev/null 2>&1; then
    warn "GitHub release ${VERSION} already exists — uploading missing assets only"
  else
    gh release create "$VERSION" \
      --repo "$GITHUB_REPO" \
      --title "$VERSION" \
      --notes "Release ${VERSION}" \
      --latest
    ok "GitHub release created"
  fi

  # Upload assets
  for file in "$DIST"/*.tar.gz "$DIST"/checksums-sha256.txt; do
    name=$(basename "$file")
    gh release upload "$VERSION" "$file" --repo "$GITHUB_REPO" --clobber 2>/dev/null \
      && ok "Uploaded $name" \
      || warn "Failed to upload $name"
  done
fi

# --- Update Homebrew tap ---

if [ "$SKIP_HOMEBREW" != "true" ]; then
  step "Updating Homebrew tap"

  formula="$HOMEBREW_TAP/tinycode.rb"
  version_num="${VERSION#v}"

  # Read checksums from the file we just generated
  declare -A checksums
  while read -r hash name; do
    checksums["$name"]="$hash"
  done < "$DIST/checksums-sha256.txt"

  darwin_arm64="${checksums[tinycode-darwin-arm64.tar.gz]}"
  darwin_amd64="${checksums[tinycode-darwin-amd64.tar.gz]}"
  linux_arm64="${checksums[tinycode-linux-arm64.tar.gz]}"
  linux_amd64="${checksums[tinycode-linux-amd64.tar.gz]}"

  if [ -z "$darwin_arm64" ] || [ -z "$darwin_amd64" ] || [ -z "$linux_arm64" ] || [ -z "$linux_amd64" ]; then
    fail "Missing checksums — cannot update formula"
  fi

  # Determine download URL base (GitHub if published there, else Gitea)
  if [ "$SKIP_GITHUB" != "true" ]; then
    url_base="https://github.com/${GITHUB_REPO}/releases/download/${VERSION}"
  else
    url_base="${GITEA_URL}/${GITEA_OWNER}/${GITEA_REPO}/releases/download/${VERSION}"
  fi

  cat > "$formula" << FORMULA
class Tinycode < Formula
  desc "Local-first, model-agnostic AI coding assistant — single binary, no runtime dependencies"
  homepage "https://github.com/${GITHUB_REPO}"
  version "${version_num}"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "${url_base}/tinycode-darwin-arm64.tar.gz"
      sha256 "${darwin_arm64}"
    else
      url "${url_base}/tinycode-darwin-amd64.tar.gz"
      sha256 "${darwin_amd64}"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "${url_base}/tinycode-linux-arm64.tar.gz"
      sha256 "${linux_arm64}"
    else
      url "${url_base}/tinycode-linux-amd64.tar.gz"
      sha256 "${linux_amd64}"
    end
  end

  def install
    # Tarballs extract to a subdirectory (e.g. tinycode-darwin-arm64/tinycode)
    if File.exist?("tinycode")
      bin.install "tinycode"
    else
      Dir.glob("tinycode-*/tinycode").each do |f|
        bin.install f
      end
    end
  end

  test do
    assert_match "tinycode", shell_output("#{bin}/tinycode version")
  end
end
FORMULA

  ok "Formula updated: version=${version_num}"
  ok "  darwin-arm64: ${darwin_arm64:0:16}..."
  ok "  darwin-amd64: ${darwin_amd64:0:16}..."
  ok "  linux-arm64:  ${linux_arm64:0:16}..."
  ok "  linux-amd64:  ${linux_amd64:0:16}..."

  # Commit and push
  cd "$HOMEBREW_TAP"
  git add tinycode.rb
  git commit -m "tinycode ${VERSION}" || warn "Nothing to commit (formula unchanged)"
  git push || warn "Failed to push homebrew-tap"
  cd "$PROJECT_DIR"
  ok "Homebrew tap pushed"
fi

# --- Summary ---

echo ""
echo -e "${GREEN}${BOLD}Release ${VERSION} complete!${NC}"
echo ""
echo "  Artifacts:  $DIST"
[ "$SKIP_GITEA" != "true" ]    && echo "  Gitea:      ${GITEA_URL}/${GITEA_OWNER}/${GITEA_REPO}/releases/tag/${VERSION}"
[ "$SKIP_GITHUB" != "true" ]   && echo "  GitHub:     https://github.com/${GITHUB_REPO}/releases/tag/${VERSION}"
[ "$SKIP_HOMEBREW" != "true" ] && echo "  Homebrew:   brew install bobbyjohnstx/tap/tinycode"
echo ""
echo "  Install:    brew upgrade tinycode"
echo "  Verify:     tinycode version"
