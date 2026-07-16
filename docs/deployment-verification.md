# Deployment Verification Guide

A companion to the `/tc-doctor` skill. While tc-doctor automates all checks, this guide explains what each check does, why it matters, and how to manually troubleshoot when things go wrong.

---

## Quick Start: Run the Full Diagnostic

```bash
/tc-doctor
```

This runs 14 automated checks across your environment:
1. Directory structure
2. Agents (container only)
3. Skills (container only)
4. System tools
5. OpenShift oc CLI (if cluster-admin mode)
6. Ollama installation & health
7. Model availability
8. Model functionality (warmup probe)
9. Mac-specific checks
10. tinycode ↔ Provider integration
11. vLLM / Custom providers
12. tmux / swarm readiness
13. Disk space
14. Summary (issues, fixes, manual actions)

Read the report carefully. All `✗` items require action. All `⚠` items are warnings (may not break functionality). All `✓` items passed. All `ℹ` items are informational.

---

## Prerequisites Verification

Before running any checks, verify the basics are in place.

### Ollama Installation

**Check:** Is Ollama installed?

```bash
ollama --version
```

**Good output:** `ollama version 0.5.0` or similar (0.5+ required).

**Bad output:** `command not found`.

**Fix:** Install Ollama from [https://ollama.com/download](https://ollama.com/download)

```bash
# macOS
brew install ollama

# Linux
curl -fsSL https://ollama.com/install.sh | sh

# Windows
Download installer from ollama.com
```

### Ollama Running

**Check:** Is the ollama process running?

```bash
pgrep -f "ollama serve" || echo "NOT RUNNING"
```

**Good output:** Process ID like `12345`.

**Bad output:** `NOT RUNNING`.

**Fix:** Start ollama in a separate terminal:

```bash
ollama serve
```

On macOS, the Ollama app can also run in the background. Open the app to ensure it starts automatically on login.

### Ollama API Reachable

**Check:** Can you reach the Ollama API?

```bash
curl -s http://localhost:11434/api/tags | head -20
```

**Good output:** JSON with `models` array (even if empty).

**Bad output:** `Connection refused` or timeout.

**Possible causes:**
- Ollama is not running (see above)
- Ollama is listening on a different port
- Ollama is on a different host (container deployment)

**Fix:**

```bash
# If using non-standard port
export TINYCODE_OLLAMA_HOST=http://localhost:YOUR_PORT

# If Ollama is on another machine (e.g., remote server or Docker host)
export TINYCODE_OLLAMA_HOST=http://ollama-host:11434

# In container, reach the host machine
export TINYCODE_OLLAMA_HOST=http://host.docker.internal:11434

# For Kubernetes services
export TINYCODE_OLLAMA_HOST=http://ollama-service.default.svc.cluster.local:11434
```

---

## Model Health

### Is the Model Pulled?

**Check:** List available models in Ollama.

```bash
curl -s http://localhost:11434/api/tags | grep -o '"name"[^,]*' | grep -o '"[^"]*"$'
```

Or simpler:

```bash
ollama list
```

**Good output:** At least one model listed, e.g., `qwen3.5:9b`.

**Bad output:** `models: []` or error.

**Fix:** Pull a model:

```bash
# Recommended: qwen3.5:9b (9B, benchmark champion 14/15, excellent tool calling)
ollama pull qwen3.5:9b

# Or other options
ollama pull mistral          # 7B, fast
ollama pull llama3.2         # 3B, very fast but limited
ollama pull gemma4:12b       # 12B, good tool support
```

**Note:** Model pulls are large (3-45GB) and take 10+ minutes.

### Is the Model Loaded?

**Check:** Is the model currently in GPU/memory?

```bash
curl -s http://localhost:11434/api/ps | grep -o '"name"[^,]*'
```

Or simpler:

```bash
ollama ps
```

**Good output:** Your model name listed, e.g., `qwen3.5:9b`.

**Bad output:** Empty, or a different model.

**Why it matters:** Loaded models respond immediately. Unloaded models must be loaded first (cold start = 20-120s).

**How to keep a model loaded:**

tinycode sends `keep_alive: "10m"` with all requests, keeping the model in memory for 10 minutes after the last request. If you're idle longer than that, the model unloads.

For persistent loading, edit Ollama's config (platform-dependent):

- **macOS:** `~/.ollama/models/` — check if model is pinned
- **Linux:** `/usr/share/ollama/.ollama/models/`

Or manually pre-load:

```bash
# This loads the model and keeps it warm
ollama run qwen3.5:9b "Say you are ready"
# Exit with Ctrl+D after it responds
```

### Does the Model Support Tool Calling?

**Check:** Run the warmup probe (automatic in tc-doctor, 8th check).

```bash
curl -s --max-time 120 http://localhost:11434/api/chat \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen3.5:9b",
    "messages": [{"role": "user", "content": "Call the ready tool."}],
    "tools": [{
      "type": "function",
      "function": {
        "name": "ready",
        "description": "Confirm you are ready",
        "parameters": {"type": "object", "properties": {}}
      }
    }],
    "stream": false,
    "keep_alive": "10m"
  }' | grep -o '"tool_calls"' || echo "NO TOOL CALLS"
```

**Good output:** Contains `"tool_calls"` (model made a function call).

**Bad output:** `NO TOOL CALLS` (model only returned text).

**Why it matters:** Without tool calling, models can't read files, write code, or run commands. tinycode detects this and disables tools for models that don't support them.

**Models with good tool calling:**
- `qwen3.5:9b` ✓ (benchmark champion)
- `qwen3:14b` ✓
- `north-mini-code-1.0` ✓
- `gemma4:12b` ✓

**Models WITHOUT tool calling:**
- `granite` ✗ (no tools)
- `codellama` ✗ (no tools)
- `deepseek-r1:distilled` ✗ (no tools)
- `llama3.2:3b` ✗ (too small)

**Fix:** Switch to a model with tool support:

```bash
ollama pull qwen3.5:9b
# Then in tinycode: <leader>m → select qwen3.5:9b
```

### Model Size vs Available RAM

**Check:** How much RAM do you have?

```bash
# macOS
sysctl -n hw.memsize | awk '{print $1 / 1073741824 " GB"}'

# Linux
free -h | grep Mem | awk '{print $2}'
```

**Good combinations:**
- **8GB RAM:** qwen2.5:latest (7B), llama3.2 (3B) — slow but works
- **16GB RAM:** qwen3.5:9b ✓ (recommended), mistral, neural-chat
- **32GB+ RAM:** qwen3:14b, gemma4:12b, larger models

**Rule of thumb:** Model size + overhead should be ≤ 80% of available RAM. A 9B model uses ~6-9GB depending on quantization.

**Check RAM pressure on macOS:**

```bash
sysctl vm.swapusage | grep used
```

If swap usage is > 1GB, you're hitting disk (very slow). Close memory-heavy apps: Docker, Chrome, IDEs.

---

## Provider Integration

### tinycode Sees the Provider

**Check:** Is the provider connected to tinycode?

```bash
curl -s http://localhost:4096/provider | grep -o '"connected"\s*:\s*\[[^]]*\]'
```

**Good output:** `"connected": ["ollama"]` or similar.

**Bad output:** `"connected": []` (no providers).

**Fix:**

1. Verify provider is running (`curl http://localhost:11434/api/tags`).
2. Restart tinycode (provider discovery runs at startup).
3. Check `~/.config/tinycode/config.json` for filters:

```json
{
  "disabled_providers": ["ollama"]  // ← This hides ollama
}
```

Remove `ollama` from `disabled_providers` if present.

### Provider Filtering

**Check:** Is the provider being filtered out?

```bash
grep -E 'enabled_providers|disabled_providers' ~/.config/tinycode/config.json
```

**Good:** No filter, or `ollama` is in `enabled_providers`.

**Bad:** `ollama` is in `disabled_providers`.

**Why it matters:** Filters apply during startup. If a provider is disabled, tinycode won't connect to it even if it's running.

**Fix:** Edit config to enable the provider:

```json
{
  "enabled_providers": ["ollama"],
  // or
  "disabled_providers": ["openai", "anthropic"]
}
```

Or remove the filter entirely to see all providers.

### Model Name Format

**Check:** Is your configured model using the right format?

```bash
grep -o '"model"\s*:\s*"[^"]*"' ~/.config/tinycode/config.json | head -1
```

**Good format:** `"model": "ollama/qwen3.5:9b"` (provider/model).

**Bad format:** `"model": "qwen3.5:9b"` (no provider prefix).

**Fix:** Update config with the provider:

```json
{
  "model": "ollama/qwen3.5:9b"
}
```

---

## ramalama Deployment

### Is ramalama CLI Installed?

```bash
ramalama --version
```

**Good output:** Version info.

**Bad output:** `command not found`.

**Fix:** Install ramalama:

```bash
# Requires Go 1.22+
go install github.com/containers/ramalama/cmd/ramalama@latest

# Or use the container image directly (no CLI needed)
podman pull quay.io/ramalama/ramalama
```

### Is the Container Runtime Available?

```bash
podman --version || docker --version || echo "NO CONTAINER RUNTIME"
```

**Good output:** podman or docker version.

**Bad output:** `NO CONTAINER RUNTIME`.

**Fix:** Install podman or docker.

### Is ramalama Serving?

```bash
curl -s http://localhost:8080/v1/models | grep -o '"id"[^,]*' | head -5
```

**Good output:** Model IDs listed.

**Bad output:** `Connection refused`.

**Fix:** Start ramalama:

```bash
ramalama serve ollama://qwen3.5:9b
```

Then set the environment variable:

```bash
export TINYCODE_RAMALAMA_HOST=http://localhost:8080
```

Note: ramalama auto-selects ports in 8080-8180. Check `ramalama ps` for the actual port.

---

## vLLM / Custom Providers

### Can You Reach the Endpoint?

```bash
curl -s http://your-vllm-host:8000/v1/models | head -20
```

**Good output:** JSON with models array.

**Bad output:** `Connection refused` or `No route to host`.

**Fixes:**
1. Verify vLLM is running: `ps aux | grep vllm`
2. Check port: vLLM defaults to 8000, not 11434
3. Check hostname: If on a different machine:
   ```bash
   export TINYCODE_VLLM_HOST=http://vllm-server:8000
   ```
4. Check firewall:
   ```bash
   sudo firewall-cmd --add-port=8000/tcp --permanent
   sudo firewall-cmd --reload
   ```

### Provider ID Collisions

**Check:** Do you have duplicate provider IDs?

```bash
grep -o '"id"\s*:\s*"[^"]*"' ~/.config/tinycode/config.json | sort | uniq -d
```

**Good:** No duplicates.

**Bad:** Duplicates like `"vllm"` (auto-discovered AND configured).

**Fix:** Use unique IDs in config:

```json
{
  "provider": {
    "vllm-custom": {
      "npm": "@ai-sdk/openai-compatible",
      "api": "http://your-vllm:8000/v1"
    }
  }
}
```

---

## Container & Kubernetes Deployment

### GPU Exposure (Kubernetes)

**Check:** Are GPUs visible to the container?

```bash
nvidia-smi
# or
lspci | grep -i gpu
```

**Good output:** GPU device listed (e.g., NVIDIA RTX 3090).

**Bad output:** `command not found` or no devices.

**Fix:** Pass GPU resource request:

```yaml
resources:
  limits:
    nvidia.com/gpu: 1
  requests:
    nvidia.com/gpu: 1
```

### Network Policies

**Check:** Can tinycode reach model endpoints?

```bash
curl -s http://ollama-service.default.svc.cluster.local:11434/api/tags
```

**Bad output:** `Connection refused` or timeout.

**Fix:**
1. Verify service is running: `kubectl get svc -n default ollama-service`
2. Check network policies don't block traffic
3. If using different namespace: `export TINYCODE_OLLAMA_HOST=http://ollama-service.MODEL_NAMESPACE.svc.cluster.local:11434`

### Memory Limits vs Model Size

**Check:** What's the container's memory limit?

```bash
# In container
cat /sys/fs/cgroup/memory/memory.limit_in_bytes | numfmt --to=iec
```

**Rule:** Limit should be model size + 2GB overhead.

- **qwen3.5:9b:** 12GB limit recommended
- **qwen3:14b:** 18GB limit
- **mistral:7b:** 10GB limit

**Fix in Kubernetes:**

```yaml
resources:
  requests:
    memory: "12Gi"
  limits:
    memory: "14Gi"
```

### vLLM Auto-Discovery (K8s)

**Check:** Does the K8s service have the right annotations?

```bash
kubectl get svc vllm-service -o yaml | grep -A5 annotations
```

**Good:**

```yaml
annotations:
  tinycode/discovery: "true"
  tinycode/model: "qwen3.5:9b"
```

**Fix:** Add annotations:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: vllm-service
  annotations:
    tinycode/discovery: "true"
    tinycode/model: "qwen3.5:9b"
```

### tinycode-Operator Status

**Check:** Is the operator deployed and managing instances?

```bash
oc get pods -n tinycode-system | grep operator
oc get tinycodeinstances -n tinycode
```

**Good:** Operator pod running, instances in Active state.

**Bad:** Operator pod missing or in error state.

**Fix:** Reinstall the operator:

```bash
oc apply -f https://raw.githubusercontent.com/bobbyjohnstx/tinycode-operator/main/config/manager/manager.yaml
```

---

## Mac-Specific Checks

### Metal GPU Acceleration

**Check:** Does your Mac support Metal GPU?

```bash
system_profiler SPDisplaysDataType | grep "Metal: Supported"
```

**Good output:** Shows Metal is supported.

**Bad output:** Metal not found (using CPU only, very slow).

**Context:** All recent Macs (2016+) support Metal. If not present, verify with:

```bash
sysctl -n machdep.cpu.brand_string
```

If it says "Apple", Metal should be available.

### Ollama Architecture (Rosetta Check)

**Check:** Is Ollama running natively (arm64) or through Rosetta (x86_64)?

```bash
file $(which ollama) | grep -o "arm64\|x86_64"
```

**Good output:** `arm64` (native Apple Silicon).

**Bad output:** `x86_64` (Rosetta emulation, 2-3x slower).

**Fix:** Reinstall the native arm64 version:

```bash
# Uninstall
brew uninstall ollama

# Reinstall (will auto-detect arm64)
brew install ollama
```

### RAM Pressure & Swap

**Check:** How much swap are you using?

```bash
sysctl vm.swapusage | grep used | awk '{print $3}'
```

**Good:** 0-100MB.

**Bad:** > 1GB (disk is being used as RAM, very slow).

**Fix:**
1. Close memory-heavy apps: Docker, Chrome, IDEs
2. Reduce model size: Use qwen3.5:9b (9B) instead of larger models
3. Check Activity Monitor → Memory tab

---

## Performance Verification

### Expected Response Times

These are baseline expectations for the first request (cold start).

| Model Size | RAM | Cold Start | Warm (keep_alive) | Notes |
| --- | --- | --- | --- | --- |
| 3B | 8GB | 10-20s | 1-3s | llama3.2 (very limited) |
| 7B | 8GB | 30-60s | 2-5s | mistral, qwen2.5:latest |
| 9B | 16GB | 20-40s | 2-4s | qwen3.5:9b (recommended) |
| 12B | 24GB | 40-80s | 3-6s | gemma4:12b, qwen3:14b |
| 14B+ | 32GB+ | 60-120s | 4-8s | Large models, slower |

**Why cold starts are slow:** The model must be loaded from disk into GPU/CPU memory. This is a one-time cost per model. After 10 minutes of idle time (configurable via `keep_alive`), the model unloads and the next request is slow again.

### Diagnosing Slow Responses

**Is the model cold or warm?**

```bash
# Check if loaded
ollama ps

# If not listed, it will be loaded on the next request (slow)
# If listed, it's warm (fast)
```

**Is Ollama CPU-bound?**

```bash
top -n1 | grep ollama
```

If `%CPU` is near 100% and saturated, Ollama is processing. Wait or switch to a smaller model.

**Is the model thrashing (swapping)?**

```bash
# macOS
sysctl vm.swapusage | grep used

# Linux
free | grep Swap | awk '{print $3 " / " $2}'
```

If swap > 1GB, model is too large for available RAM. Switch to a smaller model or add RAM.

### Per-Agent Tool Scoping Impact

**Context:** Each agent gets only its required tools, not the full ~18,500 token tool definition. This speeds up prompt processing.

**Check:** Are agents responsive?

```bash
/ask architect "what files are in src/?"
```

**Expected:** ~1-5s response time on a 9B model.

**Slow response:** > 10s suggests:
1. Model is cold (see "Is the model cold or warm?" above)
2. Model is too small for the task (switch to qwen3.5:9b)
3. System is overloaded

### Context Window Utilization

**Check:** How full is your session context?

In the TUI:

```
<leader>s    # View status
```

Look for "context usage: XXX / YYYY".

**Good:** < 80% of context window.

**Bad:** > 90% (approaching overflow, compaction will trigger).

**Fix:** If approaching limit, compact the session:

```
<leader>c    # Compact session
```

This removes unnecessary messages and older tool outputs, shrinking the session to ~30% of original size.

---

## Log Locations

### Ollama Logs

**macOS:**

```bash
tail -f ~/.ollama/logs/server.log
```

**Linux:**

```bash
journalctl --user-unit=ollama -f
# or
tail -f ~/.ollama/logs/server.log
```

**What to look for:**
- Model load/unload events: `loaded the model`
- Memory errors: `out of memory`, `CUDA error`
- Connection errors: `listen tcp :11434: address already in use`

### tinycode Logs

**TUI mode:**

```bash
# In another terminal, while tinycode TUI is running
tail -f ~/.local/share/tinycode/logs/*.log
```

**Serve mode:**

```bash
bun dev serve 2>&1 | tee tinycode.log
# Log goes to terminal; check for errors at startup
```

**What to look for:**
- Provider connection: `Ollama provider connected`
- Model warmup: `Warming model qwen3.5:9b`
- Tool calls: `executing tool shell`
- Errors: `HTTP 500`, `connection refused`

### vLLM Logs

**Check the vLLM startup output:**

```bash
# If running directly
vllm serve --model qwen3.5:9b 2>&1 | tee vllm.log

# If in container
podman logs -f <container-id>
```

**What to look for:**
- Model loaded: `INFO: Loaded xxx tokens` or `model ready`
- GPU allocation: `Allocated X.XXG on cuda:0`
- Request processing: `prompt_ids=`

---

## Troubleshooting Checklists

### "Model not responding" (timeout)

1. **Is Ollama running?**
   ```bash
   curl http://localhost:11434/api/tags
   ```

2. **Is the model pulled?**
   ```bash
   ollama list | grep qwen3.5:9b
   ```

3. **Is the model loaded (first request)?**
   ```bash
   ollama ps
   ```
   First request loads the model (20-120s). Subsequent requests are fast.

4. **Is the model too large for your RAM?**
   ```bash
   free -h | grep Mem
   ```
   Model size should fit in < 80% of available RAM.

5. **Is the system overloaded?**
   ```bash
   top -n1 | head -15
   ```
   Check CPU and memory usage. Close other apps if needed.

### "Tool calls keep failing"

1. **Does the model support tool calling?**
   Run `/tc-doctor` (check 8).

2. **Is the model large enough?**
   Models < 7B don't reliably support tools. Use qwen3.5:9b or larger.

3. **Are tool-call repairs failing?**
   tinycode auto-repairs common JSON issues. After 3 failures, check logs:
   ```bash
   tail ~/.local/share/tinycode/logs/session.log | grep "tool_call"
   ```

### "Provider not found after connecting"

1. **Is the provider still running?**
   ```bash
   curl http://localhost:11434/api/tags
   ```

2. **Restart tinycode:**
   Provider discovery runs at startup. Exit and re-enter tinycode.

3. **Check filters:**
   ```bash
   grep -E 'enabled|disabled' ~/.config/tinycode/config.json
   ```

### "Server won't start"

1. **Is port 4096 in use?**
   ```bash
   lsof -i :4096
   kill -9 <PID>
   ```

2. **Use a different port:**
   ```json
   {
     "server": {
       "port": 5096
     }
   }
   ```

3. **Check Bun is installed:**
   ```bash
   bun --version
   ```

---

## Still Stuck?

1. Run `/tc-doctor` and include the full output in any issue report
2. Check the [GitHub Discussions](https://github.com/bobbyjohnstx/tinycode/discussions)
3. File an issue with:
   - Output of `/tc-doctor`
   - Steps to reproduce
   - Your config: `cat ~/.config/tinycode/config.json` (redact API keys)
   - Relevant logs (Ollama, tinycode, vLLM)
4. For Kubernetes issues, see [tinycode-operator troubleshooting](https://github.com/bobbyjohnstx/tinycode-operator#troubleshooting)
