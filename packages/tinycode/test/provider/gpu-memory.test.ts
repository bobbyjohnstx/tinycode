import { describe, test, expect, beforeEach } from "bun:test"
import { _resetCache, gpuMemoryBudget, MAX_BUDGET_BYTES } from "../../src/provider/gpu-memory"

describe("gpu-memory", () => {
  beforeEach(() => {
    _resetCache()
  })

  describe("detectGPUMemory", () => {
    test("returns a result with totalBytes > 0 and a valid source", async () => {
      const { detectGPUMemory } = await import("../../src/provider/gpu-memory")
      _resetCache()
      const result = await detectGPUMemory()
      expect(result.totalBytes).toBeGreaterThan(0)
      expect(["macos-unified", "nvidia-smi", "amd-sysfs", "fallback"]).toContain(result.source)
    })

    test("caches the result after first call", async () => {
      const { detectGPUMemory } = await import("../../src/provider/gpu-memory")
      _resetCache()
      const first = await detectGPUMemory()
      const second = await detectGPUMemory()
      expect(first).toStrictEqual(second)
    })

    test("on macOS returns macos-unified source", async () => {
      if (process.platform !== "darwin") return
      const { detectGPUMemory } = await import("../../src/provider/gpu-memory")
      _resetCache()
      const result = await detectGPUMemory()
      expect(result.source).toBe("macos-unified")
      // Mac should have at least 4 GB
      expect(result.totalBytes).toBeGreaterThan(4 * 1024 * 1024 * 1024)
    })
  })

  describe("gpuMemoryBudget", () => {
    test("returns 50% of total for small memory", () => {
      const total = 16 * 1024 * 1024 * 1024 // 16 GB
      expect(gpuMemoryBudget(total)).toBe(total * 0.5)
    })

    test("caps at MAX_BUDGET_BYTES for large memory", () => {
      const total = 128 * 1024 * 1024 * 1024 // 128 GB
      expect(gpuMemoryBudget(total)).toBe(MAX_BUDGET_BYTES)
    })

    test("returns exactly MAX_BUDGET_BYTES at 64 GB", () => {
      const total = 64 * 1024 * 1024 * 1024 // 64 GB
      // 50% of 64 GB = 32 GB = MAX_BUDGET_BYTES
      expect(gpuMemoryBudget(total)).toBe(MAX_BUDGET_BYTES)
    })

    test("returns 50% for 8 GB fallback", () => {
      const total = 8 * 1024 * 1024 * 1024
      expect(gpuMemoryBudget(total)).toBe(4 * 1024 * 1024 * 1024)
    })
  })
})
