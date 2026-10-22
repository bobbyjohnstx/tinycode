import { describe, expect, test } from "bun:test"
import {
  isShellPermission,
  permissionAllowAlwaysScope,
  permissionDockTitle,
  permissionToolKey,
} from "./permission-tool-label"

describe("permissionToolKey", () => {
  test("aliases shell family to bash", () => {
    expect(permissionToolKey("shell")).toBe("bash")
    expect(permissionToolKey("destructive-shell")).toBe("bash")
    expect(permissionToolKey("secret-shell")).toBe("bash")
  })

  test("passes through other tool ids", () => {
    expect(permissionToolKey("edit")).toBe("edit")
  })
})

describe("permissionDockTitle", () => {
  test("uses localized tool title and pattern snippet", () => {
    expect(
      permissionDockTitle({
        permission: "shell",
        patterns: ["rm -rf /tmp/demo"],
        toolTitle: (key) => (key === "bash" ? "Bash" : ""),
        fallback: "Permission required",
      }),
    ).toBe("Bash: rm -rf /tmp/demo")
  })

  test("falls back when no i18n title", () => {
    expect(
      permissionDockTitle({
        permission: "custom-tool",
        patterns: [],
        toolTitle: () => "settings.permissions.tool.custom-tool.title",
        fallback: "Permission required",
      }),
    ).toBe("custom-tool")
  })
})

describe("permissionAllowAlwaysScope", () => {
  test("prefers always over patterns", () => {
    expect(permissionAllowAlwaysScope(["/tmp/a"], ["/tmp/b"])).toBe("/tmp/a")
  })

  test("falls back to patterns", () => {
    expect(permissionAllowAlwaysScope([], ["echo hi", "ls"])).toBe("echo hi, ls")
  })

  test("returns undefined when empty", () => {
    expect(permissionAllowAlwaysScope([], [])).toBeUndefined()
    expect(permissionAllowAlwaysScope(undefined, undefined)).toBeUndefined()
  })
})

describe("isShellPermission", () => {
  test("matches shell and bash only", () => {
    expect(isShellPermission("shell")).toBe(true)
    expect(isShellPermission("bash")).toBe(true)
    expect(isShellPermission("destructive-shell")).toBe(false)
  })
})
