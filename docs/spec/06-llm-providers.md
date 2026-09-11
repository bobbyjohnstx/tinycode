# 6. LLM Providers

Packages: `internal/llm/`, `internal/provider/`

tinycode-go uses an OpenAI-compatible API client to communicate with all LLM providers. Provider discovery runs in the background, polling local endpoints and registering models.

## 6.1 LLM Client

Package: `internal/llm/`

### OpenAI Client

```go
type OpenAIClient struct {
    BaseURL string
    APIKey  string
    Client  *http.Client
}
```

All providers are accessed through `OpenAIClient.Stream()`, which sends a `POST /chat/completions` request with `stream: true` and reads SSE responses.

### Request Schema

```go
type Request struct {
    Model       string    // model API ID
    Messages    []Message // conversation history
    Tools       []Tool    // available tool definitions
    Stream      bool      // always true for streaming
    Temperature *float64  // optional
    TopP        *float64  // optional
    MaxTokens   *int      // optional
}
```

### Message Schema

```go
type Message struct {
    Role       string     // "system", "user", "assistant", "tool"
    Content    any        // string or []ContentPart
    ToolCalls  []ToolCall // assistant tool invocations
    ToolCallID string     // tool result correlation
}

type ToolCall struct {
    ID       string       // unique call ID
    Type     string       // "function"
    Function FunctionCall // {Name, Arguments}
}

type Tool struct {
    Type     string       // "function"
    Function ToolFunction // {Name, Description, Parameters}
}
```

### Streaming Architecture

1. HTTP POST with `Accept: text/event-stream`
2. Background goroutine reads SSE lines via `bufio.Scanner` (1MB buffer limit)
3. Lines dispatched to event channel (capacity: 64)
4. Timeout management: separate timer for each chunk (5 min)
5. Context cancellation propagated to HTTP request

### JSON Repair

Small models frequently produce malformed JSON in tool call arguments. `RepairToolCallJSON()` attempts to fix common issues:
- Missing closing braces/brackets
- Trailing commas
- Unquoted string values
- Truncated strings

If repair fails, the tool call is redirected to the `invalid` tool, which returns the malformed arguments as an error message to the LLM so it can retry.

## 6.2 Provider Registry

Package: `internal/provider/`

### Registry

Thread-safe registry of discovered providers and their models.

```go
type Registry struct {
    providers map[string]*Info     // providerID → provider info
    disabled  map[string]bool      // disabled by config
    enabled   map[string]bool      // enabled by config (whitelist)
    failures  map[string]int       // consecutive failure counts
}
```

### Provider Info

```go
type Info struct {
    ID      string            // e.g., "ollama", "vllm", "openrouter"
    Name    string            // display name
    Source  string            // "custom" for all Go providers
    Env     []string          // required env vars
    Options map[string]any    // provider-specific options
    Models  map[string]*Model // modelID → model
}
```

### Filtering

`SetFilters(enabled, disabled)` configures provider visibility:
- If `enabled` is non-empty, only listed providers are registered (whitelist)
- `disabled` providers are silently dropped during registration
- Configured via `config.Info.EnabledProviders` / `DisabledProviders`

### Failure Tracking

| Method | Description |
|--------|-------------|
| `RecordFailure(id)` | Increment consecutive failure count, return new count |
| `ResetFailures(id)` | Reset to 0, return previous count |
| `Failures(id)` | Read current count |

## 6.3 Model Schema

```go
type Model struct {
    ID           string            // model identifier within provider
    ProviderID   string            // parent provider ID
    Name         string            // display name
    Family       string            // model family (e.g., "qwen", "llama")
    API          ModelAPI          // {ID, URL} for API calls
    Status       string            // "active"
    Headers      map[string]string // extra HTTP headers
    Options      map[string]any    // provider-specific options
    Cost         ModelCost         // {Input, Output} per million tokens
    Limit        ModelLimit        // {Context, Output} token limits
    Capabilities ModelCaps         // feature flags
}
```

### Model Capabilities

```go
type ModelCaps struct {
    Temperature bool         // supports temperature parameter
    Reasoning   bool         // has reasoning/thinking output
    Attachment  bool         // supports file/image attachments
    ToolCall    bool         // supports function calling
    Input       ModalityCaps // {Text, Audio, Image, Video, PDF}
    Output      ModalityCaps // {Text, Audio, Image, Video, PDF}
}
```

### Model Size Detection

