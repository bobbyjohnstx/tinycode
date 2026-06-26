import { describe, expect, test } from "bun:test"
import { MessageV2 } from "../../src/session/message-v2"
import { MessageID, PartID, SessionID } from "../../src/session/schema"
import {
  extractFileOps,
  formatFileOps,
  parseFileOpsFromSummary,
  maskObservations,
  serializeForSummary,
} from "../../src/session/compaction"

const sessionID = SessionID.make("session")

function createMessage(
  id: string,
  role: "user" | "assistant",
  parts: MessageV2.Part[],
): MessageV2.WithParts {
  const info: MessageV2.User | MessageV2.Assistant =
    role === "user"
      ? ({
          id: MessageID.make(id),
          sessionID,
          role: "user",
          time: { created: Date.now() },
          agent: "build",
          model: { providerID: "test", modelID: "test-model" },
        } as MessageV2.User)
      : ({
          id: MessageID.make(id),
          sessionID,
          role: "assistant",
          time: { created: Date.now() },
          parentID: MessageID.make("msg_parent"),
          mode: "build",
          agent: "build",
          path: { cwd: "/tmp", root: "/tmp" },
          cost: 0,
          tokens: {
            output: 0,
            input: 0,
            reasoning: 0,
            cache: { read: 0, write: 0 },
          },
          modelID: "test-model",
          providerID: "test",
        } as MessageV2.Assistant)

  return { info, parts }
}

function createToolPart(
  messageID: string,
  tool: string,
  status: "completed" | "running" | "pending",
  input: Record<string, unknown>,
  output?: string,
): MessageV2.ToolPart {
  const base = {
    id: PartID.ascending(),
    messageID: MessageID.make(messageID),
    sessionID,
    type: "tool" as const,
    callID: crypto.randomUUID(),
    tool,
  }

  if (status === "completed") {
    return {
      ...base,
      state: {
        status: "completed",
        input,
        output: output ?? "",
        title: tool,
        metadata: {},
        time: { start: Date.now(), end: Date.now() },
      },
    } as MessageV2.ToolPart
  }

  if (status === "running") {
    return {
      ...base,
      state: {
        status: "running",
        input,
        time: { start: Date.now() },
      },
    } as MessageV2.ToolPart
  }

  return {
    ...base,
    state: {
      status: "pending",
      input,
      raw: "",
    },
  } as MessageV2.ToolPart
}

