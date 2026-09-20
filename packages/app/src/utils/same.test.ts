import { describe, expect, test } from "bun:test"
import { same } from "./same"

describe("same", () => {
  test("returns true for identical array references", () => {
    const arr = [1, 2, 3]
    expect(same(arr, arr)).toBe(true)
  })

  test("returns true for arrays with equal elements", () => {
    expect(same([1, 2, 3], [1, 2, 3])).toBe(true)
    expect(same(["a", "b"], ["a", "b"])).toBe(true)
  })

  test("returns false for arrays with different elements", () => {
    expect(same([1, 2, 3], [1, 2, 4])).toBe(false)
  })

  test("returns false for arrays with different lengths", () => {
    expect(same([1, 2], [1, 2, 3])).toBe(false)
  })

  test("returns true for two empty arrays", () => {
    expect(same([], [])).toBe(true)
  })

  test("returns true for both undefined", () => {
    expect(same(undefined, undefined)).toBe(true)
  })

  test("returns false when one is undefined and other is an array", () => {
    expect(same(undefined, [1])).toBe(false)
    expect(same([1], undefined)).toBe(false)
  })

  test("uses reference equality for elements not deep equality", () => {
    const obj = { id: 1 }
    expect(same([obj], [obj])).toBe(true)
    expect(same([{ id: 1 }], [{ id: 1 }])).toBe(false)
  })
})
