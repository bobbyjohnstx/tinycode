import { test, expect, describe } from "bun:test"

// Test the ECONNREFUSED error classification logic
// This tests the logic from src/session/message-v2.ts around line 1131-1147

type SystemError = Error & {
  code?: string
  syscall?: string
}

// Recreate the error classification logic from message-v2.ts
function classifyError(e: unknown): { friendly: boolean; message: string } {
  if ((e as SystemError)?.code === "ECONNREFUSED") {
    return {
      friendly: true,
      message:
        "Connection refused — the model provider is not reachable. Check that Ollama or vLLM is running.",
    }
  }
  return {
    friendly: false,
    message: e instanceof Error ? e.message : String(e),
  }
}

describe("ECONNREFUSED error handling", () => {
  test("error with ECONNREFUSED code returns friendly message", () => {
    const error: SystemError = new Error("connect ECONNREFUSED 127.0.0.1:11434")
    error.code = "ECONNREFUSED"
    error.syscall = "connect"

    const result = classifyError(error)
    expect(result.friendly).toBe(true)
    expect(result.message).toContain("Connection refused")
    expect(result.message).toContain("Ollama")
    expect(result.message).toContain("vLLM")
  })

  test("error with ECONNREFUSED in code field only", () => {
    const error = new Error("Some network error") as SystemError
    error.code = "ECONNREFUSED"

    const result = classifyError(error)
    expect(result.friendly).toBe(true)
    expect(result.message).toContain("model provider is not reachable")
  })

  test("non-connection error passes through unchanged", () => {
    const error = new Error("Some other error")

    const result = classifyError(error)
    expect(result.friendly).toBe(false)
    expect(result.message).toBe("Some other error")
  })

  test("ENOTFOUND error is not classified as ECONNREFUSED", () => {
    const error = new Error("getaddrinfo ENOTFOUND") as SystemError
    error.code = "ENOTFOUND"

    const result = classifyError(error)
    expect(result.friendly).toBe(false)
    expect(result.message).toBe("getaddrinfo ENOTFOUND")
  })

  test("ETIMEDOUT error is not classified as ECONNREFUSED", () => {
    const error = new Error("connect ETIMEDOUT") as SystemError
    error.code = "ETIMEDOUT"

    const result = classifyError(error)
    expect(result.friendly).toBe(false)
    expect(result.message).toBe("connect ETIMEDOUT")
  })

  test("generic network error without code field", () => {
    const error = new Error("fetch failed")

    const result = classifyError(error)
    expect(result.friendly).toBe(false)
    expect(result.message).toBe("fetch failed")
  })

  test("non-Error object is stringified", () => {
    const result = classifyError("string error")
    expect(result.friendly).toBe(false)
    expect(result.message).toBe("string error")
  })

  test("null error is stringified", () => {
    const result = classifyError(null)
    expect(result.friendly).toBe(false)
    expect(result.message).toBe("null")
  })

  test("undefined error is stringified", () => {
    const result = classifyError(undefined)
    expect(result.friendly).toBe(false)
    expect(result.message).toBe("undefined")
  })
})
