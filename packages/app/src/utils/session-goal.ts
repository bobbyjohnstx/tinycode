/** Mirrors internal/command.WorkLoopPrefix for self-assessment goals. */
export const WORK_LOOP_PREFIX = `You are in WORK-LOOP mode. Iterate on the user's task until it is complete or you are blocked.

PROTOCOL:
1. Understand: Read the task. Check relevant files.
2. Plan: Identify the single most impactful next action. State it in one sentence.
3. Act: Execute the action using available tools.
4. Verify: Confirm the action worked (run tests, read the file, check output).
5. Assess:
   - If the task is complete → write a brief summary and STOP.
   - If more work remains → return to step 2.
   - If blocked (same action failed 3 times) → explain the blocker and STOP.

Do NOT ask for confirmation between iterations. Keep going until done or blocked.

USER TASK:
`

export const DEFAULT_GOAL_MAX_ITERATIONS = 10
export const GOAL_STUCK_THRESHOLD = 3

type GoalCategory = {
  patterns: string[]
  commands: Record<string, string>
}

const goalCategories: GoalCategory[] = [
  {
    patterns: ["tests pass", "all tests pass", "test passes"],
    commands: {
      node: "npm test",
      python: "pytest",
      rust: "cargo test",
      make: "make test",
      go: "go test ./... -count=1",
    },
  },
  {
    patterns: ["build succeeds", "build passes", "builds"],
    commands: {
      node: "npm run build",
      rust: "cargo build",
      make: "make build",
      go: "go build ./...",
    },
  },
  {
    patterns: ["no lint errors", "lint passes", "lint clean"],
    commands: {
      node: "npm run lint",
      rust: "cargo clippy",
      make: "make lint",
      go: "go vet ./...",
    },
  },
  {
    patterns: ["no vet errors", "vet passes", "vet clean"],
    commands: {
      go: "go vet ./...",
    },
  },
]

const ecosystemIndicators: Array<{ file: string; ecosystem: string }> = [
  { file: "package.json", ecosystem: "node" },
  { file: "Cargo.toml", ecosystem: "rust" },
  { file: "requirements.txt", ecosystem: "python" },
  { file: "pyproject.toml", ecosystem: "python" },
  { file: "setup.py", ecosystem: "python" },
  { file: "Makefile", ecosystem: "make" },
  { file: "go.mod", ecosystem: "go" },
]

export type GoalPhase = "prompt" | "verify"

export type GoalState = {
  text: string
  command?: string
  iteration: number
  maxIterations: number
  recentOutputs: string[]
  phase: GoalPhase
}

const goals = new Map<string, GoalState>()

export function detectEcosystem(filenames: string[]): string {
  const set = new Set(filenames.map((f) => f.split(/[\\/]/).pop() ?? f))
  for (const ind of ecosystemIndicators) {
    if (set.has(ind.file)) return ind.ecosystem
  }
  return ""
}

/** Maps a natural-language condition to a shell command for the given ecosystem. */
export function resolveGoalCommand(condition: string, ecosystem: string): string | undefined {
  const lower = condition.trim().toLowerCase()
  for (const cat of goalCategories) {
    for (const pattern of cat.patterns) {
      if (!lower.includes(pattern)) continue
      return cat.commands[ecosystem]
    }
  }
  return undefined
}

export function getGoal(sessionID: string): GoalState | undefined {
  return goals.get(sessionID)
}

export function clearGoal(sessionID: string): boolean {
  return goals.delete(sessionID)
}

export function setGoal(sessionID: string, text: string, command?: string): GoalState {
  const state: GoalState = {
    text,
    command: command || undefined,
    iteration: 0,
    maxIterations: DEFAULT_GOAL_MAX_ITERATIONS,
    recentOutputs: [],
    phase: "prompt",
  }
  goals.set(sessionID, state)
  return state
}

export function goalStatusText(goal: GoalState): string {
  return `Goal: ${goal.iteration}/${goal.maxIterations} — ${goal.text}`
}

export function recordGoalOutput(goal: GoalState, output: string): boolean {
  goal.recentOutputs.push(output)
  if (goal.recentOutputs.length > GOAL_STUCK_THRESHOLD) {
    goal.recentOutputs = goal.recentOutputs.slice(-GOAL_STUCK_THRESHOLD)
  }
  if (goal.recentOutputs.length < GOAL_STUCK_THRESHOLD) return false
  const first = goal.recentOutputs[0]
  return goal.recentOutputs.every((o) => o === first)
}

export function buildInitialGoalPrompt(text: string, command?: string): string {
  if (command) {
    return `Goal: ${text}\nCommand to verify: \`${command}\`\nPlease work toward making this command succeed (exit code 0). Start by running it to see the current state.`
  }
  return WORK_LOOP_PREFIX + text
}

export function buildContinueGoalPrompt(goal: GoalState, output: string): string {
  let clipped = output
  if (clipped.length > 4000) clipped = clipped.slice(0, 4000) + "\n... (truncated)"
  return `The goal '${goal.text}' is not met yet (iteration ${goal.iteration}/${goal.maxIterations}).\nVerification command: \`${goal.command}\`\nOutput:\n\`\`\`\n${clipped}\n\`\`\`\nPlease continue working toward making this command succeed.`
}
