import { describe, expect, test } from "bun:test"
import { DEFAULT_THEMES } from "./default-themes"
import { resolveThemeVariant, resolveTheme, themeToCss } from "./resolve"

describe("all default themes resolve without errors", () => {
  for (const [id, theme] of Object.entries(DEFAULT_THEMES)) {
    describe(`${id} (${theme.name})`, () => {
      test("resolves light variant to valid tokens", () => {
        const tokens = resolveThemeVariant(theme.light, false)
        expect(tokens["background-base"]).toBeDefined()
        expect(tokens["text-base"]).toBeDefined()
        expect(tokens["border-base"]).toBeDefined()
        expect(tokens["surface-interactive-base"]).toBeDefined()
      })

      test("resolves dark variant to valid tokens", () => {
        const tokens = resolveThemeVariant(theme.dark, true)
        expect(tokens["background-base"]).toBeDefined()
        expect(tokens["text-base"]).toBeDefined()
        expect(tokens["border-base"]).toBeDefined()
        expect(tokens["surface-interactive-base"]).toBeDefined()
      })

      test("generates non-empty CSS for both variants", () => {
        const result = resolveTheme(theme)
        const lightCss = themeToCss(result.light)
        const darkCss = themeToCss(result.dark)
        expect(lightCss.length).toBeGreaterThan(100)
        expect(darkCss.length).toBeGreaterThan(100)
      })

      test("produces distinct light and dark backgrounds", () => {
        const result = resolveTheme(theme)
        expect(result.light["background-base"]).not.toBe(result.dark["background-base"])
      })
    })
  }
})
