import { describe, expect, test } from "bun:test"
import {
  formatSelectedLineLabel,
  previewSelectedLines,
  cloneSelectedLineRange,
  lineInSelectedRange,
  isSingleLineSelection,
  createLineNumberSelectionBridge,
} from "./selection-bridge"

describe("formatSelectedLineLabel", () => {
  const t = (key: string, params: Record<string, string | number>) => {
    if (key === "ui.sessionReview.selection.line") return `line ${params.line}`
    return `lines ${params.start}-${params.end}`
  }

  test("formats single line selection", () => {
    expect(formatSelectedLineLabel({ start: 5, end: 5 }, t)).toBe("line 5")
  })

  test("formats multi-line selection with ascending range", () => {
    expect(formatSelectedLineLabel({ start: 3, end: 7 }, t)).toBe("lines 3-7")
  })

  test("normalizes reversed range so start is less than end", () => {
    expect(formatSelectedLineLabel({ start: 10, end: 3 }, t)).toBe("lines 3-10")
  })
})

describe("previewSelectedLines", () => {
  const source = "line one\nline two\nline three\nline four\nline five"

  test("returns first two lines of selection", () => {
    expect(previewSelectedLines(source, { start: 1, end: 3 })).toBe("line one\nline two")
  })

  test("returns single line for single-line selection", () => {
    expect(previewSelectedLines(source, { start: 2, end: 2 })).toBe("line two")
  })

  test("returns undefined for empty selection beyond source", () => {
    expect(previewSelectedLines(source, { start: 100, end: 200 })).toBeUndefined()
  })

  test("normalizes reversed range", () => {
    expect(previewSelectedLines(source, { start: 3, end: 1 })).toBe("line one\nline two")
  })

  test("clamps start to 1", () => {
    expect(previewSelectedLines(source, { start: 0, end: 2 })).toBe("line one\nline two")
  })
})

describe("cloneSelectedLineRange", () => {
  test("creates a new object with same start and end", () => {
    const original = { start: 1, end: 5 }
    const cloned = cloneSelectedLineRange(original)
    expect(cloned).toEqual({ start: 1, end: 5 })
    expect(cloned).not.toBe(original)
  })

  test("copies side when present", () => {
    const original = { start: 1, end: 5, side: "additions" as const }
    const cloned = cloneSelectedLineRange(original)
    expect(cloned.side).toBe("additions")
  })

  test("copies endSide when present", () => {
    const original = { start: 1, end: 5, side: "additions" as const, endSide: "deletions" as const }
    const cloned = cloneSelectedLineRange(original)
    expect(cloned.endSide).toBe("deletions")
  })

  test("omits side when not present", () => {
    const original = { start: 1, end: 5 }
    const cloned = cloneSelectedLineRange(original)
    expect("side" in cloned).toBe(false)
  })
})

describe("lineInSelectedRange", () => {
  test("returns false for null range", () => {
    expect(lineInSelectedRange(null, 5)).toBe(false)
  })

  test("returns false for undefined range", () => {
    expect(lineInSelectedRange(undefined, 5)).toBe(false)
  })

  test("returns true when line is within range", () => {
    expect(lineInSelectedRange({ start: 1, end: 10 }, 5)).toBe(true)
  })

  test("returns true when line equals start", () => {
    expect(lineInSelectedRange({ start: 5, end: 10 }, 5)).toBe(true)
  })

  test("returns true when line equals end", () => {
    expect(lineInSelectedRange({ start: 5, end: 10 }, 10)).toBe(true)
  })

  test("returns false when line is before range", () => {
    expect(lineInSelectedRange({ start: 5, end: 10 }, 3)).toBe(false)
  })

  test("returns false when line is after range", () => {
    expect(lineInSelectedRange({ start: 5, end: 10 }, 15)).toBe(false)
  })

  test("handles reversed range correctly", () => {
    expect(lineInSelectedRange({ start: 10, end: 5 }, 7)).toBe(true)
  })

  test("checks side for boundary lines", () => {
    const range = { start: 5, end: 10, side: "additions" as const }
    expect(lineInSelectedRange(range, 5, "additions")).toBe(true)
    expect(lineInSelectedRange(range, 5, "deletions")).toBe(false)
  })

  test("returns true for interior lines regardless of side", () => {
    const range = { start: 5, end: 10, side: "additions" as const, endSide: "deletions" as const }
    expect(lineInSelectedRange(range, 7, "additions")).toBe(true)
    expect(lineInSelectedRange(range, 7, "deletions")).toBe(true)
  })
})

describe("isSingleLineSelection", () => {
  test("returns false for null", () => {
    expect(isSingleLineSelection(null)).toBe(false)
  })

  test("returns true when start equals end with no endSide", () => {
    expect(isSingleLineSelection({ start: 5, end: 5 })).toBe(true)
  })

  test("returns true when start equals end and endSide matches side", () => {
    expect(isSingleLineSelection({ start: 5, end: 5, side: "additions", endSide: "additions" })).toBe(true)
  })

  test("returns false when start differs from end", () => {
    expect(isSingleLineSelection({ start: 5, end: 6 })).toBe(false)
  })

  test("returns false when endSide differs from side on same line", () => {
    expect(isSingleLineSelection({ start: 5, end: 5, side: "additions", endSide: "deletions" })).toBe(false)
  })
})

describe("createLineNumberSelectionBridge", () => {
  test("begins in text mode for non-number-column clicks", () => {
    const bridge = createLineNumberSelectionBridge()
    bridge.begin(false, 1)
    const mode = bridge.finish()
    expect(mode).toBe("text")
  })

  test("begins in numbers mode for number-column clicks", () => {
    const bridge = createLineNumberSelectionBridge()
    bridge.begin(true, 5)
    const mode = bridge.finish()
    expect(mode).toBe("numbers")
  })

  test("track returns false in text mode", () => {
    const bridge = createLineNumberSelectionBridge()
    bridge.begin(false, 1)
    expect(bridge.track(1, 2)).toBe(false)
  })

  test("track returns true in numbers mode with button pressed", () => {
    const bridge = createLineNumberSelectionBridge()
    bridge.begin(true, 1)
    expect(bridge.track(1, 2)).toBe(true)
  })

  test("consume returns true after multi-line number drag", () => {
    const bridge = createLineNumberSelectionBridge()
    bridge.begin(true, 1)
    bridge.track(1, 3)
    bridge.finish()
    const result = bridge.consume({ start: 1, end: 3 })
    expect(result).toBe(true)
  })

  test("consume returns false when no drag movement occurred", () => {
    const bridge = createLineNumberSelectionBridge()
    bridge.begin(true, 1)
    bridge.finish()
    const result = bridge.consume({ start: 1, end: 1 })
    expect(result).toBe(false)
  })

  test("reset clears pending state", () => {
    const bridge = createLineNumberSelectionBridge()
    bridge.begin(true, 1)
    bridge.track(1, 3)
    bridge.finish()
    bridge.reset()
    const result = bridge.consume({ start: 1, end: 3 })
    expect(result).toBe(false)
  })
})
