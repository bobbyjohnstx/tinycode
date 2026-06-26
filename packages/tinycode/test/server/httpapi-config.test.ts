import { afterEach, describe, expect } from "bun:test"
import path from "path"
import { Server } from "../../src/server/server"
import * as Log from "@/core/util/log"
import { Effect, Fiber } from "effect"
import { resetDatabase } from "../fixture/db"
import { disposeAllInstances, tmpdir } from "../fixture/fixture"
import { it } from "../lib/effect"
import { waitGlobalBusEvent } from "./global-bus"

void Log.init({ print: false })

function app() {
  return Server.Default().app
}

function waitDisposed(directory: string) {
  return waitGlobalBusEvent({
    message: "timed out waiting for instance disposal",
    predicate: (event) => event.payload.type === "server.instance.disposed" && event.directory === directory,
  })
}

const tmpdirEffect = (options: Parameters<typeof tmpdir>[0]) =>
  Effect.acquireRelease(
    Effect.promise(() => tmpdir(options)),
    (tmp) => Effect.promise(() => tmp[Symbol.asyncDispose]()),
  )

afterEach(async () => {
  await disposeAllInstances()
  await resetDatabase()
})

describe("config HttpApi", () => {
  it.live(
    "serves config update through the default server app",
    Effect.gen(function* () {
      const tmp = yield* tmpdirEffect({ config: { formatter: false, lsp: false } })
      const disposed = yield* waitDisposed(tmp.path).pipe(Effect.forkScoped)

      const response = yield* Effect.promise(() =>
        Promise.resolve(
          app().request("/config", {
            method: "PATCH",
            headers: {
              "content-type": "application/json",
              "x-tinycode-directory": tmp.path,
            },
            body: JSON.stringify({ username: "patched-user", formatter: false, lsp: false }),
          }),
        ),
      )

      expect(response.status).toBe(200)
      expect(yield* Effect.promise(() => response.json())).toMatchObject({
        username: "patched-user",
        formatter: false,
        lsp: false,
      })
      yield* Fiber.join(disposed)
      expect(yield* Effect.promise(() => Bun.file(path.join(tmp.path, "config.json")).json())).toMatchObject({
        username: "patched-user",
        formatter: false,
        lsp: false,
      })
    }),
  )

  it.live(
    "serves config with active provider model status",
    Effect.gen(function* () {
      const tmp = yield* tmpdirEffect({
        config: {
          formatter: false,
          lsp: false,
          provider: {
            omniroute: {
              models: {
                "gpt-4o": {
                  status: "active",
                },
              },
            },
          },
        },
      })

      const response = yield* Effect.promise(() =>
        Promise.resolve(
          app().request("/config", {
            headers: {
              "x-tinycode-directory": tmp.path,
            },
          }),
        ),
      )

      expect(response.status).toBe(200)
      expect(yield* Effect.promise(() => response.json())).toMatchObject({
        provider: {
          omniroute: {
            models: {
              "gpt-4o": {
                status: "active",
              },
            },
          },
        },
      })
    }),
  )

  it.live(
    "redacts secrets from config response",
    Effect.gen(function* () {
      const tmp = yield* tmpdirEffect({
        config: {
          formatter: false,
          lsp: false,
          provider: {
            test: {
              options: {
                apiKey: "sk-secret-key-12345",
                baseURL: "https://api.test.com",
                enterpriseUrl: "https://enterprise.test.com",
              },
              models: {
                "test-model": {
                  headers: {
                    "X-Model-Token": "model-secret",
                    "Authorization": "Bearer model-auth",
                    "Content-Type": "application/json",
                  },
                },
              },
            },
          },
          mcp: {
            testMcp: {
              type: "remote",
              url: "https://mcp.test.com",
              oauth: {
                clientSecret: "oauth-secret-123",
              },
              headers: {
                "X-Auth-Header": "mcp-secret",
                "Content-Type": "application/json",
              },
            },
            localMcp: {
              type: "local",
              command: ["node", "mcp-server.js"],
              environment: {
                "API_KEY": "env-secret",
                "NORMAL_VAR": "normal-value",
              },
            },
          },
        },
      })

      const response = yield* Effect.promise(() =>
        Promise.resolve(
          app().request("/config", {
            headers: {
              "x-tinycode-directory": tmp.path,
            },
          }),
        ),
      )

      expect(response.status).toBe(200)
      const config = yield* Effect.promise(() => response.json())

      // Provider secrets should be redacted
      expect(config.provider.test.options.apiKey).toBe("***")
      expect(config.provider.test.options.enterpriseUrl).toBe("***")
      expect(config.provider.test.options.baseURL).toBe("https://api.test.com")
      expect(config.provider.test.models["test-model"].headers["X-Model-Token"]).toBe("***")
      expect(config.provider.test.models["test-model"].headers["Authorization"]).toBe("***")
      expect(config.provider.test.models["test-model"].headers["Content-Type"]).toBe("application/json")

      // MCP secrets should be redacted
      expect(config.mcp.testMcp.oauth.clientSecret).toBe("***")
      expect(config.mcp.testMcp.headers["X-Auth-Header"]).toBe("***")
      expect(config.mcp.testMcp.headers["Content-Type"]).toBe("application/json")
      expect(config.mcp.localMcp.environment.API_KEY).toBe("***")
      expect(config.mcp.localMcp.environment.NORMAL_VAR).toBe("normal-value")
    }),
  )
})
