import { describe, expect, test } from "bun:test"
import { matchClientSlash } from "./session-slash"

describe("matchClientSlash", () => {
  test("matches btw with and without args", () => {
    expect(matchClientSlash("/btw")).toEqual({ kind: "btw", args: "" })
    expect(matchClientSlash("/btw what is this")).toEqual({ kind: "btw", args: "what is this" })
  })

  test("matches goal", () => {
    expect(matchClientSlash("/goal clear")).toEqual({ kind: "goal", args: "clear" })
  })

  test("matches export variants", () => {
    expect(matchClientSlash("/export")).toEqual({ kind: "export", args: "" })
    expect(matchClientSlash("/export html")).toEqual({ kind: "export-html", args: "" })
    expect(matchClientSlash("/export-html")).toEqual({ kind: "export-html", args: "" })
  })

  test("ignores unrelated prompts", () => {
    expect(matchClientSlash("hello")).toBeUndefined()
    expect(matchClientSlash("/ask build hi")).toBeUndefined()
  })
})
