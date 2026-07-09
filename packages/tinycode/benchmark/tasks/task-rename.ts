import { $ } from "bun"
import type { TaskDefinition, TaskResult, NdjsonEvent } from "./types"

export const task: TaskDefinition = {
  id: 4,
  name: "Multi-File Rename",
  prompt:
    'Rename the function "multiply" to "product" in src/math.ts and update all references across the project so that everything still works. Run the tests to confirm.',

  async verify(fixtureDir: string, events: NdjsonEvent[]): Promise<TaskResult> {
    // Check 1: No "multiply" references
    const multiplyGrep = await $`grep -r "multiply" src/ test/`
      .cwd(fixtureDir)
      .nothrow()
      .quiet()

    const noMultiply = multiplyGrep.exitCode !== 0 // Exit code 1 means no matches

    // Check 2: "product" exists in multiple places
    const productGrep = await $`grep -r "product" src/ test/`
      .cwd(fixtureDir)
      .nothrow()
      .quiet()

    const productMatches =
      productGrep.exitCode === 0 &&
      productGrep.text().trim().split("\n").filter(Boolean).length >= 2

    // Check 3: Tests pass
    const testResult = await $`bun test test/math.test.ts`
      .cwd(fixtureDir)
      .nothrow()
      .quiet()

    const testsPassed = testResult.exitCode === 0

    if (noMultiply && productMatches && testsPassed) {
      return {
        score: 3,
        reason: "All checks passed: multiply removed, product added, tests pass",
        details: { noMultiply, productMatches, testsPassed },
      }
    }

    if (noMultiply && (!productMatches || !testsPassed)) {
      return {
        score: 2,
        reason: "multiply removed but tests fail or product not found everywhere",
        details: { noMultiply, productMatches, testsPassed },
      }
    }

    // Check if any tool calls were made
    const hasToolCalls = events.some((event) => event.type === "tool_use")

    if (!hasToolCalls) {
      return {
        score: 1,
        reason: "No tool calls (text advice only)",
        details: { noMultiply, productMatches, testsPassed, hasToolCalls },
      }
    }

    return {
      score: 0,
      reason: "Incomplete or failed",
      details: { noMultiply, productMatches, testsPassed },
    }
  },
}
