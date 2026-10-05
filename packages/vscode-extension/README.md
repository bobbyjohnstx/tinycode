# tinycode VS Code Extension

AI coding assistant powered by local (or cloud) LLMs via the Go `tinycode acp` binary and the [Agent Client Protocol](https://agentclientprotocol.com).

## Prerequisites

Build the Go binary:

```bash
make build   # → dist/tinycode
```

Ensure `tinycode` is on your PATH, or set `tinycode.path` in VS Code settings to the absolute path of `dist/tinycode`.

> The TypeScript `packages/tinycode` tree is legacy. This extension speaks ACP to the **Go** binary.

## Installation

1. Open `packages/vscode-extension/` in VS Code
2. `npm install`
3. `npm run build`
4. Press F5 to launch the Extension Development Host

## Usage

1. Open a project folder
2. The extension auto-starts `tinycode acp --cwd <workspace>`
3. Open Chat and message `@tinycode …`

## Configuration

```json
{
  "tinycode.path": "/absolute/path/to/dist/tinycode"
}
```

## How It Works

1. Spawns `tinycode acp --cwd <workspace>` (stdio NDJSON)
2. Completes ACP `initialize` (protocol version `1`) with a client that handles:
   - `session/update` notifications (streamed text/tools)
   - `session/request_permission` requests (quick-pick → allow_once / allow_always / reject_once)
3. Creates a session via `session/new`
4. Chat prompts use `session/prompt` and wait for `stopReason`

## Status

| Feature | Status |
|---------|--------|
| Spawn Go ACP + `--cwd` | Working |
| Official wire methods | Working (`session/*`) |
| Streamed assistant text | Working |
| Permission quick-pick | Working |
| Image / embedded context prompts | Not advertised by agent yet |
| JetBrains / Zed packaging | Not included here (same protocol) |

## Troubleshooting

**Extension fails to start**

- Confirm `tinycode path` points at the Go binary (`./dist/tinycode version`)
- Check the **tinycode** output channel for stderr from bootstrap/discovery

**No chat response**

- Ensure a model is available (`tinycode models`) or configure a default model
- Restart with **tinycode: Stop** then **tinycode: Start**

**Permission prompts**

- Choose Allow Once / Allow Always / Reject; dismissing the picker cancels (deny)

## Development

```bash
npm run watch   # rebuild on change
```
