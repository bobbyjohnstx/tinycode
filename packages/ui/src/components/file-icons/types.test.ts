import { describe, expect, test } from "bun:test"
import { iconNames } from "./types"

describe("file icon names", () => {
  test("contains at least 100 file type icons", () => {
    expect(iconNames.length).toBeGreaterThanOrEqual(100)
  })

  test("all names are non-empty strings", () => {
    for (const name of iconNames) {
      expect(typeof name).toBe("string")
      expect(name.length).toBeGreaterThan(0)
    }
  })

  test("contains no duplicate entries", () => {
    const unique = new Set(iconNames)
    expect(unique.size).toBe(iconNames.length)
  })

  test("includes common programming language icons", () => {
    expect(iconNames).toContain("Typescript")
    expect(iconNames).toContain("Javascript")
    expect(iconNames).toContain("Python")
    expect(iconNames).toContain("Rust")
    expect(iconNames).toContain("Go")
  })

  test("includes folder icons", () => {
    const folderIcons = iconNames.filter((name) => name.startsWith("Folder"))
    expect(folderIcons.length).toBeGreaterThan(50)
  })
})