`parseModelSize()` extracts parameter count from model names using regex `(\d+(?:\.\d+)?)\s*[bB]`. Used by the agent registry to select compact prompt variants for models ≤8B.

### Model Suggestion

When `GetModel()` fails, `suggestModel()` does a case-insensitive substring match against registered model names and returns the best suggestion (e.g., "did you mean 'qwen3:8b'?").

## 6.4 Discovery

Package: `internal/provider/discovery.go`

Background polling discovers local LLM providers.

### Discovery Constants

| Constant | Value | Description |
|----------|-------|-------------|
| `probeTimeout` | 2 seconds | HTTP timeout for discovery probes |
| `pollInterval` | 30 seconds | Time between discovery polls |
| `maxConsecutiveFailures` | 3 | Failures before provider goes dormant |

### Discovery Flow

```
Start(ctx, ollamaURL, vllmURL, lmStudioURL)
  ├── First poll runs synchronously (providers available before server starts)
  └── Background goroutine polls every 30s
       ├── discoverOllama()   → GET /api/tags
       ├── discoverVLLM()     → GET /v1/models
       └── discoverLMStudio() → GET /v1/models
```

### Failure Handling

1. Each failed poll increments the provider's failure counter
2. After 3 consecutive failures:
   - Provider removed from registry
   - Marked as dormant (polling stops)
   - `provider.removed` event published
3. On next successful poll:
   - Failure counter reset
   - `provider.reconnected` event published

### Model Diffing

`diffModels()` logs added/removed models compared to the existing registration, providing visibility into model changes between polls.

## 6.5 Supported Providers

### Ollama

- **Discovery URL:** `localhost:11434` (from `OLLAMA_HOST` env or default)
- **API:** `GET /api/tags` for model listing
- **Model ID:** Model name as reported by Ollama (e.g., `qwen3:8b`)
- **Default context:** From `details.context_length`, fallback 8192
- **Capabilities:** All models assumed to support tool calls (verified by warmup probe)
- **Vision:** Detected from Ollama's `capabilities` array (`"vision"` → `Input.Image = true`)
- **Family:** From `details.family`

### Auto-Profiling

Ollama auto-profiling creates custom Modelfiles with optimized `num_ctx` based on GPU memory:

1. Detect GPU memory via `DetectGPUMemory()` (cached, runs once)
2. Query model info via `ShowModel()` (Ollama `/api/show`)
3. Calculate optimal `num_ctx` via `CalculateNumCtx(gpuMemory, modelInfo, advertisedCtx)`
4. Create profile via `CreateProfile()` — sends Modelfile to Ollama
5. Profile name format: `<model>-tc-<numctx>` (e.g., `qwen3:8b-tc-16384`)

Stale profiles (base model deleted) are automatically cleaned up.

Configuration via `config.Info`:
```json
{
  "auto_profile": {
    "enabled": true,
    "default_num_ctx": 32768,
    "max_num_ctx": 65536,
    "models": {
      "qwen3:8b": { "num_ctx": 16384 },
      "llama3.3:latest": { "skip": true }
    }
  }
}
```

### vLLM

- **Discovery URL:** `localhost:8000` (from `TINYCODE_VLLM_URLS` env)
- **API:** OpenAI-compatible `GET /v1/models`
- **Context:** From `max_model_len`, fallback 8192
- **Output limit:** `context / 2`
- **Capabilities:** Temperature + tool call enabled by default

### LM Studio

- **Discovery URL:** `localhost:1234` (from `TINYCODE_LM_STUDIO_URL` env)
- **API:** OpenAI-compatible `GET /v1/models`
- **Context:** Fixed 8192
- **Output limit:** Fixed 4096
- **Capabilities:** Temperature + tool call enabled by default

### OpenRouter

- **Discovery:** `DiscoverOpenRouter()` called with `OPENROUTER_API_KEY`
- **API:** `GET https://openrouter.ai/api/v1/models`
- **Rich metadata:** Cost, architecture, supported parameters, reasoning, modalities
- **Output limit:** `top_provider.max_completion_tokens`, fallback `min(16384, context/5)`
- **Capabilities:** Parsed from `supported_parameters` (temperature, tools) and `architecture.input_modalities`
- **Reasoning:** Detected from `reasoning.mandatory` or `reasoning.default_enabled`

## 6.6 Warmup Probes

After Ollama model registration, a background warmup probe is triggered:

