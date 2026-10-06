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

## Step 3: Add MCP Servers via Config

There is no `tinycode mcp add` CLI yet. Edit the user or project config file and add an `mcp` entry. Prefer project scope (`.tinycode/tinycode.jsonc` / `tinycode.json`) when the user asks for project-scoped setup; otherwise use `~/.config/tinycode/tinycode.jsonc`.

Use `{env:VAR}` for secrets (not `$VAR` / `${VAR}`).

### Examples

```jsonc
{
  "mcp": {
    "context7": {
      "command": "npx",
      "args": ["-y", "@upstash/context7-mcp"]
    },
    "exa": {
      "command": "npx",
      "args": ["-y", "exa-mcp-server"],
      "env": { "EXA_API_KEY": "{env:EXA_API_KEY}" }
    },
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/path/to/allow"]
    },
    "github": {
      "command": "docker",
      "args": ["run", "-i", "--rm", "-e", "GITHUB_PERSONAL_ACCESS_TOKEN", "ghcr.io/github/github-mcp-server"],
      "env": { "GITHUB_PERSONAL_ACCESS_TOKEN": "{env:GITHUB_PERSONAL_ACCESS_TOKEN}" }
    },
    "github-http": {
      "url": "https://api.githubcopilot.com/mcp/",
      "transport": "streamable-http",
      "headers": { "Authorization": "Bearer {env:GITHUB_PERSONAL_ACCESS_TOKEN}" }
    }
  }
}
```

Ask for:
1. Server name (identifier)
2. Transport type: `stdio` (default), `sse`, or `streamable-http`
3. For stdio: `command` + `args` (and optional `env`)
4. For remote: `url`, optional `headers` / `oauth`
5. Environment variables (optional)

## Step 4: Verify

1. Restart tinycode (or reload config if the session supports it)
2. In the TUI, open `/mcp` (or **Ctrl+X i**) and confirm status `connected` and tool counts
3. Or with `tinycode serve`: `GET /mcp/status`

## Output Contract

Report on completion:
- Which servers were written into which config file
- Which were deferred (missing API key)
- How to verify (`/mcp` or `GET /mcp/status`)
- Restart reminder
