import { describe, expect, test, afterEach } from "bun:test"
import path from "path"
import fs from "fs"
import os from "os"
import { Effect } from "effect"
import { Global } from "@opencode-ai/core/global"
import { InstallationChannel } from "@opencode-ai/core/installation/version"
import { RuntimeFlags } from "@/effect/runtime-flags"
import { Database } from "@/storage/db"
import { it } from "../lib/effect"

describe("Database.getChannelPath", () => {
  it.effect("returns database path for the current channel", () =>
    Effect.gen(function* () {
      const flags = yield* RuntimeFlags.Service
      const expected = ["latest", "beta", "prod"].includes(InstallationChannel)
        ? path.join(Global.Path.data, "tinycode.db")
        : path.join(Global.Path.data, `tinycode-${InstallationChannel.replace(/[^a-zA-Z0-9._-]/g, "-")}.db`)

      expect(Database.getChannelPath(flags)).toBe(expected)
    }).pipe(Effect.provide(RuntimeFlags.layer())),
  )

  it.effect("uses the shared database path when channel databases are disabled", () =>
    Effect.gen(function* () {
      const flags = yield* RuntimeFlags.Service

      expect(Database.getChannelPath(flags)).toBe(path.join(Global.Path.data, "tinycode.db"))
    }).pipe(Effect.provide(RuntimeFlags.layer({ disableChannelDb: true }))),
  )

  it.effect("accepts RuntimeFlags with skipMigrations for database callers", () =>
    Effect.gen(function* () {
      const flags = yield* RuntimeFlags.Service

      expect(flags.skipMigrations).toBe(true)
      expect(Database.getChannelPath(flags)).toBe(Database.getChannelPath({ disableChannelDb: flags.disableChannelDb }))
    }).pipe(Effect.provide(RuntimeFlags.layer({ skipMigrations: true }))),
  )
})

describe("Database migration from opencode.db", () => {
  let originalData: string
  let tmpDir: string

  function setup() {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "tinycode-db-test-"))
    originalData = Global.Path.data
    ;(Global.Path as { data: string }).data = tmpDir
  }

  afterEach(() => {
    if (originalData) (Global.Path as { data: string }).data = originalData
    if (tmpDir) fs.rmSync(tmpDir, { recursive: true, force: true })
  })

  test("renames opencode.db to tinycode.db on first access", () => {
    setup()
    fs.writeFileSync(path.join(tmpDir, "opencode.db"), "test-db")
    fs.writeFileSync(path.join(tmpDir, "opencode.db-wal"), "test-wal")
    fs.writeFileSync(path.join(tmpDir, "opencode.db-shm"), "test-shm")

    const result = Database.getChannelPath({ disableChannelDb: true })

    expect(result).toBe(path.join(tmpDir, "tinycode.db"))
    expect(fs.existsSync(path.join(tmpDir, "tinycode.db"))).toBe(true)
    expect(fs.existsSync(path.join(tmpDir, "tinycode.db-wal"))).toBe(true)
    expect(fs.existsSync(path.join(tmpDir, "tinycode.db-shm"))).toBe(true)
    expect(fs.existsSync(path.join(tmpDir, "opencode.db"))).toBe(false)
    expect(fs.existsSync(path.join(tmpDir, "opencode.db-wal"))).toBe(false)
    expect(fs.existsSync(path.join(tmpDir, "opencode.db-shm"))).toBe(false)
    expect(fs.readFileSync(path.join(tmpDir, "tinycode.db"), "utf-8")).toBe("test-db")
  })

  test("does not overwrite existing tinycode.db", () => {
    setup()
    fs.writeFileSync(path.join(tmpDir, "opencode.db"), "old-data")
    fs.writeFileSync(path.join(tmpDir, "tinycode.db"), "new-data")

    Database.getChannelPath({ disableChannelDb: true })

    expect(fs.readFileSync(path.join(tmpDir, "tinycode.db"), "utf-8")).toBe("new-data")
    expect(fs.existsSync(path.join(tmpDir, "opencode.db"))).toBe(true)
  })

  test("handles missing WAL/SHM files gracefully", () => {
    setup()
    fs.writeFileSync(path.join(tmpDir, "opencode.db"), "db-only")

    Database.getChannelPath({ disableChannelDb: true })

    expect(fs.existsSync(path.join(tmpDir, "tinycode.db"))).toBe(true)
    expect(fs.existsSync(path.join(tmpDir, "tinycode.db-wal"))).toBe(false)
    expect(fs.existsSync(path.join(tmpDir, "tinycode.db-shm"))).toBe(false)
  })

  test("no-ops when neither database exists", () => {
    setup()

    const result = Database.getChannelPath({ disableChannelDb: true })

    expect(result).toBe(path.join(tmpDir, "tinycode.db"))
    expect(fs.existsSync(path.join(tmpDir, "tinycode.db"))).toBe(false)
  })
})
