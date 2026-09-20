import { describe, expect, test } from "bun:test"
import { lineCommentStyles } from "./line-comment-styles"

describe("line comment styles", () => {
  test("is a non-empty CSS string", () => {
    expect(typeof lineCommentStyles).toBe("string")
    expect(lineCommentStyles.length).toBeGreaterThan(100)
  })

  test("contains the root component selector", () => {
    expect(lineCommentStyles).toContain('[data-component="line-comment"]')
  })

  test("contains expected data-slot selectors", () => {
    expect(lineCommentStyles).toContain('[data-slot="line-comment-button"]')
    expect(lineCommentStyles).toContain('[data-slot="line-comment-popover"]')
    expect(lineCommentStyles).toContain('[data-slot="line-comment-content"]')
    expect(lineCommentStyles).toContain('[data-slot="line-comment-textarea"]')
  })

  test("contains inline variant styles", () => {
    expect(lineCommentStyles).toContain("[data-inline]")
  })

  test("uses CSS custom properties for theming", () => {
    expect(lineCommentStyles).toContain("var(--")
  })

  test("includes focus and hover states", () => {
    expect(lineCommentStyles).toContain(":focus")
    expect(lineCommentStyles).toContain(":focus-visible")
  })
})
