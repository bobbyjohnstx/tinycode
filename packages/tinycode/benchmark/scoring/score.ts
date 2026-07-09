import type { RunResult } from "../tasks/types"

export interface ModelScore {
  model: string
  totalScore: number
  maxScore: number
  tier: string
  taskScores: Record<number, number>
  runs: RunResult[]
}

export type Tier = "Full Agentic" | "Limited" | "Chat Only" | "Not Recommended"

export function getTier(score: number, maxScore: number): Tier {
  if (score >= 12 && score <= maxScore) return "Full Agentic"
  if (score >= 8 && score < 12) return "Limited"
  if (score >= 4 && score < 8) return "Chat Only"
  return "Not Recommended"
}

export function aggregateScores(runs: RunResult[]): ModelScore[] {
  const modelMap = new Map<string, RunResult[]>()

  // Group runs by model
  for (const run of runs) {
    const existing = modelMap.get(run.model) || []
    existing.push(run)
    modelMap.set(run.model, existing)
  }

  const modelScores: ModelScore[] = []

  for (const [model, modelRuns] of modelMap.entries()) {
    // Group by task
    const taskMap = new Map<number, RunResult[]>()
    for (const run of modelRuns) {
      const existing = taskMap.get(run.taskId) || []
      existing.push(run)
      taskMap.set(run.taskId, existing)
    }

    // Calculate average score per task
    const taskScores: Record<number, number> = {}
    for (const [taskId, taskRuns] of taskMap.entries()) {
      const avgScore =
        taskRuns.reduce((sum, run) => sum + run.score, 0) / taskRuns.length
      taskScores[taskId] = avgScore
    }

    const totalScore = Object.values(taskScores).reduce((sum, score) => sum + score, 0)
    const maxScore = Object.keys(taskScores).length * 3 // 3 points per task

    modelScores.push({
      model,
      totalScore,
      maxScore,
      tier: getTier(totalScore, maxScore),
      taskScores,
      runs: modelRuns,
    })
  }

  return modelScores.sort((a, b) => b.totalScore - a.totalScore)
}
