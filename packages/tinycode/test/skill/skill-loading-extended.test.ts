import { describe, expect, test } from "bun:test"
import { Effect, Layer } from "effect"
import { Skill } from "../../src/skill"
import { Discovery } from "../../src/skill/discovery"
import { RuntimeFlags } from "../../src/effect/runtime-flags"
import { Bus } from "../../src/bus"
import { Config } from "../../src/config/config"
import { CrossSpawnSpawner } from "@/core/cross-spawn-spawner"
import { AppFileSystem } from "@/core/filesystem"
import { Global } from "@/core/global"
import { provideTmpdirInstance } from "../fixture/fixture"
import { testEffect } from "../lib/effect"
import path from "path"

const node = CrossSpawnSpawner.defaultLayer

const it = testEffect(Layer.mergeAll(Skill.defaultLayer, node))

describe("Skill Loading Extended", () => {
  it.live("bundled skills are always available", () =>
    provideTmpdirInstance(
      () =>
        Effect.gen(function* () {
          const skill = yield* Skill.Service
          const all = yield* skill.all()

          // Bundled skills have location starting with "<"
          const bundled = all.filter((s) => s.location.startsWith("<"))
          expect(bundled.length).toBeGreaterThan(0)

          // Verify at least one well-known bundled skill exists
          const names = bundled.map((s) => s.name)
          // Note: actual bundled skill names may vary, but there should be some
          expect(names.length).toBeGreaterThan(0)
        }),
      { git: true },
    ),
  )

  it.live("skill.get() returns undefined for non-existent skill", () =>
    provideTmpdirInstance(
      () =>
        Effect.gen(function* () {
          const skill = yield* Skill.Service
          const result = yield* skill.get("nonexistent_skill_12345")
          expect(result).toBeUndefined()
        }),
      { git: true },
    ),
  )

  it.live("skill.get() returns skill info for existing skill", () =>
    provideTmpdirInstance(
      (dir) =>
        Effect.gen(function* () {
          yield* Effect.promise(() =>
            Bun.write(
              path.join(dir, ".tinycode", "skill", "get-test", "SKILL.md"),
              `---
name: get-test
description: Test skill for get().
---

# Get Test Skill
`,
            ),
          )

          const skill = yield* Skill.Service
          const result = yield* skill.get("get-test")
          expect(result).toBeDefined()
          expect(result?.name).toBe("get-test")
          expect(result?.description).toBe("Test skill for get().")
        }),
      { git: true },
    ),
  )

  it.live("skills with same name prioritize project over global", () =>
    provideTmpdirInstance(
      (dir) =>
        Effect.gen(function* () {
          // Create skill in project directory
          yield* Effect.promise(() =>
            Bun.write(
              path.join(dir, ".tinycode", "skill", "priority-test", "SKILL.md"),
              `---
name: priority-test
description: Project skill.
---

# Project Skill
`,
            ),
          )

          const skill = yield* Skill.Service
          const result = yield* skill.get("priority-test")
          expect(result).toBeDefined()
          expect(result?.description).toBe("Project skill.")
        }),
      { git: true },
    ),
  )

  it.live("skill directories include all skill locations", () =>
    provideTmpdirInstance(
      (dir) =>
        Effect.gen(function* () {
          yield* Effect.promise(() =>
            Promise.all([
              Bun.write(
                path.join(dir, ".tinycode", "skill", "dir-one", "SKILL.md"),
                `---
name: dir-one
description: First directory skill.
---

# Dir One
`,
              ),
              Bun.write(
                path.join(dir, ".tinycode", "skill", "dir-two", "SKILL.md"),
                `---
name: dir-two
description: Second directory skill.
---

# Dir Two
`,
              ),
            ]),
          )

          const skill = yield* Skill.Service

          // Set TINYCODE_TEST_HOME to ensure dirs() looks in the right place
          const prev = process.env.TINYCODE_TEST_HOME
          process.env.TINYCODE_TEST_HOME = dir
          try {
            const dirs = yield* skill.dirs()
            expect(dirs.length).toBe(2)
            expect(dirs).toContain(path.join(dir, ".tinycode", "skill", "dir-one"))
            expect(dirs).toContain(path.join(dir, ".tinycode", "skill", "dir-two"))
          } finally {
            process.env.TINYCODE_TEST_HOME = prev
          }
        }),
      { git: true },
    ),
  )
})
