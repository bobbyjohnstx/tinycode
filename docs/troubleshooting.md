# Troubleshooting

Solutions to common tinycode issues and configuration problems.

## LLM Connection Issues

### Ollama not detected

**Problem:** tinycode doesn't see Ollama even though it's running.

**Solution:**
1. Run `/tc-doctor` — it checks Ollama install, process, API reachability, and provider integration in one pass
2. Verify Ollama is running: `curl http://localhost:11434/api/tags`
3. Ensure it's listening on `localhost:11434` (default). If you changed the port:
   ```bash
   export TINYCODE_OLLAMA_HOST=http://localhost:YOUR_PORT
   ```
4. Check if Ollama is in `disabled_providers` in your config
5. In containers, use: `export TINYCODE_OLLAMA_HOST=http://host.docker.internal:11434`
6. Restart tinycode after Ollama starts (provider discovery runs on startup)

**Alternative:** Manually connect via `<leader>m` → `Ctrl+A` → add custom provider

### vLLM not connecting

**Problem:** "Connection refused" or timeout when connecting to vLLM.

**Solution:**
1. Verify vLLM is listening: `curl http://localhost:8000/v1/models`
2. If running on a different host:
   ```bash
   export TINYCODE_VLLM_URLS=http://your-vllm-host:8000
   ```
3. If behind a firewall, ensure port 8000 is open
4. Check vLLM didn't crash:
   ```bash
   ps aux | grep vllm
   ```

### "Provider not found" after connecting

**Problem:** Model appears in provider list but can't select it.

**Solution:**
1. Verify provider is actually running
2. Check `~/.config/tinycode/config.json` for `enabled_providers` / `disabled_providers` filters
3. Try `/tc-doctor` to diagnose configuration
4. If using custom endpoint, verify it's OpenAI-compatible:
   ```bash
   curl http://your-endpoint/v1/models
   ```

## Server & Network Issues

### "Server won't start" or "port already in use"

**Problem:** `bun dev serve` fails with address already in use.

**Solution:**
1. Find process using port 4096:
   ```bash
   lsof -i :4096
   kill -9 <PID>
   ```
2. Or use a different port in config:
   ```json
   {
     "server": {
       "port": 5096
     }
   }
   ```
3. Restart with `bun dev serve`

### Server won't start on remote machine

**Problem:** `bun dev serve` works locally but fails on remote.

**Solution:**
1. Ensure Bun is installed: `bun --version`
2. Bind to all interfaces:
   ```bash
   bun dev serve --hostname 0.0.0.0
   # or in config:
   { "server": { "hostname": "0.0.0.0" } }
   ```
3. Set password for remote access:
   ```bash
   export TINYCODE_SERVER_PASSWORD=your-secure-password
   ```
4. Open firewall port:
   ```bash
   sudo firewall-cmd --add-port=4096/tcp --permanent
   sudo firewall-cmd --reload
   ```
5. Access via `http://<server-ip>:4096`

### Web UI loads but can't send messages

**Problem:** Web UI shows but submitting prompts does nothing.

**Solution:**
1. Check browser console for errors (F12 in DevTools)
2. Verify server is responding:
   ```bash
   curl http://localhost:4096/health
   ```
3. Check CORS isn't blocking requests (should not be by default)
4. Try restarting browser and clearing cache

## Session & Storage Issues

### "Session not found" error

**Problem:** Session list is empty or switching sessions fails.

**Solution:**
1. Check database file exists:
   ```bash
   ls -la ~/.config/tinycode/db.sqlite
   ```
2. Verify database isn't corrupted:
   ```bash
   sqlite3 ~/.config/tinycode/db.sqlite ".tables"
   ```
3. If corrupted, back up and delete:
   ```bash
   mv ~/.config/tinycode/db.sqlite ~/.config/tinycode/db.sqlite.bak
   # Restart tinycode — it will recreate the database
   ```
4. Check disk space isn't full:
   ```bash
   df -h ~/.config
   ```

### Sessions take too long to load

**Problem:** Session list loads slowly or switching sessions lags.

