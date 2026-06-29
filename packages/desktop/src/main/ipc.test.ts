import { describe, expect, test } from "bun:test"

/**
 * URL scheme validation for the `open-link` IPC handler.
 * Only http:, https:, and mailto: are allowed.
 */
describe("URL scheme validation", () => {
  const ALLOWED_LINK_SCHEMES = new Set(["http:", "https:", "mailto:"])

  function isAllowedUrl(url: string): boolean {
    try {
      const parsed = new URL(url)
      return ALLOWED_LINK_SCHEMES.has(parsed.protocol)
    } catch {
      return false
    }
  }

  test("http:// URLs are allowed", () => {
    expect(isAllowedUrl("http://example.com")).toBe(true)
    expect(isAllowedUrl("http://localhost:3000")).toBe(true)
  })

  test("https:// URLs are allowed", () => {
    expect(isAllowedUrl("https://example.com")).toBe(true)
    expect(isAllowedUrl("https://secure.example.com/path?query=1")).toBe(true)
  })

  test("mailto: URLs are allowed", () => {
    expect(isAllowedUrl("mailto:user@example.com")).toBe(true)
    expect(isAllowedUrl("mailto:support@tinycode.dev?subject=Help")).toBe(true)
  })

  test("file:// URLs are blocked", () => {
    expect(isAllowedUrl("file:///etc/passwd")).toBe(false)
    expect(isAllowedUrl("file:///C:/Windows/System32")).toBe(false)
  })

  test("smb:// URLs are blocked", () => {
    expect(isAllowedUrl("smb://server/share")).toBe(false)
  })

  test("javascript: URLs are blocked", () => {
    expect(isAllowedUrl("javascript:alert(1)")).toBe(false)
  })

  test("data: URLs are blocked", () => {
    expect(isAllowedUrl("data:text/html,<script>alert(1)</script>")).toBe(false)
  })

  test("invalid URL strings are blocked without crashing", () => {
    expect(isAllowedUrl("not a url")).toBe(false)
    expect(isAllowedUrl("")).toBe(false)
    expect(isAllowedUrl("://malformed")).toBe(false)
  })

  test("relative URLs are blocked", () => {
    expect(isAllowedUrl("/relative/path")).toBe(false)
    expect(isAllowedUrl("../parent")).toBe(false)
  })
})
