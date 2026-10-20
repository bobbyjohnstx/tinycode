import { describe, expect, test } from "bun:test"
import {
  isStreamResyncEvent,
  normalizeGlobalSseEvent,
  resolveSseConnectionState,
  SSE_OFFLINE_FAIL_THRESHOLD,
  SSE_RECONNECTING_DEBOUNCE_MS,
} from "./sse-helpers"

describe("normalizeGlobalSseEvent", () => {
  test("passes through enveloped GlobalEvent frames", () => {
    const raw = {
      directory: "/tmp/demo",
      payload: { type: "server.connected", properties: { timestamp: 1 } },
    }
    expect(normalizeGlobalSseEvent(raw)).toEqual(raw)
  })

  test("defaults missing directory to global", () => {
    const raw = {
      payload: { type: "session.idle", properties: { sessionID: "s1" } },
    }
    expect(normalizeGlobalSseEvent(raw)).toEqual({
      directory: "global",
      payload: raw.payload,
    })
  })

  test("normalizes bare stream.gap before payload access", () => {
    const raw = { lastSeen: "evt_123" }
    expect(normalizeGlobalSseEvent(raw)).toEqual({
      directory: "global",
      payload: { type: "stream.gap", properties: { lastSeen: "evt_123" } },
    })
  })

  test("returns undefined for empty or unknown frames", () => {
    expect(normalizeGlobalSseEvent(undefined)).toBeUndefined()
    expect(normalizeGlobalSseEvent(null)).toBeUndefined()
    expect(normalizeGlobalSseEvent({})).toBeUndefined()
    expect(normalizeGlobalSseEvent("heartbeat")).toBeUndefined()
  })
})

describe("isStreamResyncEvent", () => {
  test("matches connected, disposed, and gap", () => {
    expect(isStreamResyncEvent("server.connected")).toBe(true)
    expect(isStreamResyncEvent("global.disposed")).toBe(true)
    expect(isStreamResyncEvent("stream.gap")).toBe(true)
    expect(isStreamResyncEvent("session.idle")).toBe(false)
  })
})

describe("resolveSseConnectionState", () => {
  test("stays connected while healthy", () => {
    expect(resolveSseConnectionState({ failCount: 0, reconnectingElapsedMs: 0 })).toBe("connected")
  })

  test("debounces reconnecting until threshold", () => {
    expect(
      resolveSseConnectionState({
        failCount: 1,
        reconnectingElapsedMs: SSE_RECONNECTING_DEBOUNCE_MS - 1,
      }),
    ).toBe("connected")
    expect(
      resolveSseConnectionState({
        failCount: 1,
        reconnectingElapsedMs: SSE_RECONNECTING_DEBOUNCE_MS,
      }),
    ).toBe("reconnecting")
  })

  test("goes offline after consecutive failures", () => {
    expect(
      resolveSseConnectionState({
        failCount: SSE_OFFLINE_FAIL_THRESHOLD,
        reconnectingElapsedMs: SSE_RECONNECTING_DEBOUNCE_MS,
      }),
    ).toBe("offline")
  })
})
