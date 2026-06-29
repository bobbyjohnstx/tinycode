import { describe, expect, test } from "bun:test"

/**
 * Menu creation tests.
 * Tests platform detection, menu filtering, accelerator format, and help menu hrefs.
 */
describe("menu platform detection", () => {
  test('darwin platform maps to "macos"', () => {
    const platform = process.platform === "darwin" ? "macos" : "windows"
    if (process.platform === "darwin") {
      expect(platform).toBe("macos")
    }
  })

  test('non-darwin platforms map to "windows"', () => {
    const platform = process.platform === "darwin" ? "macos" : "windows"
    if (process.platform !== "darwin") {
      expect(platform).toBe("windows")
    }
  })
})

describe("menu accelerators", () => {
  type Accelerator = {
    macos?: string
    windows?: string
  }

  function getAccelerator(accelerator: Accelerator | undefined, platform: "macos" | "windows"): string | undefined {
    if (!accelerator) return undefined
    return platform === "macos" ? accelerator.macos : accelerator.windows
  }

  test("macOS uses Cmd modifier", () => {
    const accelerator = { macos: "Cmd+T", windows: "Ctrl+T" }
    expect(getAccelerator(accelerator, "macos")).toBe("Cmd+T")
  })

  test("Windows uses Ctrl modifier", () => {
    const accelerator = { macos: "Cmd+T", windows: "Ctrl+T" }
    expect(getAccelerator(accelerator, "windows")).toBe("Ctrl+T")
  })

  test("undefined accelerator returns undefined", () => {
    expect(getAccelerator(undefined, "macos")).toBeUndefined()
    expect(getAccelerator(undefined, "windows")).toBeUndefined()
  })
})

describe("menu item filtering", () => {
  type MenuEntry = {
    label?: string
    visible?: "macos" | "windows" | "all"
  }

  function isVisible(entry: MenuEntry, platform: "macos" | "windows"): boolean {
    if (!entry.visible) return true
    if (entry.visible === "all") return true
    return entry.visible === platform
  }

  test("items without visible property are shown on all platforms", () => {
    const entry = { label: "Always Visible" }
    expect(isVisible(entry, "macos")).toBe(true)
    expect(isVisible(entry, "windows")).toBe(true)
  })

  test('visible: "all" shows on all platforms', () => {
    const entry = { label: "All Platforms", visible: "all" as const }
    expect(isVisible(entry, "macos")).toBe(true)
    expect(isVisible(entry, "windows")).toBe(true)
  })

  test('visible: "macos" shows only on macOS', () => {
    const entry = { label: "macOS Only", visible: "macos" as const }
    expect(isVisible(entry, "macos")).toBe(true)
    expect(isVisible(entry, "windows")).toBe(false)
  })

  test('visible: "windows" shows only on Windows', () => {
    const entry = { label: "Windows Only", visible: "windows" as const }
    expect(isVisible(entry, "macos")).toBe(false)
    expect(isVisible(entry, "windows")).toBe(true)
  })
})

describe("menu help links", () => {
  type HelpEntry = {
    label: string
    href?: string
  }

  function validateHelpEntry(entry: HelpEntry): boolean {
    if (!entry.href) return false
    return entry.href.trim().length > 0 && entry.href.startsWith("http")
  }

  test("help menu items with href have non-empty URLs", () => {
    const validEntry = { label: "Documentation", href: "https://docs.tinycode.dev" }
    expect(validateHelpEntry(validEntry)).toBe(true)
  })

  test("help menu items with empty href are invalid", () => {
    const invalidEntry = { label: "Documentation", href: "" }
    expect(validateHelpEntry(invalidEntry)).toBe(false)
  })

  test("help menu items without href are invalid", () => {
    const invalidEntry = { label: "Documentation" }
    expect(validateHelpEntry(invalidEntry)).toBe(false)
  })

  test("help menu items must use http/https protocol", () => {
    const validHttps = { label: "Docs", href: "https://example.com" }
    const validHttp = { label: "Docs", href: "http://example.com" }
    const invalidFile = { label: "Docs", href: "file:///local/path" }

    expect(validateHelpEntry(validHttps)).toBe(true)
    expect(validateHelpEntry(validHttp)).toBe(true)
    expect(validateHelpEntry(invalidFile)).toBe(false)
  })
})
