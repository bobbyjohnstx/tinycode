import { describe, expect, test } from "bun:test"
import { patchFile, patchFiles } from "./apply-patch-file"

describe("patchFile", () => {
  test("returns undefined for null input", () => {
    expect(patchFile(null)).toBeUndefined()
  })

  test("returns undefined for non-object input", () => {
    expect(patchFile("string")).toBeUndefined()
    expect(patchFile(42)).toBeUndefined()
    expect(patchFile(true)).toBeUndefined()
  })

  test("returns undefined when type is missing", () => {
    expect(patchFile({ filePath: "/a.ts", relativePath: "a.ts", before: "x" })).toBeUndefined()
  })

  test("returns undefined when type is invalid", () => {
    expect(patchFile({ filePath: "/a.ts", relativePath: "a.ts", type: "rename", before: "x" })).toBeUndefined()
  })

  test("returns undefined when filePath is missing", () => {
    expect(patchFile({ type: "add", before: "x" })).toBeUndefined()
  })

  test("returns undefined when no content is provided", () => {
    expect(patchFile({ filePath: "/a.ts", relativePath: "a.ts", type: "update" })).toBeUndefined()
  })

  test("parses add type with after content", () => {
    const result = patchFile({
      filePath: "/tmp/new.ts",
      relativePath: "new.ts",
      type: "add",
      before: "",
      after: "hello\n",
      additions: 1,
      deletions: 0,
    })
    expect(result).toBeDefined()
    expect(result?.type).toBe("add")
    expect(result?.filePath).toBe("/tmp/new.ts")
    expect(result?.relativePath).toBe("new.ts")
    expect(result?.additions).toBe(1)
    expect(result?.deletions).toBe(0)
  })

  test("parses delete type with before content", () => {
    const result = patchFile({
      filePath: "/tmp/old.ts",
      relativePath: "old.ts",
      type: "delete",
      before: "goodbye\n",
      after: "",
      additions: 0,
      deletions: 1,
    })
    expect(result).toBeDefined()
    expect(result?.type).toBe("delete")
  })

  test("parses move type with movePath", () => {
    const result = patchFile({
      filePath: "/tmp/old.ts",
      relativePath: "old.ts",
      type: "move",
      movePath: "/tmp/new.ts",
      before: "content\n",
      after: "content\n",
      additions: 0,
      deletions: 0,
    })
    expect(result).toBeDefined()
    expect(result?.type).toBe("move")
    expect(result?.movePath).toBe("/tmp/new.ts")
  })

  test("uses filePath as relativePath when relativePath is absent", () => {
    const result = patchFile({
      filePath: "/tmp/a.ts",
      type: "update",
      before: "old\n",
      after: "new\n",
    })
    expect(result).toBeDefined()
    expect(result?.relativePath).toBe("/tmp/a.ts")
  })

  test("defaults additions and deletions to 0 when not numbers", () => {
    const result = patchFile({
      filePath: "/tmp/a.ts",
      relativePath: "a.ts",
      type: "update",
      before: "old\n",
      after: "new\n",
    })
    expect(result).toBeDefined()
    expect(result?.additions).toBe(0)
    expect(result?.deletions).toBe(0)
  })

  test("accepts diff field as alternate to patch", () => {
    const result = patchFile({
      filePath: "/tmp/a.ts",
      relativePath: "a.ts",
      type: "update",
      diff: "@@ -1 +1 @@\n-old\n+new\n",
      additions: 1,
      deletions: 1,
    })
    expect(result).toBeDefined()
    expect(result?.view).toBeDefined()
  })
})

describe("patchFiles", () => {
  test("returns empty array for non-array input", () => {
    expect(patchFiles(null)).toEqual([])
    expect(patchFiles(undefined)).toEqual([])
    expect(patchFiles("string")).toEqual([])
    expect(patchFiles(42)).toEqual([])
  })

  test("returns empty array for empty array", () => {
    expect(patchFiles([])).toEqual([])
  })

  test("filters out invalid entries", () => {
    const result = patchFiles([null, undefined, "string", { filePath: "/a.ts" }])
    expect(result).toEqual([])
  })

  test("parses multiple valid entries", () => {
    const result = patchFiles([
      {
        filePath: "/tmp/a.ts",
        relativePath: "a.ts",
        type: "update",
        before: "old\n",
        after: "new\n",
        additions: 1,
        deletions: 1,
      },
      {
        filePath: "/tmp/b.ts",
        relativePath: "b.ts",
        type: "add",
        before: "",
        after: "hello\n",
        additions: 1,
        deletions: 0,
      },
    ])
    expect(result).toHaveLength(2)
    expect(result[0].relativePath).toBe("a.ts")
    expect(result[1].relativePath).toBe("b.ts")
  })
})
