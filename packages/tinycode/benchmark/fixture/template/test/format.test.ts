import { expect, test } from "bun:test"
import { formatUserSummary } from "../src/format"

test("formats non-empty user", () => {
  expect(formatUserSummary("alice", 10, 20)).toBe("ALICE: 30 points")
})

test("formats empty user", () => {
  expect(formatUserSummary("", 5, 5)).toBe(": 10 points")  // FAILS: TypeError on undefined.toUpperCase()
})
