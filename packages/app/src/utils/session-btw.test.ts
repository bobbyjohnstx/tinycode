import { afterEach, describe, expect, test } from "bun:test"
import { askBtw, buildBtwRequest, clearBtwHistory, getBtwHistory, lastBtwAnswer } from "./session-btw"

afterEach(() => {
  clearBtwHistory()
})

describe("buildBtwRequest", () => {
  test("builds URL, body, and directory header", () => {
    const req = buildBtwRequest({
      url: "http://localhost:4096/",
      sessionID: "ses_1",
      question: "what is this?",
      directory: "/tmp/proj",
    })

    expect(req.url).toBe("http://localhost:4096/session/ses_1/btw")
    expect(req.init.method).toBe("POST")
    expect(req.init.credentials).toBe("include")
    expect(req.init.headers["Content-Type"]).toBe("application/json")
    expect(req.init.headers["x-tinycode-directory"]).toBe(encodeURIComponent("/tmp/proj"))
    expect(JSON.parse(req.init.body)).toEqual({ question: "what is this?" })
  })
})

describe("askBtw", () => {
  test("posts question and stores answer in history", async () => {
    const calls: Array<{ url: string; init: RequestInit }> = []
    const answer = await askBtw({
      url: "http://localhost:4096",
      directory: "/repo",
      sessionID: "ses_abc",
      question: "side q",
      fetch: (async (url, init) => {
        calls.push({ url: String(url), init: init ?? {} })
        return new Response(JSON.stringify({ answer: "side a" }), { status: 200 })
      }) as typeof fetch,
    })

    expect(answer).toBe("side a")
    expect(calls).toHaveLength(1)
    expect(calls[0]?.url).toBe("http://localhost:4096/session/ses_abc/btw")
    expect(getBtwHistory()).toEqual([{ question: "side q", answer: "side a" }])
    expect(lastBtwAnswer()?.answer).toBe("side a")
  })

  test("throws on non-ok response", async () => {
    await expect(
      askBtw({
        url: "http://localhost:4096",
        directory: "/repo",
        sessionID: "ses_abc",
        question: "q",
        fetch: (async () => new Response("boom", { status: 500 })) as typeof fetch,
      }),
    ).rejects.toThrow(/boom|500/)
  })
})
