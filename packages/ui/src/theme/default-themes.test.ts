import { describe, expect, test } from "bun:test"
import { DEFAULT_THEMES } from "./default-themes"

describe("default themes", () => {
  test("contains at least 30 themes", () => {
    const count = Object.keys(DEFAULT_THEMES).length
    expect(count).toBeGreaterThanOrEqual(30)
  })

  test("every theme has an id matching its map key", () => {
    for (const [key, theme] of Object.entries(DEFAULT_THEMES)) {
      expect(theme.id).toBe(key)
    }
  })

  test("every theme has a non-empty name", () => {
    for (const theme of Object.values(DEFAULT_THEMES)) {
      expect(typeof theme.name).toBe("string")
      expect(theme.name.length).toBeGreaterThan(0)
    }
  })

  test("every theme has both light and dark variants", () => {
    for (const [key, theme] of Object.entries(DEFAULT_THEMES)) {
      expect(theme.light).toBeDefined()
      expect(theme.dark).toBeDefined()
      const lightHasColors = "seeds" in theme.light || "palette" in theme.light
      const darkHasColors = "seeds" in theme.dark || "palette" in theme.dark
      expect(lightHasColors).toBe(true)
      expect(darkHasColors).toBe(true)
    }
  })

  test("oc-2 default theme exists and is accessible", () => {
    expect(DEFAULT_THEMES["oc-2"]).toBeDefined()
    expect(DEFAULT_THEMES["oc-2"].name).toBeTruthy()
  })
})
