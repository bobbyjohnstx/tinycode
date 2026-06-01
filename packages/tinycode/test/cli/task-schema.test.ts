/**
 * Tests for the Task tool schema fix:
 * description is now optional so local LLMs that omit it don't crash.
 */

import { describe, expect, test } from "bun:test"
import { Schema } from "effect"
import { Parameters } from "@/tool/task"

const decode = Schema.decodeUnknownSync(Parameters)

describe("Task tool Parameters schema", () => {
  test("accepts full params including description", () => {
    const result = decode({
      description: "Fix the bug",
      prompt: "Find and fix the auth bug in src/auth.ts",
      subagent_type: "executor",
    })
    expect(result.description).toBe("Fix the bug")
    expect(result.prompt).toBe("Find and fix the auth bug in src/auth.ts")
    expect(result.subagent_type).toBe("executor")
  })

  test("accepts params without description — does not throw SchemaError", () => {
    // This was the failing case: local LLMs often omit description
    expect(() =>
      decode({
        prompt: "What time is it?",
        subagent_type: "explore",
      }),
    ).not.toThrow()
  })

  test("description is undefined when omitted", () => {
    const result = decode({
      prompt: "What time is it?",
      subagent_type: "explore",
    })
    expect(result.description).toBeUndefined()
  })

  test("accepts background flag", () => {
    const result = decode({
      prompt: "Run analysis",
      subagent_type: "general-purpose",
      background: true,
    })
    expect(result.background).toBe(true)
  })

  test("rejects missing required prompt field", () => {
    expect(() =>
      decode({
        description: "Do something",
        subagent_type: "executor",
        // prompt is missing
      }),
    ).toThrow()
  })

  test("rejects missing required subagent_type field", () => {
    expect(() =>
      decode({
        prompt: "Do something",
        // subagent_type is missing
      }),
    ).toThrow()
  })
})
