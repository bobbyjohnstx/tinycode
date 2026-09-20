import { describe, expect, test } from "bun:test"
import { resolveFileDiff, normalize, text } from "./session-diff"

describe("resolveFileDiff edge cases", () => {
  test("produces empty diff for empty before and after", () => {
    const diff = resolveFileDiff({ file: "a.ts" })
    expect(diff.name).toBe("a.ts")
    expect(diff.additionLines).toEqual([])
    expect(diff.deletionLines).toEqual([])
  })

  test("handles addition-only diff (empty before)", () => {
    const diff = resolveFileDiff({
      file: "new.ts",
      before: "",
      after: "hello world\n",
    })
    expect(diff.additionLines.join("")).toContain("hello world")
  })

  test("handles deletion-only diff (empty after)", () => {
    const diff = resolveFileDiff({
      file: "old.ts",
      before: "goodbye world\n",
      after: "",
    })
    expect(diff.deletionLines.join("")).toContain("goodbye world")
  })

  test("handles identical before and after", () => {
    const diff = resolveFileDiff({
      file: "same.ts",
      before: "no change\n",
      after: "no change\n",
    })
    expect(diff.additionLines.join("")).toBe("no change\n")
    expect(diff.deletionLines.join("")).toBe("no change\n")
  })

  test("handles multiline content", () => {
    const diff = resolveFileDiff({
      file: "multi.ts",
      before: "line1\nline2\nline3\n",
      after: "line1\nchanged\nline3\n",
    })
    expect(diff.additionLines.join("")).toContain("changed")
    expect(diff.deletionLines.join("")).toContain("line2")
  })
})

describe("normalize edge cases", () => {
  test("preserves file name in normalized diff", () => {
    const view = normalize({
      file: "src/app.ts",
      additions: 5,
      deletions: 2,
      status: "modified",
      before: "old\n",
      after: "new\n",
    })
    expect(view.file).toBe("src/app.ts")
    expect(view.additions).toBe(5)
    expect(view.deletions).toBe(2)
    expect(view.status).toBe("modified")
  })

  test("handles added status", () => {
    const view = normalize({
      file: "new.ts",
      additions: 10,
      deletions: 0,
      status: "added",
      before: "",
      after: "content\n",
    })
    expect(view.status).toBe("added")
    expect(text(view, "additions")).toContain("content")
  })

  test("handles deleted status", () => {
    const view = normalize({
      file: "old.ts",
      additions: 0,
      deletions: 5,
      status: "deleted",
      before: "content\n",
      after: "",
    })
    expect(view.status).toBe("deleted")
    expect(text(view, "deletions")).toContain("content")
  })

  test("text returns empty string for no changes on requested side", () => {
    const view = normalize({
      file: "empty.ts",
      additions: 0,
      deletions: 0,
      before: "",
      after: "",
    })
    expect(text(view, "additions")).toBe("")
    expect(text(view, "deletions")).toBe("")
  })
})
