---
name: tc-doctor
description: Diagnose and fix tinycode environment issues — Ollama install, model health, tool-call capability, directories, agents, skills, connectivity, and container health
---

# tc-doctor

Self-diagnostic skill that checks the tinycode environment is correctly configured and fixes problems it finds. No dependencies beyond basic shell tools (curl, grep, awk, sed).

## When to use

- The user says "doctor", "tc-doctor", "health check", "fix my setup"
- After a fresh install, deployment, or container restart
- When agents, skills, or tools aren't appearing
- When the model connection is failing or responses are slow
- When tool calling isn't working (model responds with text only)
- When asking why a tinycode feature isn't working (not their own application code)

## When NOT to use

- The user is asking about their own application code (not tinycode itself)
- The user wants to configure a new provider (use /mcp-setup or /customize-tinycode instead)
- Debugging a specific session failure or code bug (use /debug or /trace)
- Choosing which model to use or comparing model capabilities (advise directly)
- Troubleshooting MCP server connections (use /mcp-setup)

## Preamble: Detect Platform

Before running any checks, detect the platform. Gate container-specific checks behind `IS_CONTAINER`.

```bash
# Platform detection
IS_CONTAINER=false
if [ -f /run/.containerenv ] || [ -f /.dockerenv ]; then
  IS_CONTAINER=true
fi

IS_MAC=false
if [ "$(uname)" = "Darwin" ]; then
  IS_MAC=true
fi

OLLAMA_HOST="${TINYCODE_OLLAMA_HOST:-http://localhost:11434}"
TINYCODE_API="http://localhost:4096"

echo "## tinycode Doctor Report"
echo ""
echo "### Environment"
echo "- Platform: $(uname -s) $(uname -m)"
echo "- Container: $IS_CONTAINER"
echo "- Working directory: $(pwd)"
echo "- User: $(id -un) ($(id -u))"
echo "- Ollama endpoint: $OLLAMA_HOST"
if [ -n "$TINYCODE_OLLAMA_HOST" ]; then
  echo "  Source: TINYCODE_OLLAMA_HOST env var"
else
  echo "  Source: default"
fi
echo ""
echo "### Checks"
```

## Checks to run (in order)

### 1. Directory structure
Verify these directories exist and are writable. Create any that are missing:

```bash
mkdir -p .tinycode 2>/dev/null && echo "✓ .tinycode/" || echo "✗ .tinycode/ — not writable"

for dir in .tinycode/plans .tinycode/state; do
  mkdir -p "$dir" 2>/dev/null && echo "✓ $dir" || echo "✗ $dir — not writable"
done

ls ~/.config/tinycode/ >/dev/null 2>&1 && echo "✓ ~/.config/tinycode/" || echo "✗ ~/.config/tinycode/ — missing"
ls ~/.local/share/tinycode/ >/dev/null 2>&1 && echo "✓ ~/.local/share/tinycode/" || echo "✗ ~/.local/share/tinycode/ — missing"
```

### 2. Agents (container only)
On desktop installs, agents are loaded from source at runtime — skip this check.

```bash
if [ "$IS_CONTAINER" = "true" ]; then
  AGENT_COUNT=$(ls ~/.config/tinycode/agent/*.md 2>/dev/null | wc -l)
  echo "Agent files: $AGENT_COUNT"
  if [ "$AGENT_COUNT" -lt 20 ]; then
    echo "✗ Expected 20+ agent files. Check /opt/tinycode-defaults/agent/ and entrypoint.sh"
    if [ -d /opt/tinycode-defaults/agent ]; then
      mkdir -p ~/.config/tinycode/agent
      cp -n /opt/tinycode-defaults/agent/*.md ~/.config/tinycode/agent/ 2>/dev/null
      echo "  → Copied bundled agents. Restart tinycode to pick them up."
    fi
  else
    echo "✓ Agents loaded"
  fi
else
  echo "✓ Agents (loaded from source on desktop)"
fi
```

### 3. Skills (container only)

