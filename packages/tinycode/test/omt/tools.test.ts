import { describe, test, expect } from "bun:test"
import { tmpdir } from "../fixture/fixture"
import { createTools } from "../../src/omt/tools"

const EXPECTED_TOOL_KEYS = [
  "omt_state_read",
  "omt_state_write",
  "omt_state_clear",
  "omt_state_list_active",
  "omt_state_get_status",
  "omt_notepad_read",
  "omt_notepad_write_priority",
  "omt_notepad_write_working",
  "omt_notepad_write_manual",
  "omt_notepad_prune",
  "omt_notepad_stats",
  "omt_project_memory_read",
  "omt_project_memory_write",
  "omt_project_memory_add_note",
  "omt_project_memory_add_directive",
  "omt_wiki_list",
  "omt_wiki_read",
  "omt_wiki_query",
  "omt_wiki_add",
  "omt_wiki_ingest",
  "omt_wiki_delete",
  "omt_ast_grep_search",
  "omt_ast_grep_replace",
] as const

// ============================================================================
// Shape tests
// ============================================================================

describe("createTools shape", () => {
  test("returns object with all 23 expected tool keys", async () => {
    await using tmp = await tmpdir()
    const tools = createTools(tmp.path)
    for (const key of EXPECTED_TOOL_KEYS) {
      expect(Object.prototype.hasOwnProperty.call(tools, key)).toBe(true)
    }
    expect(Object.keys(tools)).toHaveLength(EXPECTED_TOOL_KEYS.length)
  })

  test("each tool has a description string", async () => {
    await using tmp = await tmpdir()
    const tools = createTools(tmp.path)
    for (const [key, tool] of Object.entries(tools)) {
      expect(typeof tool.description).toBe("string")
      expect(tool.description.length).toBeGreaterThan(0)
      // @ts-ignore — key is for error reporting only
      void key
    }
  })

  test("each tool has an args object", async () => {
    await using tmp = await tmpdir()
    const tools = createTools(tmp.path)
    for (const [key, tool] of Object.entries(tools)) {
      expect(typeof tool.args).toBe("object")
      expect(tool.args).not.toBeNull()
      // @ts-ignore
      void key
    }
  })
})

// ============================================================================
// Smoke tests
// ============================================================================

describe("omt_state_read smoke test", () => {
  test("returns 'No state found' message when state dir is empty", async () => {
    await using tmp = await tmpdir()
    const tools = createTools(tmp.path)
    const result = await tools.omt_state_read.execute({ mode: "autopilot" }, {} as any)
    expect(typeof result).toBe("string")
    expect(result).toContain("No state found")
  })
})

describe("omt_notepad_read smoke test", () => {
  test("returns 'does not exist' message when no notepad file exists", async () => {
    await using tmp = await tmpdir()
    const tools = createTools(tmp.path)
    const result = await tools.omt_notepad_read.execute({}, {} as any)
    expect(typeof result).toBe("string")
    expect(result).toContain("does not exist")
  })
})

describe("omt_state_write + omt_state_read round-trip", () => {
  test("written state can be read back", async () => {
    await using tmp = await tmpdir()
    const tools = createTools(tmp.path)

    await tools.omt_state_write.execute(
      {
        mode: "autopilot",
        active: true,
        iteration: 5,
        current_phase: "execution",
      },
      {} as any,
    )

    const readResult = await tools.omt_state_read.execute({ mode: "autopilot" }, {} as any)
    expect(typeof readResult).toBe("string")
    expect(readResult).toContain("autopilot")
    expect(readResult).toContain('"active": true')
    expect(readResult).toContain('"iteration": 5')
    expect(readResult).toContain('"current_phase": "execution"')
  })
})
