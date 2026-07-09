import { $ } from "bun"
import * as fs from "fs/promises"
import path from "path"
import type { TaskDefinition, TaskResult, NdjsonEvent } from "./types"

export const task: TaskDefinition = {
  id: 2,
  name: "Fix Failing Test",
  prompt:
    'Run "bun test test/math.test.ts". One test fails. Fix the source code so all tests pass. Do not modify the test file.',

  async verify(fixtureDir: string, events: NdjsonEvent[]): Promise<TaskResult> {
    // Check 1: Tests pass
    const testResult = await $`bun test test/math.test.ts`
      .cwd(fixtureDir)
      .nothrow()
      .quiet()

    const testsPassed = testResult.exitCode === 0

    // Check 2: Test file is unmodified
    const templateTestPath = path.join(
      import.meta.dir,
      "../fixture/template/test/math.test.ts"
    )
    const fixtureTestPath = path.join(fixtureDir, "test/math.test.ts")

    const templateContent = await fs.readFile(templateTestPath, "utf-8")
    const fixtureContent = await fs.readFile(fixtureTestPath, "utf-8")

    const testFileUnmodified = templateContent === fixtureContent

    // Check 3: Shell tool used
    const shellTools = ["shell", "bash"]
    const usedShell = events.some(
      (event) =>
        event.type === "tool_use" &&
        event.part?.tool &&
        shellTools.includes(event.part.tool.toLowerCase())
    )

    if (testsPassed && testFileUnmodified && usedShell) {
      return {
        score: 3,
        reason: "Tests pass, test file unmodified, shell tool used",
        details: { testsPassed, testFileUnmodified, usedShell },
      }
    }

    if (usedShell && !testsPassed) {
      return {
        score: 2,
        reason: "Shell tool used but tests still fail",
        details: { testsPassed, testFileUnmodified, usedShell },
      }
    }

    if (!usedShell) {
      return {
        score: 1,
        reason: "No tool calls (text advice only)",
        details: { testsPassed, testFileUnmodified, usedShell },
      }
    }

    return {
      score: 0,
      reason: "Incomplete or failed",
      details: { testsPassed, testFileUnmodified, usedShell },
    }
  },
}
