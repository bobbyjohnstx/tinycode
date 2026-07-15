import { describe, test, expect, mock, afterEach } from "bun:test"
import { isLocalProvider } from "../../src/provider/warmup"

describe("warmup", () => {
  describe("isLocalProvider", () => {
    test("identifies ollama as local", () => {
      expect(isLocalProvider("ollama")).toBe(true)
    })

    test("identifies ramalama as local", () => {
      expect(isLocalProvider("ramalama")).toBe(true)
    })

    test("identifies vllm as local", () => {
      expect(isLocalProvider("vllm")).toBe(true)
    })

    test("identifies maas as local", () => {
      expect(isLocalProvider("maas")).toBe(true)
    })

    test("rejects cloud providers", () => {
      expect(isLocalProvider("anthropic")).toBe(false)
      expect(isLocalProvider("openai")).toBe(false)
      expect(isLocalProvider("google")).toBe(false)
      expect(isLocalProvider("openrouter")).toBe(false)
    })

    test("rejects empty string", () => {
      expect(isLocalProvider("")).toBe(false)
    })

    test("rejects custom provider IDs", () => {
      expect(isLocalProvider("vllm-qwen3")).toBe(false)
      expect(isLocalProvider("my-provider")).toBe(false)
    })
  })

  describe("warmup (Ollama native)", () => {
    const originalFetch = globalThis.fetch

    afterEach(() => {
      globalThis.fetch = originalFetch
    })

    test("returns ready=true and toolcall=true when model responds with tool calls", async () => {
      globalThis.fetch = mock(() =>
        Promise.resolve(
          new Response(
            JSON.stringify({
              message: {
                role: "assistant",
                tool_calls: [{ function: { name: "ready", arguments: "{}" } }],
              },
            }),
            { status: 200 },
          ),
        ),
      ) as unknown as typeof fetch

      const { warmup } = await import("../../src/provider/warmup")
      const result = await warmup("test-model", 5000)
      expect(result.ready).toBe(true)
      expect(result.toolcall).toBe(true)
      expect(result.model).toBe("test-model")
      expect(result.durationMs).toBeGreaterThanOrEqual(0)
    })

    test("returns ready=true and toolcall=false when model responds without tool calls", async () => {
      globalThis.fetch = mock(() =>
        Promise.resolve(
          new Response(
            JSON.stringify({
              message: { role: "assistant", content: "I am ready." },
            }),
            { status: 200 },
          ),
        ),
      ) as unknown as typeof fetch

      const { warmup } = await import("../../src/provider/warmup")
      const result = await warmup("test-model", 5000)
      expect(result.ready).toBe(true)
      expect(result.toolcall).toBe(false)
    })

    test("returns ready=false on HTTP error", async () => {
      globalThis.fetch = mock(() =>
        Promise.resolve(new Response("Internal Server Error", { status: 500 })),
      ) as unknown as typeof fetch

      const { warmup } = await import("../../src/provider/warmup")
      const result = await warmup("test-model", 5000)
      expect(result.ready).toBe(false)
      expect(result.toolcall).toBe(false)
    })

    test("returns ready=false on network error", async () => {
      globalThis.fetch = mock(() => Promise.reject(new Error("ECONNREFUSED"))) as unknown as typeof fetch

      const { warmup } = await import("../../src/provider/warmup")
      const result = await warmup("test-model", 5000)
      expect(result.ready).toBe(false)
      expect(result.toolcall).toBe(false)
    })
  })

  describe("warmupOpenAI", () => {
    const originalFetch = globalThis.fetch

    afterEach(() => {
      globalThis.fetch = originalFetch
    })

    test("returns ready=true and toolcall=true when model responds with tool calls", async () => {
      globalThis.fetch = mock(() =>
        Promise.resolve(
          new Response(
            JSON.stringify({
              choices: [
                {
                  message: {
                    role: "assistant",
                    tool_calls: [{ id: "call_1", function: { name: "ready", arguments: "{}" } }],
                  },
                },
              ],
            }),
            { status: 200 },
          ),
        ),
      ) as unknown as typeof fetch

      const { warmupOpenAI } = await import("../../src/provider/warmup")
      const result = await warmupOpenAI("http://localhost:8080", "test-model", 5000)
      expect(result.ready).toBe(true)
      expect(result.toolcall).toBe(true)
      expect(result.model).toBe("test-model")
    })

    test("returns ready=true and toolcall=false when model responds with text only", async () => {
      globalThis.fetch = mock(() =>
        Promise.resolve(
          new Response(
            JSON.stringify({
              choices: [
                {
                  message: { role: "assistant", content: "I am ready." },
                },
              ],
            }),
            { status: 200 },
          ),
        ),
      ) as unknown as typeof fetch

      const { warmupOpenAI } = await import("../../src/provider/warmup")
      const result = await warmupOpenAI("http://localhost:8080", "test-model", 5000)
      expect(result.ready).toBe(true)
      expect(result.toolcall).toBe(false)
    })

    test("returns ready=false on connection refused", async () => {
      globalThis.fetch = mock(() => Promise.reject(new Error("ECONNREFUSED"))) as unknown as typeof fetch

      const { warmupOpenAI } = await import("../../src/provider/warmup")
      const result = await warmupOpenAI("http://localhost:8080", "test-model", 5000)
      expect(result.ready).toBe(false)
      expect(result.toolcall).toBe(false)
    })

    test("sends request to /v1/chat/completions", async () => {
      let capturedURL = ""
      globalThis.fetch = mock((input: RequestInfo | URL) => {
        capturedURL = input.toString()
        return Promise.resolve(
          new Response(JSON.stringify({ choices: [{ message: { content: "ok" } }] }), { status: 200 }),
        )
      }) as unknown as typeof fetch

      const { warmupOpenAI } = await import("../../src/provider/warmup")
      await warmupOpenAI("http://localhost:8080", "test-model", 5000)
      expect(capturedURL).toBe("http://localhost:8080/v1/chat/completions")
    })

    test("strips trailing slash from baseURL", async () => {
      let capturedURL = ""
      globalThis.fetch = mock((input: RequestInfo | URL) => {
        capturedURL = input.toString()
        return Promise.resolve(
          new Response(JSON.stringify({ choices: [{ message: { content: "ok" } }] }), { status: 200 }),
        )
      }) as unknown as typeof fetch

      const { warmupOpenAI } = await import("../../src/provider/warmup")
      await warmupOpenAI("http://localhost:8080/", "test-model", 5000)
      expect(capturedURL).toBe("http://localhost:8080/v1/chat/completions")
    })
  })
})
