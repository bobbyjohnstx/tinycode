import { describe, expect, test } from "bun:test"
import {
  visibleWidth,
  truncateToWidth,
  truncate,
  truncateLeft,
  truncateMiddle,
} from "../../src/util/locale"

describe("util.locale", () => {
  describe("visibleWidth", () => {
    test("returns correct width for plain ASCII", () => {
      expect(visibleWidth("hello")).toBe(5)
      expect(visibleWidth("")).toBe(0)
      expect(visibleWidth("a")).toBe(1)
    })

    test("ignores ANSI escape codes", () => {
      expect(visibleWidth("\x1b[31mhello\x1b[0m")).toBe(5)
      expect(visibleWidth("\x1b[1m\x1b[34mbold blue\x1b[0m")).toBe(9)
      expect(visibleWidth("\x1b[0m")).toBe(0)
    })

    test("counts CJK characters as width 2", () => {
      expect(visibleWidth("你好")).toBe(4) // nihao
      expect(visibleWidth("a你b")).toBe(4) // a + CJK + b
    })

    test("handles emoji", () => {
      // Bun.stringWidth handles emoji width
      const w = visibleWidth("\u{1F600}")
      expect(w).toBeGreaterThanOrEqual(1)
    })
  })

  describe("truncateToWidth", () => {
    test("returns string unchanged when within width", () => {
      expect(truncateToWidth("hello", 10)).toBe("hello")
      expect(truncateToWidth("hello", 5)).toBe("hello")
    })

    test("truncates with ellipsis when exceeding width", () => {
      const result = truncateToWidth("hello world", 8)
      expect(visibleWidth(result)).toBeLessThanOrEqual(8)
      expect(result).toContain("…")
    })

    test("handles ANSI-colored strings", () => {
      const result = truncateToWidth("\x1b[31mhello world\x1b[0m", 8)
      expect(visibleWidth(result)).toBeLessThanOrEqual(8)
      expect(result).toContain("…")
    })

    test("handles empty string", () => {
      expect(truncateToWidth("", 5)).toBe("")
    })

    test("handles width 0", () => {
      expect(truncateToWidth("hello", 0)).toBe("")
    })

    test("handles width 1", () => {
      const result = truncateToWidth("hello", 1)
      expect(visibleWidth(result)).toBeLessThanOrEqual(1)
    })

    test("handles CJK characters", () => {
      // Each CJK char is width 2, so 3 CJK chars = width 6
      const result = truncateToWidth("你好世", 5)
      expect(visibleWidth(result)).toBeLessThanOrEqual(5)
    })

    test("handles mixed content", () => {
      const result = truncateToWidth("abc你好def", 6)
      expect(visibleWidth(result)).toBeLessThanOrEqual(6)
    })
  })

  describe("existing truncate functions", () => {
    test("truncate works for plain strings", () => {
      expect(truncate("hello", 10)).toBe("hello")
      expect(truncate("hello world", 8)).toBe("hello w…")
    })

    test("truncateLeft works for plain strings", () => {
      expect(truncateLeft("hello", 10)).toBe("hello")
      expect(truncateLeft("hello world", 8)).toBe("…o world")
    })

    test("truncateMiddle works for plain strings", () => {
      expect(truncateMiddle("hello", 10)).toBe("hello")
      expect(truncateMiddle("this is a very long string that should be truncated", 20)).toHaveLength(
        20,
      )
    })
  })
})
