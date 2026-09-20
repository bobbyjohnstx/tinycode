/**
 * Tests for pure functions exported from message-part.tsx.
 *
 * Run with browser conditions to resolve solid-js client bundle:
 *   bun test --preload ./test-preload.ts --conditions browser src/components/message-part-logic.test.ts
 *
 * When run without --conditions browser (e.g. `bun test src`), these tests
 * are skipped because @kobalte/core requires the solid-js client bundle.
 */

import { describe, expect, test } from "bun:test"
import type { Part as PartType, ToolPart, TextPart, ReasoningPart } from "@tinycode/sdk/v2"

// Attempt dynamic import; skip all tests if browser conditions are unavailable.
let groupParts: any
let sameGroups: any
let renderable: any
let partDefaultOpen: any
let loadError: string | undefined

try {
  // Setup minimal browser globals before importing message-part
  if (typeof globalThis.window === "undefined") {
    // @ts-expect-error - minimal stub
    globalThis.window = {
      history: {
        state: null as any, length: 1,
        pushState(data: any) { (globalThis as any).window.history.state = data },
        replaceState(data: any) { (globalThis as any).window.history.state = data },
        back() {}, go() {}, forward() {},
      },
      location: { pathname: "/", assign: () => {}, href: "", origin: "", search: "", hash: "" },
      addEventListener: () => {}, removeEventListener: () => {},
      getComputedStyle: () => new Proxy({}, { get: () => "" }),
      innerWidth: 1024, innerHeight: 768,
      matchMedia: () => ({ matches: false, addEventListener: () => {}, removeEventListener: () => {}, addListener: () => {}, removeListener: () => {} }),
      requestAnimationFrame: (cb: () => void) => setTimeout(cb, 0),
      cancelAnimationFrame: (id: number) => clearTimeout(id),
      scrollTo: () => {}, dispatchEvent: () => true, CSS: undefined,
    }
  }
  if (typeof globalThis.document === "undefined") {
    const noop = () => {}
    const el = (): any => ({
      style: new Proxy({}, { set: () => true, get: () => "" }),
      setAttribute: noop, removeAttribute: noop, getAttribute: () => null,
      appendChild: noop, removeChild: noop, remove: noop,
      contains: () => false, closest: () => null,
      querySelector: () => null, querySelectorAll: () => [],
      addEventListener: noop, removeEventListener: noop,
      getBoundingClientRect: () => ({ top: 0, left: 0, right: 0, bottom: 0, width: 0, height: 0, x: 0, y: 0 }),
      focus: noop, blur: noop, isConnected: false, dataset: {},
      innerHTML: "", textContent: "", tagName: "DIV", nodeType: 1,
      childNodes: [], children: [], parentElement: null,
      setProperty: noop, removeProperty: noop,
    })
    // @ts-expect-error
    globalThis.document = {
      createElement: () => el(), createDocumentFragment: () => ({ appendChild: noop }),
      createTreeWalker: () => ({ nextNode: () => null }),
      createRange: () => ({ setStart: noop, setEnd: noop, getClientRects: () => [], cloneRange: () => ({}) }),
      getElementById: () => null, querySelector: () => null, querySelectorAll: () => [],
      body: el(), head: el(), documentElement: el(),
      activeElement: null, execCommand: () => false,
      addEventListener: noop, removeEventListener: noop,
      createComment: () => ({ nodeType: 8 }),
      createTextNode: (t: string) => ({ nodeType: 3, data: t, textContent: t }),
      importNode: (n: any) => n,
    }
  }
  if (typeof globalThis.navigator === "undefined") {
    // @ts-expect-error
    globalThis.navigator = { clipboard: undefined, userAgent: "bun-test" }
  }
  if (typeof globalThis.MutationObserver === "undefined") {
    // @ts-expect-error
    globalThis.MutationObserver = class { observe() {} disconnect() {} takeRecords() { return [] } }
  }
  if (typeof globalThis.ResizeObserver === "undefined") {
    // @ts-expect-error
    globalThis.ResizeObserver = class { observe() {} unobserve() {} disconnect() {} }
  }
  if (typeof globalThis.IntersectionObserver === "undefined") {
    // @ts-expect-error
    globalThis.IntersectionObserver = class { observe() {} unobserve() {} disconnect() {} }
  }

  const mod = await import("./message-part")
  groupParts = mod.groupParts
  sameGroups = mod.sameGroups
  renderable = mod.renderable
  partDefaultOpen = mod.partDefaultOpen
} catch (err) {
  loadError = err instanceof Error ? err.message : String(err)
}

