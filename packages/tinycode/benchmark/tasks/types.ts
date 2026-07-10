export interface TaskDefinition {
  id: number
  name: string
  prompt: string
  setup?: (fixtureDir: string) => Promise<void>
  verify: (fixtureDir: string, events: NdjsonEvent[]) => Promise<TaskResult>
}

export interface TaskResult {
  score: 0 | 1 | 2 | 3
  reason: string
  details?: Record<string, unknown>
}

export interface RunResult {
  model: string
  agent: string
  task: string
  taskId: number
  score: number
  reason: string
  durationMs: number
  toolCallCount: number
  toolFailureCount: number
  timedOut: boolean
  events: Array<{ type: string; tool?: string; text?: string }>
}

export interface NdjsonEvent {
  type: string
  timestamp?: number
  sessionID?: string
  part?: {
    type?: string
    tool?: string
    text?: string
    state?: {
      status?: string
    }
    [key: string]: unknown
  }
  error?: unknown
  [key: string]: unknown
}
