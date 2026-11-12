# Connecting tinycode to Red Hat MAAS (models.corp)

## Prerequisites

- Red Hat VPN connected
- tinycode installed (`brew install bobbyjohnstx/tap/tinycode` or `make build`)

## One-time setup

### 1. Add MAAS endpoint to config

Edit `~/.config/tinycode/tinycode.jsonc`:

```jsonc
{
  "provider": {
    "vllm": {
      "options": {
        "baseURL": "https://models.corp.redhat.com/oai"
      }
    }
  }
}
```

Or set the environment variable:

```bash
export TINYCODE_VLLM_HOST=https://models.corp.redhat.com/oai
```

### 2. Set your API key

```bash
tinycode providers login vllm
# Paste your MAAS API key (starts with sk-oai-...)
```

Get a key at the [MaaS portal](https://models.corp.redhat.com) (sign in with SSO, click New key).

## Daily use

```bash
tinycode                              # start TUI, /connect to pick a model
tinycode -m vllm/claude-haiku-5-5     # start with a specific model
tinycode -m vllm/qwen3-8-27b-fp8     # internal RHOAI model (no sandbox limits)
```

## In the TUI

| Command | What it does |
|---------|-------------|
| `/connect` | Model picker (shows all MAAS models under "vLLM") |
| `/thinking high` | Enable extended thinking (vLLM: thinking_token_budget, all providers supported) |
| `/thinking off` | Disable reasoning (sends enable_thinking:false to vLLM) |
| `/effort high` | Increase reasoning depth |
| `/markdown-style dark` | Change text display style |

## Key models

| Model | Use for | Rate limit |
|-------|---------|-----------|
| `claude-haiku-5-5` | Fast daily work | Sandbox: 10 req/min |
| `claude-sonnet-5-5` | Complex tasks | Sandbox: 10 req/min |
| `qwen3-8-27b-fp8` | Best internal model, no sandbox limits | Personal: 30 req/min |
| `granite-4-1-30b` | Red Hat model, good with code-reviewer agent | Personal: 30 req/min |
| `gemini-3-7-flash` | Highest benchmark score (19/20) | Sandbox: 10 req/min |
| `gpt-oss-120b` | Fastest top scorer (18/20 in 33s) | External (IBM) |

### Account types

| Type | Tokens/min | Requests/min | How to get |
|------|-----------|-------------|-----------|
| Sandbox | 50,000 | 10 | Self-service, accept AUP |
| Personal | 300,000 | 30 | Self-service, accept AUP |
| Enterprise | Up to 80M | By tier | Requires AIA + CMDB AppCode |

Token budgets count both prompt + completion.

## Troubleshooting

```bash
tinycode doctor                       # diagnose connection issues
```

| Problem | Fix |
|---------|-----|
| `no such host` | Connect to VPN |
| `invalid_api_key` | Rotate key in MaaS portal, re-run `tinycode providers login vllm` |
| `rate limit` (403 disguised) | Switch to internal model (qwen, granite) or wait 60s |
| Models not showing | `/connect` then press `r` to rescan |
| `max_tokens > max_model_len` | tinycode auto-clamps since v2.7.1; rebuild if on older version |

## Project-level config

Add to `tinycode.jsonc` in your project root to set a default model per project:

```jsonc
{
  "model": "vllm/qwen3-8-27b-fp8",
  "provider": {
    "vllm": {
      "options": {
        "baseURL": "https://models.corp.redhat.com/oai"
      }
    }
  }
}
```
