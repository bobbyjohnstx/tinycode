---
name: mcp-setup
description: Configure MCP servers via a guided menu — curated bundles or custom stdio/HTTP servers — with scope control and verification
---

# MCP Setup

Configure Model Context Protocol (MCP) servers to extend tinycode's capabilities with external tools like web search, file system access, and GitHub integration.

**Interactive OAuth is not supported.** Do not walk users through a browser OAuth login for MCP. Use a static Bearer token in `headers`, or `{env:VAR}` substitution for secrets. The PKCE helpers in `internal/mcp/oauth.go` are library-only and are not wired into serve/CLI/TUI.

## When to Use

Use this skill when:
- The user says "set up MCP", "add an MCP server", "configure Context7/Exa/Filesystem/GitHub"
- The user wants web search, docs context, file access, or GitHub integration via MCP
- The user wants to add a custom stdio or HTTP MCP server

## When Not to Use

- The user wants to write or code a new MCP server implementation
- The user wants to change tinycode settings or permissions — use `update-config`
- Deep runtime debugging of a failing MCP server — use `debug`
- The user only wants to inspect connection status — use `/mcp` in the TUI or `GET /mcp/status` when serving

## Step 1: Choose a Setup Path

Present options one question at a time:
1. **Recommended starter setup** — fast path for common MCP additions
2. **Individual popular server** — pick one server
3. **Custom server** — add your own stdio or HTTP MCP server

## Step 2: Gather Required Information

- **Context7**: No API key required. Ready to use immediately.
- **Exa Web Search**: Requires API key from https://exa.ai
- **Filesystem**: Ask for allowed directories (default: current working directory)
- **GitHub**: Requires Personal Access Token from https://github.com/settings/tokens (scopes: repo, read:org)

## Step 3: Add MCP Servers via CLI

Use `tinycode mcp add`. Default scope is user config; add `--project` for project scope (`.tinycode/tinycode.json`). Use `{env:VAR}` for secrets (not `$VAR` / `${VAR}`).

### Examples

```bash
tinycode mcp add context7 -- npx -y @upstash/context7-mcp
tinycode mcp add -e EXA_API_KEY exa -- npx -y exa-mcp-server
tinycode mcp add filesystem -- npx -y @modelcontextprotocol/server-filesystem /path/to/allow
tinycode mcp add --transport http github https://api.githubcopilot.com/mcp/
tinycode mcp auth github --env GITHUB_PERSONAL_ACCESS_TOKEN
```

Ask for:
1. Server name (identifier)
2. Transport type: `stdio` (default), `sse`, or `streamable-http` (`http` alias)
3. For stdio: command + args (and optional `-e`)
4. For remote: URL, then `tinycode mcp auth` for Bearer / `{env:VAR}` — **not** interactive OAuth
5. Scope: user vs `--project`

## Step 4: Verify

1. `tinycode mcp list` (and optionally `tinycode mcp debug <name>`)
2. Restart tinycode (or reload config if the session supports it)
3. In the TUI, open `/mcp` (or **Ctrl+X i**) and confirm status `connected` and tool counts
4. Or with `tinycode serve`: `GET /mcp/status`
## Output Contract

Report on completion:
- Which servers were written into which config file (from `tinycode mcp list`)
- Which were deferred (missing API key)
- How to verify (`tinycode mcp list` / `debug`, `/mcp`, or `GET /mcp/status`)
- Restart reminder