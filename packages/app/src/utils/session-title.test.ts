import { describe, expect, test } from "bun:test"
import { sessionTitle } from "./session-title"

describe("sessionTitle", () => {
  test("strips ISO timestamp from 'New session' default title", () => {
    expect(sessionTitle("New session - 2024-09-15T14:30:00.000Z")).toBe("New session")
  })

  test("strips ISO timestamp from 'Child session' default title", () => {
    expect(sessionTitle("Child session - 2024-09-15T14:30:00.000Z")).toBe("Child session")
  })

  test("returns custom titles unchanged", () => {
    expect(sessionTitle("Fix the authentication bug")).toBe("Fix the authentication bug")
  })

  test("returns undefined for undefined input", () => {
    expect(sessionTitle(undefined)).toBeUndefined()
  })

  test("returns empty string unchanged", () => {
    expect(sessionTitle("")).toBe("")
  })

  test("does not strip partial timestamp formats", () => {
    expect(sessionTitle("New session - 2024-09-15")).toBe("New session - 2024-09-15")
  })
})
