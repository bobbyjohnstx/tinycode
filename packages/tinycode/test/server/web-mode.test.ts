import { describe, expect } from "bun:test"
import { Context, Effect, Layer } from "effect"
import { HttpApiApp } from "../../src/server/routes/instance/httpapi/server"
import { resetDatabase } from "../fixture/db"
import { testEffect } from "../lib/effect"

// Container / web-mode smoke tests (closes Gitea issue #4).
//
// Verifies the three things a container deployment needs:
//   1. /global/health is reachable and returns healthy status
//   2. Sessions can be created via HTTP
//   3. The tool catalogue is non-empty

const testStateLayer = Layer.effectDiscard(
  Effect.gen(function* () {
    yield* Effect.promise(() => resetDatabase())
    yield* Effect.addFinalizer(() => Effect.promise(() => resetDatabase()))
  }),
)

const it = testEffect(testStateLayer)
const handlerContext = Context.empty() as Context.Context<unknown>
const handler = () => HttpApiApp.webHandler().handler

const req = (path: string, init?: RequestInit) =>
  Effect.promise(() =>
    handler()(
      new Request(`http://localhost${path}`, {
        ...init,
        headers: { "x-tinycode-directory": process.cwd(), "content-type": "application/json", ...init?.headers },
      }),
      handlerContext,
    ),
  )

describe("web mode integration", () => {
  it.live("GET /global/health returns 200 with healthy:true", () =>
    Effect.gen(function* () {
      const response = yield* Effect.promise(() =>
        handler()(new Request("http://localhost/global/health"), handlerContext),
      )
      expect(response.status).toBe(200)
      const body = yield* Effect.promise(() => response.json())
      expect(body).toMatchObject({ healthy: true, version: expect.any(String) })
    }),
  )

  it.live("POST /session creates a session", () =>
    Effect.gen(function* () {
      const response = yield* req("/session", {
        method: "POST",
        body: JSON.stringify({ title: "web-mode smoke" }),
      })
      expect(response.status).toBe(200)
      const body = yield* Effect.promise(() => response.json())
      expect(body).toMatchObject({ id: expect.any(String) })
    }),
  )

  it.live("GET /experimental/tool/ids returns a non-empty tool list", () =>
    Effect.gen(function* () {
      const response = yield* req("/experimental/tool/ids")
      expect(response.status).toBe(200)
      const body = yield* Effect.promise(() => response.json())
      expect(Array.isArray(body)).toBe(true)
      expect((body as string[]).length).toBeGreaterThan(0)
    }),
  )
})
