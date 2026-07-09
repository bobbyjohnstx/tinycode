import path from "path"
import type { TaskDefinition, TaskResult, NdjsonEvent } from "./types"

export const task: TaskDefinition = {
  id: 3,
  name: "Add Input Validation",
  prompt:
    'Add input validation to the "processUser" function in src/validate.ts. It should throw an Error if: name is not a string, age is not a number, age is less than 0, or age is greater than 150. Keep the existing return statement for valid inputs.',

  async verify(fixtureDir: string, events: NdjsonEvent[]): Promise<TaskResult> {
    // Import the module and test validation
    const modulePath = path.join(fixtureDir, "src/validate.ts")

    let mod: { processUser: (input: unknown) => string }
    try {
      mod = await import(modulePath)
    } catch (err) {
      return {
        score: 0,
        reason: "Failed to import validate module",
        details: { error: String(err) },
      }
    }

    const cases: Array<{ input: unknown; shouldThrow: boolean }> = [
      { input: { name: "Alice", age: 30 }, shouldThrow: false },
      { input: { name: 123, age: 30 }, shouldThrow: true },
      { input: { name: "Bob", age: "thirty" }, shouldThrow: true },
      { input: { name: "Carol", age: -1 }, shouldThrow: true },
      { input: { name: "Dave", age: 151 }, shouldThrow: true },
    ]

    let passed = 0
    for (const { input, shouldThrow } of cases) {
      try {
        mod.processUser(input)
        if (!shouldThrow) passed++
      } catch {
        if (shouldThrow) passed++
      }
    }

    // Check if tools were used
    const hasToolCalls = events.some((event) => event.type === "tool_use")

    if (passed === 5 && hasToolCalls) {
      return {
        score: 3,
        reason: "All 5 validation cases pass, tools used",
        details: { passed, total: cases.length, hasToolCalls },
      }
    }

    if (passed >= 3 && hasToolCalls) {
      return {
        score: 2,
        reason: `${passed}/5 cases pass, tools used`,
        details: { passed, total: cases.length, hasToolCalls },
      }
    }

    if (passed < 3 || !hasToolCalls) {
      return {
        score: 1,
        reason: `${passed}/5 cases pass or no tool calls`,
        details: { passed, total: cases.length, hasToolCalls },
      }
    }

    return {
      score: 0,
      reason: "File not modified or import failed",
      details: { passed, total: cases.length, hasToolCalls },
    }
  },
}