```bash
if [ "$IS_CONTAINER" = "true" ]; then
  SKILL_COUNT=$(find ~/.config/tinycode/skills -name "SKILL.md" 2>/dev/null | wc -l)
  echo "Skill files: $SKILL_COUNT"
  if [ "$SKILL_COUNT" -lt 5 ]; then
    echo "✗ Expected 5+ skill files. Check /opt/tinycode-defaults/skills/"
    if [ -d /opt/tinycode-defaults/skills ]; then
      for skill_dir in /opt/tinycode-defaults/skills/*/; do
        skill_name=$(basename "$skill_dir")
        mkdir -p ~/.config/tinycode/skills/$skill_name
        cp -n "$skill_dir"SKILL.md ~/.config/tinycode/skills/$skill_name/ 2>/dev/null
      done
      echo "  → Copied bundled skills. Restart tinycode to pick them up."
    fi
  else
    echo "✓ Skills loaded"
  fi
else
  echo "✓ Skills (loaded from source on desktop)"
fi
```

### 4. Tools availability

```bash
for tool in curl git; do
  if command -v $tool >/dev/null 2>&1; then
    echo "✓ $tool: $(command -v $tool)"
  else
    echo "✗ $tool: NOT FOUND"
  fi
done

# Optional tools
if command -v tmux >/dev/null 2>&1; then
  echo "✓ tmux: $(command -v tmux)"
else
  echo "ℹ tmux: not found (optional — needed for /swarm)"
fi
```

### 5. oc CLI (if cluster-admin mode)
```bash
if [ "${TINYCODE_CLUSTER_ADMIN}" = "true" ] || [ -f ~/.kube/config ]; then
  if command -v oc >/dev/null 2>&1; then
    echo "✓ oc: $(oc version --client 2>/dev/null | head -1)"
    oc whoami 2>/dev/null && echo "✓ Cluster auth valid" || echo "✗ Cluster auth failed — kubeconfig may be expired"
  else
    echo "✗ oc: NOT FOUND — cluster-admin mode requires oc CLI"
  fi
fi
```

### 6. Ollama installation & health

These checks form a dependency chain. If any step fails, skip subsequent Ollama checks.

```bash
# 6a. Is Ollama installed?
if command -v ollama >/dev/null 2>&1; then
  OLLAMA_VERSION=$(ollama --version 2>&1 | head -1)
  echo "✓ ollama installed: $OLLAMA_VERSION"
else
  echo "✗ ollama not found in PATH"
  echo "  Fix: Install from https://ollama.com/download"
  if [ "$IS_MAC" = "true" ]; then
    echo "  macOS: brew install ollama"
  else
    echo "  Linux: curl -fsSL https://ollama.com/install.sh | sh"
  fi
  echo ""
  echo "SKIP: Remaining Ollama checks skipped (not installed)"
  # Skip to Check 10
fi
```

```bash
# 6b. Is ollama serve running?
if pgrep -x "ollama" >/dev/null 2>&1 || pgrep -f "ollama serve" >/dev/null 2>&1; then
  echo "✓ ollama process running"
else
  echo "✗ ollama process not running"
  if [ "$IS_MAC" = "true" ]; then
    echo "  Fix: Open the Ollama app, or run: ollama serve"
  else
    echo "  Fix: Run 'ollama serve' in a separate terminal"
    echo "  Or: systemctl --user enable --now ollama"
  fi
fi
```

```bash
# 6c. Can we reach the Ollama API?
TAGS_JSON=$(curl -sf --max-time 5 "${OLLAMA_HOST}/api/tags" 2>/dev/null)
if [ -n "$TAGS_JSON" ]; then
  echo "✓ Ollama API responding at ${OLLAMA_HOST}"
else
  echo "✗ Ollama API not reachable at ${OLLAMA_HOST}"
  if command -v lsof >/dev/null 2>&1; then
    LISTENERS=$(lsof -i :11434 2>/dev/null | grep LISTEN | wc -l | tr -d ' ')
    if [ "$LISTENERS" -gt 1 ]; then
      echo "  ⚠ Multiple processes listening on port 11434 — port conflict"
    fi
  fi
  if [ "$IS_CONTAINER" = "true" ]; then
    echo "  Container detected — Ollama may be on the host."
    echo "  Try: export TINYCODE_OLLAMA_HOST=http://host.docker.internal:11434"
  fi
  echo ""
  echo "SKIP: Remaining model checks skipped (Ollama not reachable)"
fi
```