describe("extractFileOps", () => {
  test("extracts read file paths from tool parts", () => {
    const messages = [
      createMessage("msg1", "assistant", [createToolPart("msg1", "read", "completed", { file_path: "src/index.ts" })]),
    ]

    const result = extractFileOps(messages)

    expect(result.read.has("src/index.ts")).toBe(true)
    expect(result.modified.size).toBe(0)
  })

  test("extracts write file paths as modified", () => {
    const messages = [
      createMessage("msg1", "assistant", [
        createToolPart("msg1", "write", "completed", { file_path: "src/new.ts" }),
      ]),
    ]

    const result = extractFileOps(messages)

    expect(result.modified.has("src/new.ts")).toBe(true)
    expect(result.read.size).toBe(0)
  })

  test("extracts edit file paths as modified", () => {
    const messages = [
      createMessage("msg1", "assistant", [
        createToolPart("msg1", "edit", "completed", { file_path: "src/old.ts" }),
      ]),
    ]

    const result = extractFileOps(messages)

    expect(result.modified.has("src/old.ts")).toBe(true)
    expect(result.read.size).toBe(0)
  })

  test("handles apply_patch tool as modified", () => {
    const messages = [
      createMessage("msg1", "assistant", [
        createToolPart("msg1", "apply_patch", "completed", { file_path: "src/patch.ts" }),
      ]),
    ]

    const result = extractFileOps(messages)

    expect(result.modified.has("src/patch.ts")).toBe(true)
    expect(result.read.size).toBe(0)
  })

  test("handles grep tool as read", () => {
    const messages = [
      createMessage("msg1", "assistant", [
        createToolPart("msg1", "grep", "completed", { file_path: "src/search.ts" }),
      ]),
    ]

    const result = extractFileOps(messages)

    expect(result.read.has("src/search.ts")).toBe(true)
    expect(result.modified.size).toBe(0)
  })

  test("handles glob tool as read", () => {
    const messages = [
      createMessage("msg1", "assistant", [
        createToolPart("msg1", "glob", "completed", { file_path: "src/**/*.ts" }),
      ]),
    ]

    const result = extractFileOps(messages)

    expect(result.read.has("src/**/*.ts")).toBe(true)
    expect(result.modified.size).toBe(0)
  })

  test("handles missing file_path gracefully", () => {
    const messages = [createMessage("msg1", "assistant", [createToolPart("msg1", "bash", "completed", { cmd: "ls" })])]

    const result = extractFileOps(messages)

    expect(result.read.size).toBe(0)
    expect(result.modified.size).toBe(0)
  })

  test("deduplicates paths", () => {
    const messages = [
      createMessage("msg1", "assistant", [
        createToolPart("msg1", "read", "completed", { file_path: "a.ts" }),
        createToolPart("msg1", "read", "completed", { file_path: "a.ts" }),
      ]),
    ]

    const result = extractFileOps(messages)

    expect(result.read.size).toBe(1)
    expect(result.read.has("a.ts")).toBe(true)
  })

  test("modified files take precedence over read files", () => {
    const messages = [
      createMessage("msg1", "assistant", [
        createToolPart("msg1", "read", "completed", { file_path: "src/file.ts" }),
        createToolPart("msg1", "write", "completed", { file_path: "src/file.ts" }),
      ]),
    ]

    const result = extractFileOps(messages)

    expect(result.modified.has("src/file.ts")).toBe(true)
    expect(result.read.has("src/file.ts")).toBe(false)
  })

  test("accepts filePath, file_path, and path variants", () => {
    const messages = [
      createMessage("msg1", "assistant", [
        createToolPart("msg1", "read", "completed", { filePath: "a.ts" }),
        createToolPart("msg1", "read", "completed", { file_path: "b.ts" }),
        createToolPart("msg1", "read", "completed", { path: "c.ts" }),
      ]),
    ]

    const result = extractFileOps(messages)

    expect(result.read.has("a.ts")).toBe(true)
    expect(result.read.has("b.ts")).toBe(true)
    expect(result.read.has("c.ts")).toBe(true)
  })

  test("ignores user messages", () => {
    const messages = [
      createMessage("msg1", "user", [createToolPart("msg1", "read", "completed", { file_path: "user.ts" })]),
    ]

    const result = extractFileOps(messages)

    expect(result.read.size).toBe(0)
    expect(result.modified.size).toBe(0)
  })

  test("ignores tool parts without input", () => {
    const messages = [
      createMessage("msg1", "assistant", [
        {
          id: PartID.ascending(),
          messageID: MessageID.make("msg1"),
          sessionID,
          type: "tool" as const,
          callID: crypto.randomUUID(),
          tool: "read",
          state: {
            status: "pending" as const,
            raw: "",
          },
        } as MessageV2.ToolPart,
      ]),
    ]

    const result = extractFileOps(messages)

    expect(result.read.size).toBe(0)
    expect(result.modified.size).toBe(0)
  })

  test("processes multiple messages", () => {
    const messages = [
      createMessage("msg1", "assistant", [createToolPart("msg1", "read", "completed", { file_path: "a.ts" })]),
      createMessage("msg2", "assistant", [createToolPart("msg2", "write", "completed", { file_path: "b.ts" })]),
    ]

    const result = extractFileOps(messages)

    expect(result.read.has("a.ts")).toBe(true)
    expect(result.modified.has("b.ts")).toBe(true)
  })
})

