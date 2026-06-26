import { type LanguageModelV3 } from "@ai-sdk/provider"

export type MockScenario =
  | { type: "text"; content: string; usage?: { inputTokens?: number; outputTokens?: number } }
  | {
      type: "tool-call"
      calls: Array<{ id: string; name: string; args: Record<string, unknown> }>
      usage?: { inputTokens?: number; outputTokens?: number }
    }
  | { type: "error"; error: Error }

/**
 * Mock implementation of LanguageModelV3 for deterministic testing.
 *
 * Accepts a sequence of scenarios that define responses for successive calls.
 * Each call to `doStream` or `doGenerate` advances to the next scenario.
 */
export class MockLanguageModel implements LanguageModelV3 {
  private scenarios: MockScenario[]
  private callCount = 0

  readonly specificationVersion = "v3" as const
  readonly provider = "mock" as const
  readonly modelId = "mock-model"
  readonly defaultObjectGenerationMode = "json" as const

  constructor(scenarios?: MockScenario[]) {
    this.scenarios = scenarios ?? [{ type: "text", content: "Mock response" }]
  }

  async doGenerate(_options: Parameters<LanguageModelV3["doGenerate"]>[0]): Promise<{
    text?: string
    toolCalls?: Array<{ toolCallId: string; toolName: string; args: unknown }>
    finishReason: "stop" | "length" | "content-filter" | "tool-calls" | "error" | "other" | "unknown"
    usage: { promptTokens: number; completionTokens: number }
    rawCall: { rawPrompt: unknown; rawSettings: Record<string, unknown> }
    rawResponse?: { headers?: Record<string, string> }
    warnings?: Array<{ type: string; message: string }>
    providerMetadata?: Record<string, unknown>
  }> {
    const scenario = this.getNextScenario()

    if (scenario.type === "error") {
      throw scenario.error
    }

    if (scenario.type === "tool-call") {
      return {
        toolCalls: scenario.calls.map((call) => ({
          toolCallId: call.id,
          toolName: call.name,
          args: call.args,
        })),
        finishReason: "tool-calls",
        usage: {
          promptTokens: scenario.usage?.inputTokens ?? 100,
          completionTokens: scenario.usage?.outputTokens ?? 50,
        },
        rawCall: { rawPrompt: null, rawSettings: {} },
      }
    }

    return {
      text: scenario.content,
      finishReason: "stop",
      usage: {
        promptTokens: scenario.usage?.inputTokens ?? 100,
        completionTokens: scenario.usage?.outputTokens ?? 50,
      },
      rawCall: { rawPrompt: null, rawSettings: {} },
    }
  }

  async *doStream(_options: Parameters<LanguageModelV3["doStream"]>[0]): AsyncIterable<
    | {
        type: "text-delta"
        textDelta: string
      }
    | {
        type: "tool-call-delta"
        toolCallId: string
        toolName: string
        argsTextDelta: string
      }
    | {
        type: "tool-call"
        toolCallId: string
        toolName: string
        args: unknown
      }
    | {
        type: "finish"
        finishReason: "stop" | "length" | "content-filter" | "tool-calls" | "error" | "other" | "unknown"
        usage: { promptTokens: number; completionTokens: number }
        providerMetadata?: Record<string, unknown>
      }
    | {
        type: "error"
        error: unknown
      }
  > {
    const scenario = this.getNextScenario()

    if (scenario.type === "error") {
      yield { type: "error", error: scenario.error }
      return
    }

    if (scenario.type === "tool-call") {
      for (const call of scenario.calls) {
        yield {
          type: "tool-call",
          toolCallId: call.id,
          toolName: call.name,
          args: call.args,
        }
      }
      yield {
        type: "finish",
        finishReason: "tool-calls",
        usage: {
          promptTokens: scenario.usage?.inputTokens ?? 100,
          completionTokens: scenario.usage?.outputTokens ?? 50,
        },
      }
      return
    }

    // Text response: stream character by character
    for (const char of scenario.content) {
      yield {
        type: "text-delta",
        textDelta: char,
      }
    }

    yield {
      type: "finish",
      finishReason: "stop",
      usage: {
        promptTokens: scenario.usage?.inputTokens ?? 100,
        completionTokens: scenario.usage?.outputTokens ?? 50,
      },
    }
  }

  private getNextScenario(): MockScenario {
    const index = this.callCount++
    if (index >= this.scenarios.length) {
      return this.scenarios[this.scenarios.length - 1] ?? { type: "text", content: "Mock response" }
    }
    return this.scenarios[index]
  }
}
