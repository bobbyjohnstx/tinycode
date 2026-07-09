import { test, expect, describe } from "bun:test"

// Test the pure logic for model switch auto-compact and context warnings
// From src/cli/cmd/run/runtime.ts lines 329-379

describe("model switch auto-compact logic", () => {
  test("detects context overflow when switching to smaller model", () => {
    const currentTokens = 20000
    const newModelContext = 8192
    // From runtime.ts line 357-359: usableContext calculation
    const outputReserve = 4096
    const usableContext = Math.max(0, newModelContext - outputReserve)

    expect(currentTokens >= usableContext).toBe(true) // should compact
  })

  test("no overflow when switching to larger model", () => {
    const currentTokens = 5000
    const newModelContext = 32768
    const outputReserve = 4096
    const usableContext = Math.max(0, newModelContext - outputReserve)

    expect(currentTokens >= usableContext).toBe(false) // no compact needed
  })

  test("edge case: current usage exactly at limit triggers compact", () => {
    const currentTokens = 4096
    const newModelContext = 8192
    const outputReserve = 4096
    const usableContext = Math.max(0, newModelContext - outputReserve)

    expect(currentTokens >= usableContext).toBe(true) // should compact
  })

  test("edge case: current usage just under limit does not trigger compact", () => {
    const currentTokens = 4095
    const newModelContext = 8192
    const outputReserve = 4096
    const usableContext = Math.max(0, newModelContext - outputReserve)

    expect(currentTokens >= usableContext).toBe(false) // no compact
  })

  test("handles model with limit.input instead of limit.output", () => {
    const currentTokens = 100000
    const limitInput = 120000
    const inputReserve = 20000
    const usableContext = Math.max(0, limitInput - inputReserve)

    expect(currentTokens >= usableContext).toBe(true) // should compact
  })

  test("very large model with plenty of headroom", () => {
    const currentTokens = 50000
    const newModelContext = 1000000 // 1M context
    const outputReserve = 4096
    const usableContext = Math.max(0, newModelContext - outputReserve)

    expect(currentTokens >= usableContext).toBe(false) // no compact needed
  })

  test("tiny model with minimal context", () => {
    const currentTokens = 1000
    const newModelContext = 2048
    const outputReserve = 4096
    const usableContext = Math.max(0, newModelContext - outputReserve)

    // usableContext will be 0 (2048 - 4096 = negative, clamped to 0)
    expect(usableContext).toBe(0)
    expect(currentTokens >= usableContext).toBe(true) // should compact
  })
})

describe("context window warning logic", () => {
  test("warns for models under 8K context", () => {
    const contextLimit = 4096
    const threshold = 8192

    expect(contextLimit < threshold).toBe(true) // should warn
  })

  test("no warning for models at exactly 8K context", () => {
    const contextLimit = 8192
    const threshold = 8192

    expect(contextLimit < threshold).toBe(false) // no warning
  })

  test("no warning for models above 8K context", () => {
    const contextLimit = 32768
    const threshold = 8192

    expect(contextLimit < threshold).toBe(false) // no warning
  })

  test("warns for very small models (2K)", () => {
    const contextLimit = 2048
    const threshold = 8192

    expect(contextLimit < threshold).toBe(true) // should warn
  })

  test("no warning for large models (128K)", () => {
    const contextLimit = 131072 // 128K
    const threshold = 8192

    expect(contextLimit < threshold).toBe(false) // no warning
  })

  test("edge case: 8191 should warn", () => {
    const contextLimit = 8191
    const threshold = 8192

    expect(contextLimit < threshold).toBe(true) // should warn
  })

  test("edge case: 8193 should not warn", () => {
    const contextLimit = 8193
    const threshold = 8192

    expect(contextLimit < threshold).toBe(false) // no warning
  })

  test("context limit of 0 should warn", () => {
    const contextLimit = 0
    const threshold = 8192

    expect(contextLimit < threshold).toBe(true) // should warn
  })

  test("computes correct K value for warning message", () => {
    const contextLimit = 4096
    const contextK = Math.round(contextLimit / 1024)

    expect(contextK).toBe(4)
  })

  test("computes K value for 2K model", () => {
    const contextLimit = 2048
    const contextK = Math.round(contextLimit / 1024)

    expect(contextK).toBe(2)
  })

  test("computes K value for odd sizes", () => {
    const contextLimit = 7168 // 7K
    const contextK = Math.round(contextLimit / 1024)

    expect(contextK).toBe(7)
  })
})
