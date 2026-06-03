/**
 * Tests for the config migration helper (opencode → tinycode paths).
 */
import { describe, test, expect } from "bun:test"
import { mkdtempSync, rmSync, mkdirSync, writeFileSync, existsSync, readFileSync } from "fs"
import { tmpdir } from "os"
import { join } from "path"
import { migrateFiles } from "../../src/config/migration"

function makeTempDir() {
  return mkdtempSync(join(tmpdir(), "tc-migration-test-"))
}

// ---------------------------------------------------------------------------
// Test 5: copies files when destination is missing, creating dirs as needed
// ---------------------------------------------------------------------------

describe("migrateFiles", () => {
  test("copies config files from old path to new path when destination missing", async () => {
    const base = makeTempDir()
    const oldConfig = join(base, "opencode")
    const newConfig = join(base, "tinycode")   // does NOT exist yet
    const oldData = join(base, "data-old")
    const newData = join(base, "data-new")

    // Seed the old config directory
    mkdirSync(oldConfig, { recursive: true })
    writeFileSync(join(oldConfig, "config.json"), JSON.stringify({ model: "ollama/llama3" }))
    writeFileSync(join(oldConfig, "auth.json"), JSON.stringify({ type: "api" }))

    try {
      await migrateFiles(oldConfig, newConfig, oldData, newData)

      // Destination dir is created automatically, files are there
      expect(existsSync(join(newConfig, "config.json"))).toBe(true)
      expect(existsSync(join(newConfig, "auth.json"))).toBe(true)

      // Content is preserved
      const cfg = JSON.parse(readFileSync(join(newConfig, "config.json"), "utf-8"))
      expect(cfg.model).toBe("ollama/llama3")

      // Old files are NOT deleted
      expect(existsSync(join(oldConfig, "config.json"))).toBe(true)
    } finally {
      rmSync(base, { recursive: true, force: true })
    }
  })

  // ---------------------------------------------------------------------------
  // Test 6: does not overwrite existing destination files
  // ---------------------------------------------------------------------------

  test("does not overwrite existing destination files", async () => {
    const base = makeTempDir()
    const oldConfig = join(base, "opencode")
    const newConfig = join(base, "tinycode")
    const oldData = join(base, "data-old")
    const newData = join(base, "data-new")

    mkdirSync(oldConfig, { recursive: true })
    mkdirSync(newConfig, { recursive: true })

    const originalContent = JSON.stringify({ model: "already-set" })
    const oldContent = JSON.stringify({ model: "from-opencode" })

    writeFileSync(join(oldConfig, "config.json"), oldContent)
    writeFileSync(join(newConfig, "config.json"), originalContent)  // already exists

    try {
      await migrateFiles(oldConfig, newConfig, oldData, newData)

      // Destination should be UNCHANGED
      const content = readFileSync(join(newConfig, "config.json"), "utf-8")
      expect(content).toBe(originalContent)
    } finally {
      rmSync(base, { recursive: true, force: true })
    }
  })

  // ---------------------------------------------------------------------------
  // Test 7: no-op when old path does not exist
  // ---------------------------------------------------------------------------

  test("is a no-op and does not throw when old path does not exist", async () => {
    const base = makeTempDir()
    const oldConfig = join(base, "opencode-nonexistent")  // does not exist
    const newConfig = join(base, "tinycode")
    const oldData = join(base, "data-old-nonexistent")
    const newData = join(base, "data-new")

    try {
      await migrateFiles(oldConfig, newConfig, oldData, newData)
      // Nothing was created in the new config dir
      expect(existsSync(newConfig)).toBe(false)
    } finally {
      rmSync(base, { recursive: true, force: true })
    }
  })

  test("migrates db.sqlite from data directory", async () => {
    const base = makeTempDir()
    const oldConfig = join(base, "opencode")
    const newConfig = join(base, "tinycode")
    const oldData = join(base, "data-old")
    const newData = join(base, "data-new")

    mkdirSync(oldData, { recursive: true })
    writeFileSync(join(oldData, "db.sqlite"), "SQLite mock data")

    try {
      await migrateFiles(oldConfig, newConfig, oldData, newData)

      expect(existsSync(join(newData, "db.sqlite"))).toBe(true)
      expect(readFileSync(join(newData, "db.sqlite"), "utf-8")).toBe("SQLite mock data")
    } finally {
      rmSync(base, { recursive: true, force: true })
    }
  })
})