```bash
# 6d. Ollama version check
OLLAMA_VER=$(ollama --version 2>&1 | grep -oE "[0-9]+\.[0-9]+\.[0-9]+")
if [ -n "$OLLAMA_VER" ]; then
  MAJOR=$(echo "$OLLAMA_VER" | cut -d. -f1)
  MINOR=$(echo "$OLLAMA_VER" | cut -d. -f2)
  if [ "$MAJOR" -eq 0 ] && [ "$MINOR" -lt 5 ]; then
    echo "⚠ Ollama $OLLAMA_VER may be too old — update recommended"
    echo "  Some features (tool calling, keep_alive) require recent versions"
  else
    echo "✓ Ollama version $OLLAMA_VER"
  fi
fi
```

### 6e. ramalama provider

```bash
# Check if ramalama CLI is installed
if command -v ramalama >/dev/null 2>&1; then
  RAMALAMA_VERSION=$(ramalama --version 2>&1 | head -1)
  echo "✓ ramalama installed: $RAMALAMA_VERSION"

  # Check container runtime
  if command -v podman >/dev/null 2>&1; then
    echo "✓ podman available: $(podman --version 2>&1 | head -1)"
  elif command -v docker >/dev/null 2>&1; then
    echo "✓ docker available: $(docker --version 2>&1 | head -1)"
  else
    echo "⚠ No container runtime found (podman or docker required for ramalama)"
  fi
else
  echo "ℹ ramalama not installed (optional — container-based LLM serving)"
fi

# Check TINYCODE_RAMALAMA_HOST env var
if [ -n "$TINYCODE_RAMALAMA_HOST" ]; then
  RAMALAMA_HOST=$(echo "$TINYCODE_RAMALAMA_HOST" | sed 's|/+$||')
  echo "  TINYCODE_RAMALAMA_HOST: $RAMALAMA_HOST"
  if curl -sf --max-time 3 "${RAMALAMA_HOST}/v1/models" >/dev/null 2>&1; then
    echo "✓ ramalama API responding at ${RAMALAMA_HOST}"
    RAMALAMA_MODELS=$(curl -sf --max-time 3 "${RAMALAMA_HOST}/v1/models" 2>/dev/null | grep -oE '"id"\s*:\s*"[^"]*"' | sed 's/"id"\s*:\s*"//;s/"//')
    RAMALAMA_COUNT=$(echo "$RAMALAMA_MODELS" | grep -c . 2>/dev/null || echo 0)
    echo "  Serving $RAMALAMA_COUNT model(s)"
    echo "$RAMALAMA_MODELS" | while read -r name; do
      echo "    $name"
    done
  else
    echo "✗ ramalama API not reachable at ${RAMALAMA_HOST}"
    echo "  Fix: Verify ramalama is serving: ramalama serve <model>"
  fi
else
  echo "ℹ TINYCODE_RAMALAMA_HOST not set (set to enable ramalama provider)"
  echo "  Example: export TINYCODE_RAMALAMA_HOST=http://localhost:8080"
fi
```

### 7. Model availability

All JSON parsing uses grep/sed/awk — no python3 required.

```bash
# 7a. List available models
# Extract model names from the /api/tags JSON response
if [ -n "$TAGS_JSON" ]; then
  MODEL_NAMES=$(echo "$TAGS_JSON" | grep -oE '"name"\s*:\s*"[^"]*"' | sed 's/"name"\s*:\s*"//;s/"//')
  MODEL_COUNT=$(echo "$MODEL_NAMES" | grep -c . 2>/dev/null || echo 0)

  if [ "$MODEL_COUNT" -eq 0 ]; then
    echo "✗ No models pulled in Ollama"
    echo "  Fix: ollama pull qwen3.5:9b"
  else
    echo "✓ $MODEL_COUNT model(s) available in Ollama"
    echo "$MODEL_NAMES" | head -10 | while read -r name; do
      echo "    $name"
    done
    if [ "$MODEL_COUNT" -gt 10 ]; then
      echo "    ... and $((MODEL_COUNT - 10)) more"
    fi
  fi
else
  echo "✗ Cannot list Ollama models (API unreachable)"
fi
```

