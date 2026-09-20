import { describe, expect, test } from "bun:test"
import {
  hexToRgb,
  rgbToHex,
  rgbToOklch,
  oklchToRgb,
  hexToOklch,
  oklchToHex,
  fitOklch,
  generateScale,
  mixColors,
  shift,
  blend,
  lighten,
  darken,
  withAlpha,
} from "./color"

describe("hexToRgb", () => {
  test("converts 6-digit hex to normalized rgb", () => {
    const result = hexToRgb("#ff0000")
    expect(result.r).toBeCloseTo(1, 5)
    expect(result.g).toBeCloseTo(0, 5)
    expect(result.b).toBeCloseTo(0, 5)
  })

  test("converts 3-digit shorthand hex", () => {
    const result = hexToRgb("#fff")
    expect(result.r).toBeCloseTo(1, 5)
    expect(result.g).toBeCloseTo(1, 5)
    expect(result.b).toBeCloseTo(1, 5)
  })

  test("handles black", () => {
    const result = hexToRgb("#000000")
    expect(result.r).toBeCloseTo(0, 5)
    expect(result.g).toBeCloseTo(0, 5)
    expect(result.b).toBeCloseTo(0, 5)
  })

  test("strips leading hash", () => {
    const result = hexToRgb("#808080")
    expect(result.r).toBeCloseTo(128 / 255, 3)
    expect(result.g).toBeCloseTo(128 / 255, 3)
    expect(result.b).toBeCloseTo(128 / 255, 3)
  })
})

describe("rgbToHex", () => {
  test("converts normalized rgb to hex string", () => {
    expect(rgbToHex(1, 0, 0)).toBe("#ff0000")
  })

  test("converts white", () => {
    expect(rgbToHex(1, 1, 1)).toBe("#ffffff")
  })

  test("converts black", () => {
    expect(rgbToHex(0, 0, 0)).toBe("#000000")
  })

  test("clamps values outside 0-1 range", () => {
    const result = rgbToHex(1.5, -0.5, 0.5)
    expect(result).toBe("#ff0080")
  })
})

describe("hexToOklch and oklchToHex roundtrip", () => {
  test("roundtrips a mid-gamut color through oklch", () => {
    const oklch = hexToOklch("#336699")
    expect(oklch.l).toBeGreaterThan(0)
    expect(oklch.c).toBeGreaterThan(0)
    const hex = oklchToHex(oklch)
    expect(hex).toBe("#336699")
  })

  test("roundtrips white through oklch", () => {
    const oklch = hexToOklch("#ffffff")
    expect(oklch.l).toBeCloseTo(1, 1)
    expect(oklch.c).toBeCloseTo(0, 2)
    const hex = oklchToHex(oklch)
    expect(hex).toBe("#ffffff")
  })

  test("roundtrips black through oklch", () => {
    const oklch = hexToOklch("#000000")
    expect(oklch.l).toBeCloseTo(0, 1)
    const hex = oklchToHex(oklch)
    expect(hex).toBe("#000000")
  })
})

describe("fitOklch", () => {
  test("returns in-gamut colors unchanged", () => {
    const oklch = hexToOklch("#336699")
    const fitted = fitOklch(oklch)
    expect(fitted.l).toBeCloseTo(oklch.l, 5)
    expect(fitted.c).toBeCloseTo(oklch.c, 5)
  })

  test("clamps lightness to 0-1 range", () => {
    const fitted = fitOklch({ l: 1.5, c: 0.1, h: 180 })
    expect(fitted.l).toBe(1)
  })

  test("reduces chroma for out-of-gamut colors", () => {
    const fitted = fitOklch({ l: 0.5, c: 0.8, h: 120 })
    expect(fitted.c).toBeLessThan(0.8)
    const rgb = oklchToRgb(fitted)
    expect(rgb.r).toBeGreaterThanOrEqual(0)
    expect(rgb.r).toBeLessThanOrEqual(1)
  })
})

