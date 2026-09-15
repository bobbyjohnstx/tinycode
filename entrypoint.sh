#!/bin/sh
set -e

# ── Fix OpenShift environment overrides ──────────────────────────────────────
# OpenShift sets HOME=/ (read-only) and SHELL=/sbin/nologin (exits immediately).
export HOME="/home/tinycode"
export SHELL="/bin/sh"

WORKDIR="${TINYCODE_WORKDIR:-/projects}"

# ── Symlink bundled plugins into user plugin dir ─────────────────────────────
# Plugins built into the image live at /opt/tinycode/plugins/.
# Symlink them into ~/.config/tinycode/plugins/ so tinycode discovers them.
PLUGIN_DIR="$XDG_CONFIG_HOME/tinycode/plugins"
mkdir -p "$PLUGIN_DIR"
for plugin in /opt/tinycode/plugins/*; do
  [ -f "$plugin" ] || continue
  name=$(basename "$plugin")
  if [ ! -e "$PLUGIN_DIR/$name" ]; then
    ln -sf "$plugin" "$PLUGIN_DIR/$name"
    echo "[tinycode] Bundled plugin: $name"
  fi
done

# ── Write container defaults ─────────────────────────────────────────────────
# Write to config.json (lowest-priority config file). User customizations in
# tinycode.json or tinycode.jsonc take precedence and survive image upgrades.
DEFAULTS_FILE="$XDG_CONFIG_HOME/tinycode/config.json"

# Build plugin list from bundled plugins
PLUGIN_LIST=""
for plugin in "$PLUGIN_DIR"/*; do
  [ -f "$plugin" ] || [ -L "$plugin" ] || continue
  name=$(basename "$plugin")
  [ -n "$PLUGIN_LIST" ] && PLUGIN_LIST="$PLUGIN_LIST, "
  PLUGIN_LIST="$PLUGIN_LIST\"$name\""
done

# Include model override if set
if [ -n "${TINYCODE_MODEL:-}" ]; then
  printf '{\n  "plugins": [%s],\n  "model": "%s"\n}\n' "$PLUGIN_LIST" "$TINYCODE_MODEL" > "$DEFAULTS_FILE"
else
  printf '{\n  "plugins": [%s]\n}\n' "$PLUGIN_LIST" > "$DEFAULTS_FILE"
fi

# ── Provider configuration ───────────────────────────────────────────────────
# Default Ollama to host.containers.internal for local model access from container.
export TINYCODE_OLLAMA_HOST="${TINYCODE_OLLAMA_HOST:-http://host.containers.internal:11434}"

# ── In-cluster auto-detection ────────────────────────────────────────────────
if [ -n "${KUBERNETES_SERVICE_HOST:-}" ] && [ "${TINYCODE_AUTO_DETECT:-true}" != "false" ]; then
  echo "[tinycode] Running in-cluster (Kubernetes detected)"
fi

# ── GitOps mode ──────────────────────────────────────────────────────────────
if [ -n "${TINYCODE_GIT_REPO:-}" ]; then
  GIT_BRANCH="${TINYCODE_GIT_BRANCH:-}"
  GIT_TIMEOUT="${TINYCODE_GIT_CLONE_TIMEOUT:-300}"

  if [ -f "/home/tinycode/.git-credentials" ]; then
    git config --global credential.helper "store --file=/home/tinycode/.git-credentials"
  fi

  if [ -d "$WORKDIR/.git" ]; then
    if [ "${TINYCODE_GIT_PULL_ON_RESTART:-false}" = "true" ]; then
      echo "[tinycode] Pulling latest..."
      cd "$WORKDIR" && git pull --ff-only || echo "[tinycode] WARNING: Git pull failed. Skipping."
      cd /
    fi
  else
    SAFE_URL=$(echo "$TINYCODE_GIT_REPO" | sed -E 's|://[^@]*@|://***@|g')
    echo "[tinycode] Cloning $SAFE_URL (branch: ${GIT_BRANCH:-default})..."
    timeout "$GIT_TIMEOUT" git clone --depth 1 \
      ${GIT_BRANCH:+--branch "$GIT_BRANCH"} \
      "$TINYCODE_GIT_REPO" "$WORKDIR" || echo "[tinycode] WARNING: Git clone failed."
  fi
fi

# ── Workspace git init ───────────────────────────────────────────────────────
# tinycode needs a git repo for worktree detection. Init if not present.
if command -v git >/dev/null 2>&1; then
  git config --global --add safe.directory "$WORKDIR" 2>/dev/null || true
  if [ ! -d "$WORKDIR/.git" ]; then
    cd "$WORKDIR"
    git init -q 2>/dev/null || true
    git config user.email "tinycode@container" 2>/dev/null || true
    git config user.name "tinycode" 2>/dev/null || true
  fi
fi

# ── Start tinycode ───────────────────────────────────────────────────────────
cd "$WORKDIR"

# If no arguments, default to web mode with 0.0.0.0 binding for container networking.
if [ $# -eq 0 ]; then
  exec tinycode web --hostname 0.0.0.0
fi

exec tinycode "$@"
