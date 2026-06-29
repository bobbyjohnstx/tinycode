# tinycode VS Code Extension

AI coding assistant powered by local LLMs via tinycode.

## Prerequisites

Install tinycode first:

```bash
# From the tinycode repository
bun install
bun ./packages/tinycode/script/build.ts --single

# Or install from a pre-built binary
```

Ensure `tinycode` is in your PATH, or configure the path in settings (see below).

## Installation

1. Open this directory in VS Code: `packages/vscode-extension/`
2. Install dependencies: `npm install`
3. Build the extension: `npm run build`
4. Press F5 to launch the extension in a new Extension Development Host window

## Usage

1. Open a project folder in VS Code
2. The extension will auto-start tinycode in ACP mode
3. Open the chat panel (Ctrl+Shift+P → "Chat: Open Chat")
4. Type `@tinycode` followed by your question or request

Example:
```
@tinycode explain this file
@tinycode add error handling to the UserService class
@tinycode write unit tests for the login function
```

## Configuration

Configure the tinycode binary path in VS Code settings:

```json
{
  "tinycode.path": "/path/to/tinycode"
}
```

If `tinycode` is in your PATH, you can leave this at the default value `"tinycode"`.

## Commands

- **tinycode: Start AI Assistant** — Manually start the tinycode agent
- **tinycode: Stop AI Assistant** — Stop the running agent

## How It Works

The extension spawns `tinycode acp --cwd <workspace-folder>` as a child process and communicates via the [Agent Client Protocol](https://agentclientprotocol.com) over stdio. All prompts are sent to the tinycode server, which executes them using the configured LLM (local via Ollama/vLLM or cloud via API key).

## Troubleshooting

**Extension fails to start:**
- Check that `tinycode` is installed and in your PATH
- Open the Output panel (View → Output) and select "tinycode" from the dropdown
- Check for error messages in the tinycode log

**No response in chat:**
- Ensure a workspace folder is open (File → Open Folder)
- Check the tinycode output channel for errors
- Restart the extension with "tinycode: Stop AI Assistant" then "tinycode: Start AI Assistant"

**Permission prompts:**
The extension will show a quick-pick menu when tinycode requests permission to run tools (file read/write, shell commands). Select:
- **Allow Once** — grant permission for this operation only
- **Allow Always** — add the tool to the allowlist
- **Deny** — reject the operation

## Development

```bash
# Install dependencies
npm install

# Build
npm run build

# Watch mode (auto-rebuild on changes)
npm run watch
```

## License

MIT
