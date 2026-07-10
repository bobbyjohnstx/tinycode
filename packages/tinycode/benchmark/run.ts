#!/usr/bin/env bun
/**
 * Model Compatibility Benchmark Runner
 *
 * Usage:
 *   bun run benchmark/run.ts --models qwen3:14b,llama3.1:8b --tasks 1,2,3,4,5 --runs 10 --timeout 300
 *
 * Arguments:
 *   --models    Comma-separated model list. Supports any provider/model format:
 *               Local (Ollama):  qwen3:14b, llama3.1:8b, qwen3:3b
 *               Cloud reference: anthropic/claude-sonnet-4-20250514, glm/glm-5.2-cloud
 *   --agent     Agent persona to use (e.g., debugger, executor). Default: build
 *   --tasks     Comma-separated task IDs to run (1-5), default: all
 *   --runs      Number of runs per model/task combination (default: 10)
 *   --timeout   Timeout in seconds per task (default: 300)
 *   --help      Show this help message
 *
 * Examples:
 *   # Full local benchmark
 *   bun benchmark --models "qwen3:14b,qwen3.5:9b,llama3.1:8b,mistral-nemo:12b,phi4:14b,qwen3:3b" --runs 10
 *
 *   # Quick single-model test
 *   bun benchmark --models "qwen3:14b" --tasks 2 --runs 1 --timeout 120
 *
 *   # Test with a specialized agent
 *   bun benchmark --models "qwen3:14b" --agent debugger --tasks 5 --runs 3
 *
 *   # Cloud ceiling reference (requires API key configured)
 *   bun benchmark --models "anthropic/claude-sonnet-4-20250514" --runs 3
 */

import { $ } from "bun"
import { parseArgs } from "util"
import { createFixtureDir } from "./fixture/setup"
import { parseNdjsonEvents } from "./scoring/extract"
import { aggregateScores } from "./scoring/score"
import { generateJsonReport } from "./report/json"
import { generateMarkdownReport } from "./report/markdown"
import type { RunResult } from "./tasks/types"

// Import all tasks
import { task as task1 } from "./tasks/task-tool-diagnostic"
import { task as task2 } from "./tasks/task-fix-test"
import { task as task3 } from "./tasks/task-validation"
import { task as task4 } from "./tasks/task-rename"
import { task as task5 } from "./tasks/task-debug"

const ALL_TASKS = [task1, task2, task3, task4, task5]

interface Args {
  models: string[]
  agent: string
  tasks: number[]
  runs: number
  timeout: number
  help: boolean
}

function parseArguments(): Args {
  const { values } = parseArgs({
    args: process.argv.slice(2),
    options: {
      models: { type: "string" },
      agent: { type: "string" },
      tasks: { type: "string" },
      runs: { type: "string" },
      timeout: { type: "string" },
      help: { type: "boolean", short: "h" },
    },
    strict: false,
  })

  if (values.help) {
    console.log(`
Model Compatibility Benchmark Runner

Usage:
  bun run benchmark/run.ts --models qwen3:14b,llama3.1:8b --tasks 1,2,3,4,5 --runs 10 --timeout 300

Arguments:
  --models    Comma-separated list of Ollama models (e.g., qwen3:14b,llama3.1:8b)
  --agent     Agent persona to use (e.g., debugger, executor). Default: build
  --tasks     Comma-separated task IDs to run (1-5), default: all
  --runs      Number of runs per model/task combination (default: 10)
  --timeout   Timeout in seconds per task (default: 300)
  --help      Show this help message
`)
    process.exit(0)
  }

  const models =
    typeof values.models === "string" ? values.models.split(",").map((s: string) => s.trim()) : []
  const agent = typeof values.agent === "string" ? values.agent : "build"
  const tasks =
    typeof values.tasks === "string"
      ? values.tasks.split(",").map((s: string) => parseInt(s.trim()))
      : [1, 2, 3, 4, 5]
  const runs = typeof values.runs === "string" ? parseInt(values.runs) : 10
  const timeout = typeof values.timeout === "string" ? parseInt(values.timeout) : 300

  return { models, agent, tasks, runs, timeout, help: false }
}

async function validateOllamaModels(models: string[]): Promise<void> {
  console.log("Validating Ollama models...")

  const result = await $`ollama list`.nothrow().quiet()

  if (result.exitCode !== 0) {
    console.error("Error: Ollama is not running or not installed")
    process.exit(1)
  }

  const installedModels = result.text().split("\n").map((line) => {
    const parts = line.trim().split(/\s+/)
    return parts[0]
  })

  for (const model of models) {
    if (!installedModels.some((installed) => installed === model)) {
      console.error(`Error: Model "${model}" not found in Ollama`)
      console.error(`Available models: ${installedModels.filter(Boolean).join(", ")}`)
      process.exit(1)
    }
  }

  console.log(`All ${models.length} models validated`)
}