```bash
# 7b. Is the configured default model available?
CONFIG_FILE=""
for f in "${HOME}/.config/tinycode/tinycode.jsonc" "${HOME}/.config/tinycode/tinycode.json" "${HOME}/.config/tinycode/config.json"; do
  [ -f "$f" ] && CONFIG_FILE="$f" && break
done

CONFIGURED_MODEL=""
MODEL_ID=""
PROVIDER_ID=""

if [ -n "$CONFIG_FILE" ]; then
  # Strip JSONC comments and extract model field
  CONFIGURED_MODEL=$(sed 's|//.*$||' "$CONFIG_FILE" | sed 's|/\*.*\*/||g' | grep -oE '"model"\s*:\s*"[^"]*"' | head -1 | sed 's/"model"\s*:\s*"//;s/"//')

  if [ -n "$CONFIGURED_MODEL" ]; then
    PROVIDER_ID=$(echo "$CONFIGURED_MODEL" | cut -d'/' -f1)
    MODEL_ID=$(echo "$CONFIGURED_MODEL" | cut -d'/' -f2-)
    echo "  Configured model: $CONFIGURED_MODEL (from $CONFIG_FILE)"

    if [ "$PROVIDER_ID" = "ollama" ] && [ -n "$MODEL_NAMES" ]; then
      if echo "$MODEL_NAMES" | grep -qx "$MODEL_ID"; then
        echo "✓ Model '$MODEL_ID' is available in Ollama"
      elif echo "$MODEL_NAMES" | grep -qx "${MODEL_ID}:latest"; then
        echo "✓ Model '$MODEL_ID' found as '${MODEL_ID}:latest'"
      else
        BASE_NAME=$(echo "$MODEL_ID" | cut -d: -f1)
        SIMILAR=$(echo "$MODEL_NAMES" | grep "^${BASE_NAME}" | head -3 | tr '\n' ', ' | sed 's/,$//')
        if [ -n "$SIMILAR" ]; then
          echo "⚠ Model '$MODEL_ID' not found, but similar exist: $SIMILAR"
          echo "  Fix: Update config to match, or: ollama pull $MODEL_ID"
        else
          echo "✗ Model '$MODEL_ID' is NOT available in Ollama"
          echo "  Fix: ollama pull $MODEL_ID"
        fi
      fi
    fi
  else
    echo "  No model configured (will use auto-detected default)"
  fi
else
  echo "  No tinycode config file found"
fi
```

```bash
# 7c. Recommended model check
if [ -n "$MODEL_NAMES" ]; then
  if echo "$MODEL_NAMES" | grep -qE "^qwen3\.5:9b$|^qwen3\.5:9b:latest$"; then
    echo "✓ Recommended model qwen3.5:9b is available"
  else
    echo "ℹ Recommended model qwen3.5:9b is not pulled"
    echo "  Suggestion: ollama pull qwen3.5:9b"
    echo "  (Benchmark champion — 14/15 score, 93% of Claude Opus)"
  fi
fi
```

```bash
# 7d. Model size vs available RAM
if [ "$IS_MAC" = "true" ]; then
  TOTAL_RAM_BYTES=$(sysctl -n hw.memsize 2>/dev/null)
  if [ -n "$TOTAL_RAM_BYTES" ]; then
    TOTAL_RAM_GB=$((TOTAL_RAM_BYTES / 1073741824))
    echo "  System RAM: ${TOTAL_RAM_GB}GB"
    if [ "$TOTAL_RAM_GB" -lt 16 ]; then
      echo "⚠ Only ${TOTAL_RAM_GB}GB RAM — models larger than 7B may be very slow"
      echo "  Recommended: qwen2.5:latest (7B) or llama3.2:latest (3B)"
    elif [ "$TOTAL_RAM_GB" -lt 32 ]; then
      echo "ℹ ${TOTAL_RAM_GB}GB RAM — models up to ~12B should work well"
    else
      echo "✓ ${TOTAL_RAM_GB}GB RAM — sufficient for most local models"
    fi
  fi
elif [ -f /proc/meminfo ]; then
  TOTAL_RAM_KB=$(awk '/MemTotal/ {print $2}' /proc/meminfo)
  if [ -n "$TOTAL_RAM_KB" ]; then
    TOTAL_RAM_GB=$((TOTAL_RAM_KB / 1048576))
    echo "  System RAM: ${TOTAL_RAM_GB}GB"
    if [ "$TOTAL_RAM_GB" -lt 16 ]; then
      echo "⚠ Only ${TOTAL_RAM_GB}GB RAM — models larger than 7B may be very slow"
    fi
  fi
fi
```

