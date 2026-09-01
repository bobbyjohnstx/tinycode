export * as OllamaProfile from "./ollama-profile"

import * as Log from "@/core/util/log"
import { detectGPUMemory, gpuMemoryBudget } from "./gpu-memory"

const log = Log.create({ service: "ollama-profile" })

// --- Quantization bytes-per-parameter lookup ---

const QUANT_BPP: Record<string, number> = {
  Q4_0: 0.5,
  Q4_K_S: 0.53,
  Q4_K_M: 0.55,
  Q5_0: 0.625,
  Q5_K_S: 0.63,
  Q5_K_M: 0.65,
  Q6_K: 0.75,
  Q8_0: 1.0,
  FP16: 2.0,
  F16: 2.0,
  BF16: 2.0,
}
const DEFAULT_BPP = 0.6

// --- Types ---

export type OllamaShowResult = {
  parameterSize: string
  quantizationLevel: string
  blockCount: number
  embeddingLength: number
  headCount: number
  headCountKV: number
  contextLength: number
}

export type AutoProfileConfig = {
  enabled?: boolean
  default_num_ctx?: number
  cleanup_on_exit?: boolean
  models?: Record<string, { num_ctx?: number; skip?: boolean }>
}

// --- Profile naming helpers ---

const PROFILE_SUFFIX_RE = /-tc\d+k$/

export function profileName(baseName: string, numCtx: number): string {
  return `${baseName}-tc${Math.round(numCtx / 1024)}k`
}

export function isProfile(modelName: string): boolean {
  return PROFILE_SUFFIX_RE.test(modelName)
}

export function baseModelName(name: string): string {
  return name.replace(PROFILE_SUFFIX_RE, "")
}

/**
 * Extract num_ctx from a profile name suffix (e.g., "qwen3.5:9b-tc18k" -> 18432).
 * Returns 0 if the name is not a profile.
 */
export function numCtxFromProfileName(name: string): number {
  const match = name.match(/-tc(\d+)k$/)
  if (!match) return 0
  return parseInt(match[1], 10) * 1024
}

// --- Localhost detection ---

const LOCAL_HOSTNAMES = new Set(["localhost", "127.0.0.1", "[::1]", "host.docker.internal", "host.containers.internal"])

export function isLocalOllama(baseURL: string): boolean {
  try {
    const url = new URL(baseURL)
    return LOCAL_HOSTNAMES.has(url.hostname)
  } catch {
    return false
  }
}

// --- num_ctx calculation ---

function parseParameterSize(paramSize: string): number {
  const match = paramSize.match(/([\d.]+)\s*[bB]/)
  if (!match) return 0
  return parseFloat(match[1]) * 1e9
}

function quantBytesPerParam(quantLevel: string): number {
  return QUANT_BPP[quantLevel] ?? DEFAULT_BPP
}

const MIN_KV_BUDGET = 100 * 1024 * 1024 // 100 MB floor
const MIN_NUM_CTX = 2048
const MAX_NUM_CTX = 131072

export function calculateNumCtx(
  gpuMemoryBytes: number,
  modelInfo: OllamaShowResult,
  advertisedContextLength: number,
): number {
  const budget = gpuMemoryBudget(gpuMemoryBytes)
  const modelWeightBytes = parseParameterSize(modelInfo.parameterSize) * quantBytesPerParam(modelInfo.quantizationLevel)
  const kvBudgetBytes = Math.max(budget - modelWeightBytes, MIN_KV_BUDGET)

  if (modelInfo.headCount <= 0 || modelInfo.blockCount <= 0 || modelInfo.embeddingLength <= 0) {
    return Math.min(8192, advertisedContextLength)
  }

  const headDim = modelInfo.embeddingLength / modelInfo.headCount
  const kvBytesPerToken = 2 * modelInfo.blockCount * headDim * modelInfo.headCountKV * 2
  let numCtx = Math.floor(kvBudgetBytes / kvBytesPerToken)
  // Round down to nearest 1024
  numCtx = Math.floor(numCtx / 1024) * 1024
  // Clamp
  numCtx = Math.max(MIN_NUM_CTX, Math.min(numCtx, advertisedContextLength, MAX_NUM_CTX))
  return numCtx
}

// --- Ollama API functions ---

const API_TIMEOUT_MS = 5_000
const CREATE_TIMEOUT_MS = 30_000

