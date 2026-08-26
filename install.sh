#!/usr/bin/env sh
# Copyright 2026 Bobby Johns
# MIT License - https://opensource.org/licenses/MIT
#
# Installer for tinycode - Local-LLM-first AI coding assistant
# Usage: curl -fsSL https://raw.githubusercontent.com/bobbyjohnstx/tinycode/main/install.sh | sh

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RESET='\033[0m'

# Configuration
REPO="bobbyjohnstx/tinycode"
INSTALL_DIR="${TINYCODE_INSTALL_DIR:-$HOME/.local/bin}"
TMP_DIR="/tmp/tinycode-install-$$"

# Logging functions
info() {
  printf "${BLUE}==>${RESET} %s\n" "$*"
}

success() {
  printf "${GREEN}==>${RESET} %s\n" "$*"
}

warn() {
  printf "${YELLOW}==>${RESET} %s\n" "$*" >&2
}

error() {
  printf "${RED}ERROR:${RESET} %s\n" "$*" >&2
  cleanup
  exit 1
}

# Cleanup temporary files
cleanup() {
  if [ -d "$TMP_DIR" ]; then
    rm -rf "$TMP_DIR"
  fi
}

trap cleanup EXIT INT TERM

# Detect platform and architecture
detect_platform() {
  OS=$(uname -s | tr '[:upper:]' '[:lower:]')
  ARCH=$(uname -m)

  case "$OS" in
    darwin)
      PLATFORM_OS="darwin"
      ;;
    linux)
      PLATFORM_OS="linux"
      ;;
    *)
      error "Unsupported operating system: $OS. tinycode supports macOS (darwin) and Linux only."
      ;;
  esac

  case "$ARCH" in
    x86_64)
      PLATFORM_ARCH="x64"
      ;;
    arm64|aarch64)
      PLATFORM_ARCH="arm64"
      ;;
    *)
      error "Unsupported architecture: $ARCH. tinycode supports x86_64 and arm64/aarch64 only."
      ;;
  esac

  PLATFORM="${PLATFORM_OS}-${PLATFORM_ARCH}"
  info "Detected platform: ${PLATFORM_OS}/${PLATFORM_ARCH}"
}

# Determine latest version from GitHub API
get_latest_version() {
  if [ -n "${VERSION:-}" ]; then
    info "Using pinned version: $VERSION"
    return
  fi

  info "Fetching latest release from GitHub..."

  # Try curl first, then wget
  if command -v curl >/dev/null 2>&1; then
    VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
      | grep '"tag_name"' \
      | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
  elif command -v wget >/dev/null 2>&1; then
    VERSION=$(wget -qO- "https://api.github.com/repos/${REPO}/releases/latest" \
      | grep '"tag_name"' \
      | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
  else
    error "Neither curl nor wget found. Please install curl or wget and try again."
  fi

  if [ -z "$VERSION" ]; then
    error "Failed to determine latest version. Check your network connection or set VERSION manually."
  fi

  info "Latest version: $VERSION"
}

# Download the binary
download_binary() {
  BINARY_NAME="tinycode-${PLATFORM}"
  DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${BINARY_NAME}.tar.gz"

  info "Downloading tinycode ${VERSION} for ${PLATFORM}..."

  mkdir -p "$TMP_DIR"

  if command -v curl >/dev/null 2>&1; then
    if ! curl -fsSL "$DOWNLOAD_URL" -o "$TMP_DIR/tinycode.tar.gz"; then
      error "Download failed. Please verify that release ${VERSION} includes ${BINARY_NAME}.tar.gz at:\n  $DOWNLOAD_URL"
    fi
  elif command -v wget >/dev/null 2>&1; then
    if ! wget -q "$DOWNLOAD_URL" -O "$TMP_DIR/tinycode.tar.gz"; then
      error "Download failed. Please verify that release ${VERSION} includes ${BINARY_NAME}.tar.gz at:\n  $DOWNLOAD_URL"
    fi
  fi

  success "Download complete"
}

# Extract and install the binary
install_binary() {
  info "Installing to $INSTALL_DIR..."

  mkdir -p "$INSTALL_DIR"

  # Extract tarball (expect structure: tinycode-<platform>/bin/tinycode)
  if ! tar -xzf "$TMP_DIR/tinycode.tar.gz" -C "$TMP_DIR"; then
    error "Failed to extract archive. The downloaded file may be corrupted."
  fi

  # Find the binary (handle both flat and nested structures)
  BINARY_PATH=""
  if [ -f "$TMP_DIR/${BINARY_NAME}/bin/tinycode" ]; then
    BINARY_PATH="$TMP_DIR/${BINARY_NAME}/bin/tinycode"
  elif [ -f "$TMP_DIR/bin/tinycode" ]; then
    BINARY_PATH="$TMP_DIR/bin/tinycode"
  elif [ -f "$TMP_DIR/tinycode" ]; then
    BINARY_PATH="$TMP_DIR/tinycode"
  else
    error "Binary not found in archive. Expected structure: ${BINARY_NAME}/bin/tinycode"
  fi

  # Copy to install directory
  cp "$BINARY_PATH" "$INSTALL_DIR/tinycode"
  chmod +x "$INSTALL_DIR/tinycode"

  success "Binary installed to $INSTALL_DIR/tinycode"
}

# Check if install directory is in PATH
check_path() {
  case ":$PATH:" in
    *":$INSTALL_DIR:"*)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

# Print PATH setup instructions
print_path_instructions() {
  if check_path; then
    return
  fi

  warn "$INSTALL_DIR is not in your PATH."
  echo ""
  echo "Add this to your shell profile to use tinycode from any directory:"
  echo ""

  # Detect shell
  SHELL_NAME=$(basename "$SHELL")
  case "$SHELL_NAME" in
    bash)
      PROFILE="~/.bashrc"
      ;;
    zsh)
      PROFILE="~/.zshrc"
      ;;
    fish)
      PROFILE="~/.config/fish/config.fish"
      echo "  set -gx PATH $INSTALL_DIR \$PATH"
      echo ""
      return
      ;;
    *)
      PROFILE="~/.profile"
      ;;
  esac

  echo "  echo 'export PATH=\"$INSTALL_DIR:\$PATH\"' >> $PROFILE"
  echo "  source $PROFILE"
  echo ""
}

# Verify installation
verify_installation() {
  info "Verifying installation..."

  if [ ! -x "$INSTALL_DIR/tinycode" ]; then
    error "Installation failed: $INSTALL_DIR/tinycode is not executable"
  fi

  INSTALLED_VERSION=$("$INSTALL_DIR/tinycode" --version 2>/dev/null || echo "unknown")

  success "tinycode $INSTALLED_VERSION installed successfully!"
  echo ""
  echo "Run 'tinycode' to get started."
  echo ""

  print_path_instructions
}

# Print usage banner
print_banner() {
  cat << 'EOF'
  _   _                        _
 | |_(_)_ __  _   _  ___ ___  __| | ___
 | __| | '_ \| | | |/ __/ _ \/ _` |/ _ \
 | |_| | | | | |_| | (_| (_) | (_| |  __/
  \__|_|_| |_|\__, |\___\___/ \__,_|\___|
              |___/

EOF
}

# Main installation flow
main() {
  print_banner
  detect_platform
  get_latest_version
  download_binary
  install_binary
  verify_installation
}

main
