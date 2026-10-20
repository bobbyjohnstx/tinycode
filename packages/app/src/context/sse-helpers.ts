import type { Event } from "@tinycode/sdk/v2/client"

export type SseConnectionState = "connected" | "reconnecting" | "offline"

/** Normalize a yielded SSE frame into the GlobalEvent envelope the SPA expects. */
export function normalizeGlobalSseEvent(
  raw: unknown,
): { directory: string; payload: Event } | undefined {
  if (!raw || typeof raw !== "object") return undefined
  const obj = raw as Record<string, unknown>

  const payload = obj.payload
  if (payload && typeof payload === "object" && "type" in (payload as object)) {
    return {
      directory: typeof obj.directory === "string" ? obj.directory : "global",
      payload: payload as Event,
    }
  }

  // Bare stream.gap: Go sends `event: stream.gap` + `data: {"lastSeen":...}` (no envelope)
  if ("lastSeen" in obj) {
    return {
      directory: "global",
      payload: {
        type: "stream.gap",
        properties: { lastSeen: obj.lastSeen },
      } as Event,
    }
  }

  return undefined
}

export function isStreamResyncEvent(type: string): boolean {
  return type === "server.connected" || type === "global.disposed" || type === "stream.gap"
}

export const SSE_RECONNECTING_DEBOUNCE_MS = 2_000
export const SSE_OFFLINE_FAIL_THRESHOLD = 3
export const SSE_HEALTHY_CLEAR_MS = 1_000

/** Pure next-state helper for unit tests and the connection machine. */
export function resolveSseConnectionState(input: {
  failCount: number
  reconnectingElapsedMs: number
}): SseConnectionState {
  if (input.failCount <= 0) return "connected"
  if (input.failCount >= SSE_OFFLINE_FAIL_THRESHOLD) return "offline"
  if (input.reconnectingElapsedMs >= SSE_RECONNECTING_DEBOUNCE_MS) return "reconnecting"
  return "connected"
}