describe("formatFileOps", () => {
  test("formats modified and read files as XML", () => {
    const ops = {
      read: new Set(["a.ts", "b.ts"]),
      modified: new Set(["c.ts"]),
    }

    const result = formatFileOps(ops)

    expect(result).toContain("<modified-files>")
    expect(result).toContain("  c.ts")
    expect(result).toContain("</modified-files>")
    expect(result).toContain("<read-files>")
    expect(result).toContain("  a.ts")
    expect(result).toContain("  b.ts")
    expect(result).toContain("</read-files>")
  })

  test("sorts paths alphabetically", () => {
    const ops = {
      read: new Set(["z.ts", "a.ts", "m.ts"]),
      modified: new Set(["y.ts", "b.ts"]),
    }

    const result = formatFileOps(ops)

    const modifiedIndex = result.indexOf("b.ts")
    const modifiedYIndex = result.indexOf("y.ts")
    expect(modifiedIndex).toBeLessThan(modifiedYIndex)

    const aIndex = result.indexOf("a.ts")
    const mIndex = result.indexOf("m.ts")
    const zIndex = result.indexOf("z.ts")
    expect(aIndex).toBeLessThan(mIndex)
    expect(mIndex).toBeLessThan(zIndex)
  })

  test("returns empty string when no files", () => {
    const ops = {
      read: new Set<string>(),
      modified: new Set<string>(),
    }

    const result = formatFileOps(ops)

    expect(result).toBe("")
  })

  test("formats only modified when read is empty", () => {
    const ops = {
      read: new Set<string>(),
      modified: new Set(["modified.ts"]),
    }

    const result = formatFileOps(ops)

    expect(result).toContain("<modified-files>")
    expect(result).toContain("  modified.ts")
    expect(result).toContain("</modified-files>")
    expect(result).not.toContain("<read-files>")
  })

  test("formats only read when modified is empty", () => {
    const ops = {
      read: new Set(["read.ts"]),
      modified: new Set<string>(),
    }

    const result = formatFileOps(ops)

    expect(result).toContain("<read-files>")
    expect(result).toContain("  read.ts")
    expect(result).toContain("</read-files>")
    expect(result).not.toContain("<modified-files>")
  })

  test("starts with double newline when files present", () => {
    const ops = {
      read: new Set(["a.ts"]),
      modified: new Set<string>(),
    }

    const result = formatFileOps(ops)

    expect(result.startsWith("\n\n")).toBe(true)
  })
})

describe("parseFileOpsFromSummary", () => {
  test("parses XML blocks from summary text", () => {
    const summary = `Some summary

<modified-files>
  src/a.ts
  src/b.ts
</modified-files>

<read-files>
  src/c.ts
</read-files>`

    const result = parseFileOpsFromSummary(summary)

    expect(result.modified.has("src/a.ts")).toBe(true)
    expect(result.modified.has("src/b.ts")).toBe(true)
    expect(result.read.has("src/c.ts")).toBe(true)
  })

  test("returns empty sets for summary without XML", () => {
    const summary = "Just a plain summary"

    const result = parseFileOpsFromSummary(summary)

    expect(result.read.size).toBe(0)
    expect(result.modified.size).toBe(0)
  })

  test("handles whitespace and empty lines", () => {
    const summary = `<modified-files>

  src/a.ts

  src/b.ts

</modified-files>`

    const result = parseFileOpsFromSummary(summary)

    expect(result.modified.size).toBe(2)
    expect(result.modified.has("src/a.ts")).toBe(true)
    expect(result.modified.has("src/b.ts")).toBe(true)
  })

  test("handles case-insensitive tag matching", () => {
    const summary = `<MODIFIED-FILES>
  src/a.ts
</MODIFIED-FILES>

<READ-FILES>
  src/b.ts
</READ-FILES>`

    const result = parseFileOpsFromSummary(summary)

    expect(result.modified.has("src/a.ts")).toBe(true)
    expect(result.read.has("src/b.ts")).toBe(true)
  })

  test("parses only modified when read is absent", () => {
    const summary = `<modified-files>
  src/a.ts
</modified-files>`

    const result = parseFileOpsFromSummary(summary)

    expect(result.modified.has("src/a.ts")).toBe(true)
    expect(result.read.size).toBe(0)
  })

  test("parses only read when modified is absent", () => {
    const summary = `<read-files>
  src/a.ts
</read-files>`

    const result = parseFileOpsFromSummary(summary)

    expect(result.read.has("src/a.ts")).toBe(true)
    expect(result.modified.size).toBe(0)
  })
})

