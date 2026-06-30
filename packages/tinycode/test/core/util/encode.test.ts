import { describe, expect, test } from "bun:test"
import { base64Encode, base64Decode, checksum, sampledChecksum } from "@/core/util/encode"

describe("base64Encode and base64Decode", () => {
  test("encodes and decodes simple strings", () => {
    const original = "hello world"
    const encoded = base64Encode(original)
    const decoded = base64Decode(encoded)
    expect(decoded).toBe(original)
  })

  test("encodes and decodes unicode strings", () => {
    const original = "Hello 世界! 🌍"
    const encoded = base64Encode(original)
    const decoded = base64Decode(encoded)
    expect(decoded).toBe(original)
  })

  test("produces URL-safe base64 (no +, /, or =)", () => {
    const original = "test string with special chars: +/="
    const encoded = base64Encode(original)
    expect(encoded).not.toContain("+")
    expect(encoded).not.toContain("/")
    expect(encoded).not.toContain("=")
  })

  test("handles empty strings", () => {
    const encoded = base64Encode("")
    const decoded = base64Decode(encoded)
    expect(decoded).toBe("")
  })

  test("handles long strings", () => {
    const original = "a".repeat(1000)
    const encoded = base64Encode(original)
    const decoded = base64Decode(encoded)
    expect(decoded).toBe(original)
  })
})

describe("checksum", () => {
  test("generates consistent checksums for same input", () => {
    const input = "test string"
    const hash1 = checksum(input)
    const hash2 = checksum(input)
    expect(hash1).toBe(hash2)
  })

  test("generates different checksums for different inputs", () => {
    const hash1 = checksum("test1")
    const hash2 = checksum("test2")
    expect(hash1).not.toBe(hash2)
  })

  test("returns undefined for empty strings", () => {
    expect(checksum("")).toBeUndefined()
  })

  test("generates checksum in base36 format", () => {
    const hash = checksum("test")
    expect(hash).toMatch(/^[0-9a-z]+$/)
  })
})

describe("sampledChecksum", () => {
  test("uses full checksum for strings under limit", () => {
    const input = "short string"
    const sampled = sampledChecksum(input)
    const full = checksum(input)
    expect(sampled).toBe(full)
  })

  test("samples large strings", () => {
    const input = "a".repeat(1_000_000)
    const sampled = sampledChecksum(input)
    expect(sampled).toContain(":")
    expect(sampled).toContain("1000000")
  })

  test("returns undefined for empty strings", () => {
    expect(sampledChecksum("")).toBeUndefined()
  })

  test("uses custom limit", () => {
    const input = "a".repeat(100)
    const sampled = sampledChecksum(input, 50)
    expect(sampled).toContain(":")
    expect(sampled).toContain("100")
  })

  test("generates consistent checksums for same large input", () => {
    const input = "a".repeat(1_000_000)
    const hash1 = sampledChecksum(input)
    const hash2 = sampledChecksum(input)
    expect(hash1).toBe(hash2)
  })
})