1. `WarmupProbe()` sends a minimal tool-call request to the model
2. If the model doesn't support tool calls, `ToolCall` capability is set to `false`
3. `provider.warmup.complete` event published with `{modelID, toolCapable}`

Each model is only probed once (tracked by `warmedModels` map).

## 6.7 Retry Logic

Source: `internal/provider/retry.go`

### Constants

| Constant | Value | Description |
|----------|-------|-------------|
| `MaxRetries` | 5 | Maximum retry attempts |
| `RetryInitDelay` | 2 seconds | Initial backoff delay |
| `RetryBackoff` | 2.0 | Exponential backoff multiplier |
| `RetryMaxDelay` | 30 seconds | Maximum backoff delay |
| `RetryJitter` | 0.25 | ±25% jitter factor |

### Delay Formula

```
delay = min(InitDelay × Backoff^attempt, MaxDelay) ± (delay × Jitter × random)
```

### Retryable Conditions

**HTTP status codes:** 429, 500, 502, 503, 504

**Error message patterns** (28 patterns, case-insensitive):
fetch failed, connection refused, ECONNRESET, ECONNREFUSED, ETIMEDOUT, ENOTFOUND, socket hang up, network error, request timeout, gateway timeout, service unavailable, bad gateway, internal server error, server error, overloaded, rate limit, too many requests, try again later, at capacity, temporarily unavailable, resource exhausted, deadline exceeded, unavailable, connection reset, broken pipe, EPIPE, aborted, stream error, unexpected EOF, incomplete chunked encoding

### Context Overflow Detection

`IsOverflow()` checks 18 patterns matching context length exceeded errors (e.g., "prompt is too long", "exceeds the context window", "context_length_exceeded"), plus HTTP 400/413 prefix check. Overflow triggers compaction instead of retry.

## 6.8 GPU Memory Detection

Source: `internal/provider/gpu_memory.go`

| Platform | Method | Budget |
|----------|--------|--------|
| macOS (Apple Silicon) | `sysctl -n hw.memsize` | 75% of total unified memory |
| Linux (NVIDIA) | `nvidia-smi --query-gpu=memory.total --format=csv,noheader,nounits` | Sum of all GPUs in MiB → bytes |

Detection is cached via `sync.Once` — runs once per process lifetime.

### GPU Budget for Auto-Profiling

`GPUMemoryBudget()` returns 50% of GPU memory, capped at 32GB. This budget is used in `CalculateNumCtx()`.

## 6.9 CalculateNumCtx Algorithm

Source: `internal/provider/ollama.go`

Determines optimal context window size for Ollama models based on available GPU memory:

1. **GPU budget** = `min(gpuMemory / 2, 32GB)`
2. **Model weight bytes** = parameterSize × bitsPerParameter
   - Parameter size parsed from string (e.g., "8.0B" → 8×10⁹)
   - Quantization lookup table:

     | Quantization | Bytes per Parameter |
     |-------------|-------------------|
     | Q4_0 | 0.50 |
     | Q4_K_S | 0.53 |
     | Q4_K_M | 0.55 |
     | Q5_0 | 0.625 |
     | Q5_K_S | 0.63 |
     | Q5_K_M | 0.65 |
     | Q6_K | 0.75 |
     | Q8_0 | 1.0 |
     | FP16/F16/BF16 | 2.0 |
     | (default) | 0.6 |

3. **KV cache budget** = GPU budget − model weights (minimum 100MB)
4. **KV bytes per token** = `2 × blockCount × headDim × headCountKV × 2`
   - `headDim = embeddingLength / headCount`
5. **numCtx** = kvBudget / kvBytesPerToken, rounded down to nearest 1024
6. **Clamped:** min=2048, max=min(advertisedCtx, 131072)

### Profile Naming

Pattern: `{model-name}-tc{N}k` where N = round(numCtx/1024)
- Example: `qwen3:8b-tc32k`
- Detection: regex `-tc\d+k$`
- `BaseModelName()` strips the `-tc{N}k` suffix

### numCtx Priority

1. Per-model override in `AutoProfileConfig.Models[name].NumCtx`
2. Default override in `AutoProfileConfig.DefaultNumCtx`
3. Calculated from GPU memory via `CalculateNumCtx()`

## 6.10 Model Resolution

`ParseModel("provider/model")` splits a model string into `(providerID, modelID)`.

The server resolves models via `registry.GetModel(providerID, modelID)`. On failure, it provides a "did you mean?" suggestion based on substring matching.