describe("maskObservations", () => {
  test("masks older tool outputs, preserves recent ones", () => {
    const messages = [
      createMessage("msg1", "assistant", [
        createToolPart("msg1", "bash", "completed", { cmd: "ls" }, "output1"),
        createToolPart("msg1", "read", "completed", { file_path: "a.ts" }, "output2"),
      ]),
      createMessage("msg2", "assistant", [
        createToolPart("msg2", "write", "completed", { file_path: "b.ts" }, "output3"),
        createToolPart("msg2", "grep", "completed", { pattern: "foo" }, "output4"),
      ]),
      createMessage("msg3", "assistant", [createToolPart("msg3", "edit", "completed", { file_path: "c.ts" }, "output5")]),
    ]

    maskObservations(messages, 2)

    const allParts = messages.flatMap((m) => m.parts).filter((p): p is MessageV2.ToolPart => p.type === "tool")

    expect(allParts[0].state.output).toContain("[output masked")
    expect(allParts[0].state.output).toContain("bash")
    expect(allParts[1].state.output).toContain("[output masked")
    expect(allParts[1].state.output).toContain("read")
    expect(allParts[2].state.output).toContain("[output masked")
    expect(allParts[2].state.output).toContain("write")

    expect(allParts[3].state.output).toBe("output4")
    expect(allParts[4].state.output).toBe("output5")
  })

  test("masked output includes tool name", () => {
    const messages = [createMessage("msg1", "assistant", [createToolPart("msg1", "bash", "completed", { cmd: "ls" }, "output")])]

    maskObservations(messages, 0)

    const part = messages[0].parts[0] as MessageV2.ToolPart
    expect(part.state.output).toContain("bash")
  })

  test("masked output includes file path when present", () => {
    const messages = [
      createMessage("msg1", "assistant", [
        createToolPart("msg1", "read", "completed", { file_path: "src/index.ts" }, "file contents"),
      ]),
    ]

    maskObservations(messages, 0)

    const part = messages[0].parts[0] as MessageV2.ToolPart
    expect(part.state.output).toContain("read")
    expect(part.state.output).toContain("src/index.ts")
  })

  test("skips parts without output", () => {
    const messages = [
      createMessage("msg1", "assistant", [createToolPart("msg1", "bash", "running", { cmd: "ls" })]),
      createMessage("msg2", "assistant", [createToolPart("msg2", "read", "completed", { file_path: "a.ts" }, "output")]),
    ]

    maskObservations(messages, 0)

    const part1 = messages[0].parts[0] as MessageV2.ToolPart
    expect(part1.state.status).toBe("running")

    const part2 = messages[1].parts[0] as MessageV2.ToolPart
    expect(part2.state.output).toContain("[output masked")
  })

  test("preserves all when preserveRecentCount exceeds total", () => {
    const messages = [
      createMessage("msg1", "assistant", [createToolPart("msg1", "bash", "completed", { cmd: "ls" }, "output1")]),
      createMessage("msg2", "assistant", [createToolPart("msg2", "read", "completed", { file_path: "a.ts" }, "output2")]),
    ]

    maskObservations(messages, 5)

    const part1 = messages[0].parts[0] as MessageV2.ToolPart
    const part2 = messages[1].parts[0] as MessageV2.ToolPart
    expect(part1.state.output).toBe("output1")
    expect(part2.state.output).toBe("output2")
  })

  test("masks all when preserveRecentCount is zero", () => {
    const messages = [
      createMessage("msg1", "assistant", [createToolPart("msg1", "bash", "completed", { cmd: "ls" }, "output1")]),
      createMessage("msg2", "assistant", [createToolPart("msg2", "read", "completed", { file_path: "a.ts" }, "output2")]),
    ]

    maskObservations(messages, 0)

    const part1 = messages[0].parts[0] as MessageV2.ToolPart
    const part2 = messages[1].parts[0] as MessageV2.ToolPart
    expect(part1.state.output).toContain("[output masked")
    expect(part2.state.output).toContain("[output masked")
  })
})

