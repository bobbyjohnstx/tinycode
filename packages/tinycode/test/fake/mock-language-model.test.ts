import { describe, expect, test } from "bun:test"
import { MockLanguageModel } from "./mock-language-model"

describe("MockLanguageModel", () => {
  test("simple text response", async () => {
    const mock = new MockLanguageModel([{ type: "text", content: "Hello World" }])

    const result = await mock.doGenerate({
      prompt: [],
    })

    expect(result.content).toHaveLength(1)
    expect(result.content[0].type).toBe("text")
    if (result.content[0].type === "text") {
      expect(result.content[0].text).toBe("Hello World")
    }
    expect(result.finishReason.unified).toBe("stop")
    expect(result.usage.inputTokens.total).toBe(100)
    expect(result.usage.outputTokens.total).toBe(50)
  })

  test("tool call response", async () => {
    const mock = new MockLanguageModel([
      {
        type: "tool-call",
        calls: [{ id: "call-1", name: "Read", args: { file_path: "/test.txt" } }],
      },
    ])

    const result = await mock.doGenerate({
      prompt: [],
    })

    expect(result.content).toHaveLength(1)
    expect(result.content[0].type).toBe("tool-call")
    if (result.content[0].type === "tool-call") {
      expect(result.content[0].toolName).toBe("Read")
      expect(result.content[0].input).toEqual({ file_path: "/test.txt" })
    }
    expect(result.finishReason.unified).toBe("tool-calls")
  })

  test("streaming text response", async () => {
    const mock = new MockLanguageModel([{ type: "text", content: "Hi" }])

    const result = await mock.doStream({
      prompt: [],
    })

    const chunks: string[] = []
    const reader = result.stream.getReader()
    try {
      while (true) {
        const { done, value } = await reader.read()
        if (done) break

        if (value.type === "text-delta") {
          chunks.push(value.delta)
        } else if (value.type === "finish") {
          expect(value.finishReason.unified).toBe("stop")
        }
      }
    } finally {
      reader.releaseLock()
    }

    expect(chunks.join("")).toBe("Hi")
  })

  test("multiple scenario progression", async () => {
    const mock = new MockLanguageModel([
      { type: "text", content: "First" },
      { type: "text", content: "Second" },
      { type: "text", content: "Third" },
    ])

    const first = await mock.doGenerate({
      prompt: [],
    })
    expect(first.content[0].type).toBe("text")
    if (first.content[0].type === "text") {
      expect(first.content[0].text).toBe("First")
    }

    const second = await mock.doGenerate({
      prompt: [],
    })
    expect(second.content[0].type).toBe("text")
    if (second.content[0].type === "text") {
      expect(second.content[0].text).toBe("Second")
    }

    const third = await mock.doGenerate({
      prompt: [],
    })
    expect(third.content[0].type).toBe("text")
    if (third.content[0].type === "text") {
      expect(third.content[0].text).toBe("Third")
    }

    // Should repeat last scenario
    const fourth = await mock.doGenerate({
      prompt: [],
    })
    expect(fourth.content[0].type).toBe("text")
    if (fourth.content[0].type === "text") {
      expect(fourth.content[0].text).toBe("Third")
    }
  })

  test("error scenario", async () => {
    const mock = new MockLanguageModel([{ type: "error", error: new Error("Test error") }])

    try {
      await mock.doGenerate({
        prompt: [],
      })
      expect(true).toBe(false) // Should not reach here
    } catch (error) {
      expect((error as Error).message).toBe("Test error")
    }
  })

  test("custom token usage", async () => {
    const mock = new MockLanguageModel([
      {
        type: "text",
        content: "Custom",
        usage: { inputTokens: 500, outputTokens: 200 },
      },
    ])

    const result = await mock.doGenerate({
      prompt: [],
    })

    expect(result.usage.inputTokens.total).toBe(500)
    expect(result.usage.outputTokens.total).toBe(200)
  })
})
