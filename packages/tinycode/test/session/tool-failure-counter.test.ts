import { describe, test, expect } from "bun:test"

describe("tool-call failure counter logic", () => {
  // Simulate the counter behavior from processor.ts
  function processToolResult(counter: number, toolName: string): { counter: number; shouldWarn: boolean } {
    if (toolName === "invalid") {
      const newCount = counter + 1
      return { counter: newCount, shouldWarn: newCount >= 3 }
    }
    return { counter: 0, shouldWarn: false }
  }

  test("increments on invalid tool result", () => {
    const result = processToolResult(0, "invalid")
    expect(result.counter).toBe(1)
    expect(result.shouldWarn).toBe(false)
  })

  test("resets on successful tool result", () => {
    const result = processToolResult(5, "bash")
    expect(result.counter).toBe(0)
    expect(result.shouldWarn).toBe(false)
  })

  test("warns at exactly 3 consecutive failures", () => {
    let counter = 0
    counter = processToolResult(counter, "invalid").counter // 1
    counter = processToolResult(counter, "invalid").counter // 2
    const result = processToolResult(counter, "invalid")    // 3
    expect(result.counter).toBe(3)
    expect(result.shouldWarn).toBe(true)
  })

  test("does not warn at 2 consecutive failures", () => {
    let counter = 0
    counter = processToolResult(counter, "invalid").counter // 1
    const result = processToolResult(counter, "invalid")    // 2
    expect(result.shouldWarn).toBe(false)
  })

  test("resets after a successful call breaks the streak", () => {
    let counter = 0
    counter = processToolResult(counter, "invalid").counter // 1
    counter = processToolResult(counter, "invalid").counter // 2
    counter = processToolResult(counter, "bash").counter    // reset to 0
    const result = processToolResult(counter, "invalid")    // 1 again
    expect(result.counter).toBe(1)
    expect(result.shouldWarn).toBe(false)
  })

  test("continues warning after 3 (at 4, 5, etc.)", () => {
    let counter = 0
    for (let i = 0; i < 4; i++) {
      counter = processToolResult(counter, "invalid").counter
    }
    expect(processToolResult(counter, "invalid").shouldWarn).toBe(true)
  })
})