### 8. Model functionality (warmup probe)

This sends a tool-call probe to verify the model responds and supports tool calling. Uses a 120s timeout since cold-loading a model can be slow. The probe also pre-loads the model into GPU memory for faster first use.

**Skip this check if Ollama is not reachable (Check 6c failed).**

```bash
# Determine which model to probe
TARGET_MODEL="${MODEL_ID}"
if [ -z "$TARGET_MODEL" ] && [ -n "$MODEL_NAMES" ]; then
  TARGET_MODEL=$(echo "$MODEL_NAMES" | head -1)
fi

if [ -n "$TARGET_MODEL" ]; then
  echo "  Probing model: $TARGET_MODEL (this may take a moment on first load)..."
  START_S=$(date +%s)

  PROBE_RESULT=$(curl -sf --max-time 120 "${OLLAMA_HOST}/api/chat" \
    -H "Content-Type: application/json" \
    -d "{
      \"model\": \"$TARGET_MODEL\",
      \"messages\": [{\"role\": \"user\", \"content\": \"Call the ready tool to confirm you are ready.\"}],
      \"tools\": [{
        \"type\": \"function\",
        \"function\": {
          \"name\": \"ready\",
          \"description\": \"Confirm model is loaded and ready\",
          \"parameters\": {\"type\": \"object\", \"properties\": {}}
        }
      }],
      \"stream\": false,
      \"keep_alive\": \"10m\"
    }" 2>/dev/null)

  END_S=$(date +%s)
  DURATION_S=$((END_S - START_S))

  if [ -z "$PROBE_RESULT" ]; then
    echo "✗ Model '$TARGET_MODEL' did not respond within 120s"
    echo "  The model may be too large for available memory."
    echo "  Check: ollama ps   (loaded models)"
    echo "  Check: ollama logs (errors)"
  else
    echo "✓ Model responded (${DURATION_S}s)"

    # Tool calling check — look for tool_calls in the response
    if echo "$PROBE_RESULT" | grep -q '"tool_calls"'; then
      echo "✓ Tool calling supported"
    else
      echo "✗ Tool calling NOT supported by this model"
      echo "  Without tool calling, the model cannot read/write files or run commands."
      echo "  Models with tool calling: qwen3.5:9b, north-mini-code-1.0, gemma4:12b"
      echo "  Models WITHOUT: granite, codellama, deepseek-r1 (distilled)"
      echo "  Fix: ollama pull qwen3.5:9b && update config"
    fi

    # Load time assessment
    if [ "$DURATION_S" -gt 60 ]; then
      echo "⚠ Load time: ${DURATION_S}s (very slow — model may be too large for hardware)"
    elif [ "$DURATION_S" -gt 20 ]; then
      echo "ℹ Load time: ${DURATION_S}s (normal for cold start)"
    else
      echo "✓ Load time: ${DURATION_S}s"
    fi

    # Check if model stayed loaded (keep_alive working)
    PS_RESULT=$(curl -sf --max-time 3 "${OLLAMA_HOST}/api/ps" 2>/dev/null)
    if [ -n "$PS_RESULT" ]; then
      if echo "$PS_RESULT" | grep -q '"name"'; then
        LOADED_NAME=$(echo "$PS_RESULT" | grep -oE '"name"\s*:\s*"[^"]*"' | head -1 | sed 's/"name"\s*:\s*"//;s/"//')
        echo "✓ Model loaded in memory: $LOADED_NAME"
      else
        echo "⚠ No models showing in ollama ps — keep_alive may not be working"
      fi
    fi
  fi
else
  echo "SKIP No model available to probe"
fi
```

