import { $ } from "bun"
import type { TaskDefinition, TaskResult, NdjsonEvent } from "./types"

export const task: TaskDefinition = {
  id: 5,
  name: "Debug from Stack Trace",
  prompt:
    'Running "bun test test/format.test.ts" produces: TypeError: Cannot read properties of undefined (reading \'toUpperCase\'). Find and fix the root cause. The bug is NOT in format.ts -- trace the error to its source.',

  async verify(fixtureDir: string, events: NdjsonEvent[]): Promise<TaskResult> {
    // Check 1: Tests pass
    const testResult = await $`bun test test/format.test.ts`
      .cwd(fixtureDir)
      .nothrow()
      .quiet()

    const testsPassed = testResult.exitCode === 0

    // Check 2: src/user.ts was modified
    const userDiff = await $`git diff src/user.ts`.cwd(fixtureDir).nothrow().quiet()

    const userModified = userDiff.text().trim().length > 0

    // Check 3: src/format.ts is either unmodified OR has only trivial changes
    const formatDiff = await $`git diff src/format.ts`.cwd(fixtureDir).nothrow().quiet()

    const formatDiffText = formatDiff.text().trim()
    const formatUnmodifiedOrTrivial =
      formatDiffText.length === 0 ||
      (formatDiffText.split("\n").filter((line) => line.startsWith("+") || line.startsWith("-"))
        .length < 5)

    if (testsPassed && userModified) {
      return {
        score: 3,
        reason: "Tests pass AND fix is in src/user.ts (correct root cause)",
        details: { testsPassed, userModified, formatUnmodifiedOrTrivial },
      }
    }

    if (testsPassed && !userModified) {
      return {
        score: 2,
        reason: "Tests pass but fix is in src/format.ts (symptom fix, not root cause)",
        details: { testsPassed, userModified, formatUnmodifiedOrTrivial },
      }
    }

    // Check if any tool calls were made
    const hasToolCalls = events.some((event) => event.type === "tool_use")

    if (!hasToolCalls || !testsPassed) {
      return {
        score: 1,
        reason: "No tool calls or tests still fail",
        details: { testsPassed, userModified, hasToolCalls },
      }
    }

    return {
      score: 0,
      reason: "Incomplete or failed",
      details: { testsPassed, userModified },
    }
  },
}
