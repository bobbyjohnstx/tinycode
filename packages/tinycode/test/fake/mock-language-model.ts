import { type LanguageModelV3, type LanguageModelV3GenerateResult, type LanguageModelV3StreamResult } from "@ai-sdk/provider"

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
  readonly supportedUrls: Record<string, RegExp[]> = {}

  constructor(scenarios?: MockScenario[]) {
    this.scenarios = scenarios ?? [{ type: "text", content: "Mock response" }]
  }

  async doGenerate(
    _options: Parameters<LanguageModelV3["doGenerate"]>[0]
  ): Promise<LanguageModelV3GenerateResult> {
    const scenario = this.getNextScenario()

    if (scenario.type === "error") {
      throw scenario.error
    }

    const usage = {
      inputTokens: {
        total: scenario.usage?.inputTokens ?? 100,
        noCache: scenario.usage?.inputTokens ?? 100,
        cacheRead: undefined,
        cacheWrite: undefined,
      },
      outputTokens: {
        total: scenario.usage?.outputTokens ?? 50,
        text: scenario.usage?.outputTokens ?? 50,
        reasoning: undefined,
      },
    }

    const finishReason = {
      unified: (scenario.type === "tool-call" ? "tool-calls" : "stop") as
        | "stop"
        | "length"
        | "content-filter"
        | "tool-calls"
        | "error"
        | "other",
      raw: undefined,
    }

    if (scenario.type === "tool-call") {
      return {
        content: scenario.calls.map((call) => ({
          type: "tool-call" as const,
          toolCallId: call.id,
          toolName: call.name,
          input: JSON.stringify(call.args),
        })),
        finishReason,
        usage,
        warnings: [],
      }
    }

    return {
      content: [{ type: "text" as const, text: scenario.content }],
      finishReason,
      usage,
      warnings: [],
    }
  }

  async doStream(_options: Parameters<LanguageModelV3["doStream"]>[0]): Promise<LanguageModelV3StreamResult> {
    const scenario = this.getNextScenario()

    const stream = new ReadableStream({
      async start(controller) {
        if (scenario.type === "error") {
          controller.error(scenario.error)
          return
        }

        if (scenario.type === "tool-call") {
          for (const call of scenario.calls) {
            const id = call.id
            controller.enqueue({
              type: "tool-input-start" as const,
              id,
              toolName: call.name,
            })
            controller.enqueue({
              type: "tool-input-delta" as const,
              id,
              delta: JSON.stringify(call.args),
            })
            controller.enqueue({
              type: "tool-input-end" as const,
              id,
              input: call.args,
            })
          }
          controller.enqueue({
            type: "finish" as const,
            finishReason: { unified: "tool-calls" as const, raw: undefined },
            usage: {
              inputTokens: {
                total: scenario.usage?.inputTokens ?? 100,
                noCache: scenario.usage?.inputTokens ?? 100,
                cacheRead: undefined,
                cacheWrite: undefined,
              },
              outputTokens: {
                total: scenario.usage?.outputTokens ?? 50,
                text: scenario.usage?.outputTokens ?? 50,
                reasoning: undefined,
              },
            },
          })
          controller.close()
          return
        }

        // Text response: stream character by character
        const id = "text-0"
        controller.enqueue({ type: "text-start" as const, id })
        for (const char of scenario.content) {
          controller.enqueue({
            type: "text-delta" as const,
            id,
            delta: char,
          })
        }
        controller.enqueue({ type: "text-end" as const, id })
        controller.enqueue({
          type: "finish" as const,
          finishReason: { unified: "stop" as const, raw: undefined },
          usage: {
            inputTokens: {
              total: scenario.usage?.inputTokens ?? 100,
              noCache: scenario.usage?.inputTokens ?? 100,
              cacheRead: undefined,
              cacheWrite: undefined,
            },
            outputTokens: {
              total: scenario.usage?.outputTokens ?? 50,
              text: scenario.usage?.outputTokens ?? 50,
              reasoning: undefined,
            },
          },
        })
        controller.close()
      },
    })

    return { stream }
  }

  private getNextScenario(): MockScenario {
    const index = this.callCount++
    if (index >= this.scenarios.length) {
      return this.scenarios[this.scenarios.length - 1] ?? { type: "text", content: "Mock response" }
    }
    return this.scenarios[index]
  }
}
