export * as GPUMemory from "./gpu-memory"

import * as Log from "@/core/util/log"

const log = Log.create({ service: "gpu-memory" })

export type GPUMemoryResult = {
  totalBytes: number
  source: "macos-unified" | "nvidia-smi" | "amd-sysfs" | "fallback"
}

const FALLBACK_BYTES = 8 * 1024 * 1024 * 1024 // 8 GB
const SPAWN_TIMEOUT_MS = 2_000

let cached: GPUMemoryResult | undefined

async function runCommand(cmd: string[]): Promise<string | null> {
  try {
    const proc = Bun.spawn(cmd, {
      stdout: "pipe",
      stderr: "ignore",
    })
    const timer = setTimeout(() => proc.kill(), SPAWN_TIMEOUT_MS)
    const output = await new Response(proc.stdout).text()
    clearTimeout(timer)
    const exitCode = await proc.exited
    if (exitCode !== 0) return null
    return output.trim()
  } catch {
    return null
  }
}

async function detectMacOS(): Promise<GPUMemoryResult | null> {
  if (process.platform !== "darwin") return null
  const output = await runCommand(["sysctl", "-n", "hw.memsize"])
  if (!output) return null
  const bytes = parseInt(output, 10)
  if (isNaN(bytes) || bytes <= 0) return null
  return { totalBytes: bytes, source: "macos-unified" }
}

async function detectNvidiaSmi(): Promise<GPUMemoryResult | null> {
  if (process.platform !== "linux") return null
  const output = await runCommand(["nvidia-smi", "--query-gpu=memory.total", "--format=csv,noheader,nounits"])
  if (!output) return null
  // Use first line (GPU 0)
  const firstLine = output.split("\n")[0]?.trim()
  if (!firstLine) return null
  const mib = parseInt(firstLine, 10)
  if (isNaN(mib) || mib <= 0) return null
  return { totalBytes: mib * 1048576, source: "nvidia-smi" }
}

async function detectAmdSysfs(): Promise<GPUMemoryResult | null> {
  if (process.platform !== "linux") return null
  try {
    const file = Bun.file("/sys/class/drm/card0/device/mem_info_vram_total")
    const content = await file.text()
    const bytes = parseInt(content.trim(), 10)
    if (isNaN(bytes) || bytes <= 0) return null
    return { totalBytes: bytes, source: "amd-sysfs" }
  } catch {
    return null
  }
}

export async function detectGPUMemory(): Promise<GPUMemoryResult> {
  if (cached) return cached

  const result =
    (await detectMacOS()) ??
    (await detectNvidiaSmi()) ??
    (await detectAmdSysfs()) ??
    ({ totalBytes: FALLBACK_BYTES, source: "fallback" as const })

  log.debug("gpu memory detected", { totalBytes: result.totalBytes, source: result.source })
  cached = result
  return result
}

/** Maximum memory budget in bytes (32 GB) */
export const MAX_BUDGET_BYTES = 32 * 1024 * 1024 * 1024

/**
 * Compute the GPU memory budget for KV cache sizing.
 * Takes 50% of total GPU memory, capped at MAX_BUDGET_BYTES.
 */
export function gpuMemoryBudget(totalBytes: number): number {
  return Math.min(totalBytes * 0.5, MAX_BUDGET_BYTES)
}

/** Reset cached value (for testing) */
export function _resetCache(): void {
  cached = undefined
}
