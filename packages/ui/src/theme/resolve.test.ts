import { describe, expect, test } from "bun:test"
import { resolveThemeVariant, resolveTheme, themeToCss } from "./resolve"
import type { ThemeVariant, DesktopTheme } from "./types"

const seedVariant: ThemeVariant = {
  seeds: {
    neutral: "#1a1b26",
    primary: "#7aa2f7",
    success: "#9ece6a",
    warning: "#e0af68",
    error: "#f7768e",
    info: "#7dcfff",
    interactive: "#7aa2f7",
    diffAdd: "#9ece6a",
    diffDelete: "#f7768e",
  },
}

const paletteVariant: ThemeVariant = {
  palette: {
    neutral: "#1a1b26",
    ink: "#c0caf5",
    primary: "#7aa2f7",
    success: "#9ece6a",
    warning: "#e0af68",
    error: "#f7768e",
    info: "#7dcfff",
  },
}

describe("resolveThemeVariant", () => {
  test("produces a token map from seed variant", () => {
    const tokens = resolveThemeVariant(seedVariant, true)
    expect(tokens["background-base"]).toBeDefined()
    expect(tokens["text-base"]).toBeDefined()
    expect(tokens["border-base"]).toBeDefined()
    expect(typeof tokens["background-base"]).toBe("string")
  })

  test("produces a token map from palette variant", () => {
    const tokens = resolveThemeVariant(paletteVariant, true)
    expect(tokens["background-base"]).toBeDefined()
    expect(tokens["text-base"]).toBeDefined()
    expect(tokens["border-base"]).toBeDefined()
  })

  test("produces different tokens for light vs dark mode", () => {
    const dark = resolveThemeVariant(seedVariant, true)
    const light = resolveThemeVariant(seedVariant, false)
    expect(dark["background-base"]).not.toBe(light["background-base"])
  })

  test("applies overrides to resolved tokens", () => {
    const variant: ThemeVariant = {
      ...seedVariant,
      overrides: { "background-base": "#000000" },
    }
    const tokens = resolveThemeVariant(variant, true)
    expect(tokens["background-base"]).toBe("#000000")
  })

  test("throws when variant has neither palette nor seeds", () => {
    expect(() => resolveThemeVariant({} as ThemeVariant, true)).toThrow("requires `palette` or `seeds`")
  })
})

describe("resolveTheme", () => {
  test("returns both light and dark token maps", () => {
    const theme: DesktopTheme = {
      name: "Test",
      id: "test",
      light: seedVariant,
      dark: seedVariant,
    }
    const result = resolveTheme(theme)
    expect(result.light).toBeDefined()
    expect(result.dark).toBeDefined()
    expect(result.light["background-base"]).not.toBe(result.dark["background-base"])
  })
})

describe("themeToCss", () => {
  test("converts tokens to CSS custom property declarations", () => {
    const tokens = resolveThemeVariant(seedVariant, true)
    const css = themeToCss(tokens)
    expect(css).toContain("--background-base:")
    expect(css).toContain("--text-base:")
    expect(css).toContain("--border-base:")
  })

  test("formats each token as --key: value", () => {
    const css = themeToCss({ "my-token": "#ff0000" })
    expect(css).toBe("--my-token: #ff0000;")
  })
})
