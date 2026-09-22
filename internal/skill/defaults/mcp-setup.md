---
name: mcp-setup
description: Configure MCP servers via a guided menu — curated bundles or custom stdio/HTTP servers — with scope control and verification
---

# MCP Setup

Configure Model Context Protocol (MCP) servers to extend tinycode's capabilities with external tools like web search, file system access, and GitHub integration.

## When to Use

Use this skill when:
- The user says "set up MCP", "add an MCP server", "configure Context7/Exa/Filesystem/GitHub"
- The user wants web search, docs context, file access, or GitHub integration via MCP
- The user wants to add a custom stdio or HTTP MCP server

## When Not to Use

- The user wants to write or code a new MCP server implementation
- The user wants to change tinycode settings or permissions — use `update-config`
- Deep runtime debugging of a failing MCP server — use `debug`
- The user just wants to list or remove servers — run `tinycode mcp list` or `tinycode mcp remove <name>` directly

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

## Step 3: Add MCP Servers Using CLI

```bash
# Context7
tinycode mcp add context7 -- npx -y @upstash/context7-mcp

# Exa
tinycode mcp add -e EXA_API_KEY=<key> exa -- npx -y exa-mcp-server

# Filesystem
tinycode mcp add filesystem -- npx -y @modelcontextprotocol/server-filesystem <paths>

# GitHub (Docker)
tinycode mcp add -e GITHUB_PERSONAL_ACCESS_TOKEN=<token> github -- docker run -i --rm -e GITHUB_PERSONAL_ACCESS_TOKEN ghcr.io/github/github-mcp-server

# GitHub (HTTP)
tinycode mcp add --transport http github https://api.githubcopilot.com/mcp/
```

Add `-s project` for project-scoped config instead of user-level.

## Step 4: Verify

Run `tinycode mcp list` and confirm servers appear.

## Custom MCP Server

Ask for:
1. Server name (identifier)
2. Transport type: `stdio` (default) or `http`
3. For stdio: command and arguments
4. For http: URL
5. Environment variables (optional)

## Output Contract

Report on completion:
- Which servers were configured (verified via `tinycode mcp list`)
- Which were deferred (missing API key)
- The `tinycode mcp list` output
- Restart reminder
