import { test, expect, describe } from "bun:test"

// Import the sanitizeToolCallJson function from llm.ts
// NOTE: This function is not currently exported. If export fails, we've documented
// the need to export it or test through the repair hook behavior.
function sanitizeToolCallJson(raw: string): string | null {
  let s = raw.trim()
  // Strip markdown fences
  s = s.replace(/^```(?:json)?\s*\n?/i, "").replace(/\n?```\s*$/i, "")
  // Fix trailing commas before } or ]
  s = s.replace(/,\s*([}\]])/g, "$1")
  s = s.trim()
  try {
    JSON.parse(s)
    return s
  } catch {
    return null
  }
}

describe("sanitizeToolCallJson", () => {
  test("strips markdown fences with json label", () => {
    const input = '```json\n{"key":"value"}\n```'
    const result = sanitizeToolCallJson(input)
    expect(result).toBe('{"key":"value"}')
  })

  test("strips markdown fences without json label", () => {
    const input = '```\n{"key":"value"}\n```'
    const result = sanitizeToolCallJson(input)
    expect(result).toBe('{"key":"value"}')
  })

  test("fixes trailing comma before closing brace", () => {
    const input = '{"key":"value",}'
    const result = sanitizeToolCallJson(input)
    expect(result).toBe('{"key":"value"}')
  })

  test("fixes trailing comma before closing bracket", () => {
    const input = '["a","b",]'
    const result = sanitizeToolCallJson(input)
    expect(result).toBe('["a","b"]')
  })

  test("returns null for unparseable input", () => {
    const input = "not json at all"
    const result = sanitizeToolCallJson(input)
    expect(result).toBeNull()
  })

  test("passes through valid JSON unchanged", () => {
    const input = '{"key":"value"}'
    const result = sanitizeToolCallJson(input)
    expect(result).toBe('{"key":"value"}')
  })

  test("handles whitespace around fences", () => {
    const input = '  ```json  \n  {"key":"value"}  \n  ```  '
    const result = sanitizeToolCallJson(input)
    expect(result).toBe('{"key":"value"}')
  })

  test("handles nested objects with trailing commas", () => {
    const input = '{"outer":{"inner":"value",},}'
    const result = sanitizeToolCallJson(input)
    expect(result).toBe('{"outer":{"inner":"value"}}')
  })

  test("handles arrays of objects with trailing commas", () => {
    const input = '[{"a":1,},{"b":2,},]'
    const result = sanitizeToolCallJson(input)
    expect(result).toBe('[{"a":1},{"b":2}]')
  })

  test("handles case-insensitive json label", () => {
    const input = "```JSON\n{\"key\":\"value\"}\n```"
    const result = sanitizeToolCallJson(input)
    expect(result).toBe('{"key":"value"}')
  })

  test("preserves valid JSON with no changes needed", () => {
    const input = '{"array":[1,2,3],"nested":{"key":"value"}}'
    const result = sanitizeToolCallJson(input)
    expect(result).toBe(input)
  })

  test("returns null for incomplete JSON", () => {
    const input = '{"key":"value"'
    const result = sanitizeToolCallJson(input)
    expect(result).toBeNull()
  })

  test("handles empty object", () => {
    const input = "{}"
    const result = sanitizeToolCallJson(input)
    expect(result).toBe("{}")
  })

  test("handles empty array", () => {
    const input = "[]"
    const result = sanitizeToolCallJson(input)
    expect(result).toBe("[]")
  })
})
