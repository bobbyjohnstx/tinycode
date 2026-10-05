import { describe, expect, test } from "bun:test"
import {
  buildInitialGoalPrompt,
  clearGoal,
  detectEcosystem,
  getGoal,
  recordGoalOutput,
  resolveGoalCommand,
  setGoal,
  WORK_LOOP_PREFIX,
} from "./session-goal"

describe("detectEcosystem", () => {
  test("prefers package.json (node) over later indicators", () => {
    expect(detectEcosystem(["README.md", "package.json", "go.mod"])).toBe("node")
  })

  test("detects go.mod", () => {
    expect(detectEcosystem(["go.mod", "main.go"])).toBe("go")
  })

  test("returns empty when unknown", () => {
    expect(detectEcosystem(["README.md"])).toBe("")
  })
})

describe("resolveGoalCommand", () => {
  test("maps tests pass for go", () => {
    expect(resolveGoalCommand("make sure all tests pass", "go")).toBe("go test ./... -count=1")
  })

  test("maps build succeeds for node", () => {
    expect(resolveGoalCommand("build succeeds", "node")).toBe("npm run build")
  })

  test("returns undefined for unmatched ecosystem command", () => {
    expect(resolveGoalCommand("vet passes", "node")).toBeUndefined()
  })

  test("returns undefined for freeform goals", () => {
    expect(resolveGoalCommand("make auth work", "go")).toBeUndefined()
  })
})

describe("goal state helpers", () => {
  test("set/get/clear and stuck detection", () => {
    const g = setGoal("ses_1", "tests pass", "go test ./...")
    expect(getGoal("ses_1")).toBe(g)
    expect(recordGoalOutput(g, "a")).toBe(false)
    expect(recordGoalOutput(g, "a")).toBe(false)
    expect(recordGoalOutput(g, "a")).toBe(true)
    expect(clearGoal("ses_1")).toBe(true)
    expect(getGoal("ses_1")).toBeUndefined()
  })

  test("buildInitialGoalPrompt uses work-loop prefix without command", () => {
    const text = buildInitialGoalPrompt("fix the bug")
    expect(text.startsWith(WORK_LOOP_PREFIX)).toBe(true)
    expect(text.endsWith("fix the bug")).toBe(true)
  })

  test("buildInitialGoalPrompt mentions verify command when present", () => {
    const text = buildInitialGoalPrompt("tests pass", "go test ./...")
    expect(text).toContain("go test ./...")
    expect(text).toContain("Goal: tests pass")
  })
})
