import { $ } from "bun"
import type { TaskDefinition, TaskResult, NdjsonEvent } from "./types"

export const task: TaskDefinition = {
  id: 1,
  name: "Tool-Calling Diagnostic",
  prompt:
    'Find the function "divide" in this project, read its implementation, and add a @throws JSDoc comment above the function describing when it would throw (e.g., division by zero). Do not fix the function, just document it.',

  async verify(fixtureDir: string, events: NdjsonEvent[]): Promise<TaskResult> {
    // Check 1: @throws in src/math.ts
    const grepResult = await $`grep -c "@throws" src/math.ts`
      .cwd(fixtureDir)
      .nothrow()
      .quiet()

    const hasThrowsDoc = grepResult.exitCode === 0 && parseInt(grepResult.text().trim()) >= 1

    // Check 2: Tool calls for discovery (grep, glob, or read)
    const discoveryTools = ["grep", "glob", "read"]
    const hasDiscovery = events.some(
      (event) =>
        event.type === "tool_use" &&
        event.part?.tool &&
        discoveryTools.includes(event.part.tool)
    )

    // Check 3: Tool calls for modification (edit or write)
    const modificationTools = ["edit", "write"]
    const hasModification = events.some(
      (event) =>
        event.type === "tool_use" &&
        event.part?.tool &&
        modificationTools.includes(event.part.tool)
    )

    if (hasThrowsDoc && hasDiscovery && hasModification) {
      return {
        score: 3,
        reason: "All checks passed: @throws added, discovery and modification tools used",
        details: { hasThrowsDoc, hasDiscovery, hasModification },
      }
    }

    if ((hasDiscovery || hasModification) && !hasThrowsDoc) {
      return {
        score: 2,
        reason: "Tools used but @throws not in file",
        details: { hasThrowsDoc, hasDiscovery, hasModification },
      }
    }

    if (!hasDiscovery && !hasModification) {
      return {
        score: 1,
        reason: "No tool calls (chat-only response)",
        details: { hasThrowsDoc, hasDiscovery, hasModification },
      }
    }

    return {
      score: 0,
      reason: "Incomplete or failed",
      details: { hasThrowsDoc, hasDiscovery, hasModification },
    }
  },
}
