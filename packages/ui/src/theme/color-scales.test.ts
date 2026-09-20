import { describe, expect, test } from "bun:test"
import {
  generateNeutralScale,
  generateAlphaScale,
  hexToOklch,
  hexToRgb,
} from "./color"

describe("generateNeutralScale", () => {
  test("produces 12-step scale for dark mode without ink", () => {
    const scale = generateNeutralScale("#1a1b26", true)
    expect(scale).toHaveLength(12)
    scale.forEach((hex) => {
      expect(hex).toMatch(/^#[0-9a-f]{6}$/)
    })
  })

  test("produces 12-step scale for light mode without ink", () => {
    const scale = generateNeutralScale("#f5f5f5", false)
    expect(scale).toHaveLength(12)
    scale.forEach((hex) => {
      expect(hex).toMatch(/^#[0-9a-f]{6}$/)
    })
  })

  test("dark mode first step is darker than last step", () => {
    const scale = generateNeutralScale("#1a1b26", true)
    const first = hexToOklch(scale[0])
    const last = hexToOklch(scale[11])
    expect(first.l).toBeLessThan(last.l)
  })

  test("light mode first step is lighter than last step", () => {
    const scale = generateNeutralScale("#f5f5f5", false)
    const first = hexToOklch(scale[0])
    const last = hexToOklch(scale[11])
    expect(first.l).toBeGreaterThan(last.l)
  })

  test("neutral scale has low chroma values", () => {
    const scale = generateNeutralScale("#1a1b26", true)
    for (const hex of scale) {
      const oklch = hexToOklch(hex)
      expect(oklch.c).toBeLessThan(0.1)
    }
  })

  test("produces 12-step scale with ink parameter", () => {
    const scale = generateNeutralScale("#1a1b26", true, "#c0caf5")
    expect(scale).toHaveLength(12)
    scale.forEach((hex) => {
      expect(hex).toMatch(/^#[0-9a-f]{6}$/)
    })
  })

  test("ink-based dark scale transitions from dark background to ink color", () => {
    const ink = "#c0caf5"
    const scale = generateNeutralScale("#1a1b26", true, ink)
    const first = hexToOklch(scale[0])
    const last = hexToOklch(scale[11])
    expect(first.l).toBeLessThan(last.l)
    // Last step should be close to ink lightness
    const inkOklch = hexToOklch(ink)
    expect(Math.abs(last.l - inkOklch.l)).toBeLessThan(0.1)
  })

  test("light mode with ink produces valid scale", () => {
    const scale = generateNeutralScale("#f5f5f5", false, "#1a1a1a")
    expect(scale).toHaveLength(12)
    const first = hexToOklch(scale[0])
    const last = hexToOklch(scale[11])
    expect(first.l).toBeGreaterThan(last.l)
  })

  test("different seeds produce different scales", () => {
    const scale1 = generateNeutralScale("#1a1b26", true)
    const scale2 = generateNeutralScale("#2d2a2e", true)
    expect(scale1[0]).not.toBe(scale2[0])
  })
})

describe("generateAlphaScale", () => {
  test("produces 12-step alpha scale for dark mode", () => {
    const baseScale = generateNeutralScale("#1a1b26", true)
    const alphaScale = generateAlphaScale(baseScale, true)
    expect(alphaScale).toHaveLength(12)
    alphaScale.forEach((hex) => {
      expect(hex).toMatch(/^#[0-9a-f]{6}$/)
    })
  })

  test("produces 12-step alpha scale for light mode", () => {
    const baseScale = generateNeutralScale("#f5f5f5", false)
    const alphaScale = generateAlphaScale(baseScale, false)
    expect(alphaScale).toHaveLength(12)
    alphaScale.forEach((hex) => {
      expect(hex).toMatch(/^#[0-9a-f]{6}$/)
    })
  })

  test("dark mode alpha scale starts near black", () => {
    const baseScale = generateNeutralScale("#1a1b26", true)
    const alphaScale = generateAlphaScale(baseScale, true)
    // First step has very low alpha (0.02), blended on black background
    const first = hexToRgb(alphaScale[0])
    expect(first.r).toBeLessThan(0.15)
    expect(first.g).toBeLessThan(0.15)
    expect(first.b).toBeLessThan(0.15)
  })

  test("light mode alpha scale starts near white", () => {
    const baseScale = generateNeutralScale("#f5f5f5", false)
    const alphaScale = generateAlphaScale(baseScale, false)
    // First step has very low alpha (0.01), blended on white background
    const first = hexToRgb(alphaScale[0])
    expect(first.r).toBeGreaterThan(0.9)
    expect(first.g).toBeGreaterThan(0.9)
    expect(first.b).toBeGreaterThan(0.9)
  })

  test("alpha scale gets progressively stronger", () => {
    const baseScale = generateNeutralScale("#1a1b26", true)
    const alphaScale = generateAlphaScale(baseScale, true)
    const firstLightness = hexToOklch(alphaScale[0]).l
    const lastLightness = hexToOklch(alphaScale[11]).l
    // In dark mode: higher alpha means color moves toward the source (which is lighter)
    expect(lastLightness).toBeGreaterThan(firstLightness)
  })
})
