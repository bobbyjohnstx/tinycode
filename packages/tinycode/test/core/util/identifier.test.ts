import { describe, expect, test } from "bun:test"
import { Identifier } from "@/core/util/identifier"

describe("Identifier", () => {
  describe("ascending", () => {
    test("generates identifier with correct length", () => {
      const id = Identifier.ascending()
      expect(id.length).toBe(26)
    })

    test("generates unique identifiers", () => {
      const ids = new Set()
      for (let i = 0; i < 100; i++) {
        ids.add(Identifier.ascending())
      }
      expect(ids.size).toBe(100)
    })

    test("generates lexicographically sortable ascending IDs", () => {
      const ids: string[] = []
      for (let i = 0; i < 10; i++) {
        ids.push(Identifier.ascending())
      }
      const sorted = [...ids].sort()
      expect(ids).toEqual(sorted)
    })

    test("uses hexadecimal prefix for time component", () => {
      const id = Identifier.ascending()
      const timePrefix = id.slice(0, 12)
      expect(timePrefix).toMatch(/^[0-9a-f]{12}$/)
    })
  })

  describe("descending", () => {
    test("generates identifier with correct length", () => {
      const id = Identifier.descending()
      expect(id.length).toBe(26)
    })

    test("generates unique identifiers", () => {
      const ids = new Set()
      for (let i = 0; i < 100; i++) {
        ids.add(Identifier.descending())
      }
      expect(ids.size).toBe(100)
    })

    test("generates lexicographically sortable descending IDs", () => {
      const ids: string[] = []
      for (let i = 0; i < 10; i++) {
        ids.push(Identifier.descending())
      }
      const sorted = [...ids].sort().reverse()
      expect(ids).toEqual(sorted)
    })
  })

  describe("create with timestamp", () => {
    test("generates IDs with time component based on timestamp", () => {
      const timestamp = 1234567890000
      const id1 = Identifier.create(false, timestamp)
      // Should have 26 characters total
      expect(id1.length).toBe(26)
      // First 12 chars are hex encoded timestamp + counter
      expect(id1.slice(0, 12)).toMatch(/^[0-9a-f]{12}$/)
    })

    test("handles overflow with counter", () => {
      const timestamp = 1234567890000
      const ids = []
      for (let i = 0; i < 5; i++) {
        ids.push(Identifier.create(false, timestamp))
      }
      // All should have same time prefix but different counters
      const uniquePrefixes = new Set(ids.map(id => id.slice(0, 10)))
      expect(uniquePrefixes.size).toBe(1)
    })
  })
})
