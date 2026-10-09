import { describe, expect, test } from "bun:test"
import { dict } from "@/i18n/en"
import { formatterView, visibleFormatters } from "./formatter-status"

describe("visibleFormatters", () => {
  test("hides the list until the status request has settled", () => {
    expect(visibleFormatters(false, [])).toBeUndefined()
    expect(visibleFormatters(false, undefined)).toBeUndefined()
  })

  test("returns an empty list when formatting is off", () => {
    expect(visibleFormatters(true, [])).toEqual([])
    expect(visibleFormatters(true, undefined)).toEqual([])
  })

  test("keeps a disabled formatter in the list", () => {
    const row = { name: "gofmt", extensions: [".go"], enabled: false }
    expect(visibleFormatters(true, [row])).toEqual([row])
  })
})

describe("formatterView", () => {
  test("stays loading until the request settles", () => {
    expect(formatterView(false, [])).toBe("loading")
    expect(formatterView(false, [{ name: "gofmt", extensions: [".go"], enabled: true }])).toBe("loading")
  })

  test("is empty only after a settled empty response", () => {
    expect(formatterView(true, [])).toBe("empty")
  })

  test("lists settled formatters, including a disabled one", () => {
    expect(formatterView(true, [{ name: "gofmt", extensions: [".go"], enabled: true }])).toBe("list")
    expect(formatterView(true, [{ name: "gofmt", extensions: [".go"], enabled: false }])).toBe("list")
  })
})

describe("formatter status copy", () => {
  test("empty state names the config switch", () => {
    expect(dict["dialog.formatter.empty"]).toContain(`"formatter": true`)
  })

  test("disabled rows have a label", () => {
    expect(dict["dialog.formatter.off"]).toBe("off")
  })
})
