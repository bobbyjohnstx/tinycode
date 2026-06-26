import { describe, expect } from "bun:test"
import { Effect, Layer } from "effect"
import { Agent } from "@/agent/agent"
import { Provider } from "@/provider/provider"
import { Auth } from "@/auth"
import { Config } from "@/config/config"
import { RuntimeFlags } from "@/effect/runtime-flags"
import { Plugin } from "@/plugin"
import { Skill } from "@/skill"
import { ProviderTest } from "../fake/provider"
import { testEffect } from "../lib/effect"

describe("Agent with Mock LLM", () => {
  // Create a mock provider that returns a tool call
  const mockToolCall = ProviderTest.fake({
    scenarios: [
      {
        type: "tool-call",
        calls: [{ id: "call-1", name: "read", args: { file_path: "/test.txt" } }],
      },
    ],
  })

  const toolCallLayer = Agent.layer.pipe(
    Layer.provide(Plugin.defaultLayer),
    Layer.provide(mockToolCall.layer),
    Layer.provide(Auth.defaultLayer),
    Layer.provide(Config.defaultLayer),
    Layer.provide(Skill.defaultLayer),
    Layer.provide(RuntimeFlags.defaultLayer),
  )

  const it1 = testEffect(toolCallLayer)

  it1.instance("agent can load with mock provider configured for tool calls", () =>
    Effect.gen(function* () {
      const agentService = yield* Agent.Service

      // Verify agent exists
      const agent = yield* agentService.get("build")
      expect(agent).toBeDefined()
      expect(agent?.name).toBe("build")
    }),
  )

  // Create a mock provider that returns text
  const mockText = ProviderTest.fake({
    scenarios: [{ type: "text", content: "Hello from mock" }],
  })

  const textLayer = Agent.layer.pipe(
    Layer.provide(Plugin.defaultLayer),
    Layer.provide(mockText.layer),
    Layer.provide(Auth.defaultLayer),
    Layer.provide(Config.defaultLayer),
    Layer.provide(Skill.defaultLayer),
    Layer.provide(RuntimeFlags.defaultLayer),
  )

  const it2 = testEffect(textLayer)

  it2.instance("agent can load with mock provider configured for text", () =>
    Effect.gen(function* () {
      const agentService = yield* Agent.Service

      const agent = yield* agentService.get("plan")
      expect(agent).toBeDefined()
      expect(agent?.name).toBe("plan")
    }),
  )

  // Create a mock provider that throws errors
  const mockError = ProviderTest.fake({
    scenarios: [{ type: "error", error: new Error("Mock LLM error") }],
  })

  const errorLayer = Agent.layer.pipe(
    Layer.provide(Plugin.defaultLayer),
    Layer.provide(mockError.layer),
    Layer.provide(Auth.defaultLayer),
    Layer.provide(Config.defaultLayer),
    Layer.provide(Skill.defaultLayer),
    Layer.provide(RuntimeFlags.defaultLayer),
  )

  const it3 = testEffect(errorLayer)

  it3.instance("agent can load with mock provider configured for errors", () =>
    Effect.gen(function* () {
      const agentService = yield* Agent.Service

      const agent = yield* agentService.get("general")
      expect(agent).toBeDefined()
      expect(agent?.name).toBe("general")
    }),
  )
})