async function runTask(
  model: string,
  agent: string,
  taskId: number,
  runNumber: number,
  timeoutSeconds: number
): Promise<RunResult> {
  const task = ALL_TASKS.find((t) => t.id === taskId)
  if (!task) {
    throw new Error(`Task ${taskId} not found`)
  }

  console.log(
    `  Run ${runNumber}: ${task.name} (timeout: ${timeoutSeconds}s)`
  )

  // Create fresh fixture directory
  const fixture = await createFixtureDir()

  let timedOut = false
  let durationMs = 0
  let stdout = ""
  let error: string | undefined

  try {
    const startTime = Date.now()

    // Build command
    const tinycodeCommand = [
      "bun",
      "dev",
      "run",
      "--model",
      `ollama/${model}`,
      "--agent",
      agent,
      "--dangerously-skip-permissions",
      "--format",
      "json",
      "--dir",
      fixture.path,
      task.prompt,
    ]

    // Spawn process with timeout
    const proc = Bun.spawn(tinycodeCommand, {
      cwd: "/Users/bjohns/projects/tinycode",
      stdout: "pipe",
      stderr: "pipe",
    })

    // Set up timeout
    const timeoutHandle = setTimeout(() => {
      timedOut = true
      proc.kill()
    }, timeoutSeconds * 1000)

    // Collect stdout
    const stdoutChunks: string[] = []
    const reader = proc.stdout.getReader()

    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      stdoutChunks.push(new TextDecoder().decode(value))
    }

    stdout = stdoutChunks.join("")

    await proc.exited
    clearTimeout(timeoutHandle)

    durationMs = Date.now() - startTime
  } catch (err) {
    error = String(err)
  }

  // Parse events
  const { events, toolCallCount, toolFailureCount } = parseNdjsonEvents(stdout)

  // Run verification
  let score = 0
  let reason = "Error or timeout"

  if (timedOut) {
    score = 0
    reason = "Timeout"
  } else if (!error) {
    try {
      const result = await task.verify(fixture.path, events)
      score = result.score
      reason = result.reason
    } catch (err) {
      error = String(err)
      reason = `Verification failed: ${error}`
    }
  }

  // Clean up fixture
  await fixture.cleanup()

  return {
    model: `ollama/${model}`,
    agent,
    task: task.name,
    taskId: task.id,
    score,
    reason,
    durationMs,
    toolCallCount,
    toolFailureCount,
    timedOut,
    events: events.map((e) => ({
      type: e.type,
      tool: e.part?.tool,
      text: e.part?.text,
    })),
  }
}

async function main() {
  const args = parseArguments()

  if (args.models.length === 0) {
    console.error("Error: No models specified. Use --models to specify at least one model.")
    console.error('Example: --models "qwen3:14b,llama3.1:8b"')
    process.exit(1)
  }

  console.log("\n=== Model Compatibility Benchmark ===\n")
  console.log(`Models: ${args.models.join(", ")}`)
  console.log(`Agent: ${args.agent}`)
  console.log(`Tasks: ${args.tasks.join(", ")}`)
  console.log(`Runs per task: ${args.runs}`)
  console.log(`Timeout: ${args.timeout}s`)
  console.log("")

  // Validate Ollama models
  await validateOllamaModels(args.models)
  console.log("")

  const totalRuns = args.models.length * args.tasks.length * args.runs
  let currentRun = 0

  const allResults: RunResult[] = []

  // Run benchmark
  for (const model of args.models) {
    console.log(`\nModel: ${model}`)

    for (const taskId of args.tasks) {
      const task = ALL_TASKS.find((t) => t.id === taskId)
      if (!task) continue

      console.log(`\n  Task ${taskId}: ${task.name}`)

      for (let runNumber = 1; runNumber <= args.runs; runNumber++) {
        currentRun++
        console.log(
          `    [${currentRun}/${totalRuns}]`,
        )

        const result = await runTask(model, args.agent, taskId, runNumber, args.timeout)
        allResults.push(result)

        console.log(
          `      Score: ${result.score}/3, Duration: ${Math.round(result.durationMs / 1000)}s, Tools: ${result.toolCallCount}, Failures: ${result.toolFailureCount}`
        )
        console.log(`      Reason: ${result.reason}`)
      }
    }
  }

  console.log("\n\n=== Generating Reports ===\n")

  // Aggregate scores
  const modelScores = aggregateScores(allResults)

  // Generate JSON report (returns both path and full report object)
  const jsonPath = await generateJsonReport(modelScores, allResults)
  console.log(`JSON report: ${jsonPath}`)

  // Read back the JSON report to get the full metadata
  const reportFile = await import("fs/promises")
  const reportData = JSON.parse(await reportFile.readFile(jsonPath, "utf-8"))

  // Generate markdown report using the full report data
  const mdPath = await generateMarkdownReport(reportData)
  console.log(`Markdown report: ${mdPath}`)

  console.log("\n\n=== Summary ===\n")

  for (const model of modelScores) {
    console.log(
      `${model.model}: ${model.totalScore.toFixed(1)}/${model.maxScore} (${model.tier})`
    )
    for (const [taskId, score] of Object.entries(model.taskScores)) {
      console.log(`  Task ${taskId}: ${score.toFixed(1)}/3`)
    }
  }

  console.log("\nDone!")
}

main().catch((err) => {
  console.error("Fatal error:", err)
  process.exit(1)
})