export async function showModel(baseURL: string, model: string): Promise<OllamaShowResult | null> {
  try {
    const controller = new AbortController()
    const timer = setTimeout(() => controller.abort(), API_TIMEOUT_MS)
    const res = await fetch(`${baseURL}/api/show`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name: model }),
      signal: controller.signal,
    })
    clearTimeout(timer)
    if (!res.ok) return null
    const body = (await res.json()) as {
      details?: { parameter_size?: string; quantization_level?: string }
      model_info?: Record<string, unknown>
    }
    if (!body.model_info) return null

    // Find architecture-prefixed keys (e.g., "qwen3.block_count")
    const info = body.model_info
    const blockCount = findModelInfoValue(info, "block_count")
    const embeddingLength = findModelInfoValue(info, "embedding_length")
    const headCount = findModelInfoValue(info, "head_count")
    const headCountKV = findModelInfoValue(info, "head_count_kv")
    const contextLength = findModelInfoValue(info, "context_length")

    return {
      parameterSize: body.details?.parameter_size ?? "",
      quantizationLevel: body.details?.quantization_level ?? "",
      blockCount,
      embeddingLength,
      headCount,
      headCountKV,
      contextLength,
    }
  } catch {
    return null
  }
}

function findModelInfoValue(info: Record<string, unknown>, suffix: string): number {
  for (const [key, value] of Object.entries(info)) {
    if (key.endsWith(`.${suffix}`) || key === suffix) {
      if (typeof value === "number") return value
    }
  }
  return 0
}

export async function createProfile(
  baseURL: string,
  opts: { baseName: string; profileName: string; numCtx: number },
): Promise<boolean> {
  try {
    const controller = new AbortController()
    const timer = setTimeout(() => controller.abort(), CREATE_TIMEOUT_MS)
    const res = await fetch(`${baseURL}/api/create`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        model: opts.profileName,
        from: opts.baseName,
        parameters: { num_ctx: opts.numCtx },
      }),
      signal: controller.signal,
    })
    clearTimeout(timer)
    if (!res.ok) return false
    // Read streaming response to completion — wait for {"status": "success"}
    const text = await res.text()
    return text.includes('"success"')
  } catch {
    return false
  }
}

export async function deleteProfile(baseURL: string, name: string): Promise<boolean> {
  try {
    const controller = new AbortController()
    const timer = setTimeout(() => controller.abort(), API_TIMEOUT_MS)
    const res = await fetch(`${baseURL}/api/delete`, {
      method: "DELETE",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name }),
      signal: controller.signal,
    })
    clearTimeout(timer)
    return res.ok
  } catch {
    return false
  }
}

// --- On-demand profile orchestration ---

export async function ensureOllamaProfile(
  baseURL: string,
  modelName: string,
  config?: AutoProfileConfig,
): Promise<string> {
  // Guard: disabled, remote, or already a profile
  if (config?.enabled === false) return modelName
  if (!isLocalOllama(baseURL)) return modelName
  if (isProfile(modelName)) return modelName

  // Guard: per-model skip
  const modelConfig = config?.models?.[modelName]
  if (modelConfig?.skip) return modelName

  // Determine num_ctx: per-model override > global override > calculate
  let numCtx: number
  if (modelConfig?.num_ctx) {
    numCtx = modelConfig.num_ctx
  } else if (config?.default_num_ctx) {
    numCtx = config.default_num_ctx
  } else {
    // Get GPU memory and model info
    const gpuMem = await detectGPUMemory()
    const modelInfo = await showModel(baseURL, modelName)
    if (!modelInfo) {
      log.info("could not fetch model info, skipping profiling", { model: modelName })
      return modelName
    }
    const advertisedContext = modelInfo.contextLength > 0 ? modelInfo.contextLength : 131072
    numCtx = calculateNumCtx(gpuMem.totalBytes, modelInfo, advertisedContext)
  }

  const name = profileName(modelName, numCtx)

  // Check if profile already exists
  const existing = await showModel(baseURL, name)
  if (existing) {
    log.debug("reusing existing profile", { profile: name })
    return name
  }

  // Create the profile
  const ok = await createProfile(baseURL, { baseName: modelName, profileName: name, numCtx })
  if (ok) {
    log.info("created ollama profile", { profile: name, numCtx })
  } else {
    log.warn("failed to create ollama profile, using base model", { profile: name, model: modelName })
    return modelName
  }

  return name
}

// --- Stale profile cleanup ---

export async function cleanupStaleProfiles(
  baseURL: string,
  currentBaseModels: Set<string>,
  profileEntries: string[],
): Promise<void> {
  for (const entry of profileEntries) {
    if (!isProfile(entry)) continue
    const base = baseModelName(entry)
    if (!currentBaseModels.has(base)) {
      const ok = await deleteProfile(baseURL, entry)
      if (ok) {
        log.info("deleted stale profile", { profile: entry, reason: "base model removed" })
      }
    }
  }
}
