import { describe, expect, test } from "bun:test"
import { formatDuration } from "@/util/format"

describe("formatDuration", () => {
  test("returns empty string for zero or negative values", () => {
    expect(formatDuration(0)).toBe("")
    expect(formatDuration(-5)).toBe("")
  })

  test("formats seconds", () => {
    expect(formatDuration(1)).toBe("1s")
    expect(formatDuration(30)).toBe("30s")
    expect(formatDuration(59)).toBe("59s")
  })

  test("formats minutes and seconds", () => {
    expect(formatDuration(60)).toBe("1m")
    expect(formatDuration(90)).toBe("1m 30s")
    expect(formatDuration(120)).toBe("2m")
    expect(formatDuration(150)).toBe("2m 30s")
    expect(formatDuration(3599)).toBe("59m 59s")
  })

  test("formats hours and minutes", () => {
    expect(formatDuration(3600)).toBe("1h")
    expect(formatDuration(3660)).toBe("1h 1m")
    expect(formatDuration(7200)).toBe("2h")
    expect(formatDuration(7320)).toBe("2h 2m")
    expect(formatDuration(86399)).toBe("23h 59m")
  })

  test("formats days", () => {
    expect(formatDuration(86400)).toBe("~1 day")
    expect(formatDuration(172800)).toBe("~2 days")
    expect(formatDuration(259200)).toBe("~3 days")
    expect(formatDuration(604799)).toBe("~6 days")
  })

  test("formats weeks", () => {
    expect(formatDuration(604800)).toBe("~1 week")
    expect(formatDuration(1209600)).toBe("~2 weeks")
    expect(formatDuration(1814400)).toBe("~3 weeks")
  })
})
