import { describe, expect, test } from "bun:test"

/**
 * Tray creation and management tests.
 * Tests the tray creation logic without Electron dependencies.
 */

describe("tray menu structure", () => {
  type MenuItem = {
    label?: string
    type?: "separator"
    click?: () => void
  }

  function buildTrayMenu(deps: { showWindow: () => void; quit: () => void }): MenuItem[] {
    return [
      { label: "Show Window", click: deps.showWindow },
      { type: "separator" },
      { label: "Quit", click: deps.quit },
    ]
  }

  test("menu has Show Window, separator, and Quit entries", () => {
    const deps = {
      showWindow: () => {},
      quit: () => {},
    }
    const menu = buildTrayMenu(deps)

    expect(menu.length).toBe(3)
    expect(menu[0].label).toBe("Show Window")
    expect(menu[1].type).toBe("separator")
    expect(menu[2].label).toBe("Quit")
  })

  test("Show Window menu item has click handler", () => {
    let clicked = false
    const deps = {
      showWindow: () => {
        clicked = true
      },
      quit: () => {},
    }
    const menu = buildTrayMenu(deps)

    menu[0].click?.()
    expect(clicked).toBe(true)
  })

  test("Quit menu item has click handler", () => {
    let clicked = false
    const deps = {
      showWindow: () => {},
      quit: () => {
        clicked = true
      },
    }
    const menu = buildTrayMenu(deps)

    menu[2].click?.()
    expect(clicked).toBe(true)
  })
})

describe("tray icon path", () => {
  function getTrayIconFileName(platform: string): string {
    return platform === "win32" ? "icon.ico" : "32x32.png"
  }

  test("Windows uses icon.ico", () => {
    expect(getTrayIconFileName("win32")).toBe("icon.ico")
  })

  test("macOS uses 32x32.png", () => {
    expect(getTrayIconFileName("darwin")).toBe("32x32.png")
  })

  test("Linux uses 32x32.png", () => {
    expect(getTrayIconFileName("linux")).toBe("32x32.png")
  })
})
