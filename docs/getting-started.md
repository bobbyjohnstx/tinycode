# Getting Started with tinycode

Step-by-step walkthrough to get tinycode running and productive in 10 minutes.

## Prerequisites

- **Go 1.22+** -- [install here](https://go.dev/dl/)
- **make**
- **Git**
- One of:
  - **Ollama** (recommended for getting started) -- [install here](https://ollama.ai)
  - **vLLM** -- fast inference via OpenAI-compatible API
  - **LM Studio** -- desktop app with OpenAI-compatible API
  - **API key** to OpenRouter, Anthropic, OpenAI, or another cloud provider

## Step 1: Install tinycode

### From source

```bash
git clone https://github.com/bobbyjohnstx/tinycode-go.git
cd tinycode-go
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

**Skip this step** if you are using OpenRouter, Anthropic, or another cloud provider.

## Step 3: Run tinycode

```bash
./dist/tinycode
```

This starts tinycode in TUI mode against your current directory. You will see:
- Session sidebar on the left (toggled with `<leader>b`)
- Conversation area in the center
- Input prompt at the bottom

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

If tinycode did not auto-discover your LLM provider, use `/connect` in the TUI prompt to add one interactively. Or set environment variables before launching:

```bash
# Ollama on a non-default host
export OLLAMA_HOST=http://your-host:11434

# vLLM
export TINYCODE_VLLM_HOST=http://localhost:8000

# LM Studio
export TINYCODE_LMSTUDIO_HOST=http://localhost:1234

# Cloud providers
export OPENROUTER_API_KEY=your-key
export ANTHROPIC_API_KEY=your-key
export OPENAI_API_KEY=your-key
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
# Headless API proxy (no TUI)
./dist/tinycode serve

# Agent Client Protocol (IDE integration, stdio)
./dist/tinycode acp
```

## Next steps

- [User Guide](user-guide.md) -- keyboard shortcuts, agents, sessions, configuration
- [Plugin Development](plugin-development.md) -- extend tinycode with custom tools and hooks
- [Architecture](architecture.md) -- how tinycode works under the hood

## Troubleshooting

**Model not found:** Make sure you have pulled the model in Ollama first (`ollama pull <model>`).

**Cannot connect to Ollama:** Verify `ollama serve` is running. If Ollama is on a different host, set `OLLAMA_HOST`.

**Tool calling not working:** The warmup probe tests tool-call support. If a model does not support tool calling, tinycode works without tools (text-only responses). Larger models (9B+) tend to have better tool-call support.