const skip = !!loadError
const it = skip ? test.skip : test

function toolPart(overrides: Partial<ToolPart> & { tool: string }): ToolPart {
  return {
    id: `part_${Math.random().toString(36).slice(2, 8)}`,
    sessionID: "ses_1",
    messageID: "msg_1",
    type: "tool",
    callID: "call_1",
    state: { status: "completed", input: {}, output: "", title: "" },
    ...overrides,
  } as ToolPart
}

function textPart(text: string, overrides: Partial<TextPart> = {}): TextPart {
  return {
    id: `part_${Math.random().toString(36).slice(2, 8)}`,
    sessionID: "ses_1",
    messageID: "msg_1",
    type: "text",
    text,
    ...overrides,
  }
}

function reasoningPart(text: string): ReasoningPart {
  return {
    id: `part_${Math.random().toString(36).slice(2, 8)}`,
    sessionID: "ses_1",
    messageID: "msg_1",
    type: "reasoning",
    text,
    time: { start: Date.now() },
  }
}

describe("groupParts", () => {
  if (skip) {
    test.skip("skipped: requires --conditions browser", () => {})
    return
  }

  it("returns empty array for empty input", () => {
    expect(groupParts([])).toEqual([])
  })

  it("groups consecutive context tools into a single context group", () => {
    const parts = [
      { messageID: "msg_1", part: toolPart({ tool: "read", id: "p1" }) },
      { messageID: "msg_1", part: toolPart({ tool: "glob", id: "p2" }) },
      { messageID: "msg_1", part: toolPart({ tool: "grep", id: "p3" }) },
    ]
    const result = groupParts(parts)
    expect(result).toHaveLength(1)
    expect(result[0].type).toBe("context")
    if (result[0].type === "context") {
      expect(result[0].refs).toHaveLength(3)
    }
  })

  it("keeps non-context tools as individual part groups", () => {
    const parts = [
      { messageID: "msg_1", part: toolPart({ tool: "bash", id: "p1" }) },
      { messageID: "msg_1", part: toolPart({ tool: "edit", id: "p2" }) },
    ]
    const result = groupParts(parts)
    expect(result).toHaveLength(2)
    expect(result[0].type).toBe("part")
    expect(result[1].type).toBe("part")
  })

  it("separates context groups around non-context tools", () => {
    const parts = [
      { messageID: "msg_1", part: toolPart({ tool: "read", id: "p1" }) },
      { messageID: "msg_1", part: toolPart({ tool: "glob", id: "p2" }) },
      { messageID: "msg_1", part: toolPart({ tool: "bash", id: "p3" }) },
      { messageID: "msg_1", part: toolPart({ tool: "read", id: "p4" }) },
    ]
    const result = groupParts(parts)
    expect(result).toHaveLength(3)
    expect(result[0].type).toBe("context")
    expect(result[1].type).toBe("part")
    expect(result[2].type).toBe("context")
  })

  it("includes list tool in context group", () => {
    const parts = [
      { messageID: "msg_1", part: toolPart({ tool: "list", id: "p1" }) },
    ]
    const result = groupParts(parts)
    expect(result).toHaveLength(1)
    expect(result[0].type).toBe("context")
  })

  it("treats text parts as individual groups", () => {
    const parts = [
      { messageID: "msg_1", part: textPart("hello") as PartType },
    ]
    const result = groupParts(parts)
    expect(result).toHaveLength(1)
    expect(result[0].type).toBe("part")
  })
})

describe("sameGroups", () => {
  if (skip) {
    test.skip("skipped: requires --conditions browser", () => {})
    return
  }

  it("returns true for identical references", () => {
    const groups: any[] = []
    expect(sameGroups(groups, groups)).toBe(true)
  })

  it("returns true for both undefined", () => {
    expect(sameGroups(undefined, undefined)).toBe(true)
  })

  it("returns false when one is undefined", () => {
    expect(sameGroups([], undefined)).toBe(false)
    expect(sameGroups(undefined, [])).toBe(false)
  })

  it("returns false for different lengths", () => {
    const a: any[] = [{ key: "part:msg_1:p1", type: "part", ref: { messageID: "msg_1", partID: "p1" } }]
    expect(sameGroups(a, [])).toBe(false)
  })

  it("returns true for structurally equal groups", () => {
    const a: any[] = [{ key: "part:msg_1:p1", type: "part", ref: { messageID: "msg_1", partID: "p1" } }]
    const b: any[] = [{ key: "part:msg_1:p1", type: "part", ref: { messageID: "msg_1", partID: "p1" } }]
    expect(sameGroups(a, b)).toBe(true)
  })

  it("returns false when keys differ", () => {
    const a: any[] = [{ key: "part:msg_1:p1", type: "part", ref: { messageID: "msg_1", partID: "p1" } }]
    const b: any[] = [{ key: "part:msg_1:p2", type: "part", ref: { messageID: "msg_1", partID: "p2" } }]
    expect(sameGroups(a, b)).toBe(false)
  })
})

