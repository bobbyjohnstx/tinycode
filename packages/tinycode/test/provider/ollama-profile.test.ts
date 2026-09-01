import { describe, test, expect, afterEach, mock } from "bun:test"
import {
  profileName,
  isProfile,
  baseModelName,
  numCtxFromProfileName,
  isLocalOllama,
  calculateNumCtx,
  type OllamaShowResult,
} from "../../src/provider/ollama-profile"

describe("ollama-profile", () => {
  // --- Naming helpers ---

  describe("profileName", () => {
    test("generates correct profile name", () => {
      expect(profileName("qwen3.5:9b", 18432)).toBe("qwen3.5:9b-tc18k")
    })

    test("rounds to nearest k", () => {
      expect(profileName("llama3.1:8b", 16384)).toBe("llama3.1:8b-tc16k")
    })

    test("handles 2048 (2k)", () => {
      expect(profileName("tiny:1b", 2048)).toBe("tiny:1b-tc2k")
    })

    test("handles large context", () => {
      expect(profileName("big:70b", 131072)).toBe("big:70b-tc128k")
    })
  })

  describe("isProfile", () => {
    test("matches profile names", () => {
      expect(isProfile("qwen3.5:9b-tc18k")).toBe(true)
      expect(isProfile("llama3.1:8b-tc16k")).toBe(true)
      expect(isProfile("model:tag-tc2k")).toBe(true)
    })

    test("rejects non-profile names", () => {
      expect(isProfile("qwen3.5:9b")).toBe(false)
      expect(isProfile("llama3.1:8b")).toBe(false)
      expect(isProfile("tc18k")).toBe(false)
      expect(isProfile("model-tc")).toBe(false)
      expect(isProfile("model-tck")).toBe(false)
    })
  })

  describe("baseModelName", () => {
    test("strips profile suffix", () => {
      expect(baseModelName("qwen3.5:9b-tc18k")).toBe("qwen3.5:9b")
      expect(baseModelName("llama3.1:8b-tc16k")).toBe("llama3.1:8b")
    })

    test("returns original if not a profile", () => {
      expect(baseModelName("qwen3.5:9b")).toBe("qwen3.5:9b")
    })
  })

  describe("numCtxFromProfileName", () => {
    test("extracts num_ctx from profile name", () => {
      expect(numCtxFromProfileName("qwen3.5:9b-tc18k")).toBe(18432)
      expect(numCtxFromProfileName("llama3.1:8b-tc16k")).toBe(16384)
      expect(numCtxFromProfileName("tiny:1b-tc2k")).toBe(2048)
    })

    test("returns 0 for non-profile names", () => {
      expect(numCtxFromProfileName("qwen3.5:9b")).toBe(0)
    })
  })

  // --- Localhost detection ---

  describe("isLocalOllama", () => {
    test("detects localhost URLs", () => {
      expect(isLocalOllama("http://localhost:11434")).toBe(true)
      expect(isLocalOllama("http://127.0.0.1:11434")).toBe(true)
      expect(isLocalOllama("http://host.docker.internal:11434")).toBe(true)
      expect(isLocalOllama("http://host.containers.internal:11434")).toBe(true)
    })

    test("rejects remote URLs", () => {
      expect(isLocalOllama("http://192.168.1.100:11434")).toBe(false)
      expect(isLocalOllama("http://ollama.example.com:11434")).toBe(false)
      expect(isLocalOllama("https://my-ollama.cloud:11434")).toBe(false)
    })

    test("handles invalid URLs gracefully", () => {
      expect(isLocalOllama("not-a-url")).toBe(false)
      expect(isLocalOllama("")).toBe(false)
    })
  })

  // --- num_ctx calculation ---

  describe("calculateNumCtx", () => {
    // Worked example from plan: qwen3.5:9b on 16 GB Mac
    const qwen35: OllamaShowResult = {
      parameterSize: "9.2B",
      quantizationLevel: "Q4_K_M",
      blockCount: 36,
      embeddingLength: 3840,
      headCount: 24,
      headCountKV: 8,
      contextLength: 262144,
    }

    test("qwen3.5:9b on 16 GB produces 18k-19k context", () => {
      const gpuMem = 17_179_869_184 // 16 GB
      const numCtx = calculateNumCtx(gpuMem, qwen35, 262144)
      // Plan says 18432 or 19456 — should be 18432 (rounded down to 1024)
      expect(numCtx).toBeGreaterThanOrEqual(16384)
      expect(numCtx).toBeLessThanOrEqual(20480)
      expect(numCtx % 1024).toBe(0)
    })

    test("llama3.1:8b on 16 GB", () => {
      const llama31: OllamaShowResult = {
        parameterSize: "8.0B",
        quantizationLevel: "Q4_K_M",
        blockCount: 32,
        embeddingLength: 4096,
        headCount: 32,
        headCountKV: 8,
        contextLength: 131072,
      }
      const gpuMem = 17_179_869_184
      const numCtx = calculateNumCtx(gpuMem, llama31, 131072)
      expect(numCtx).toBeGreaterThanOrEqual(8192)
      expect(numCtx).toBeLessThanOrEqual(131072)
      expect(numCtx % 1024).toBe(0)
    })

    test("clamps to minimum 2048", () => {
      // Huge model with almost no KV budget
      const huge: OllamaShowResult = {
        parameterSize: "70B",
        quantizationLevel: "Q4_K_M",
        blockCount: 80,
        embeddingLength: 8192,
        headCount: 64,
        headCountKV: 8,
        contextLength: 131072,
      }
      const gpuMem = 8 * 1024 * 1024 * 1024 // 8 GB
      const numCtx = calculateNumCtx(gpuMem, huge, 131072)
      expect(numCtx).toBeGreaterThanOrEqual(2048)
    })

    test("clamps to advertised context length", () => {
      // Small model with lots of memory — should cap at advertised
      const tiny: OllamaShowResult = {
        parameterSize: "1B",
        quantizationLevel: "Q4_K_M",
        blockCount: 16,
        embeddingLength: 2048,
        headCount: 16,
        headCountKV: 4,
        contextLength: 8192,
      }
      const gpuMem = 128 * 1024 * 1024 * 1024 // 128 GB
      const numCtx = calculateNumCtx(gpuMem, tiny, 8192)
      expect(numCtx).toBeLessThanOrEqual(8192)
    })

    test("returns 8192 fallback when architecture info is missing", () => {
      const noArch: OllamaShowResult = {
        parameterSize: "9B",
        quantizationLevel: "Q4_K_M",
        blockCount: 0,
        embeddingLength: 0,
        headCount: 0,
        headCountKV: 0,
        contextLength: 0,
      }
      const numCtx = calculateNumCtx(16 * 1024 * 1024 * 1024, noArch, 131072)
      expect(numCtx).toBe(8192)
    })

    test("rounds down to nearest 1024", () => {
      const numCtx = calculateNumCtx(17_179_869_184, qwen35, 262144)
      expect(numCtx % 1024).toBe(0)
    })

    test("handles FP16 quantization", () => {
      const fp16: OllamaShowResult = {
        parameterSize: "1B",
        quantizationLevel: "FP16",
        blockCount: 16,
        embeddingLength: 2048,
        headCount: 16,
        headCountKV: 4,
        contextLength: 32768,
      }
      const numCtx = calculateNumCtx(16 * 1024 * 1024 * 1024, fp16, 32768)
      expect(numCtx).toBeGreaterThanOrEqual(2048)
      expect(numCtx).toBeLessThanOrEqual(32768)
      expect(numCtx % 1024).toBe(0)
    })
  })

  // --- API functions (mocked fetch) ---

  describe("showModel", () => {
    const originalFetch = globalThis.fetch

    afterEach(() => {
      globalThis.fetch = originalFetch
    })

    test("parses /api/show response correctly", async () => {
      globalThis.fetch = mock(() =>
        Promise.resolve(
          new Response(
            JSON.stringify({
              details: { parameter_size: "9.2B", quantization_level: "Q4_K_M" },
              model_info: {
                "qwen3.block_count": 36,
                "qwen3.embedding_length": 3840,
                "qwen3.head_count": 24,
                "qwen3.head_count_kv": 8,
              },
            }),
            { status: 200 },
          ),
        ),
      ) as unknown as typeof fetch

      const { showModel } = await import("../../src/provider/ollama-profile")
      const result = await showModel("http://localhost:11434", "qwen3.5:9b")
      expect(result).not.toBeNull()
      expect(result!.parameterSize).toBe("9.2B")
      expect(result!.quantizationLevel).toBe("Q4_K_M")
      expect(result!.blockCount).toBe(36)
      expect(result!.embeddingLength).toBe(3840)
      expect(result!.headCount).toBe(24)
      expect(result!.headCountKV).toBe(8)
    })

    test("returns null on HTTP error", async () => {
      globalThis.fetch = mock(() =>
        Promise.resolve(new Response("Not Found", { status: 404 })),
      ) as unknown as typeof fetch

      const { showModel } = await import("../../src/provider/ollama-profile")
      const result = await showModel("http://localhost:11434", "nonexistent:model")
      expect(result).toBeNull()
    })

    test("returns null on network error", async () => {
      globalThis.fetch = mock(() =>
        Promise.reject(new Error("ECONNREFUSED")),
      ) as unknown as typeof fetch

      const { showModel } = await import("../../src/provider/ollama-profile")
      const result = await showModel("http://localhost:11434", "model")
      expect(result).toBeNull()
    })
  })

  describe("createProfile", () => {
    const originalFetch = globalThis.fetch

    afterEach(() => {
      globalThis.fetch = originalFetch
    })

    test("sends correct request body and returns true on success", async () => {
      let capturedBody: string | undefined
      globalThis.fetch = mock((_input: RequestInfo | URL, init?: RequestInit) => {
        capturedBody = init?.body as string
        return Promise.resolve(
          new Response('{"status":"reading model metadata"}\n{"status":"success"}\n', { status: 200 }),
        )
      }) as unknown as typeof fetch

      const { createProfile } = await import("../../src/provider/ollama-profile")
      const result = await createProfile("http://localhost:11434", {
        baseName: "qwen3.5:9b",
        profileName: "qwen3.5:9b-tc18k",
        numCtx: 18432,
      })
      expect(result).toBe(true)
      const parsed = JSON.parse(capturedBody!)
      expect(parsed.model).toBe("qwen3.5:9b-tc18k")
      expect(parsed.from).toBe("qwen3.5:9b")
      expect(parsed.parameters.num_ctx).toBe(18432)
    })

    test("returns false on HTTP error", async () => {
      globalThis.fetch = mock(() =>
        Promise.resolve(new Response("Internal Server Error", { status: 500 })),
      ) as unknown as typeof fetch

      const { createProfile } = await import("../../src/provider/ollama-profile")
      const result = await createProfile("http://localhost:11434", {
        baseName: "model",
        profileName: "model-tc8k",
        numCtx: 8192,
      })
      expect(result).toBe(false)
    })
  })

  describe("deleteProfile", () => {
    const originalFetch = globalThis.fetch

    afterEach(() => {
      globalThis.fetch = originalFetch
    })

    test("returns true on successful deletion", async () => {
      globalThis.fetch = mock(() =>
        Promise.resolve(new Response("", { status: 200 })),
      ) as unknown as typeof fetch

      const { deleteProfile } = await import("../../src/provider/ollama-profile")
      const result = await deleteProfile("http://localhost:11434", "model-tc8k")
      expect(result).toBe(true)
    })

    test("returns false on error", async () => {
      globalThis.fetch = mock(() =>
        Promise.resolve(new Response("Not Found", { status: 404 })),
      ) as unknown as typeof fetch

      const { deleteProfile } = await import("../../src/provider/ollama-profile")
      const result = await deleteProfile("http://localhost:11434", "model-tc8k")
      expect(result).toBe(false)
    })
  })

  // --- ensureOllamaProfile ---

  describe("ensureOllamaProfile", () => {
    const originalFetch = globalThis.fetch

    afterEach(() => {
      globalThis.fetch = originalFetch
    })

    test("returns model name unchanged when disabled", async () => {
      const { ensureOllamaProfile } = await import("../../src/provider/ollama-profile")
      const result = await ensureOllamaProfile(
        "http://localhost:11434",
        "qwen3.5:9b",
        { enabled: false },
      )
      expect(result).toBe("qwen3.5:9b")
    })

    test("returns model name unchanged for remote URLs", async () => {
      const { ensureOllamaProfile } = await import("../../src/provider/ollama-profile")
      const result = await ensureOllamaProfile(
        "http://192.168.1.100:11434",
        "qwen3.5:9b",
      )
      expect(result).toBe("qwen3.5:9b")
    })

    test("returns model name unchanged when already a profile", async () => {
      const { ensureOllamaProfile } = await import("../../src/provider/ollama-profile")
      const result = await ensureOllamaProfile(
        "http://localhost:11434",
        "qwen3.5:9b-tc18k",
      )
      expect(result).toBe("qwen3.5:9b-tc18k")
    })

    test("returns model name unchanged when per-model skip is true", async () => {
      const { ensureOllamaProfile } = await import("../../src/provider/ollama-profile")
      const result = await ensureOllamaProfile(
        "http://localhost:11434",
        "qwen3.5:9b",
        { models: { "qwen3.5:9b": { skip: true } } },
      )
      expect(result).toBe("qwen3.5:9b")
    })

    test("uses per-model num_ctx override", async () => {
      let createCalled = false
      globalThis.fetch = mock((input: RequestInfo | URL) => {
        const url = input.toString()
        if (url.includes("/api/show")) {
          // First call checks if profile exists, return 404
          // Second call (if any) returns model info
          return Promise.resolve(new Response("Not Found", { status: 404 }))
        }
        if (url.includes("/api/create")) {
          createCalled = true
          return Promise.resolve(
            new Response('{"status":"success"}\n', { status: 200 }),
          )
        }
        return Promise.resolve(new Response("", { status: 404 }))
      }) as unknown as typeof fetch

      const { ensureOllamaProfile } = await import("../../src/provider/ollama-profile")
      const result = await ensureOllamaProfile(
        "http://localhost:11434",
        "qwen3.5:9b",
        { models: { "qwen3.5:9b": { num_ctx: 16384 } } },
      )
      // Should use the override num_ctx of 16384 -> profile name: qwen3.5:9b-tc16k
      expect(result).toBe("qwen3.5:9b-tc16k")
      expect(createCalled).toBe(true)
    })
  })

  // --- cleanupStaleProfiles ---

  describe("cleanupStaleProfiles", () => {
    const originalFetch = globalThis.fetch

    afterEach(() => {
      globalThis.fetch = originalFetch
    })

    test("deletes profiles whose base model is missing", async () => {
      const deleted: string[] = []
      globalThis.fetch = mock((_input: RequestInfo | URL, init?: RequestInit) => {
        if (init?.method === "DELETE") {
          const body = JSON.parse(init.body as string)
          deleted.push(body.name)
          return Promise.resolve(new Response("", { status: 200 }))
        }
        return Promise.resolve(new Response("", { status: 404 }))
      }) as unknown as typeof fetch

      const { cleanupStaleProfiles } = await import("../../src/provider/ollama-profile")
      const currentBases = new Set(["qwen3.5:9b"]) // llama3.1:8b was removed
      const profiles = ["qwen3.5:9b-tc18k", "llama3.1:8b-tc16k"]
      await cleanupStaleProfiles("http://localhost:11434", currentBases, profiles)

      expect(deleted).toEqual(["llama3.1:8b-tc16k"])
    })

    test("keeps profiles whose base model exists", async () => {
      const deleted: string[] = []
      globalThis.fetch = mock((_input: RequestInfo | URL, init?: RequestInit) => {
        if (init?.method === "DELETE") {
          const body = JSON.parse(init.body as string)
          deleted.push(body.name)
          return Promise.resolve(new Response("", { status: 200 }))
        }
        return Promise.resolve(new Response("", { status: 404 }))
      }) as unknown as typeof fetch

      const { cleanupStaleProfiles } = await import("../../src/provider/ollama-profile")
      const currentBases = new Set(["qwen3.5:9b", "llama3.1:8b"])
      const profiles = ["qwen3.5:9b-tc18k", "llama3.1:8b-tc16k"]
      await cleanupStaleProfiles("http://localhost:11434", currentBases, profiles)

      expect(deleted).toEqual([])
    })
  })
})
