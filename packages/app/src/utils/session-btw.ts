export type BtwHistoryEntry = {
  question: string
  answer: string
}

const history: BtwHistoryEntry[] = []

export function getBtwHistory(): readonly BtwHistoryEntry[] {
  return history
}

export function clearBtwHistory() {
  history.length = 0
}

export function lastBtwAnswer(): BtwHistoryEntry | undefined {
  return history[history.length - 1]
}

export function buildBtwRequest(input: { url: string; sessionID: string; question: string; directory: string }) {
  const base = input.url.replace(/\/$/, "")
  return {
    url: `${base}/session/${input.sessionID}/btw`,
    init: {
      method: "POST" as const,
      credentials: "include" as const,
      headers: {
        "Content-Type": "application/json",
        "x-tinycode-directory": encodeURIComponent(input.directory),
      },
      body: JSON.stringify({ question: input.question }),
    },
  }
}

export async function askBtw(input: {
  url: string
  directory: string
  sessionID: string
  question: string
  fetch?: typeof fetch
}): Promise<string> {
  const question = input.question.trim()
  if (!question) throw new Error("question is required")

  const { url, init } = buildBtwRequest({
    url: input.url,
    sessionID: input.sessionID,
    question,
    directory: input.directory,
  })

  const fetchFn = input.fetch ?? globalThis.fetch
  const res = await fetchFn(url, init)
  if (!res.ok) {
    const text = await res.text().catch(() => "")
    throw new Error(text || `btw failed (${res.status})`)
  }

  const data = (await res.json()) as { answer?: string }
  const answer = data.answer?.trim() ?? ""
  history.push({ question, answer })
  return answer
}
