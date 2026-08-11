/**
 * Tests that "guardrail" is a recognised permission key in the config schema.
 *
 * Uses ConfigParse.schema + Config.Info directly (pure synchronous decode, no
 * Effect layers needed) following the pattern established by the inline
 * `test("config parser preserves permission order…")` block in config.test.ts.
 */

import { test, describe, expect } from "bun:test"
import { Config } from "../../src/config/config"
import { ConfigParse } from "../../src/config/parse"

// ---------------------------------------------------------------------------
// Helper: parse a permission object via the real Config schema and return the
// decoded permission map so callers can assert on values.
// ---------------------------------------------------------------------------
function parsePermission(permissionInput: object): Config.Info["permission"] {
  const config = ConfigParse.schema(Config.Info, { permission: permissionInput }, "test:permission-guardrail")
  return config.permission
}

describe("permission schema – guardrail key", () => {
  test('accepts guardrail: "allow"', () => {
    const permission = parsePermission({ guardrail: "allow" })
    expect(permission?.guardrail).toBe("allow")
  })

  test('accepts guardrail: "ask"', () => {
    const permission = parsePermission({ guardrail: "ask" })
    expect(permission?.guardrail).toBe("ask")
  })

  test('accepts guardrail: "deny"', () => {
    const permission = parsePermission({ guardrail: "deny" })
    expect(permission?.guardrail).toBe("deny")
  })

  test("accepts guardrail alongside other known keys", () => {
    const permission = parsePermission({
      bash: "allow",
      guardrail: "ask",
      edit: "deny",
    })
    expect(permission?.bash).toBeDefined()
    expect(permission?.guardrail).toBe("ask")
  })

  test("silently ignores unknown top-level config keys for forward compatibility", () => {
    const result = ConfigParse.schema(Config.Info, { not_a_real_key: true }, "test:permission-guardrail")
    expect(result).toBeDefined()
    expect((result as any).not_a_real_key).toBeUndefined()
  })

  test("guardrail key does not appear when omitted from permission object", () => {
    const permission = parsePermission({ bash: "allow" })
    expect(permission?.guardrail).toBeUndefined()
  })

  test("guardrail as shorthand action string normalises to object form", () => {
    // The permission schema normalises a bare Action string into { "*": action }.
    // When guardrail is used as a top-level config shorthand (permission: "allow"),
    // the whole permission block becomes { "*": "allow" }. This test verifies
    // the guardrail key inside a permission object decodes correctly.
    const permission = parsePermission({ guardrail: "allow", "*": "deny" })
    expect(permission?.guardrail).toBe("allow")
    expect(permission?.["*"]).toBe("deny")
  })
})
