import type { RunResult } from "../tasks/types"

export interface ModelScore {
  model: string
  agent: string
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
  const groupMap = new Map<string, RunResult[]>()

  // Group runs by model+agent
  for (const run of runs) {
    const key = `${run.model}|${run.agent}`
    const existing = groupMap.get(key) || []
    existing.push(run)
    groupMap.set(key, existing)
  }

  const modelScores: ModelScore[] = []

  for (const [, groupRuns] of groupMap.entries()) {
    const { model, agent } = groupRuns[0]

    // Group by task
    const taskMap = new Map<number, RunResult[]>()
    for (const run of groupRuns) {
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
      agent,
      totalScore,
      maxScore,
      tier: getTier(totalScore, maxScore),
      taskScores,
      runs: groupRuns,
    })
  }

  return modelScores.sort((a, b) => b.totalScore - a.totalScore)
}
