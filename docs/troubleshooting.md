# Troubleshooting

Solutions to common tinycode issues and configuration problems.

## LLM Connection Issues

### Ollama not detected

**Problem:** tinycode doesn't see Ollama even though it's running.

**Solution:**
1. Verify Ollama is running: `curl http://localhost:11434/api/tags`
2. Ensure it's listening on `localhost:11434` (default). If you changed the port, set in config:
   ```bash
   export OLLAMA_HOST=http://0.0.0.0:11434
   # or restart Ollama with different binding
   ```
3. Check firewall isn't blocking localhost access
4. Try restarting tinycode after Ollama starts

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
1. Check what model is selected: `<leader>m`
2. Monitor model resource usage:
   ```bash
   # For Ollama
   watch "curl -s http://localhost:11434/api/tags | jq '.models[] | {name: .name, size: .size}'"
   
   # For vLLM
   watch "curl -s http://localhost:8000/v1/models | jq '.data[]'"
   ```
3. If model is small (< 7B params), consider using a larger one
4. Check system resources:
   ```bash
   top -n1 | head -20
   ```
5. If CPU-bound (not GPU), reduce batch size or switch to GPU inference if available
6. Try `/tc-doctor` to check for bottlenecks

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

### Check your configuration
```bash
/tc-doctor
```

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
