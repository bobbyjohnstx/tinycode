import * as Log from "@opencode-ai/core/util/log"
import { Effect } from "effect"

export const HEADER = "x-tinycode-sync"
export type State = Record<string, number>
const log = Log.create({ service: "fence" })

export function load(_ids?: string[]) {
  return {} as State
}

export function diff(prev: State, next: State) {
  const ids = new Set([...Object.keys(prev), ...Object.keys(next)])
  return Object.fromEntries(
    [...ids]
      .map((id) => [id, next[id] ?? -1] as const)
      .filter(([id, seq]) => {
        return (prev[id] ?? -1) !== seq
      }),
  )
}

export function parse(headers: Headers): State | undefined {
  const raw = headers.get(HEADER)
  if (!raw) return

  let data
  try {
    data = JSON.parse(raw)
  } catch {
    return
  }

  if (!data || typeof data !== "object") return

  return Object.fromEntries(
    Object.entries(data).filter((entry): entry is [string, number] => {
      return typeof entry[0] === "string" && Number.isInteger(entry[1])
    }),
  )
}

export function wait(_workspaceID: string, state: State, _signal?: AbortSignal) {
  return Effect.gen(function* () {
    log.info("fence.wait is a no-op in local-only mode", { state })
  })
}
