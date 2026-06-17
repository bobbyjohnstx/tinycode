/**
 * Curated fallback model catalog for offline/air-gapped operation.
 * Used when models.dev is unreachable and no local cache exists.
 */
import type { Provider } from "./models-dev"

function makeModel(input: {
  id: string
  name: string
  context: number
  output: number
  inputCost: number
  outputCost: number
  reasoning?: boolean
  attachment?: boolean
}) {
  return {
    id: input.id,
    name: input.name,
    release_date: "2025-01-01",
    attachment: input.attachment ?? false,
    reasoning: input.reasoning ?? false,
    temperature: true,
    tool_call: true,
    cost: {
      input: input.inputCost,
      output: input.outputCost,
    },
    limit: {
      context: input.context,
      output: input.output,
    },
  }
}

export const FALLBACK_CATALOG: Record<string, Provider> = {
  anthropic: {
    id: "anthropic",
    name: "Anthropic",
    env: ["ANTHROPIC_API_KEY"],
    npm: "@ai-sdk/anthropic",
    models: {
      "claude-sonnet-4-20250514": makeModel({
        id: "claude-sonnet-4-20250514",
        name: "Claude Sonnet 4",
        context: 200000,
        output: 16384,
        inputCost: 3,
        outputCost: 15,
        reasoning: true,
        attachment: true,
      }),
      "claude-haiku-4-20250414": makeModel({
        id: "claude-haiku-4-20250414",
        name: "Claude Haiku 4",
        context: 200000,
        output: 16384,
        inputCost: 0.8,
        outputCost: 4,
        attachment: true,
      }),
    },
  },
  openai: {
    id: "openai",
    name: "OpenAI",
    env: ["OPENAI_API_KEY"],
    npm: "@ai-sdk/openai",
    models: {
      "gpt-4.1": makeModel({
        id: "gpt-4.1",
        name: "GPT-4.1",
        context: 1047576,
        output: 32768,
        inputCost: 2,
        outputCost: 8,
        attachment: true,
      }),
      "gpt-4.1-mini": makeModel({
        id: "gpt-4.1-mini",
        name: "GPT-4.1 Mini",
        context: 1047576,
        output: 32768,
        inputCost: 0.4,
        outputCost: 1.6,
        attachment: true,
      }),
    },
  },
  google: {
    id: "google",
    name: "Google",
    env: ["GOOGLE_GENERATIVE_AI_API_KEY"],
    npm: "@ai-sdk/google",
    models: {
      "gemini-2.5-flash": makeModel({
        id: "gemini-2.5-flash",
        name: "Gemini 2.5 Flash",
        context: 1048576,
        output: 65536,
        inputCost: 0.15,
        outputCost: 0.6,
        reasoning: true,
        attachment: true,
      }),
      "gemini-2.5-pro": makeModel({
        id: "gemini-2.5-pro",
        name: "Gemini 2.5 Pro",
        context: 1048576,
        output: 65536,
        inputCost: 1.25,
        outputCost: 10,
        reasoning: true,
        attachment: true,
      }),
    },
  },
}

export * as ModelsFallback from "./models-fallback"
