import { describe, expect, test } from "bun:test"
import { maskKey } from "../../src/util/mask"

describe("util.mask", () => {
  describe("maskKey", () => {
    test("masks middle of long keys", () => {
      expect(maskKey("sk-abc123xyz789")).toBe("sk-a****z789")
    })

    test("masks keys exactly 8 characters", () => {
      expect(maskKey("12345678")).toBe("1234****5678")
    })

    test("fully masks keys shorter than 8 characters", () => {
      expect(maskKey("short")).toBe("****")
      expect(maskKey("abc")).toBe("****")
      expect(maskKey("a")).toBe("****")
    })

    test("fully masks empty string", () => {
      expect(maskKey("")).toBe("****")
    })

    test("handles typical API key formats", () => {
      expect(maskKey("sk-ant-api03-abcdefghijklmnop")).toBe("sk-a****mnop")
      expect(maskKey("gsk_1234567890abcdef")).toBe("gsk_****cdef")
    })
  })
})
