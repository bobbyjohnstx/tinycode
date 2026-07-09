import { describe, expect, test } from "bun:test"
import { Effect, Layer } from "effect"
import type { Agent } from "../../src/agent/agent"
import { NamedError } from "@/core/util/error"
import { Skill } from "../../src/skill"
import { Permission } from "../../src/permission"
import { SystemPrompt, modelSizeB } from "../../src/session/system"
import { testEffect } from "../lib/effect"
import type { Provider } from "@/provider/provider"

const skills: Skill.Info[] = [
  {
    name: "zeta-skill",
    description: "Zeta skill.",
    location: "/tmp/zeta-skill/SKILL.md",
    content: "# zeta-skill",
  },
  {
    name: "alpha-skill",
    description: "Alpha skill.",
    location: "/tmp/alpha-skill/SKILL.md",
    content: "# alpha-skill",
  },
  {
    name: "middle-skill",
    description: "Middle skill.",
    location: "/tmp/middle-skill/SKILL.md",
    content: "# middle-skill",
  },
  {
    name: "manual-skill",
    location: "/tmp/manual-skill/SKILL.md",
    content: "# manual-skill",
  },
]

const build: Agent.Info = {
  name: "build",
  mode: "primary",
  permission: Permission.fromConfig({ "*": "allow" }),
  options: {},
}

const it = testEffect(
  SystemPrompt.layer.pipe(
    Layer.provide(
      Layer.succeed(
        Skill.Service,
        Skill.Service.of({
          get: (name) => Effect.succeed(skills.find((skill) => skill.name === name)),
          require: (name) => {
            const info = skills.find((skill) => skill.name === name)
            if (info) return Effect.succeed(info)
            return Effect.fail(new Skill.NotFoundError({ name, available: skills.map((skill) => skill.name) }))
          },
          all: () => Effect.succeed(skills),
          dirs: () => Effect.succeed([]),
          available: () => Effect.succeed(skills),
        }),
      ),
    ),
  ),
)

describe("session.system", () => {
  it.effect("skills output is sorted by name and stable across calls", () =>
    Effect.gen(function* () {
      const prompt = yield* SystemPrompt.Service
      const first = yield* prompt.skills(build)
      const second = yield* prompt.skills(build)
      const output = first ?? (yield* Effect.fail(new NamedError.Unknown({ message: "missing skills output" })))

      expect(first).toBe(second)

      const alpha = output.indexOf("<name>alpha-skill</name>")
      const middle = output.indexOf("<name>middle-skill</name>")
      const zeta = output.indexOf("<name>zeta-skill</name>")

      expect(alpha).toBeGreaterThan(-1)
      expect(middle).toBeGreaterThan(alpha)
      expect(zeta).toBeGreaterThan(middle)
      expect(output).not.toContain("manual-skill")
    }),
  )
})

describe("modelSizeB", () => {
  test("config-declared size takes priority over name parsing", () => {
    const model = {
      size: 7,
      api: { id: "qwen3-14b" },
    } as Provider.Model
    expect(modelSizeB(model)).toBe(7)
  })

  test("falls back to name parsing: qwen3-14b", () => {
    const model = {
      api: { id: "qwen3-14b" },
    } as Provider.Model
    expect(modelSizeB(model)).toBe(14)
  })

  test("falls back to name parsing: llama3.1:8b", () => {
    const model = {
      api: { id: "llama3.1:8b" },
    } as Provider.Model
    expect(modelSizeB(model)).toBe(8)
  })

  test("unknown name with no config size returns undefined", () => {
    const model = {
      api: { id: "unknown-model-name" },
    } as Provider.Model
    expect(modelSizeB(model)).toBeUndefined()
  })

  test("config size of 0 is valid", () => {
    const model = {
      size: 0,
      api: { id: "tiny-model" },
    } as Provider.Model
    expect(modelSizeB(model)).toBe(0)
  })

  test("parses size with hyphen separator: model-7b", () => {
    const model = {
      api: { id: "model-7b" },
    } as Provider.Model
    expect(modelSizeB(model)).toBe(7)
  })

  test("parses size with underscore separator: model_13b", () => {
    const model = {
      api: { id: "model_13b" },
    } as Provider.Model
    expect(modelSizeB(model)).toBe(13)
  })

  test("parses size with v prefix: modelv3-70b", () => {
    const model = {
      api: { id: "modelv3-70b" },
    } as Provider.Model
    expect(modelSizeB(model)).toBe(70)
  })

  test("parses size at start: 8b-model", () => {
    const model = {
      api: { id: "8b-model" },
    } as Provider.Model
    expect(modelSizeB(model)).toBe(8)
  })

  test("ignores non-b suffix: model-7m", () => {
    const model = {
      api: { id: "model-7m" },
    } as Provider.Model
    expect(modelSizeB(model)).toBeUndefined()
  })

  test("case insensitive: MODEL-14B", () => {
    const model = {
      api: { id: "MODEL-14B" },
    } as Provider.Model
    expect(modelSizeB(model)).toBe(14)
  })

  test("handles large sizes: llama-405b", () => {
    const model = {
      api: { id: "llama-405b" },
    } as Provider.Model
    expect(modelSizeB(model)).toBe(405)
  })

  test("config size overrides even when name contains different size", () => {
    const model = {
      size: 30,
      api: { id: "model-7b" },
    } as Provider.Model
    expect(modelSizeB(model)).toBe(30)
  })
})