**Solution:**
1. Compact a session to reduce size:
   ```
   <leader>c
   ```
   This removes unnecessary messages and shrinks the session.
2. Export very large sessions to archive them
3. Clear old sessions manually if no longer needed
4. Check disk performance:
   ```bash
   iostat -x 1 5
   ```

### Can't write to `.tinycode/` directory

**Problem:** Error when trying to use notepad, wiki, or memory tools.

**Solution:**
1. Verify directory exists and is writable:
   ```bash
   ls -la .tinycode/
   chmod -R 755 .tinycode/
   ```
2. Check you have write permissions in the project directory
3. If running in container, verify `/projects` PVC is mounted and writable
4. Restart tinycode after fixing permissions

## Performance Issues

### Model responses are very slow

**Problem:** LLM takes 10+ seconds to respond or times out.

**Solution:**
1. Run `/tc-doctor` — it checks RAM vs model size, GPU acceleration, swap pressure, and cold-load time
2. Check what model is selected: `<leader>m`
3. tinycode warms the model on startup — if you see "warming model..." followed by a long load time (>60s), the model may be too large for your hardware
4. On Mac, verify Ollama is native arm64 (not Rosetta): `/tc-doctor` checks this automatically
5. Check if the model is swapping: close Docker, Chrome, and other memory-heavy apps
6. Dense models >12B are too slow on 32GB RAM — use qwen3.5:9b (9B, benchmark champion at 14/15)
7. Check `ollama ps` to see if the model is loaded or re-loading between requests
8. If model keeps re-loading, check `keep_alive` — tinycode sets `keep_alive: "10m"` by default

### TUI is sluggish or unresponsive

**Problem:** Input lag, slow rendering, or UI freeze.

**Solution:**
1. Disable animations:
   ```
   <leader>s        # View status
   Look for animation toggle
   ```
   Or in config:
   ```json
   {
     "animations": false
   }
   ```
2. Collapse code blocks to reduce rendering:
   ```
   <leader>h        # Toggle code concealment
   ```
3. Try a smaller terminal font size (renders faster)
4. Check system isn't CPU-constrained (tinycode TUI runs on single thread)
5. Reduce number of open sessions

### Web UI is slow

**Problem:** Web interface lags or is unresponsive.

**Solution:**
1. Clear browser cache (Ctrl+Shift+Delete)
2. Check network latency to server:
   ```bash
   ping <server-ip>
   ```
3. Monitor server CPU/memory:
   ```bash
   top | grep node  # if running via Node
   ```
4. Try a different browser
5. Check for browser extensions that might intercept requests (disable temporarily)

## Tool-Call Issues

### Model produces malformed tool calls

**Problem:** Tool calls fail repeatedly with JSON parse errors.

**Solution:** tinycode automatically repairs common JSON issues:
1. Strips markdown code fences (`` ```json ... ``` ``)
2. Removes trailing commas before `}` or `]`
3. Retries the repaired call

If repairs fail, the model may not support tool calling reliably. See "Tool calls keep failing" below.

### Tool calls keep failing

**Problem:** After 3+ consecutive tool-call failures, a warning toast appears.

**Solution:**
1. This usually indicates the model is too small or lacks tool-call training
2. Switch to a larger model: Press `<leader>m` and select `qwen3:14b`, `qwen3.5:9b`, or similar
3. For very small models (<7B parameters), tool calling may not work at all — see next section

### Model doesn't support tool calling

**Problem:** tinycode detects `capabilities.toolcall=false` for this model and skips tools entirely.

**Solution:**
1. tinycode auto-detects this via the provider's capability flags and confirms via warmup probe on startup
2. When disabled, tools are not injected into the request
3. The model still works for conversations — it just can't use tools
4. Models with tool calling: qwen3.5:9b, north-mini-code-1.0, gemma4:12b
5. Models WITHOUT: granite, codellama, deepseek-r1 (distilled) — these all score 5/15 with zero tool calls
6. Run `/tc-doctor` to see your model's tool-call status
7. To switch, select a model with tool-call support via `<leader>m`

## Plugin & Configuration Issues

### Plugin won't load

