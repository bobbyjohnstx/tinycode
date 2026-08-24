import { describe, expect, test } from "bun:test"
import { normalizeCustomProviderID, providerOptions } from "../../../../src/cli/cmd/tui/component/dialog-provider"

describe("providerOptions", () => {
  test("includes a synthetic Other option for custom providers", () => {
    expect(providerOptions([{ id: "openai", name: "OpenAI" }]).at(-1)).toMatchObject({
      title: "Other",
      description: "Custom provider",
      category: "Providers",
    })
  })

  test("does not use Other as the generic provider category", () => {
    // Non-priority providers (like mistral) go to "Cloud" section
    expect(providerOptions([{ id: "mistral", name: "Mistral" }])[0]?.category).toBe("Cloud")
  })

  test("does not collide with a configured provider named other", () => {
    const values = providerOptions([{ id: "other", name: "Other Provider" }]).map((option) => option.value)
    expect(new Set(values).size).toBe(values.length)
  })

  // Test 10: Local / LAN category assignment for local providers
  test("assigns 'Local / LAN' category to ollama, vllm, maas, and lmstudio", () => {
    const opts = providerOptions([
      { id: "ollama", name: "Ollama" },
      { id: "vllm", name: "vLLM" },
      { id: "maas", name: "MaaS" },
      { id: "lmstudio", name: "LM Studio" },
      { id: "anthropic", name: "Anthropic" },
      { id: "openai", name: "OpenAI" },
    ])
    const byId = Object.fromEntries(
      opts
        .filter((o): o is typeof o & { type: "provider"; providerID: string } => o.type === "provider")
        .map((o) => [o.providerID, o]),
    )
    expect(byId["ollama"]?.category).toBe("Local / LAN")
    expect(byId["vllm"]?.category).toBe("Local / LAN")
    expect(byId["maas"]?.category).toBe("Local / LAN")
    expect(byId["lmstudio"]?.category).toBe("Local / LAN")
    expect(byId["anthropic"]?.category).toBe("Cloud")
    expect(byId["openai"]?.category).toBe("Cloud")
  })

  test("normalizes and validates custom provider ids", () => {
    expect(normalizeCustomProviderID("  custom-provider  ")).toBe("custom-provider")
    expect(normalizeCustomProviderID("custom_provider")).toBe("custom_provider")
    expect(normalizeCustomProviderID("@ai-sdk/custom-provider")).toBe("custom-provider")
    expect(normalizeCustomProviderID("-custom-provider")).toBeUndefined()
    expect(normalizeCustomProviderID("Custom Provider")).toBeUndefined()
  })
})
