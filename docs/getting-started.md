# Getting Started with tinycode

Step-by-step walkthrough to get tinycode running and productive in 10 minutes.

## Prerequisites

- **Go 1.27+** -- [install here](https://go.dev/dl/)
- **make**
- **Git**
- One of:
  - **Ollama** (recommended for getting started) -- [install here](https://ollama.ai)
  - **vLLM** -- fast inference via OpenAI-compatible API
  - **LM Studio** -- desktop app with OpenAI-compatible API
  - **API key** to OpenRouter or another OpenAI-compatible cloud provider

## Step 1: Install tinycode

### From source

```bash
git clone https://github.com/bobbyjohnstx/tinycode.git
cd tinycode
make build
```

This produces the binary at `dist/tinycode`.

### Verify the build

```bash
./dist/tinycode version
```

## Step 2: Start an LLM (local option)

If you want to run locally without cloud providers, start Ollama:

```bash
# In a separate terminal
ollama serve

# In another terminal, pull a model
ollama pull qwen3.5:9b
```

**Skip this step** if you are using OpenRouter or another cloud provider.

## Step 3: Run tinycode

```bash
./dist/tinycode
```

This starts tinycode in TUI mode against your current directory. You will see:
- Conversation area in the center
- Input prompt at the bottom
- Sidebar on the right (hidden by default; toggle with `Ctrl+X b`)

**What happens on startup:**
- tinycode starts an embedded HTTP server on an ephemeral port
- Auto-discovers Ollama at `localhost:11434` (and vLLM / LM Studio if running)
- Polls discovered providers for available models
- Runs a warmup probe on Ollama models to detect tool-call support
- Loads the default **build** agent (full tool access)

To run against a different directory:

```bash
./dist/tinycode /path/to/project
```

## Step 4: Connect a provider

If something looks wrong, run `tinycode doctor` (or `/doctor` in the TUI). There is no `tinycode setup` wizard — first-run model config is the TUI connect dialog (`/connect`).

If tinycode did not auto-discover your LLM provider, the connect dialog opens on first launch when no model is set. You can also type `/connect` anytime, or set environment variables before launching:

```bash
# Ollama on a non-default host (prefer TINYCODE_OLLAMA_HOST)
export TINYCODE_OLLAMA_HOST=http://your-host:11434
# fallback if TINYCODE_OLLAMA_HOST unset: OLLAMA_HOST

# vLLM
export TINYCODE_VLLM_HOST=http://localhost:8000

# LM Studio
export TINYCODE_LMSTUDIO_HOST=http://localhost:1234

# Cloud providers
export OPENROUTER_API_KEY=your-key
```

## Step 5: Select a model

Press `<leader>m` (Ctrl+X, then M) to open the model list:

```
List Models
------------
ollama/qwen3.5:9b
ollama/mistral
```

Select one with arrow keys, press Enter.

## Step 6: Your first conversation

Type a prompt and press Enter:

```
Explain what this repository does in 2 sentences.
```

tinycode will:
1. Read files in the current directory
2. Ask the LLM to analyze them
3. Stream the response in real time

Try more prompts:

```
What files are in this project?
Write a bash script that lists all .go files
```

## Step 7: Using agents

Agents are specialized personas for different tasks. Press `Tab` to cycle through them, or `<leader>a` to see the full list.

Use `/ask <agent> <prompt>` to invoke an agent directly:

```
/ask architect analyze the data flow in this project
/ask debugger why is this test failing?
/ask code-reviewer review the changes on this branch
```

## Step 8: Using skills (slash commands)

Skills inject specialized instructions. Type `/` to see available ones:

```
/debug          # Isolate a single most-likely root cause
/trace          # Evidence-driven causal tracing with hypotheses
/verify         # Confirm changes work before claiming completion
```

## Configuration

Create `~/.config/tinycode/config.json` (JSONC supported):

```json
{
  "model": "ollama/qwen3.5:9b",
  "default_agent": "build"
}
```

See the [User Guide](user-guide.md) for all configuration options.

## Other run modes

```bash
# Non-interactive: run a single prompt and exit
./dist/tinycode run -m ollama/qwen3.5:9b "explain the main function"

# Pipe input from stdin
echo "fix the lint errors" | ./dist/tinycode run -m ollama/qwen3.5:9b

# Multi-turn: loop on stdin for multiple prompts
./dist/tinycode run --multi-turn --format json -m ollama/qwen3.5:9b

# Headless API proxy (API only, no SPA)
./dist/tinycode serve

# Browser UI (embedded SPA + API; opens auth URL)
./dist/tinycode web

# Agent Client Protocol (IDE integration, stdio)
./dist/tinycode acp
```

See the [User Guide](user-guide.md#run-mode) for full run mode documentation including NDJSON output, permission handling, and programmatic integration.

## Next steps

- [User Guide](user-guide.md) -- keyboard shortcuts, agents, sessions, configuration
- [Plugin Development](plugin-development.md) -- extend tinycode with custom tools and hooks
- [Architecture](architecture.md) -- how tinycode works under the hood

## Troubleshooting

**Model not found:** Make sure you have pulled the model in Ollama first (`ollama pull <model>`).

**Cannot connect to Ollama:** Verify `ollama serve` is running. If Ollama is on a different host, set `OLLAMA_HOST`.

**Tool calling not working:** The warmup probe tests tool-call support. If a model does not support tool calling, tinycode works without tools (text-only responses). Larger models (9B+) tend to have better tool-call support.