describe("renderable", () => {
  if (skip) {
    test.skip("skipped: requires --conditions browser", () => {})
    return
  }

  it("returns true for tool parts (non-hidden)", () => {
    expect(renderable(toolPart({ tool: "bash" }))).toBe(true)
  })

  it("returns false for todowrite tool", () => {
    expect(renderable(toolPart({ tool: "todowrite" }))).toBe(false)
  })

  it("returns false for pending question tool", () => {
    expect(renderable(toolPart({ tool: "question", state: { status: "pending", input: {} } }))).toBe(false)
  })

  it("returns true for completed question tool", () => {
    expect(renderable(toolPart({ tool: "question", state: { status: "completed", input: {}, output: "", title: "" } }))).toBe(true)
  })

  it("returns true for text part with non-empty text", () => {
    expect(renderable(textPart("hello") as PartType)).toBe(true)
  })

  it("returns false for text part with empty text", () => {
    expect(renderable(textPart("") as PartType)).toBe(false)
  })

  it("returns false for text part with whitespace-only text", () => {
    expect(renderable(textPart("   \n  ") as PartType)).toBe(false)
  })

  it("returns true for reasoning part when thinkingMode is show", () => {
    expect(renderable(reasoningPart("thinking...") as PartType, "show")).toBe(true)
  })

  it("returns false for reasoning part when thinkingMode is hide", () => {
    expect(renderable(reasoningPart("thinking...") as PartType, "hide")).toBe(false)
  })

  it("returns true for reasoning when thinkingMode is stream", () => {
    expect(renderable(reasoningPart("thinking...") as PartType, "stream")).toBe(true)
  })

  it("returns false for reasoning part with empty text", () => {
    expect(renderable(reasoningPart("") as PartType, "show")).toBe(false)
  })

  it("defaults thinkingMode to show", () => {
    expect(renderable(reasoningPart("thinking...") as PartType)).toBe(true)
  })

  it("handles boolean thinkingMode true as show", () => {
    expect(renderable(reasoningPart("thinking...") as PartType, true)).toBe(true)
  })

  it("handles boolean thinkingMode false as hide", () => {
    expect(renderable(reasoningPart("thinking...") as PartType, false)).toBe(false)
  })
})

describe("partDefaultOpen", () => {
  if (skip) {
    test.skip("skipped: requires --conditions browser", () => {})
    return
  }

  it("returns undefined for non-tool parts", () => {
    expect(partDefaultOpen(textPart("hello") as PartType)).toBeUndefined()
  })

  it("returns shell flag for bash tool", () => {
    expect(partDefaultOpen(toolPart({ tool: "bash" }), true, false)).toBe(true)
    expect(partDefaultOpen(toolPart({ tool: "bash" }), false, false)).toBe(false)
  })

  it("returns edit flag for edit tool", () => {
    expect(partDefaultOpen(toolPart({ tool: "edit" }), false, true)).toBe(true)
    expect(partDefaultOpen(toolPart({ tool: "edit" }), false, false)).toBe(false)
  })

  it("returns edit flag for write tool", () => {
    expect(partDefaultOpen(toolPart({ tool: "write" }), false, true)).toBe(true)
  })

  it("returns edit flag for apply_patch tool", () => {
    expect(partDefaultOpen(toolPart({ tool: "apply_patch" }), false, true)).toBe(true)
  })

  it("returns undefined for other tools", () => {
    expect(partDefaultOpen(toolPart({ tool: "read" }))).toBeUndefined()
    expect(partDefaultOpen(toolPart({ tool: "glob" }))).toBeUndefined()
  })

  it("defaults shell and edit flags to false", () => {
    expect(partDefaultOpen(toolPart({ tool: "bash" }))).toBe(false)
    expect(partDefaultOpen(toolPart({ tool: "edit" }))).toBe(false)
  })
})
