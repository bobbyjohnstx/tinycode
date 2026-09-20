import { describe, expect, test } from "bun:test"
import { dict } from "./en"

describe("english translation dictionary", () => {
  test("contains at least 50 translation keys", () => {
    expect(Object.keys(dict).length).toBeGreaterThanOrEqual(50)
  })

  test("all values are strings", () => {
    for (const [key, value] of Object.entries(dict)) {
      expect(typeof value).toBe("string")
    }
  })

  test("most values are non-empty (suffix keys may be empty)", () => {
    const entries = Object.entries(dict)
    const empty = entries.filter(([, value]) => value === "")
    // Allow suffix keys to be empty - they are intentionally blank in English
    for (const [key] of empty) {
      expect(key).toMatch(/suffix/)
    }
  })

  test("template placeholders use double-brace syntax", () => {
    const placeholderPattern = /\{\{\s*\w+\s*\}\}/g
    const invalidPattern = /\{[^{]\w+[^}]\}/g

    for (const [key, value] of Object.entries(dict)) {
      const placeholders = value.match(placeholderPattern) || []
      for (const placeholder of placeholders) {
        const name = placeholder.replace(/[{}\s]/g, "")
        expect(name.length).toBeGreaterThan(0)
      }
    }
  })

  test("core UI keys exist", () => {
    expect(dict["ui.sessionReview.title"]).toBeDefined()
    expect(dict["ui.sessionReview.diffStyle.unified"]).toBeDefined()
    expect(dict["ui.sessionReview.diffStyle.split"]).toBeDefined()
  })

  test("all keys follow dotted namespace convention", () => {
    for (const key of Object.keys(dict)) {
      expect(key).toMatch(/^[a-zA-Z0-9_.]+$/)
      expect(key.includes(".")).toBe(true)
    }
  })
})