### 9. Mac-specific checks

```bash
if [ "$IS_MAC" = "true" ]; then
  # GPU / Metal acceleration
  if system_profiler SPDisplaysDataType 2>/dev/null | grep -q "Metal.*Supported"; then
    echo "✓ Metal GPU acceleration available"
  else
    echo "⚠ Metal GPU not detected — Ollama will use CPU only (very slow)"
  fi

  # Check if Ollama is native arm64 on Apple Silicon
  if sysctl -n machdep.cpu.brand_string 2>/dev/null | grep -q "Apple"; then
    OLLAMA_ARCH=$(file "$(which ollama)" 2>/dev/null | grep -o "arm64\|x86_64")
    if [ "$OLLAMA_ARCH" = "x86_64" ]; then
      echo "⚠ Ollama is running as x86_64 (Rosetta) on Apple Silicon — 2-3x slower"
      echo "  Fix: Reinstall the arm64 build from https://ollama.com/download"
    elif [ "$OLLAMA_ARCH" = "arm64" ]; then
      echo "✓ Ollama is native arm64"
    fi
  fi

  # RAM pressure — swap check
  SWAP_USED=$(sysctl vm.swapusage 2>/dev/null | grep -oE "used = [0-9.]+" | grep -oE "[0-9.]+")
  if [ -n "$SWAP_USED" ]; then
    # Swap is reported in MB; warn if > 1GB used
    SWAP_MB=$(echo "$SWAP_USED" | cut -d. -f1)
    if [ "$SWAP_MB" -gt 1024 ] 2>/dev/null; then
      echo "⚠ Significant swap usage (${SWAP_MB}MB) — model inference will be slow"
      echo "  Fix: Close memory-heavy apps (Docker, Chrome, IDEs)"
    fi
  fi
fi
```

### 10. tinycode ↔ Provider integration

```bash
PROVIDER_RESULT=$(curl -sf --max-time 5 "${TINYCODE_API}/provider" 2>/dev/null)

if [ -z "$PROVIDER_RESULT" ]; then
  echo "✗ tinycode API not responding on ${TINYCODE_API}"
else
  # Check for connected providers using the "connected" array
  CONNECTED=$(echo "$PROVIDER_RESULT" | grep -oE '"connected"\s*:\s*\[[^]]*\]' | grep -oE '"[a-z][a-z0-9_-]*"' | tr -d '"' | tr '\n' ', ' | sed 's/,$//')

  if [ -n "$CONNECTED" ]; then
    echo "✓ Connected providers: $CONNECTED"
  else
    echo "✗ No connected providers"
  fi

  # Check if Ollama specifically is connected
  if echo "$CONNECTED" | grep -q "ollama"; then
    echo "✓ Ollama provider connected to tinycode"
  elif [ -n "$CONNECTED" ]; then
    echo "⚠ Ollama not in connected providers"
    echo "  Possible causes:"
    echo "    - Ollama was not running when tinycode started"
    echo "    - ollama is in disabled_providers config"
    echo "  Fix: Restart tinycode after confirming Ollama is running"
  fi

  # Check for provider filtering in config
  if [ -n "$CONFIG_FILE" ]; then
    DISABLED=$(sed 's|//.*$||' "$CONFIG_FILE" | sed 's|/\*.*\*/||g' | grep -oE '"disabled_providers"\s*:\s*\[[^]]*\]' | grep -o '"ollama"')
    if [ -n "$DISABLED" ]; then
      echo "⚠ ollama is in disabled_providers — it will be hidden from tinycode"
      echo "  Fix: Remove ollama from disabled_providers in config"
    fi

    ENABLED=$(sed 's|//.*$||' "$CONFIG_FILE" | sed 's|/\*.*\*/||g' | grep -oE '"enabled_providers"\s*:\s*\[[^]]*\]')
    if [ -n "$ENABLED" ] && ! echo "$ENABLED" | grep -q '"ollama"'; then
      echo "⚠ enabled_providers is set but does not include ollama"
      echo "  Fix: Add ollama to enabled_providers, or remove the list entirely"
    fi
  fi
fi
```

