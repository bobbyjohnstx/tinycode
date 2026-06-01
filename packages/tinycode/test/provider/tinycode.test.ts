/**
 * Tests for tinycode-specific provider changes:
 * - defaultModelIDs: must not crash on providers with empty models
 * - LocalDiscovery: Ollama/vLLM probe helpers
 */

import { describe, expect, test } from "bun:test"
import { defaultModelIDs } from "@/provider/provider"
import { ProviderID, ModelID } from "@/provider/schema"

// ---------------------------------------------------------------------------
// Helpers to build minimal Info/Model shapes for testing
// ---------------------------------------------------------------------------

function makeProvider(id: string, modelIds: string[]) {
  const models: Record<string, { id: string; name: string }> = {}
  for (const mid of modelIds) {
    models[mid] = { id: ModelID.make(mid), name: mid }
  }
  return {
    id: ProviderID.make(id),
    name: id,
    source: "config" as const,
    env: [] as string[],
    options: {},
    models: models as any,
  }
}

// ---------------------------------------------------------------------------
// defaultModelIDs
// ---------------------------------------------------------------------------

describe("defaultModelIDs", () => {
  test("returns empty object for empty providers map", () => {
    const result = defaultModelIDs({})
    expect(result).toEqual({})
  })

  test("skips providers with no models — does not crash", () => {
    // This was the bug: sort(...)[0].id on empty array threw
    const providers = {
      "openai-compatible": makeProvider("openai-compatible", []),
    }
    expect(() => defaultModelIDs(providers)).not.toThrow()
    const result = defaultModelIDs(providers)
    // Provider with no models should be absent from the result
    expect(result["openai-compatible"]).toBeUndefined()
  })

  test("returns a model ID for providers that have models", () => {
    const providers = {
      ollama: makeProvider("ollama", ["llama3.2:latest", "codellama:7b"]),
    }
    const result = defaultModelIDs(providers)
    expect(result["ollama"]).toBeDefined()
    expect(typeof result["ollama"]).toBe("string")
  })

  test("skips empty-model providers while returning IDs for populated ones", () => {
    const providers = {
      anthropic: makeProvider("anthropic", ["claude-sonnet-4-5", "claude-haiku-4-5"]),
      "openai-compatible": makeProvider("openai-compatible", []),
      ollama: makeProvider("ollama", ["llama3.2:latest"]),
    }
    const result = defaultModelIDs(providers)
    expect(result["anthropic"]).toBeDefined()
    expect(result["ollama"]).toBeDefined()
    expect(result["openai-compatible"]).toBeUndefined()
  })

  test("returns consistent results across multiple calls (deterministic sort)", () => {
    const providers = {
      openai: makeProvider("openai", ["gpt-4.1", "gpt-4o", "o3"]),
    }
    const r1 = defaultModelIDs(providers)
    const r2 = defaultModelIDs(providers)
    expect(r1["openai"]).toBe(r2["openai"])
  })
})