describe("serializeForSummary", () => {
  test("serializes user and assistant messages", () => {
    const messages = [
      createMessage("msg1", "user", [
        { id: PartID.ascending(), messageID: MessageID.make("msg1"), sessionID, type: "text", text: "hello" },
      ]),
      createMessage("msg2", "assistant", [
        { id: PartID.ascending(), messageID: MessageID.make("msg2"), sessionID, type: "text", text: "hi there" },
      ]),
    ]

    const result = serializeForSummary(messages)

    expect(result).toContain("<conversation>")
    expect(result).toContain("[User] hello")
    expect(result).toContain("[Assistant] hi there")
    expect(result).toContain("</conversation>")
  })

  test("serializes tool calls with input and output", () => {
    const messages = [
      createMessage("msg1", "assistant", [
        createToolPart("msg1", "bash", "completed", { command: "ls -la" }, "file1.txt\nfile2.txt"),
      ]),
    ]

    const result = serializeForSummary(messages)

    expect(result).toContain("[Tool: bash]")
    expect(result).toContain('"command":"ls -la"')
    expect(result).toContain("[Tool Result]")
    expect(result).toContain("file1.txt")
    expect(result).toContain("file2.txt")
  })

  test("wraps in conversation tags", () => {
    const messages = [
      createMessage("msg1", "user", [
        { id: PartID.ascending(), messageID: MessageID.make("msg1"), sessionID, type: "text", text: "test" },
      ]),
    ]

    const result = serializeForSummary(messages)

    expect(result.startsWith("<conversation>")).toBe(true)
    expect(result.endsWith("</conversation>")).toBe(true)
  })

  test("truncates long tool outputs to TOOL_OUTPUT_MAX_CHARS (2000)", () => {
    const longOutput = "x".repeat(5000)
    const messages = [createMessage("msg1", "assistant", [createToolPart("msg1", "bash", "completed", { cmd: "ls" }, longOutput)])]

    const result = serializeForSummary(messages)

    const toolResultMatch = result.match(/\[Tool Result\] (.+)/)
    expect(toolResultMatch).toBeTruthy()
    if (toolResultMatch) {
      expect(toolResultMatch[1].length).toBe(2000)
      expect(toolResultMatch[1]).toBe("x".repeat(2000))
    }
  })

  test("truncates long text parts to 2000 chars", () => {
    const longText = "y".repeat(3000)
    const messages = [
      createMessage("msg1", "user", [
        { id: PartID.ascending(), messageID: MessageID.make("msg1"), sessionID, type: "text", text: longText },
      ]),
    ]

    const result = serializeForSummary(messages)

    const userTextMatch = result.match(/\[User\] (.+)/)
    expect(userTextMatch).toBeTruthy()
    if (userTextMatch) {
      expect(userTextMatch[1].length).toBe(2000)
    }
  })

  test("truncates tool input to 500 chars", () => {
    const longInput = { data: "z".repeat(1000) }
    const messages = [createMessage("msg1", "assistant", [createToolPart("msg1", "bash", "completed", longInput, "ok")])]

    const result = serializeForSummary(messages)

    const toolCallMatch = result.match(/\[Tool: bash\] (.+)/)
    expect(toolCallMatch).toBeTruthy()
    if (toolCallMatch) {
      expect(toolCallMatch[1].length).toBe(500)
    }
  })

  test("includes reasoning parts with [Thinking] prefix", () => {
    const messages = [
      createMessage("msg1", "assistant", [
        {
          id: PartID.ascending(),
          messageID: MessageID.make("msg1"),
          sessionID,
          type: "reasoning",
          text: "I need to think about this",
          time: { start: Date.now() },
        },
      ]),
    ]

    const result = serializeForSummary(messages)

    expect(result).toContain("[Thinking] I need to think about this")
  })

  test("truncates reasoning to 1000 chars", () => {
    const longReasoning = "t".repeat(2000)
    const messages = [
      createMessage("msg1", "assistant", [
        {
          id: PartID.ascending(),
          messageID: MessageID.make("msg1"),
          sessionID,
          type: "reasoning",
          text: longReasoning,
          time: { start: Date.now() },
        },
      ]),
    ]

    const result = serializeForSummary(messages)

    const thinkingMatch = result.match(/\[Thinking\] (.+)/)
    expect(thinkingMatch).toBeTruthy()
    if (thinkingMatch) {
      expect(thinkingMatch[1].length).toBe(1000)
    }
  })

  test("omits tool output when not present", () => {
    const messages = [createMessage("msg1", "assistant", [createToolPart("msg1", "bash", "running", { cmd: "ls" })])]

    const result = serializeForSummary(messages)

    expect(result).toContain("[Tool: bash]")
    expect(result).not.toContain("[Tool Result]")
  })

  test("handles empty messages array", () => {
    const result = serializeForSummary([])

    expect(result).toBe("<conversation>\n</conversation>")
  })
})

