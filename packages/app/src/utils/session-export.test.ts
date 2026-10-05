import { describe, expect, test } from "bun:test"
import { formatTranscript, formatTranscriptHtml, sanitizeFilename, sessionSlug } from "./session-export"

describe("sanitizeFilename", () => {
  test("normalizes title to slug", () => {
    expect(sanitizeFilename("Hello World_Test!")).toBe("hello-world-test")
  })
})

describe("sessionSlug", () => {
  test("prefers title slug", () => {
    expect(sessionSlug({ id: "ses_abcdefgh", title: "My Session" })).toBe("my-session")
  })

  test("falls back to short id", () => {
    expect(sessionSlug({ id: "ses_abcdefgh" })).toBe("ses_abcd")
  })
})

describe("formatTranscript", () => {
  test("formats role headers and text parts", () => {
    const md = formatTranscript(
      { id: "ses_1", title: "Demo", time: { created: Date.UTC(2024, 0, 1) } },
      [
        { role: "user", parts: [{ type: "text", text: "hi" }] },
        {
          role: "assistant",
          agent: "build",
          modelID: "gpt",
          parts: [{ type: "text", text: "hello" }],
        },
      ],
    )

    expect(md).toContain("# Demo")
    expect(md).toContain("**Session ID:** ses_1")
    expect(md).toContain("## User")
    expect(md).toContain("hi")
    expect(md).toContain("## Assistant (Build · gpt)")
    expect(md).toContain("hello")
  })
})

describe("formatTranscriptHtml", () => {
  test("escapes html and wraps content", () => {
    const html = formatTranscriptHtml(
      { id: "ses_1", title: "A <B>" },
      [{ role: "user", parts: [{ type: "text", text: "<script>x</script>" }] }],
    )
    expect(html).toContain("<!DOCTYPE html>")
    expect(html).toContain("A &lt;B&gt;")
    expect(html).toContain("&lt;script&gt;x&lt;/script&gt;")
    expect(html).not.toContain("<script>x</script>")
  })
})
