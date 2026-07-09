import * as fs from "fs/promises"
import path from "path"
import type { BenchmarkReport } from "./json"
import type { RunResult } from "../tasks/types"

export async function generateMarkdownReport(report: BenchmarkReport): Promise<string> {
  const lines: string[] = []

  lines.push("# Model Compatibility")
  lines.push("")
  lines.push(
    "This benchmark validates local LLM compatibility with tinycode's tool-calling system."
  )
  lines.push("")

  lines.push("## Metadata")
  lines.push("")
  lines.push(`- **Last verified:** ${report.date}`)
  lines.push(`- **Hardware:** ${report.hardware.cpus} (${report.hardware.cores} cores)`)
  lines.push(`- **Memory:** ${report.hardware.memory}`)
  lines.push(`- **Ollama:** ${report.ollama.version}`)
  lines.push("")
  lines.push(
    "> **Staleness notice:** Results are considered current for 90 days. This report will be stale after " +
      new Date(new Date(report.timestamp).getTime() + 90 * 24 * 60 * 60 * 1000).toLocaleDateString(
        "en-US",
        { year: "numeric", month: "long", day: "numeric" }
      ) +
      "."
  )
  lines.push("")

  lines.push("## Results")
  lines.push("")

  // Build table
  const taskIds = [1, 2, 3, 4, 5]
  const taskNames = [
    "Tool Diagnostic",
    "Fix Test",
    "Validation",
    "Rename",
    "Debug",
  ]

  lines.push(
    "| Model | Total | Tier | " + taskNames.map((name) => `T${taskNames.indexOf(name) + 1}: ${name}`).join(" | ") + " |"
  )
  lines.push(
    "|-------|-------|------|" + taskNames.map(() => "-------").join("|") + "|"
  )

  for (const model of report.models) {
    const cells = [
      model.model.replace("ollama/", ""),
      `${model.totalScore.toFixed(1)}/${model.maxScore}`,
      model.tier,
    ]

    // Add task scores with median duration
    for (const taskId of taskIds) {
      const taskRuns = model.runs.filter((r) => r.taskId === taskId)
      const score = model.taskScores[taskId] || 0
      const durations = taskRuns.map((r) => r.durationMs).sort((a, b) => a - b)
      const medianDuration = durations.length
        ? durations[Math.floor(durations.length / 2)]
        : 0
      const medianSeconds = Math.round(medianDuration / 1000)

      cells.push(`${score.toFixed(1)}/3 (${medianSeconds}s)`)
    }

    lines.push("| " + cells.join(" | ") + " |")
  }

  lines.push("")

  lines.push("## Tier Definitions")
  lines.push("")
  lines.push("- **Full Agentic (12-15):** Reliable tool calling, suitable for production workflows")
  lines.push(
    "- **Limited (8-11):** Inconsistent tool usage, may require manual intervention"
  )
  lines.push(
    "- **Chat Only (4-7):** Minimal tool usage, primarily text-based responses"
  )
  lines.push("- **Not Recommended (0-3):** Unreliable or non-functional")
  lines.push("")

  lines.push("## Task Descriptions")
  lines.push("")
  lines.push(
    "1. **Tool Diagnostic:** Find function, read it, add JSDoc (tests grep, read, edit)"
  )
  lines.push("2. **Fix Test:** Run tests, fix source code (tests shell, edit)")
  lines.push(
    "3. **Validation:** Add input validation (tests logic, type checking)"
  )
  lines.push("4. **Rename:** Multi-file rename with test verification (tests grep, edit)")
  lines.push("5. **Debug:** Trace stack trace to root cause (tests debugging, cross-file)")
  lines.push("")

  lines.push("## Notes")
  lines.push("")
  lines.push(
    "- All tasks are machine-verified with deterministic pass/fail criteria"
  )
  lines.push("- Each model is tested 10 times per task for statistical significance")
  lines.push("- Scores are averaged across runs (0-3 points per task)")
  lines.push("- Cloud models (Claude, GPT-4, etc.) are not included in this benchmark")
  lines.push("")

  const markdown = lines.join("\n")

  // Write to docs/
  const docsDir = path.join(import.meta.dir, "../../../../docs")
  await fs.mkdir(docsDir, { recursive: true })

  const filepath = path.join(docsDir, "model-compatibility.md")
  await fs.writeFile(filepath, markdown)

  return filepath
}
