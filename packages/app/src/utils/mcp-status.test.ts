import { describe, expect, test } from "bun:test"
import { isMcpFailure, isMcpWarning, mcpIssueLevel, mcpStatusLabelKey } from "./mcp-status"

describe("mcp-status", () => {
  test("treats Go error as failure", () => {
    expect(isMcpFailure("error")).toBe(true)
    expect(isMcpFailure("failed")).toBe(true)
    expect(isMcpFailure("connected")).toBe(false)
  })

  test("maps transitional and disconnected as warning", () => {
    expect(isMcpWarning("connecting")).toBe(true)
    expect(isMcpWarning("reconnecting")).toBe(true)
    expect(isMcpWarning("disconnected")).toBe(true)
    expect(isMcpWarning("error")).toBe(false)
  })

  test("mcpIssueLevel prefers critical over warning", () => {
    expect(mcpIssueLevel([{ status: "error" }, { status: "needs_auth" }])).toBe("critical")
    expect(mcpIssueLevel([{ status: "reconnecting" }])).toBe("warning")
    expect(mcpIssueLevel([{ status: "connected" }])).toBeUndefined()
  })

  test("label keys cover Go statuses", () => {
    expect(mcpStatusLabelKey("error")).toBe("mcp.status.error")
    expect(mcpStatusLabelKey("reconnecting")).toBe("mcp.status.reconnecting")
    expect(mcpStatusLabelKey("disconnected")).toBe("mcp.status.disconnected")
    expect(mcpStatusLabelKey("connecting")).toBe("mcp.status.connecting")
  })
})
