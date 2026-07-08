import { describe, test, expect } from "bun:test"
import { readFileSync } from "fs"
import path from "path"
import { parse } from "yaml"

const WORKFLOW = path.join(import.meta.dir, "../../../../.github/workflows/release.yml")

describe("release workflow", () => {
  test("triggers on version tags", () => {
    const content = readFileSync(WORKFLOW, "utf-8")
    const workflow = parse(content)
    expect(workflow.on.push.tags).toContain("v*")
  })

  test("has contents write permission", () => {
    const content = readFileSync(WORKFLOW, "utf-8")
    const workflow = parse(content)
    expect(workflow.permissions.contents).toBe("write")
  })

  test("sets required env vars for build", () => {
    const content = readFileSync(WORKFLOW, "utf-8")
    expect(content).toContain("TINYCODE_VERSION")
    expect(content).toContain("TINYCODE_RELEASE")
    expect(content).toContain("GH_REPO")
  })
})
