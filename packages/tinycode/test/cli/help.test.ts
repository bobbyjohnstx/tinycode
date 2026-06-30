/**
 * Smoke tests for the TUI help.md content.
 *
 * These are pure filesystem/string checks — no Effect, no network, no spawned
 * processes. They verify the file exists and that critical sections and agent/
 * skill names are documented.
 */

import { describe, it, expect } from "bun:test"
import fsNode from "fs"
import path from "path"

const HELP_MD_PATH = path.resolve(import.meta.dirname, "../../src/cli/cmd/tui/help.md")

let helpContent: string

// Read once before all tests
try {
  helpContent = fsNode.readFileSync(HELP_MD_PATH, "utf8")
} catch {
  helpContent = ""
}

describe("help.md – file existence and basic integrity", () => {
  it("help.md file exists at the expected path", () => {
    const exists = fsNode.existsSync(HELP_MD_PATH)
    expect(exists).toBe(true)
  })

  it("help.md is non-empty", () => {
    expect(helpContent.length).toBeGreaterThan(0)
  })

  it("help.md is readable as UTF-8 text", () => {
    expect(typeof helpContent).toBe("string")
  })
})

describe("help.md – required sections", () => {
  it('contains an "Agents" section', () => {
    expect(helpContent.toLowerCase()).toContain("agents")
  })

  it('contains a "Skills" section', () => {
    expect(helpContent.toLowerCase()).toContain("skills")
  })

  it('contains a "Keybindings" or "Key" section', () => {
    const lower = helpContent.toLowerCase()
    const hasKeybindings = lower.includes("keybinding") || lower.includes("key binding") || lower.includes("key |")
    expect(hasKeybindings).toBe(true)
  })

  it('contains a "Permissions" or "Guardrails" section', () => {
    const lower = helpContent.toLowerCase()
    const hasPermissions = lower.includes("permission") || lower.includes("guardrail")
    expect(hasPermissions).toBe(true)
  })
})

describe("help.md – critical agents are documented", () => {
  it("mentions explore agent", () => {
    expect(helpContent).toContain("| explore")
  })

  it("mentions architect agent", () => {
    expect(helpContent).toContain("| architect")
  })

  it("mentions executor agent", () => {
    expect(helpContent).toContain("| executor")
  })
})

describe("help.md – critical skills are documented", () => {
  it("mentions /plan skill", () => {
    expect(helpContent).toContain("/plan")
  })
})

describe("help.md – provider / configuration content", () => {
  it("mentions config.json", () => {
    expect(helpContent).toContain("config.json")
  })

  it("mentions lsp configuration option", () => {
    expect(helpContent.toLowerCase()).toContain("lsp")
  })

  it("mentions permission configuration option", () => {
    expect(helpContent.toLowerCase()).toContain("permission")
  })
})