**Problem:** Plugin appears in list but fails to initialize.

**Solution:**
1. Check plugin syntax:
   ```bash
   cat ~/.tinycode/plugins/your-plugin.json
   ```
2. Verify all required fields are present
3. Check plugin logs:
   ```bash
   tail -f ~/.config/tinycode/logs/plugin.log
   ```
4. Try `/tc-doctor` for configuration validation

### Can't find agent or skill

**Problem:** Listed agents or skills don't appear in autocomplete or list.

**Solution:**
1. Restart tinycode
2. Check config for `enabled_agents` filter:
   ```bash
   grep -A5 enabled_agents ~/.config/tinycode/config.json
   ```
3. Verify built-in agents haven't been disabled:
   ```bash
   /ask architect (test if the agent loads)
   ```
4. Run `/tc-doctor` to list all available agents

### Permission prompts too frequent

**Problem:** Constantly asked to approve tool use.

**Solution:**
1. Review what's being approved (each prompt shows the tool and input)
2. Approve once if you trust the command
3. To auto-approve safe operations, configure in `~/.config/tinycode/config.json`:
   ```json
   {
     "permissions": {
       "auto_approve_read": true
     }
   }
   ```

## Container Deployment Issues

### Container exits immediately

**Problem:** `podman run` starts container but it exits right away.

**Solution:**
1. Check logs:
   ```bash
   podman logs <container-id>
   ```
2. Verify image has required dependencies:
   ```bash
   podman run -it quay.io/bjohns/tinycode-container:latest bun --version
   ```
3. If missing vLLM, set environment variable:
   ```bash
   podman run -it \
     -e TINYCODE_VLLM_URL=http://your-vllm:8000 \
     -e TINYCODE_SERVER_PASSWORD=changeme \
     quay.io/bjohns/tinycode-container:latest
   ```

### OpenShift pod won't start

**Problem:** TinycodeInstance pod is stuck in Pending or CrashLoopBackOff.

**Solution:**
1. Check pod events:
   ```bash
   oc describe pod -n tinycode <pod-name>
   ```
2. Verify PVC is provisioned:
   ```bash
   oc get pvc -n tinycode
   ```
3. Check node resources:
   ```bash
   oc describe nodes | grep -A5 "Allocated resources"
   ```
4. If SCC issue, verify operator set SecurityContext:
   ```bash
   oc get pod -o yaml <pod-name> | grep -A10 "securityContext"
   ```
5. See [tinycode-operator docs](https://github.com/bobbyjohnstx/tinycode-operator) for cluster setup

## Diagnostic Commands

### Run the full diagnostic
```bash
/tc-doctor
```
tc-doctor runs 14 checks in order: directory structure, agents, skills, system tools, oc CLI, Ollama install/health, model availability (configured model, recommended models, RAM fit), model functionality (tool-call probe with warmup), Mac-specific checks (Metal, Rosetta, swap), tinycode↔Ollama integration, vLLM/custom provider health, tmux, and disk space. All checks use pure bash — no python3 required.

### List all active sessions
```bash
<leader>l
```

### View server status
```bash
<leader>s
```

### Export a session for debugging
```bash
<leader>x              # Export current session
tinycode export --format json <session-id> > session.json
```

### Check database health
```bash
sqlite3 ~/.config/tinycode/db.sqlite ".tables"
sqlite3 ~/.config/tinycode/db.sqlite ".schema sessions"
```

### Verify provider connectivity
```bash
curl -v http://localhost:11434/api/tags          # Ollama
curl -v http://localhost:8000/v1/models          # vLLM
```

## Still Stuck?

1. Run `/tc-doctor` — it diagnoses most common issues
2. Check the [GitHub Discussions](https://github.com/bobbyjohnstx/tinycode/discussions)
3. File an issue with:
   - Output of `/tc-doctor`
   - Exact steps to reproduce
   - Your config (`~/.config/tinycode/config.json`)
   - Relevant logs (if available)
4. For cluster issues, see [tinycode-operator troubleshooting](https://github.com/bobbyjohnstx/tinycode-operator#troubleshooting)
