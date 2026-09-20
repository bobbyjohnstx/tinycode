import { describe, expect, test } from "bun:test"
import { isPromptEqual, DEFAULT_PROMPT, type Prompt } from "./prompt"

describe("isPromptEqual", () => {
  test("returns true for identical text prompts", () => {
    const a: Prompt = [{ type: "text", content: "hello", start: 0, end: 5 }]
    const b: Prompt = [{ type: "text", content: "hello", start: 0, end: 5 }]
    expect(isPromptEqual(a, b)).toBe(true)
  })

  test("returns false for different length prompts", () => {
    const a: Prompt = [{ type: "text", content: "hello", start: 0, end: 5 }]
    const b: Prompt = [
      { type: "text", content: "hello", start: 0, end: 5 },
      { type: "text", content: " world", start: 5, end: 11 },
    ]
    expect(isPromptEqual(a, b)).toBe(false)
  })

  test("returns false for different text content", () => {
    const a: Prompt = [{ type: "text", content: "hello", start: 0, end: 5 }]
    const b: Prompt = [{ type: "text", content: "world", start: 0, end: 5 }]
    expect(isPromptEqual(a, b)).toBe(false)
  })

  test("returns true for identical file attachment parts", () => {
    const a: Prompt = [{ type: "file", content: "@file.ts", start: 0, end: 8, path: "/src/file.ts" }]
    const b: Prompt = [{ type: "file", content: "@file.ts", start: 0, end: 8, path: "/src/file.ts" }]
    expect(isPromptEqual(a, b)).toBe(true)
  })

  test("returns false for file parts with different paths", () => {
    const a: Prompt = [{ type: "file", content: "@a.ts", start: 0, end: 5, path: "/src/a.ts" }]
    const b: Prompt = [{ type: "file", content: "@b.ts", start: 0, end: 5, path: "/src/b.ts" }]
    expect(isPromptEqual(a, b)).toBe(false)
  })

  test("returns false for file parts with different selections", () => {
    const a: Prompt = [
      {
        type: "file",
        content: "@file.ts",
        start: 0,
        end: 8,
        path: "/src/file.ts",
        selection: { startLine: 1, startChar: 0, endLine: 10, endChar: 0 },
      },
    ]
    const b: Prompt = [
      {
        type: "file",
        content: "@file.ts",
        start: 0,
        end: 8,
        path: "/src/file.ts",
        selection: { startLine: 5, startChar: 0, endLine: 20, endChar: 0 },
      },
    ]
    expect(isPromptEqual(a, b)).toBe(false)
  })

  test("returns true for identical agent parts", () => {
    const a: Prompt = [{ type: "agent", content: "@builder", start: 0, end: 8, name: "builder" }]
    const b: Prompt = [{ type: "agent", content: "@builder", start: 0, end: 8, name: "builder" }]
    expect(isPromptEqual(a, b)).toBe(true)
  })

  test("returns false for agent parts with different names", () => {
    const a: Prompt = [{ type: "agent", content: "@builder", start: 0, end: 8, name: "builder" }]
    const b: Prompt = [{ type: "agent", content: "@reviewer", start: 0, end: 9, name: "reviewer" }]
    expect(isPromptEqual(a, b)).toBe(false)
  })

  test("returns true for two empty prompts", () => {
    expect(isPromptEqual([], [])).toBe(true)
  })

  test("returns false for mixed part types at same position", () => {
    const a: Prompt = [{ type: "text", content: "hello", start: 0, end: 5 }]
    const b: Prompt = [{ type: "file", content: "@hello", start: 0, end: 6, path: "/hello" }]
    expect(isPromptEqual(a, b)).toBe(false)
  })
})

describe("DEFAULT_PROMPT", () => {
  test("is a single empty text part", () => {
    expect(DEFAULT_PROMPT).toHaveLength(1)
    expect(DEFAULT_PROMPT[0]?.type).toBe("text")
    expect(DEFAULT_PROMPT[0]?.content).toBe("")
  })
})
