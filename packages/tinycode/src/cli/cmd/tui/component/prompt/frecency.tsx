import path from "path"
import { Global } from "@/core/global"
import { Filesystem } from "@/util/filesystem"
import { onMount } from "solid-js"
import { createStore } from "solid-js/store"
import { createSimpleContext } from "../../context/helper"
import { appendFile, writeFile } from "fs/promises"

type FrecencyEntry = { frequency: number; lastOpen: number }
type StoredEntry = { namespace?: string; path: string; frequency: number; lastOpen: number }

function calculateFrecency(entry?: FrecencyEntry): number {
  if (!entry) return 0
  const daysSince = (Date.now() - entry.lastOpen) / 86400000
  const weight = 1 / (1 + daysSince)
  return entry.frequency * weight
}

function entryKey(namespace: string, key: string): string {
  return `${namespace}:${key}`
}

const MAX_FRECENCY_ENTRIES = 1000

export const { use: useFrecency, provider: FrecencyProvider } = createSimpleContext({
  name: "Frecency",
  init: () => {
    const frecencyPath = path.join(Global.Path.state, "frecency.jsonl")
    onMount(async () => {
      const text = await Filesystem.readText(frecencyPath).catch(() => "")
      const lines = text
        .split("\n")
        .filter(Boolean)
        .map((line) => {
          try {
            return JSON.parse(line) as StoredEntry
          } catch {
            return null
          }
        })
        .filter((line): line is StoredEntry => line !== null)

      const latest: Record<string, StoredEntry> = {}
      for (const entry of lines) {
        const ns = entry.namespace ?? "files"
        const k = entryKey(ns, entry.path)
        latest[k] = { ...entry, namespace: ns }
      }

      const sorted = Object.values(latest)
        .sort((a, b) => b.lastOpen - a.lastOpen)
        .slice(0, MAX_FRECENCY_ENTRIES)

      setStore(
        "data",
        Object.fromEntries(
          sorted.map((entry) => [entryKey(entry.namespace!, entry.path), { frequency: entry.frequency, lastOpen: entry.lastOpen }]),
        ),
      )

      if (sorted.length > 0) {
        const content = sorted.map((entry) => JSON.stringify(entry)).join("\n") + "\n"
        writeFile(frecencyPath, content).catch(() => {})
      }
    })

    const [store, setStore] = createStore({
      data: {} as Record<string, FrecencyEntry>,
    })

    function updateFrecency(namespace: string, key: string) {
      const resolvedKey = namespace === "files" ? path.resolve(process.cwd(), key) : key
      const k = entryKey(namespace, resolvedKey)
      const newEntry: FrecencyEntry = {
        frequency: (store.data[k]?.frequency || 0) + 1,
        lastOpen: Date.now(),
      }
      setStore("data", k, newEntry)
      appendFile(frecencyPath, JSON.stringify({ namespace, path: resolvedKey, ...newEntry }) + "\n").catch(() => {})

      if (Object.keys(store.data).length > MAX_FRECENCY_ENTRIES) {
        const sorted = Object.entries(store.data)
          .sort(([, a], [, b]) => b.lastOpen - a.lastOpen)
          .slice(0, MAX_FRECENCY_ENTRIES)
        setStore("data", Object.fromEntries(sorted))
      }
    }

    function getFrecency(namespace: string, key: string): number {
      const resolvedKey = namespace === "files" ? path.resolve(process.cwd(), key) : key
      return calculateFrecency(store.data[entryKey(namespace, resolvedKey)])
    }

    return {
      getFrecency,
      updateFrecency,
      data: () => store.data,
    }
  },
})
