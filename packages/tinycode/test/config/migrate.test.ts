import { describe, expect, test } from "bun:test"
import { checkLegacyEnvVars } from "../../src/config/migrate"
import { FALLBACK_CATALOG } from "../../src/core/models-fallback"

describe("config.migrate", () => {
  test("checkLegacyEnvVars does not throw", () => {
    // Should not throw even when no legacy vars are set
    expect(() => checkLegacyEnvVars()).not.toThrow()
  })
})

describe("models-fallback", () => {
  test("fallback catalog has expected providers", () => {
    expect(FALLBACK_CATALOG).toHaveProperty("anthropic")
    expect(FALLBACK_CATALOG).toHaveProperty("openai")
    expect(FALLBACK_CATALOG).toHaveProperty("google")
  })

  test("anthropic provider has correct structure", () => {
    const anthropic = FALLBACK_CATALOG.anthropic
    expect(anthropic.id).toBe("anthropic")
    expect(anthropic.name).toBe("Anthropic")
    expect(anthropic.env).toContain("ANTHROPIC_API_KEY")
    expect(Object.keys(anthropic.models).length).toBeGreaterThan(0)
  })

  test("models have required fields", () => {
    for (const [_providerID, provider] of Object.entries(FALLBACK_CATALOG)) {
      for (const [_modelID, model] of Object.entries(provider.models)) {
        expect(model.id).toBeTruthy()
        expect(model.name).toBeTruthy()
        expect(model.limit).toBeDefined()
        expect(model.limit.context).toBeGreaterThan(0)
        expect(model.limit.output).toBeGreaterThan(0)
      }
    }
  })
})
