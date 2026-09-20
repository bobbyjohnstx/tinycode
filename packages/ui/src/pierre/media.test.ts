import { describe, expect, test } from "bun:test"
import {
  normalizeMimeType,
  fileExtension,
  mediaKindFromPath,
  isBinaryContent,
  dataUrlFromMediaValue,
  svgTextFromValue,
  hasMediaValue,
} from "./media"

describe("normalizeMimeType", () => {
  test("returns undefined for undefined input", () => {
    expect(normalizeMimeType(undefined)).toBeUndefined()
  })

  test("returns undefined for empty string", () => {
    expect(normalizeMimeType("")).toBeUndefined()
  })

  test("strips parameters from mime type", () => {
    expect(normalizeMimeType("text/html; charset=utf-8")).toBe("text/html")
  })

  test("lowercases mime type", () => {
    expect(normalizeMimeType("Image/PNG")).toBe("image/png")
  })

  test("normalizes audio/x-aac to audio/aac", () => {
    expect(normalizeMimeType("audio/x-aac")).toBe("audio/aac")
  })

  test("normalizes audio/x-m4a to audio/mp4", () => {
    expect(normalizeMimeType("audio/x-m4a")).toBe("audio/mp4")
  })

  test("passes through standard mime types unchanged", () => {
    expect(normalizeMimeType("audio/mpeg")).toBe("audio/mpeg")
  })
})

describe("fileExtension", () => {
  test("returns empty string for undefined path", () => {
    expect(fileExtension(undefined)).toBe("")
  })

  test("returns empty string for path with no extension", () => {
    expect(fileExtension("Makefile")).toBe("")
  })

  test("extracts lowercase extension from filename", () => {
    expect(fileExtension("photo.PNG")).toBe("png")
  })

  test("extracts extension from full path", () => {
    expect(fileExtension("/home/user/document.pdf")).toBe("pdf")
  })

  test("handles dotfiles correctly", () => {
    expect(fileExtension(".gitignore")).toBe("gitignore")
  })
})

describe("mediaKindFromPath", () => {
  test("returns undefined for undefined path", () => {
    expect(mediaKindFromPath(undefined)).toBeUndefined()
  })

  test("returns undefined for non-media extension", () => {
    expect(mediaKindFromPath("file.txt")).toBeUndefined()
  })

  test("detects image extensions", () => {
    expect(mediaKindFromPath("photo.png")).toBe("image")
    expect(mediaKindFromPath("photo.jpg")).toBe("image")
    expect(mediaKindFromPath("photo.gif")).toBe("image")
    expect(mediaKindFromPath("photo.webp")).toBe("image")
  })

  test("detects audio extensions", () => {
    expect(mediaKindFromPath("song.mp3")).toBe("audio")
    expect(mediaKindFromPath("sound.wav")).toBe("audio")
    expect(mediaKindFromPath("track.ogg")).toBe("audio")
  })

  test("detects svg extension", () => {
    expect(mediaKindFromPath("icon.svg")).toBe("svg")
  })

  test("handles uppercase extensions", () => {
    expect(mediaKindFromPath("IMAGE.JPEG")).toBe("image")
  })
})

describe("isBinaryContent", () => {
  test("returns false for undefined", () => {
    expect(isBinaryContent(undefined)).toBeFalsy()
  })

  test("returns false for null", () => {
    expect(isBinaryContent(null)).toBeFalsy()
  })

  test("returns false for non-binary content", () => {
    expect(isBinaryContent({ type: "text" })).toBeFalsy()
  })

  test("returns true for binary content", () => {
    expect(isBinaryContent({ type: "binary" })).toBeTruthy()
  })
})

describe("dataUrlFromMediaValue", () => {
  test("returns undefined for falsy value", () => {
    expect(dataUrlFromMediaValue(undefined, "image")).toBeUndefined()
    expect(dataUrlFromMediaValue(null, "image")).toBeUndefined()
  })

  test("returns valid image data URL string as-is", () => {
    const url = "data:image/png;base64,abc123"
    expect(dataUrlFromMediaValue(url, "image")).toBe(url)
  })

  test("rejects non-image data URL for image kind", () => {
    expect(dataUrlFromMediaValue("data:audio/mp3;base64,abc", "image")).toBeUndefined()
  })

  test("returns valid audio data URL string as-is", () => {
    const url = "data:audio/mpeg;base64,abc123"
    expect(dataUrlFromMediaValue(url, "audio")).toBe(url)
  })

  test("normalizes audio/x-aac in data URL strings", () => {
    const url = "data:audio/x-aac;base64,abc"
    expect(dataUrlFromMediaValue(url, "audio")).toBe("data:audio/aac;base64,abc")
  })

  test("builds data URL from content record with base64 encoding", () => {
    const record = { content: "SGVsbG8=", encoding: "base64", mimeType: "image/png" }
    expect(dataUrlFromMediaValue(record, "image")).toBe("data:image/png;base64,SGVsbG8=")
  })

  test("returns undefined when mime type does not match kind", () => {
    const record = { content: "abc", encoding: "base64", mimeType: "audio/mpeg" }
    expect(dataUrlFromMediaValue(record, "image")).toBeUndefined()
  })
})

describe("svgTextFromValue", () => {
  test("returns undefined for non-svg content", () => {
    expect(svgTextFromValue({ content: "hello", mimeType: "text/plain" })).toBeUndefined()
  })

  test("returns content for svg mime type without base64", () => {
    const value = { content: "<svg></svg>", mimeType: "image/svg+xml" }
    expect(svgTextFromValue(value)).toBe("<svg></svg>")
  })

  test("decodes base64 svg content", () => {
    const encoded = btoa("<svg></svg>")
    const value = { content: encoded, encoding: "base64", mimeType: "image/svg+xml" }
    expect(svgTextFromValue(value)).toBe("<svg></svg>")
  })

  test("returns undefined for non-object values", () => {
    expect(svgTextFromValue("string")).toBeUndefined()
    expect(svgTextFromValue(42)).toBeUndefined()
    expect(svgTextFromValue(null)).toBeUndefined()
  })
})

describe("hasMediaValue", () => {
  test("returns false for undefined", () => {
    expect(hasMediaValue(undefined)).toBe(false)
  })

  test("returns true for non-empty string", () => {
    expect(hasMediaValue("data:image/png;base64,abc")).toBe(true)
  })

  test("returns false for empty string", () => {
    expect(hasMediaValue("")).toBe(false)
  })

  test("returns true for record with non-empty content", () => {
    expect(hasMediaValue({ content: "abc" })).toBe(true)
  })

  test("returns false for record with empty content", () => {
    expect(hasMediaValue({ content: "" })).toBe(false)
  })

  test("returns false for record without content field", () => {
    expect(hasMediaValue({ mimeType: "image/png" })).toBe(false)
  })
})
