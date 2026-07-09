import type { NdjsonEvent } from "../tasks/types"

export interface EventSummary {
  events: NdjsonEvent[]
  toolCallCount: number
  toolFailureCount: number
  toolsUsed: Set<string>
}

export function parseNdjsonEvents(stdout: string): EventSummary {
  const events: NdjsonEvent[] = []
  const toolsUsed = new Set<string>()
  let toolCallCount = 0
  let toolFailureCount = 0

  const lines = stdout.trim().split("\n").filter(Boolean)

  for (const line of lines) {
    try {
      const event = JSON.parse(line) as NdjsonEvent
      events.push(event)

      if (event.type === "tool_use") {
        toolCallCount++

        if (event.part?.tool) {
          toolsUsed.add(event.part.tool)
        }

        if (event.part?.state?.status === "error") {
          toolFailureCount++
        }
      }
    } catch (err) {
      // Skip invalid JSON lines
      continue
    }
  }

  return {
    events,
    toolCallCount,
    toolFailureCount,
    toolsUsed,
  }
}
