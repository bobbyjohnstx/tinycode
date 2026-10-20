import { describe, expect, test } from "bun:test"
import { permissionDockTitle, permissionToolKey } from "./permission-tool-label"

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
