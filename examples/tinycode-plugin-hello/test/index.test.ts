import { describe, it, expect } from "bun:test"
import { createTestHarness, createMockToolContext } from "tinycode-plugin/test"
import plugin from "../src/index"

describe("tinycode-plugin-hello", () => {
  it("hello_world tool returns greeting", async () => {
    const { hooks } = await createTestHarness(plugin)
    const toolDef = hooks.tool!["hello_world"]
    const result = await toolDef.execute(
      { name: "World" },
      createMockToolContext(),
    )
    expect(result).toContain("Hello, World!")
  })

  it("hello_world uses custom greeting from config", async () => {
    const { hooks } = await createTestHarness(plugin, {
      pluginOptions: { greeting: "Howdy" },
    })
    const toolDef = hooks.tool!["hello_world"]
    const result = await toolDef.execute(
      { name: "Partner" },
      createMockToolContext(),
    )
    expect(result).toContain("Howdy, Partner!")
  })

  it("hello_time tool returns time", async () => {
    const { hooks } = await createTestHarness(plugin)
    const toolDef = hooks.tool!["hello_time"]
    const result = await toolDef.execute({}, createMockToolContext())
    expect(result).toContain("UTC")
  })

  it("session.start hook fires without error", async () => {
    const { invoke } = await createTestHarness(plugin)
    await invoke("session.start", { sessionID: "test", agent: "build" }, {})
  })
})
