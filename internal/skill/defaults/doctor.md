---
name: doctor
description: Diagnose and fix tinycode environment issues — provider connectivity, model health, tool-call capability, directories, agents, and skills
---

# Doctor

Self-diagnostic skill that checks the tinycode environment is correctly configured and fixes problems it finds.

## When to Use

- The user says "doctor", "health check", "fix my setup"
- After a fresh install or configuration change
- When agents, skills, or tools are not appearing
- When the model connection is failing or responses are slow
- When tool calling is not working

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
