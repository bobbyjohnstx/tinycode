import { describe, expect, test } from "bun:test"
import { iconNames } from "./types"

describe("app icon names", () => {
  test("contains at least 10 app icons", () => {
    expect(iconNames.length).toBeGreaterThanOrEqual(10)
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

  test("includes common editor and terminal apps", () => {
    expect(iconNames).toContain("vscode")
    expect(iconNames).toContain("terminal")
    expect(iconNames).toContain("cursor")
  })
})