describe("mixColors", () => {
  test("returns first color at amount 0", () => {
    const result = mixColors("#336699", "#669933", 0)
    expect(result).toBe("#336699")
  })

  test("returns second color at amount 1", () => {
    const result = mixColors("#336699", "#669933", 1)
    expect(result).toBe("#669933")
  })

  test("produces a midpoint color at amount 0.5", () => {
    const result = mixColors("#000000", "#ffffff", 0.5)
    const rgb = hexToRgb(result)
    expect(rgb.r).toBeGreaterThan(0.3)
    expect(rgb.r).toBeLessThan(0.7)
  })
})

describe("shift", () => {
  test("increases lightness", () => {
    const original = hexToOklch("#808080")
    const shifted = shift("#808080", { l: 0.1 })
    const result = hexToOklch(shifted)
    expect(result.l).toBeGreaterThan(original.l)
  })

  test("applies chroma multiplier", () => {
    const original = hexToOklch("#ff6600")
    const shifted = shift("#ff6600", { c: 0.5 })
    const result = hexToOklch(shifted)
    expect(result.c).toBeLessThan(original.c)
  })

  test("shifts hue", () => {
    const original = hexToOklch("#ff0000")
    const shifted = shift("#ff0000", { h: 120 })
    const result = hexToOklch(shifted)
    const expected = (original.h + 120) % 360
    expect(Math.abs(result.h - expected)).toBeLessThan(5)
  })
})

describe("blend", () => {
  test("returns foreground at alpha 1", () => {
    expect(blend("#ff0000", "#0000ff", 1)).toBe("#ff0000")
  })

  test("returns background at alpha 0", () => {
    expect(blend("#ff0000", "#0000ff", 0)).toBe("#0000ff")
  })

  test("blends colors at alpha 0.5", () => {
    const result = blend("#ff0000", "#0000ff", 0.5)
    const rgb = hexToRgb(result)
    expect(rgb.r).toBeCloseTo(0.5, 1)
    expect(rgb.b).toBeCloseTo(0.5, 1)
  })
})

describe("lighten and darken", () => {
  test("lighten increases lightness", () => {
    const original = hexToOklch("#808080")
    const result = hexToOklch(lighten("#808080", 0.1))
    expect(result.l).toBeGreaterThan(original.l)
  })

  test("darken decreases lightness", () => {
    const original = hexToOklch("#808080")
    const result = hexToOklch(darken("#808080", 0.1))
    expect(result.l).toBeLessThan(original.l)
  })

  test("lighten clamps at maximum lightness", () => {
    const result = hexToOklch(lighten("#ffffff", 0.5))
    expect(result.l).toBeCloseTo(1, 1)
  })

  test("darken clamps at minimum lightness", () => {
    const result = hexToOklch(darken("#000000", 0.5))
    expect(result.l).toBeCloseTo(0, 1)
  })
})

describe("withAlpha", () => {
  test("produces rgba string with correct values", () => {
    const result = withAlpha("#ff0000", 0.5)
    expect(result).toBe("rgba(255, 0, 0, 0.5)")
  })

  test("handles white with full opacity", () => {
    const result = withAlpha("#ffffff", 1)
    expect(result).toBe("rgba(255, 255, 255, 1)")
  })
})

describe("generateScale", () => {
  test("produces 12-step scale for dark mode", () => {
    const scale = generateScale("#3366cc", true)
    expect(scale).toHaveLength(12)
    scale.forEach((hex) => {
      expect(hex).toMatch(/^#[0-9a-f]{6}$/)
    })
  })

  test("produces 12-step scale for light mode", () => {
    const scale = generateScale("#3366cc", false)
    expect(scale).toHaveLength(12)
    scale.forEach((hex) => {
      expect(hex).toMatch(/^#[0-9a-f]{6}$/)
    })
  })

  test("dark scale first step is darker than last step", () => {
    const scale = generateScale("#3366cc", true)
    const first = hexToOklch(scale[0])
    const last = hexToOklch(scale[11])
    expect(first.l).toBeLessThan(last.l)
  })

  test("light scale first step is lighter than last step", () => {
    const scale = generateScale("#3366cc", false)
    const first = hexToOklch(scale[0])
    const last = hexToOklch(scale[11])
    expect(first.l).toBeGreaterThan(last.l)
  })
})
