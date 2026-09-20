import { describe, expect, test } from "bun:test"
import { defaultTitle, isDefaultTitle, titleNumber } from "./terminal-title"

describe("defaultTitle", () => {
  test("generates numbered terminal title", () => {
    expect(defaultTitle(1)).toBe("Terminal 1")
    expect(defaultTitle(5)).toBe("Terminal 5")
  })
})

describe("isDefaultTitle", () => {
  test("matches the English template for the given number", () => {
    expect(isDefaultTitle("Terminal 1", 1)).toBe(true)
    expect(isDefaultTitle("Terminal 3", 3)).toBe(true)
  })

  test("rejects English template with wrong number", () => {
    expect(isDefaultTitle("Terminal 1", 2)).toBe(false)
  })

  test("matches localized templates", () => {
    expect(isDefaultTitle("ターミナル 1", 1)).toBe(true)
    expect(isDefaultTitle("终端 2", 2)).toBe(true)
    expect(isDefaultTitle("터미널 1", 1)).toBe(true)
    expect(isDefaultTitle("Терминал 3", 3)).toBe(true)
  })

  test("rejects custom titles", () => {
    expect(isDefaultTitle("My Build Terminal", 1)).toBe(false)
    expect(isDefaultTitle("", 1)).toBe(false)
  })
})

describe("titleNumber", () => {
  test("returns the terminal number from a default title", () => {
    expect(titleNumber("Terminal 1", 10)).toBe(1)
    expect(titleNumber("Terminal 5", 10)).toBe(5)
  })

  test("returns the terminal number from a localized title", () => {
    expect(titleNumber("ターミナル 3", 10)).toBe(3)
  })

  test("returns undefined for custom titles", () => {
    expect(titleNumber("My Terminal", 10)).toBeUndefined()
  })

  test("returns undefined when number exceeds max", () => {
    expect(titleNumber("Terminal 5", 3)).toBeUndefined()
  })
})
