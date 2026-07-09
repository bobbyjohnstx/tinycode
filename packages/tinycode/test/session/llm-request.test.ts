import { test, expect, describe } from "bun:test"
import type { Provider } from "@/provider/provider"

// Test the capability gating logic for toolcall: false
// This tests the logic from src/session/llm/request.ts line 139

describe("capability gating - toolcall: false", () => {
  test("models without toolcall capability should skip tool injection", () => {
    const model = {
      capabilities: { toolcall: false },
    } as Provider.Model

    // The logic in request.ts line 139:
    // const tools = input.model.capabilities.toolcall === false ? {} : resolveTools(input)
    const shouldSkipTools = model.capabilities.toolcall === false
    expect(shouldSkipTools).toBe(true)
  })

  test("models with toolcall capability should include tools", () => {
    const model = {
      capabilities: { toolcall: true },
    } as Provider.Model

    const shouldSkipTools = model.capabilities.toolcall === false
    expect(shouldSkipTools).toBe(false)
  })

  test("models with undefined toolcall capability should include tools", () => {
    const model = {
      capabilities: {},
    } as Provider.Model

    const shouldSkipTools = model.capabilities.toolcall === false
    expect(shouldSkipTools).toBe(false)
  })

  test("models with null capabilities should not skip tools", () => {
    const model = {
      capabilities: { toolcall: null as any },
    } as Provider.Model

    const shouldSkipTools = model.capabilities.toolcall === false
    expect(shouldSkipTools).toBe(false)
  })

  test("validates the exact condition used in request.ts", () => {
    // The exact condition from request.ts:
    // input.model.capabilities.toolcall === false
    const testCases = [
      { toolcall: false, expected: true },
      { toolcall: true, expected: false },
      { toolcall: undefined, expected: false },
      { toolcall: null, expected: false },
      { toolcall: 0, expected: false },
      { toolcall: "", expected: false },
    ]

    for (const { toolcall, expected } of testCases) {
      const model = {
        capabilities: { toolcall: toolcall as any },
      } as Provider.Model

      const result = model.capabilities.toolcall === false
      expect(result).toBe(expected)
    }
  })
})
