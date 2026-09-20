import { describe, expect, test, beforeEach } from "bun:test"
import { resolveThemeVariant, themeToCss } from "./resolve"
import type { DesktopTheme, ThemeVariant } from "./types"

// Test the CSS generation pipeline that loader.ts uses internally.
// We cannot test applyTheme/removeTheme/getActiveTheme/setColorScheme directly
// because they depend on a real document, but we CAN test the CSS output they produce
// by calling the same functions (resolveThemeVariant + themeToCss) they delegate to.

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

describe("theme CSS generation pipeline", () => {
  test("generates valid CSS from seed variant for dark mode", () => {
    const tokens = resolveThemeVariant(seedVariant, true)
    const css = themeToCss(tokens)
    expect(css).toContain("--background-base:")
    expect(css).toContain("--text-base:")
    expect(css).toContain("--syntax-string:")
    expect(css).toContain("--markdown-heading:")
  })

  test("generates valid CSS from seed variant for light mode", () => {
    const tokens = resolveThemeVariant(seedVariant, false)
    const css = themeToCss(tokens)
    expect(css).toContain("--background-base:")
    expect(css).toContain("--text-base:")
  })

  test("generates valid CSS from palette variant", () => {
    const tokens = resolveThemeVariant(paletteVariant, true)
    const css = themeToCss(tokens)
    expect(css).toContain("--background-base:")
    expect(css).toContain("--syntax-string:")
  })

  test("palette variant uses compact syntax tokens", () => {
    const tokens = resolveThemeVariant(paletteVariant, true)
    // Compact themes derive syntax colors from palette colors instead of hardcoded values
    const css = themeToCss(tokens)
    expect(css).toContain("--syntax-comment:")
    expect(css).toContain("--syntax-variable:")
  })

  test("CSS declarations use valid hex or var format", () => {
    const tokens = resolveThemeVariant(seedVariant, true)
    for (const [key, value] of Object.entries(tokens)) {
      expect(
        value.startsWith("#") || value.startsWith("var(") || value.startsWith("rgba("),
      ).toBe(true)
    }
  })

  test("overrides are included in generated CSS", () => {
    const variant: ThemeVariant = {
      ...seedVariant,
      overrides: { "background-base": "#000000", "text-base": "#ffffff" },
    }
    const tokens = resolveThemeVariant(variant, true)
    const css = themeToCss(tokens)
    expect(css).toContain("--background-base: #000000;")
    expect(css).toContain("--text-base: #ffffff;")
  })

  test("generates diff token colors", () => {
    const tokens = resolveThemeVariant(seedVariant, true)
    expect(tokens["surface-diff-add-base"]).toBeDefined()
    expect(tokens["surface-diff-delete-base"]).toBeDefined()
    expect(tokens["text-diff-add-base"]).toBeDefined()
    expect(tokens["text-diff-delete-base"]).toBeDefined()
  })

  test("generates avatar palette", () => {
    const tokens = resolveThemeVariant(seedVariant, true)
    expect(tokens["avatar-background-pink"]).toBeDefined()
    expect(tokens["avatar-background-mint"]).toBeDefined()
    expect(tokens["avatar-text-pink"]).toBeDefined()
    expect(tokens["avatar-text-mint"]).toBeDefined()
  })

  test("generates agent icon tokens", () => {
    const tokens = resolveThemeVariant(seedVariant, true)
    expect(tokens["icon-agent-plan-base"]).toBeDefined()
    expect(tokens["icon-agent-docs-base"]).toBeDefined()
    expect(tokens["icon-agent-ask-base"]).toBeDefined()
    expect(tokens["icon-agent-build-base"]).toBeDefined()
  })

  test("both palette and seeds throws", () => {
    const variant = {
      palette: paletteVariant.palette,
      seeds: seedVariant.seeds,
    } as unknown as ThemeVariant
    expect(() => resolveThemeVariant(variant, true)).toThrow("cannot define both")
  })

  test("light and dark produce different background colors", () => {
    const darkTokens = resolveThemeVariant(seedVariant, true)
    const lightTokens = resolveThemeVariant(seedVariant, false)
    expect(darkTokens["background-base"]).not.toBe(lightTokens["background-base"])
  })
})
