import { describe, expect, test } from "bun:test"
import { MockLanguageModel } from "./mock-language-model"

describe("MockLanguageModel", () => {
  test("simple text response", async () => {
    const mock = new MockLanguageModel([{ type: "text", content: "Hello World" }])

    const result = await mock.doGenerate({
      prompt: [],
      mode: { type: "regular" },
    })

    expect(result.text).toBe("Hello World")
    expect(result.finishReason).toBe("stop")
    expect(result.usage.promptTokens).toBe(100)
    expect(result.usage.completionTokens).toBe(50)
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
      mode: { type: "regular" },
    })

    expect(result.toolCalls).toHaveLength(1)
    expect(result.toolCalls?.[0].toolName).toBe("Read")
    expect(result.toolCalls?.[0].args).toEqual({ file_path: "/test.txt" })
    expect(result.finishReason).toBe("tool-calls")
  })

  test("streaming text response", async () => {
    const mock = new MockLanguageModel([{ type: "text", content: "Hi" }])

    const chunks: string[] = []
    for await (const chunk of mock.doStream({
      prompt: [],
      mode: { type: "regular" },
    })) {
      if (chunk.type === "text-delta") {
        chunks.push(chunk.textDelta)
      } else if (chunk.type === "finish") {
        expect(chunk.finishReason).toBe("stop")
      }
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
      mode: { type: "regular" },
    })
    expect(first.text).toBe("First")

    const second = await mock.doGenerate({
      prompt: [],
      mode: { type: "regular" },
    })
    expect(second.text).toBe("Second")

    const third = await mock.doGenerate({
      prompt: [],
      mode: { type: "regular" },
    })
    expect(third.text).toBe("Third")

    // Should repeat last scenario
    const fourth = await mock.doGenerate({
      prompt: [],
      mode: { type: "regular" },
    })
    expect(fourth.text).toBe("Third")
  })

  test("error scenario", async () => {
    const mock = new MockLanguageModel([{ type: "error", error: new Error("Test error") }])

    try {
      await mock.doGenerate({
        prompt: [],
        mode: { type: "regular" },
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
      mode: { type: "regular" },
    })

    expect(result.usage.promptTokens).toBe(500)
    expect(result.usage.completionTokens).toBe(200)
  })
})
