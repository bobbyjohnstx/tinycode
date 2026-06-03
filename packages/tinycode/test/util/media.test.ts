/**
 * Tests for PDF detection and MIME sniffing utilities in src/util/media.ts
 */
import { describe, test, expect } from "bun:test"
import { isPdfAttachment, sniffAttachmentMime } from "../../src/util/media"

// ---------------------------------------------------------------------------
// Test 8: sniffAttachmentMime detects PDF magic bytes (%PDF-)
// ---------------------------------------------------------------------------

describe("sniffAttachmentMime", () => {
  test("returns 'application/pdf' for PDF magic bytes %PDF-", () => {
    // PDF magic bytes: 0x25 0x50 0x44 0x46 0x2d (%PDF-)
    const pdfBytes = new Uint8Array([0x25, 0x50, 0x44, 0x46, 0x2d, 0x31, 0x2e, 0x34])
    expect(sniffAttachmentMime(pdfBytes, "application/octet-stream")).toBe("application/pdf")
  })

  test("returns 'image/png' for PNG magic bytes", () => {
    const pngBytes = new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a])
    expect(sniffAttachmentMime(pngBytes, "image/jpeg")).toBe("image/png")
  })

  test("returns 'image/jpeg' for JPEG magic bytes", () => {
    const jpegBytes = new Uint8Array([0xff, 0xd8, 0xff, 0xe0])
    expect(sniffAttachmentMime(jpegBytes, "application/octet-stream")).toBe("image/jpeg")
  })

  test("returns fallback for unknown bytes", () => {
    const unknownBytes = new Uint8Array([0x00, 0x01, 0x02, 0x03])
    expect(sniffAttachmentMime(unknownBytes, "text/plain")).toBe("text/plain")
    expect(sniffAttachmentMime(unknownBytes, "application/octet-stream")).toBe("application/octet-stream")
  })

  test("magic bytes take priority over fallback for PDF", () => {
    const pdfBytes = new Uint8Array([0x25, 0x50, 0x44, 0x46, 0x2d])
    // Even if fallback says "text/plain", magic bytes win
    expect(sniffAttachmentMime(pdfBytes, "text/plain")).toBe("application/pdf")
  })
})

describe("isPdfAttachment", () => {
  test("returns true for application/pdf", () => {
    expect(isPdfAttachment("application/pdf")).toBe(true)
  })

  test("returns false for non-PDF types", () => {
    expect(isPdfAttachment("image/jpeg")).toBe(false)
    expect(isPdfAttachment("text/plain")).toBe(false)
    expect(isPdfAttachment("application/octet-stream")).toBe(false)
    expect(isPdfAttachment("")).toBe(false)
  })
})
