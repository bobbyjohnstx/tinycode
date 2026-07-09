import * as fs from "fs/promises"
import os from "os"
import path from "path"
import { $ } from "bun"
import type { RunResult } from "../tasks/types"
import type { ModelScore } from "../scoring/score"

export interface BenchmarkReport {
  timestamp: string
  date: string
  hardware: {
    cpus: string
    cores: number
    memory: string
  }
  ollama: {
    version: string
  }
  models: ModelScore[]
  runs: RunResult[]
}

export async function generateJsonReport(
  models: ModelScore[],
  runs: RunResult[]
): Promise<string> {
  const timestamp = new Date().toISOString()
  const date = new Date().toLocaleDateString("en-US", {
    year: "numeric",
    month: "long",
    day: "numeric",
  })

  // Get hardware info
  const cpuInfo = os.cpus()
  const hardware = {
    cpus: cpuInfo[0]?.model || "Unknown",
    cores: cpuInfo.length,
    memory: `${Math.round(os.totalmem() / (1024 * 1024 * 1024))} GB`,
  }

  // Get Ollama version
  let ollamaVersion = "unknown"
  try {
    const result = await $`ollama --version`.nothrow().quiet()
    if (result.exitCode === 0) {
      ollamaVersion = result.text().trim()
    }
  } catch {
    ollamaVersion = "error: ollama not found"
  }

  const report: BenchmarkReport = {
    timestamp,
    date,
    hardware,
    ollama: { version: ollamaVersion },
    models,
    runs,
  }

  // Write to results directory
  const resultsDir = path.join(import.meta.dir, "../../results")
  await fs.mkdir(resultsDir, { recursive: true })

  const filename = `benchmark-${new Date().toISOString().replace(/:/g, "-").split(".")[0]}.json`
  const filepath = path.join(resultsDir, filename)

  await fs.writeFile(filepath, JSON.stringify(report, null, 2))

  return filepath
}
