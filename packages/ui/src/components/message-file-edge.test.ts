import { describe, expect, test } from "bun:test"
import type { FilePart } from "@tinycode/sdk/v2"
import { attached, inline, kind } from "./message-file"

function file(overrides: Partial<FilePart> = {}): FilePart {
  return {
    id: "part_1",
    sessionID: "ses_1",
    messageID: "msg_1",
    type: "file",
    mime: "text/plain",
    url: "file:///repo/README.txt",
    filename: "README.txt",
    ...overrides,
  }
}

describe("attached edge cases", () => {
  test("returns true for base64 data URL", () => {
    expect(attached(file({ url: "data:text/plain;base64,SGVsbG8=" }))).toBe(true)
  })

  test("returns true for data URL with charset", () => {
    expect(attached(file({ url: "data:text/html;charset=utf-8,<p>Hi</p>" }))).toBe(true)
  })

  test("returns false for file URL", () => {
    expect(attached(file({ url: "file:///path/to/file.txt" }))).toBe(false)
  })

  test("returns false for http URL", () => {
    expect(attached(file({ url: "https://example.com/file.txt" }))).toBe(false)
  })

  test("returns false for empty URL", () => {
    expect(attached(file({ url: "" }))).toBe(false)
  })
})

describe("inline edge cases", () => {
  test("returns true when source has text range and not an attachment", () => {
    expect(
      inline(
        file({
          url: "file:///repo/README.txt",
          source: {
            type: "file",
            path: "/repo/README.txt",
            text: { value: "@README.txt", start: 0, end: 11 },
          },
        }),
      ),
    ).toBe(true)
  })

  test("returns false when source is missing", () => {
    expect(inline(file())).toBe(false)
  })

  test("returns false for data URL even with source", () => {
    expect(
      inline(
        file({
          url: "data:text/plain;base64,SGVsbG8=",
          source: {
            type: "file",
            path: "/repo/README.txt",
            text: { value: "@README.txt", start: 0, end: 11 },
          },
        }),
      ),
    ).toBe(false)
  })

  test("returns false when source has no text range", () => {
    expect(
      inline(
        file({
          source: {
            type: "file",
            path: "/repo/README.txt",
          } as any,
        }),
      ),
    ).toBe(false)
  })
})

describe("kind edge cases", () => {
  test("returns image for image/png", () => {
    expect(kind(file({ mime: "image/png" }))).toBe("image")
  })

  test("returns image for image/jpeg", () => {
    expect(kind(file({ mime: "image/jpeg" }))).toBe("image")
  })

  test("returns image for image/gif", () => {
    expect(kind(file({ mime: "image/gif" }))).toBe("image")
  })

  test("returns image for image/webp", () => {
    expect(kind(file({ mime: "image/webp" }))).toBe("image")
  })

  test("returns image for image/svg+xml", () => {
    expect(kind(file({ mime: "image/svg+xml" }))).toBe("image")
  })

  test("returns file for text/plain", () => {
    expect(kind(file({ mime: "text/plain" }))).toBe("file")
  })

  test("returns file for application/json", () => {
    expect(kind(file({ mime: "application/json" }))).toBe("file")
  })

  test("returns file for application/pdf", () => {
    expect(kind(file({ mime: "application/pdf" }))).toBe("file")
  })

  test("returns file for audio/mpeg", () => {
    expect(kind(file({ mime: "audio/mpeg" }))).toBe("file")
  })

  test("returns file for video/mp4", () => {
    expect(kind(file({ mime: "video/mp4" }))).toBe("file")
  })
})
