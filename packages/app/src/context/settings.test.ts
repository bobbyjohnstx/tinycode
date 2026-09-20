import { describe, expect, test } from "bun:test"
import {
  monoFontFamily,
  sansFontFamily,
  terminalFontFamily,
  monoInput,
  sansInput,
  terminalInput,
  monoDefault,
  sansDefault,
  terminalDefault,
} from "./settings"

describe("monoFontFamily", () => {
  test("returns base monospace stack when font is undefined", () => {
    const result = monoFontFamily(undefined)
    expect(result).toContain("monospace")
    expect(result).not.toContain("undefined")
  })

  test("returns base monospace stack when font is empty string", () => {
    const result = monoFontFamily("")
    expect(result).toContain("monospace")
  })

  test("prepends custom font to monospace stack", () => {
    const result = monoFontFamily("Fira Code")
    expect(result.startsWith('"Fira Code"')).toBe(true)
    expect(result).toContain("monospace")
  })

  test("does not quote simple alphanumeric font names", () => {
    const result = monoFontFamily("Menlo")
    expect(result.startsWith("Menlo,")).toBe(true)
  })

  test("escapes quotes in font name", () => {
    const result = monoFontFamily('Font "Special"')
    expect(result).toContain('\\"Special\\"')
  })
})

describe("sansFontFamily", () => {
  test("returns base sans stack when font is undefined", () => {
    const result = sansFontFamily(undefined)
    expect(result).toContain("sans-serif")
    expect(result).not.toContain("undefined")
  })

  test("prepends custom font to sans stack", () => {
    const result = sansFontFamily("Inter")
    expect(result).toContain("Inter")
    expect(result).toContain("sans-serif")
  })
})

describe("terminalFontFamily", () => {
  test("returns base terminal stack when font is undefined", () => {
    const result = terminalFontFamily(undefined)
    expect(result).toContain("JetBrainsMono")
    expect(result).toContain("monospace")
  })

  test("prepends custom font to terminal stack", () => {
    const result = terminalFontFamily("Hack Nerd Font")
    expect(result.startsWith('"Hack Nerd Font"')).toBe(true)
    expect(result).toContain("monospace")
  })
})

describe("font input helpers", () => {
  test("monoInput returns empty string for undefined", () => {
    expect(monoInput(undefined)).toBe("")
  })

  test("monoInput passes through existing value", () => {
    expect(monoInput("Fira Code")).toBe("Fira Code")
  })

  test("sansInput returns empty string for undefined", () => {
    expect(sansInput(undefined)).toBe("")
  })

  test("terminalInput returns empty string for undefined", () => {
    expect(terminalInput(undefined)).toBe("")
  })
})

describe("font defaults", () => {
  test("monoDefault is System Mono", () => {
    expect(monoDefault).toBe("System Mono")
  })

  test("sansDefault is System Sans", () => {
    expect(sansDefault).toBe("System Sans")
  })

  test("terminalDefault is JetBrainsMono Nerd Font Mono", () => {
    expect(terminalDefault).toBe("JetBrainsMono Nerd Font Mono")
  })
})
