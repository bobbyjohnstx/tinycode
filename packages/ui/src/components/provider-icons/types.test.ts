import { describe, expect, test } from "bun:test"
import { iconNames } from "./types"

describe("provider icon names", () => {
  test("contains at least 50 provider icons", () => {
    expect(iconNames.length).toBeGreaterThanOrEqual(50)
  })

  test("all names are non-empty strings", () => {
    for (const name of iconNames) {
      expect(typeof name).toBe("string")
      expect(name.length).toBeGreaterThan(0)
    }
  })

  test("contains no duplicate entries", () => {
    const unique = new Set(iconNames)
    expect(unique.size).toBe(iconNames.length)
  })

  test("includes known major providers", () => {
    expect(iconNames).toContain("openai")
    expect(iconNames).toContain("anthropic")
    expect(iconNames).toContain("google")
  })
})
