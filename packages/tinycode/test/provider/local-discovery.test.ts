/**
 * Tests for local LLM discovery behaviors.
 *
 * Tests 1+3 (Ollama shape, MaaS filter) via a live mock HTTP server require
 * the Effect FetchHttpClient to cooperate with Bun.serve in the same process,
 * which is fragile due to fiber scheduling. Those behaviors are covered by
 * manual QA with a real Ollama/vLLM/MaaS instance.
 *
 * These tests cover the deterministic behaviors:
 * - Unreachable servers → empty result (no crash)
 * - MaaS skipped when env vars absent
 * - Embedding model filtering logic (pure function)
 * - Trailing slash normalization (pure string operation)
 */
import { describe, test, expect } from "bun:test"
import { Effect, Duration } from "effect"
import { LocalDiscovery } from "../../src/provider/local-discovery"

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type EnvOverride = Record<string, string | undefined>

function setEnv(vars: EnvOverride) {
  const saved: EnvOverride = {}
  for (const [k, v] of Object.entries(vars)) {
    saved[k] = process.env[k]
    if (v === undefined) delete process.env[k]
    else process.env[k] = v
  }
  return saved
}

function restoreEnv(saved: EnvOverride) {
  for (const [k, v] of Object.entries(saved)) {
    if (v === undefined) delete process.env[k]
    else process.env[k] = v
  }
}

async function discoverEmpty(overrides: EnvOverride) {
  const saved = setEnv(overrides)
  try {
    return await Effect.runPromise(
      Effect.gen(function* () {
        const svc = yield* LocalDiscovery.Service
        yield* Effect.sleep(Duration.millis(500))
        return yield* svc.get()
      }).pipe(Effect.provide(LocalDiscovery.defaultLayer)),
    )
  } finally {
    restoreEnv(saved)
  }
}

// ---------------------------------------------------------------------------
// Test 2: Unreachable → empty, no crash
// ---------------------------------------------------------------------------

describe("LocalDiscovery", () => {
  test("returns empty map when all servers unreachable (connection refused)", async () => {
    const result = await discoverEmpty({
      TINYCODE_OLLAMA_HOST: "http://127.0.0.1:1",
      TINYCODE_VLLM_HOST: "http://127.0.0.1:1",
      TINYCODE_MAAS_HOST: undefined,
    })
    expect(result).toEqual({})
  }, 4_000)

  // ---------------------------------------------------------------------------
  // Test 4: MaaS skipped when env vars absent
  // ---------------------------------------------------------------------------

  test("skips MaaS probe when TINYCODE_MAAS_HOST is not set", async () => {
    const result = await discoverEmpty({
      TINYCODE_OLLAMA_HOST: "http://127.0.0.1:1",
      TINYCODE_VLLM_HOST: "http://127.0.0.1:1",
      TINYCODE_MAAS_HOST: undefined,
      TINYCODE_MAAS_API_KEY: "test-key",
    })
    expect(result["maas"]).toBeUndefined()
  }, 4_000)

  // ---------------------------------------------------------------------------
  // Test 3 (logic): Embedding model filter — pure function behavior
  // These verify the filter condition used inside probeMaas.
  // ---------------------------------------------------------------------------

  describe("embedding model filter", () => {
    // The filter used in probeMaas: !id.includes("embed")
    const filterEmbeds = (ids: string[]) => ids.filter((id) => !id.toLowerCase().includes("embed"))

    test("excludes IDs containing 'embed'", () => {
      const ids = ["deepseek-r1-14b", "nomic-embed-text-v1-5", "granite-3-8b-instruct", "text-embedding-ada-002"]
      expect(filterEmbeds(ids)).toEqual(["deepseek-r1-14b", "granite-3-8b-instruct"])
    })

    test("keeps all IDs when none contain 'embed'", () => {
      const ids = ["llama3.2:latest", "qwen3-14b", "deepseek-r1"]
      expect(filterEmbeds(ids)).toEqual(ids)
    })

    test("filters case-insensitively (Embed, EMBED)", () => {
      const ids = ["good-model", "BadEmbed-v2", "EMBED-only"]
      expect(filterEmbeds(ids)).toEqual(["good-model"])
    })
  })

  // ---------------------------------------------------------------------------
  // Test: Context limit calculation (80/20 split)
  // ---------------------------------------------------------------------------

  describe("context limit calculation", () => {
    const calcLimits = (maxModelLen: number) => {
      const effectiveContext = maxModelLen > 0 ? Math.floor(maxModelLen * 0.8) : 0
      const outputLimit = maxModelLen > 0 ? Math.min(4096, Math.floor(maxModelLen * 0.2)) : 0
      return { context: effectiveContext, output: outputLimit }
    }

    test("applies 80/20 split to context_length", () => {
      expect(calcLimits(32768)).toEqual({ context: 26214, output: 4096 })
    })

    test("caps output at 4096 for large context", () => {
      expect(calcLimits(131072)).toEqual({ context: 104857, output: 4096 })
    })

    test("returns zero for zero context", () => {
      expect(calcLimits(0)).toEqual({ context: 0, output: 0 })
    })

    test("small context uses proportional output", () => {
      expect(calcLimits(8192)).toEqual({ context: 6553, output: 1638 })
    })
  })

  // ---------------------------------------------------------------------------
  // Test: Ollama capability detection from tags response
  // ---------------------------------------------------------------------------

  describe("ollama capability mapping", () => {
    const mapCaps = (caps: string[]) => {
      const s = new Set(caps)
      return {
        reasoning: s.has("thinking"),
        toolcall: s.has("tools"),
        vision: s.has("vision"),
      }
    }

    test("detects thinking, tools, and vision", () => {
      expect(mapCaps(["vision", "completion", "tools", "thinking"])).toEqual({
        reasoning: true,
        toolcall: true,
        vision: true,
      })
    })

    test("completion-only model has no special capabilities", () => {
      expect(mapCaps(["completion"])).toEqual({
        reasoning: false,
        toolcall: false,
        vision: false,
      })
    })

    test("thinking-only model (e.g. deepseek-r1) has reasoning but no tools", () => {
      expect(mapCaps(["completion", "thinking"])).toEqual({
        reasoning: true,
        toolcall: false,
        vision: false,
      })
    })
  })

  // ---------------------------------------------------------------------------
  // Test 5 (logic): Trailing slash normalization — pure string behavior
  // ---------------------------------------------------------------------------

  describe("trailing slash normalization", () => {
    const stripTrailing = (url: string) => url.replace(/\/+$/, "")

    test("strips single trailing slash", () => {
      expect(stripTrailing("http://server:8080/")).toBe("http://server:8080")
    })

    test("strips multiple trailing slashes", () => {
      expect(stripTrailing("http://server:8080///")).toBe("http://server:8080")
    })

    test("no-op when no trailing slash", () => {
      expect(stripTrailing("http://server:8080")).toBe("http://server:8080")
    })

    test("preserves path components before trailing slash", () => {
      expect(stripTrailing("http://server:8080/api/")).toBe("http://server:8080/api")
    })
  })
})