### 11. vLLM / Custom Provider Health (container/server deployments)

```bash
if [ -n "$PROVIDER_RESULT" ]; then
  # Check for non-ollama providers with potential issues
  # Look for vllm provider ID collision
  if echo "$PROVIDER_RESULT" | grep -oE '"id"\s*:\s*"[^"]*"' | grep -q '"vllm"'; then
    echo "⚠ Provider ID 'vllm' may collide with auto-discovery"
    echo "  Fix: Use a unique ID like 'vllm-custom' in config"
  fi

  # Check for small output limits (< 2000) on any model
  SMALL_OUTPUT=$(echo "$PROVIDER_RESULT" | grep -oE '"output"\s*:\s*[0-9]+' | sed 's/"output"\s*:\s*//' | while read -r val; do
    if [ "$val" -gt 0 ] && [ "$val" -lt 2000 ] 2>/dev/null; then
      echo "$val"
    fi
  done)
  if [ -n "$SMALL_OUTPUT" ]; then
    echo "⚠ Some models have output limit < 2000 tokens"
    echo "  Thinking models (qwen3, deepseek) need output >= 4000 for <think> blocks"
    echo "  Fix: Set explicit output limit in provider config override"
  fi

  # Report healthy custom providers
  CUSTOM_PROVIDERS=$(echo "$PROVIDER_RESULT" | grep -oE '"id"\s*:\s*"[^"]*"' | sed 's/"id"\s*:\s*"//;s/"//' | grep -v "ollama" | sort -u | tr '\n' ', ' | sed 's/,$//')
  if [ -n "$CUSTOM_PROVIDERS" ]; then
    echo "  Custom providers: $CUSTOM_PROVIDERS"
  fi

  echo ""
  echo "  Recommended config pattern for vLLM overrides:"
  echo '  { "provider": { "vllm-custom": { "npm": "@ai-sdk/openai-compatible",'
  echo '    "api": "http://<host>:<port>/v1",'
  echo '    "models": { "<model>": { "reasoning": true, "limit": { "context": 4000, "output": 4000 } } } } } }'
fi
```

### 12. tmux / swarm readiness
```bash
if command -v tmux >/dev/null 2>&1; then
  echo "✓ tmux available: $(tmux -V)"
else
  echo "ℹ tmux not installed — /swarm skill will not work (optional)"
fi
```

### 13. Disk space
```bash
df -h / "$HOME/.local/share/tinycode" 2>/dev/null | tail -n +2 | while read fs size used avail pct mount; do
  pct_num=${pct%\%}
  if [ "$pct_num" -gt 90 ]; then
    echo "✗ $mount: ${pct} used ($avail free) — LOW DISK SPACE"
  else
    echo "✓ $mount: ${pct} used ($avail free)"
  fi
done
```

### 14. Summary (aggregation step)

After running all checks, compile results into three summary sections:
- **Issues Found**: all lines that produced ✗ or ⚠ markers
- **Fixes Applied**: automatic actions taken (directory creation, file copying)
- **Manual Actions Needed**: all "Fix:" suggestions that require user action

## Output format

```
## tinycode Doctor Report

### Environment
- Platform: [Darwin/Linux] [arch]
- Container: [true/false]
- Working directory: [path]
- User: [uid]
- Ollama endpoint: [url]

### Checks
[results from each check above]

### Issues Found
- [list of ✗ items]

### Fixes Applied
- [list of automatic fixes taken]

### Manual Actions Needed
- [anything that couldn't be auto-fixed]
```

## Important

- Run ALL checks even if early ones fail, EXCEPT: skip model checks (7-8) if Ollama is not reachable (6c failed)
- Apply fixes automatically where safe (directory creation, file copying)
- Do NOT modify user config files (config.json, tinycode.json, tinycode.jsonc) — print recommended config for the user
- Do NOT auto-pull models — they are multi-GB downloads requiring user consent
- Do NOT restart tinycode — tell the user to restart if needed
- Report everything found, even if all checks pass
