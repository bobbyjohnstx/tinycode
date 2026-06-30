import { describe, expect, test } from "bun:test"
import { isRecord } from "@/util/record"

describe("isRecord", () => {
  test("returns true for plain objects", () => {
    expect(isRecord({})).toBe(true)
    expect(isRecord({ key: "value" })).toBe(true)
    expect(isRecord({ nested: { key: "value" } })).toBe(true)
  })

  test("returns false for arrays", () => {
    expect(isRecord([])).toBe(false)
    expect(isRecord([1, 2, 3])).toBe(false)
    expect(isRecord(["a", "b"])).toBe(false)
  })

  test("returns false for primitives", () => {
    expect(isRecord(null)).toBe(false)
    expect(isRecord(undefined)).toBe(false)
    expect(isRecord(42)).toBe(false)
    expect(isRecord("string")).toBe(false)
    expect(isRecord(true)).toBe(false)
  })

  test("returns true for object instances", () => {
    expect(isRecord(new Date())).toBe(true)
    expect(isRecord(new Error())).toBe(true)
  })
})
