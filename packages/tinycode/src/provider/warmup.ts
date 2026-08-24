export type WarmupResult = {
  ready: boolean
  toolcall: boolean
  durationMs: number
  model: string
}

const WARMUP_TOOL = {
  type: "function" as const,
  function: {
    name: "ready",
    description: "Confirm model is loaded and ready",
    parameters: { type: "object", properties: {} },
  },
}

const LOCAL_PROVIDERS = new Set(["ollama", "ramalama", "vllm", "maas", "lmstudio"])

export function isLocalProvider(providerID: string): boolean {
  return LOCAL_PROVIDERS.has(providerID)
}

export function ollamaBaseURL(): string {
  const env = process.env["TINYCODE_OLLAMA_HOST"]
  return env ? env.replace(/\/+$/, "") : "http://localhost:11434"
}

export async function warmup(model: string, timeoutMs = 15000): Promise<WarmupResult> {
  return warmupOpenAI(ollamaBaseURL(), model, timeoutMs)
}

export async function warmupOpenAI(baseURL: string, model: string, timeoutMs = 15000): Promise<WarmupResult> {
  const start = Date.now()
  const base = baseURL.replace(/\/+$/, "")
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), timeoutMs)

  try {
    const res = await fetch(`${base}/v1/chat/completions`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        model,
        messages: [{ role: "user", content: "Call the ready tool to confirm you are ready." }],
        tools: [WARMUP_TOOL],
        tool_choice: "auto",
        max_tokens: 64,
        stream: false,
      }),
      signal: controller.signal,
    })

    clearTimeout(timer)

    if (!res.ok) {
      return { ready: false, toolcall: false, durationMs: Date.now() - start, model }
    }

    const body = (await res.json()) as {
      choices?: { message?: { tool_calls?: { function?: { name: string } }[] } }[]
    }

    const toolcall =
      Array.isArray(body.choices?.[0]?.message?.tool_calls) &&
      body.choices![0].message!.tool_calls!.length > 0

    return { ready: true, toolcall, durationMs: Date.now() - start, model }
  } catch {
    clearTimeout(timer)
    return { ready: false, toolcall: false, durationMs: Date.now() - start, model }
  }
}
