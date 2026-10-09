import { describe, expect, test } from "bun:test"
import type { ProviderAuthMethod } from "@tinycode/sdk/v2/client"
import { usableAuthMethods } from "./dialog-connect-provider-auth"

const api: ProviderAuthMethod = { type: "api", label: "API key" }
const oauth: ProviderAuthMethod = { type: "oauth", label: "OAuth" }
const fallback: ProviderAuthMethod[] = [api]

describe("usableAuthMethods", () => {
  test("drops oauth and keeps api", () => {
    expect(usableAuthMethods([oauth, api], fallback)).toEqual([api])
  })

  test("uses the api-key fallback when every method is oauth", () => {
    expect(usableAuthMethods([oauth], fallback)).toEqual(fallback)
  })

  test("keeps an empty list when the server returned no methods", () => {
    expect(usableAuthMethods([], fallback)).toEqual([])
  })

  test("uses the fallback when methods are missing", () => {
    expect(usableAuthMethods(undefined, fallback)).toEqual(fallback)
  })
})
