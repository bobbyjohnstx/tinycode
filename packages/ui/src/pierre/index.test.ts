import { describe, expect, test } from "bun:test"
import { createDefaultOptions, styleVariables } from "./index"

describe("createDefaultOptions", () => {
  test("returns unified diffStyle when style is undefined", () => {
    const opts = createDefaultOptions(undefined as any)
    expect(opts.diffStyle).toBe("unified")
  })

  test("returns unified diffStyle when style is unified", () => {
    const opts = createDefaultOptions("unified")
    expect(opts.diffStyle).toBe("unified")
  })

  test("returns split diffStyle when style is split", () => {
    const opts = createDefaultOptions("split")
    expect(opts.diffStyle).toBe("split")
  })

  test("uses word-alt lineDiffType for split mode", () => {
    const opts = createDefaultOptions("split")
    expect(opts.lineDiffType).toBe("word-alt")
  })

  test("uses none lineDiffType for unified mode", () => {
    const opts = createDefaultOptions("unified")
    expect(opts.lineDiffType).toBe("none")
  })

  test("uses none lineDiffType when style is undefined", () => {
    const opts = createDefaultOptions(undefined as any)
    expect(opts.lineDiffType).toBe("none")
  })

  test("sets TinyCode theme", () => {
    const opts = createDefaultOptions("unified")
    expect(opts.theme).toBe("TinyCode")
  })

  test("sets system themeType", () => {
    const opts = createDefaultOptions("unified")
    expect(opts.themeType).toBe("system")
  })

  test("disables file header", () => {
    const opts = createDefaultOptions("unified")
    expect(opts.disableFileHeader).toBe(true)
  })

  test("enables line numbers", () => {
    const opts = createDefaultOptions("unified")
    expect(opts.disableLineNumbers).toBe(false)
  })

  test("sets overflow to wrap", () => {
    const opts = createDefaultOptions("unified")
    expect(opts.overflow).toBe("wrap")
  })

  test("sets maxLineDiffLength to 1000", () => {
    const opts = createDefaultOptions("unified")
    expect(opts.maxLineDiffLength).toBe(1000)
  })

  test("sets expansionLineCount to 20", () => {
    const opts = createDefaultOptions("unified")
    expect(opts.expansionLineCount).toBe(20)
  })

  test("includes unsafeCSS string", () => {
    const opts = createDefaultOptions("unified")
    expect(typeof opts.unsafeCSS).toBe("string")
    expect(opts.unsafeCSS.length).toBeGreaterThan(0)
  })
})

describe("styleVariables", () => {
  test("defines font family variable", () => {
    expect(styleVariables["--diffs-font-family"]).toBe("var(--font-family-mono)")
  })

  test("defines font size variable", () => {
    expect(styleVariables["--diffs-font-size"]).toBe("var(--font-size-small)")
  })

  test("defines line height as 24px", () => {
    expect(styleVariables["--diffs-line-height"]).toBe("24px")
  })

  test("defines tab size as 2", () => {
    expect(styleVariables["--diffs-tab-size"]).toBe(2)
  })

  test("defines gap block as 0", () => {
    expect(styleVariables["--diffs-gap-block"]).toBe(0)
  })

  test("defines minimum number column width", () => {
    expect(styleVariables["--diffs-min-number-column-width"]).toBe("4ch")
  })

  test("includes all expected CSS variable keys", () => {
    const expectedKeys = [
      "--diffs-font-family",
      "--diffs-font-size",
      "--diffs-line-height",
      "--diffs-tab-size",
      "--diffs-font-features",
      "--diffs-header-font-family",
      "--diffs-gap-block",
      "--diffs-min-number-column-width",
    ]
    for (const key of expectedKeys) {
      expect(key in styleVariables).toBe(true)
    }
  })
})