describe("integration: file ops carry forward across chained summaries", () => {
  test("merges old and new file operations", () => {
    const previousSummary = `## Summary
Previous work

<modified-files>
  old/modified.ts
</modified-files>

<read-files>
  old/read.ts
</read-files>`

    const previousOps = parseFileOpsFromSummary(previousSummary)

    const newMessages = [
      createMessage("msg1", "assistant", [
        createToolPart("msg1", "read", "completed", { file_path: "new/read.ts" }),
        createToolPart("msg1", "write", "completed", { file_path: "new/modified.ts" }),
      ]),
    ]

    const newOps = extractFileOps(newMessages)

    const merged = {
      modified: new Set([...previousOps.modified, ...newOps.modified]),
      read: new Set([...previousOps.read, ...newOps.read]),
    }

    for (const f of merged.modified) {
      merged.read.delete(f)
    }

    expect(merged.modified.has("old/modified.ts")).toBe(true)
    expect(merged.modified.has("new/modified.ts")).toBe(true)
    expect(merged.read.has("old/read.ts")).toBe(true)
    expect(merged.read.has("new/read.ts")).toBe(true)
  })

  test("modified files from previous summary stay modified", () => {
    const previousSummary = `<modified-files>
  src/important.ts
</modified-files>`

    const previousOps = parseFileOpsFromSummary(previousSummary)

    const newMessages = [
      createMessage("msg1", "assistant", [createToolPart("msg1", "read", "completed", { file_path: "src/important.ts" })]),
    ]

    const newOps = extractFileOps(newMessages)

    const merged = {
      modified: new Set([...previousOps.modified, ...newOps.modified]),
      read: new Set([...previousOps.read, ...newOps.read]),
    }

    for (const f of merged.modified) {
      merged.read.delete(f)
    }

    expect(merged.modified.has("src/important.ts")).toBe(true)
    expect(merged.read.has("src/important.ts")).toBe(false)
  })

  test("round-trip: format and parse preserves file operations", () => {
    const original = {
      modified: new Set(["a.ts", "b.ts", "c.ts"]),
      read: new Set(["x.ts", "y.ts"]),
    }

    const formatted = formatFileOps(original)
    const parsed = parseFileOpsFromSummary(formatted)

    expect(parsed.modified).toEqual(original.modified)
    expect(parsed.read).toEqual(original.read)
  })
})
