---
name: doctor
description: Diagnose and fix tinycode environment issues — provider connectivity, model health, tool-call capability, directories, agents, skills, config audit, and optimization suggestions
---

# Doctor

Self-diagnostic skill that checks the tinycode environment is correctly configured and fixes problems it finds.

## When to Use

- The user says "doctor", "health check", "fix my setup"
- After a fresh install or configuration change
- When agents, skills, or tools are not appearing
- When the model connection is failing or responses are slow
- When tool calling is not working
- When the user wants a config audit or optimization suggestions
- When context usage seems high or instruction files may have redundant content

## When Not to Use

- The user is asking about their own application code
- Debugging a specific session failure or code bug (use /debug or /trace)
- Configuring a new MCP server (use /mcp-setup)

## Checks to Run (in order)

### 1. Directory structure
Verify these directories exist and are writable: `.tinycode/`, `~/.config/tinycode/`, `~/.local/share/tinycode/`. Create any that are missing.

### 2. Tools availability
Check for required tools: `curl`, `git`. Check optional tools: `tmux` (needed for /swarm).

### 3. Provider connectivity
Check if the configured provider (Ollama, etc.) is reachable. Verify the API endpoint responds.

### 4. Model availability
List available models from the provider. Check if the configured default model is available. Check recommended models.

### 5. Model functionality
Send a tool-call probe to verify the model responds and supports tool calling. Use a 120s timeout for cold-loading.

### 6. System resources
Check RAM availability vs model size requirements. Check disk space. Check for swap pressure on macOS.

### 7. Integration health
Check if tinycode API is responding. Verify connected providers. Check for provider filtering in config.

### 8. Stale MCP servers
Check configured MCP servers that fail to connect or have stopped responding.

Run `tinycode mcp list` to get configured servers. For each server, check connection status (the list output shows status). If the server API is running, also check `curl -s localhost:<port>/mcp/status` for live status.

**Pass**: All configured MCP servers are connected or have no servers configured.
**Fail**: One or more servers show disconnected, errored, or unreachable status.

**Remediation**: Report each failing server name and its error. Suggest:
- Verify the server process is running (`ps aux | grep <server-name>`)
- Check if the command in the MCP config is valid and installed
- Remove stale servers with `tinycode mcp remove <name>`
- Re-add with `tinycode mcp add` if the server binary moved

### 9. Config consistency
Check for conflicting settings across config layers (global `~/.config/tinycode/`, project `.tinycode/`, macOS `~/Library/Application Support/tinycode/`).

Read config files from each layer. Config files use 3-name fallback: `tinycode.jsonc`, `tinycode.json`, `config.json`. Check these locations:
- `~/.config/tinycode/` (global)
- `~/Library/Application Support/tinycode/` (macOS global, if it exists)
- `.tinycode/` in the project root and any parent directories

For each setting that appears in multiple layers, report which layer wins (innermost/last-loaded wins) and flag contradictions where a project config silently overrides a global setting (e.g., different model, different provider endpoint, conflicting permission rules).

**Pass**: No conflicting settings across layers, or conflicts are intentional overrides.
**Fail**: Settings in different layers contradict each other in ways the user may not expect.

**Remediation**: List each conflict with the value from each layer and which one takes effect. Suggest consolidating settings to reduce confusion.

### 10. Context budget estimation
Estimate total context consumed by always-loaded files and warn if it approaches the model's context limit.

Measure the byte size of these always-loaded files (if they exist):
- `.tinycode/TINYCODE.md` or `.tinycode/CLAUDE.md` (project instructions)
- All agent `.md` files in `.tinycode/agents/`
- All skill `.md` files in `.tinycode/skills/`
- Memory files in `~/.config/tinycode/memory/` or `~/.local/share/tinycode/memory/`
- Global instructions in `~/.config/tinycode/TINYCODE.md` or `~/.config/tinycode/CLAUDE.md`

Sum the total bytes and convert to an estimated token count (1 token ≈ 4 bytes for English text). Compare against common context limits:
- 8K models: warn above 2K tokens of always-loaded content
- 32K models: warn above 8K tokens
- 128K models: warn above 32K tokens
- 1M models: warn above 200K tokens

**Pass**: Total always-loaded content is under 25% of the configured model's context limit.
**Fail**: Total always-loaded content exceeds 25% of the context limit.

**Remediation**: List each file with its size and estimated token count, sorted largest first. Suggest trimming the largest files, moving rarely-needed content to on-demand skills, or switching to a model with a larger context window.

### 11. Instruction file analysis
Find content in instruction files that could be derived from the codebase rather than explicitly stated.

Read instruction files (`.tinycode/TINYCODE.md`, `.tinycode/CLAUDE.md`, agent `.md` files) and look for:
- **Directory listings** or file trees that `ls` and `find` could produce
- **Implementation descriptions** that restate what the code does (e.g., "the server handles requests on port 8080" when that is visible in the code)
- **Type or function catalogs** listing every exported symbol
- **Historical notes** about resolved issues or completed work
- **Project overview text** derivable from README or repo structure

For each finding, explain why it is redundant and suggest either removing it or replacing it with a pitfall/constraint that prevents a silent failure.

**Pass**: Instruction files contain only non-derivable content (commands, pitfalls, constraints).
**Fail**: Instruction files contain content that restates what the code or tooling already provides.

**Remediation**: List each redundant section with a brief explanation of why it is derivable and a suggested action (remove, condense, or convert to a pitfall).

## Rules
- Run ALL checks even if early ones fail (except skip model checks if provider is unreachable)
- Apply fixes automatically where safe (directory creation)
- Do NOT modify user config files — print recommended config
- Do NOT auto-pull models — they are multi-GB downloads requiring user consent
- Report everything found, even if all checks pass

## Output Contract

```
## tinycode Doctor Report

### Environment
- Platform, container status, working directory, user, provider endpoint

### Checks
[Results from each check]

### Issues Found
- [List of problems]

### Fixes Applied
- [Automatic fixes taken]

### Manual Actions Needed
- [Anything requiring user action]
```
