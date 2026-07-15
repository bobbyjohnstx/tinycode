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

export function ollamaBaseURL(): string {
  const env = process.env["TINYCODE_OLLAMA_HOST"]
  return env ? env.replace(/\/+$/, "") : "http://localhost:11434"
}

export async function warmup(model: string, timeoutMs = 15000): Promise<WarmupResult> {
  const start = Date.now()
  const base = ollamaBaseURL()
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), timeoutMs)

  try {
    const res = await fetch(`${base}/api/chat`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        model,
        messages: [{ role: "user", content: "Call the ready tool to confirm you are ready." }],
        tools: [WARMUP_TOOL],
        stream: false,
        keep_alive: "10m",
      }),
      signal: controller.signal,
    })

    clearTimeout(timer)

    if (!res.ok) {
      return { ready: false, toolcall: false, durationMs: Date.now() - start, model }
    }

    const body = (await res.json()) as {
      message?: { tool_calls?: { function?: { name: string } }[] }
    }

    const toolcall = Array.isArray(body.message?.tool_calls) && body.message!.tool_calls.length > 0

    return { ready: true, toolcall, durationMs: Date.now() - start, model }
  } catch {
    clearTimeout(timer)
    return { ready: false, toolcall: false, durationMs: Date.now() - start, model }
  }
}
