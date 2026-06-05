/**
 * Tests for the setup command's file-system operations.
 *
 * All tests use a temporary directory — nothing writes to ~/.config/tinycode/.
 * npm install is never invoked (skipMcp is always true or the install step is
 * not reached in the scenarios we test).
 */

import { describe, it, expect, beforeEach } from "bun:test"
import fsNode from "fs"
import path from "path"
import os from "os"

// ---------------------------------------------------------------------------
// Helpers that mirror the logic in SetupCommand.handler so we can test each
// step in isolation without running the full CLI (which uses prompts/spinners
// and requires a TTY).
// ---------------------------------------------------------------------------

async function makeTmpDir(): Promise<string> {
  const dir = await fsNode.promises.mkdtemp(path.join(os.tmpdir(), "tinycode-setup-test-"))
  return dir
}

async function rmrf(dir: string) {
  await fsNode.promises.rm(dir, { recursive: true, force: true })
}

/**
 * Create the three sub-directories that setup creates under configDir.
 */
async function createDirectories(configDir: string) {
  const agentDir = path.join(configDir, "agent")
  const skillsDir = path.join(configDir, "skills")
  const mcpDir = path.join(configDir, "mcp")
  await fsNode.promises.mkdir(agentDir, { recursive: true })
  await fsNode.promises.mkdir(skillsDir, { recursive: true })
  await fsNode.promises.mkdir(mcpDir, { recursive: true })
  return { agentDir, skillsDir, mcpDir }
}

/**
 * Build the default config object the same way setup.ts does.
 */
function buildDefaultConfig(configDir: string): object {
  const mcpDir = path.join(configDir, "mcp")
  const skillsDir = path.join(configDir, "skills")
  return {
    lsp: true,
    skills: {
      paths: [skillsDir],
    },
    mcp: {
      "oh-my-tiny": {
        type: "local",
        command: ["node", path.join(mcpDir, "node_modules/oh-my-tiny/dist/mcp/server.js")],
      },
    },
  }
}

/**
 * Write the default config.json if it does not exist, or when force is true.
 */
async function writeDefaultConfig(configDir: string, force: boolean): Promise<boolean> {
  const configFile = path.join(configDir, "config.json")
  const configExists = await fsNode.promises
    .access(configFile)
    .then(() => true)
    .catch(() => false)

  if (!configExists || force) {
    const defaultConfig = buildDefaultConfig(configDir)
    await fsNode.promises.writeFile(configFile, JSON.stringify(defaultConfig, null, 2) + "\n", "utf8")
    return true
  }
  return false
}

/**
 * Copy .md files from a source directory to a destination directory,
 * returning the count of files copied.
 */
async function copyAgentFiles(sourceDir: string, destDir: string): Promise<number> {
  const files = await fsNode.promises.readdir(sourceDir).catch(() => [] as string[])
  let copied = 0
  for (const file of files) {
    if (!file.endsWith(".md")) continue
    await fsNode.promises.copyFile(path.join(sourceDir, file), path.join(destDir, file))
    copied++
  }
  return copied
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("setup – createDirectories", () => {
  it("creates agent, skills, and mcp sub-directories", async () => {
    const tmp = await makeTmpDir()
    try {
      const { agentDir, skillsDir, mcpDir } = await createDirectories(tmp)

      const agentStat = await fsNode.promises.stat(agentDir)
      const skillsStat = await fsNode.promises.stat(skillsDir)
      const mcpStat = await fsNode.promises.stat(mcpDir)

      expect(agentStat.isDirectory()).toBe(true)
      expect(skillsStat.isDirectory()).toBe(true)
      expect(mcpStat.isDirectory()).toBe(true)
    } finally {
      await rmrf(tmp)
    }
  })

  it("is idempotent – re-running does not error when directories already exist", async () => {
    const tmp = await makeTmpDir()
    try {
      await createDirectories(tmp)
      // second call must not throw
      await expect(createDirectories(tmp)).resolves.toBeDefined()
    } finally {
      await rmrf(tmp)
    }
  })

  it("directories are nested under the provided configDir", async () => {
    const tmp = await makeTmpDir()
    try {
      const { agentDir, skillsDir, mcpDir } = await createDirectories(tmp)
      expect(agentDir.startsWith(tmp)).toBe(true)
      expect(skillsDir.startsWith(tmp)).toBe(true)
      expect(mcpDir.startsWith(tmp)).toBe(true)
    } finally {
      await rmrf(tmp)
    }
  })
})

describe("setup – agent .md file copy logic", () => {
  it("copies .md files from source to destination", async () => {
    const tmp = await makeTmpDir()
    const src = path.join(tmp, "source")
    const dest = path.join(tmp, "dest")
    try {
      await fsNode.promises.mkdir(src, { recursive: true })
      await fsNode.promises.mkdir(dest, { recursive: true })

      await fsNode.promises.writeFile(path.join(src, "agent1.md"), "# Agent 1")
      await fsNode.promises.writeFile(path.join(src, "agent2.md"), "# Agent 2")
      await fsNode.promises.writeFile(path.join(src, "notmd.ts"), "// not an md file")

      const copied = await copyAgentFiles(src, dest)

      expect(copied).toBe(2)
      const destFiles = await fsNode.promises.readdir(dest)
      expect(destFiles).toContain("agent1.md")
      expect(destFiles).toContain("agent2.md")
      expect(destFiles).not.toContain("notmd.ts")
    } finally {
      await rmrf(tmp)
    }
  })

  it("returns 0 when source directory is empty", async () => {
    const tmp = await makeTmpDir()
    const src = path.join(tmp, "empty-source")
    const dest = path.join(tmp, "dest")
    try {
      await fsNode.promises.mkdir(src, { recursive: true })
      await fsNode.promises.mkdir(dest, { recursive: true })

      const copied = await copyAgentFiles(src, dest)
      expect(copied).toBe(0)
    } finally {
      await rmrf(tmp)
    }
  })

  it("returns 0 when source directory does not exist", async () => {
    const tmp = await makeTmpDir()
    const dest = path.join(tmp, "dest")
    try {
      await fsNode.promises.mkdir(dest, { recursive: true })
      const copied = await copyAgentFiles(path.join(tmp, "nonexistent"), dest)
      expect(copied).toBe(0)
    } finally {
      await rmrf(tmp)
    }
  })

  it("file contents are preserved after copy", async () => {
    const tmp = await makeTmpDir()
    const src = path.join(tmp, "source")
    const dest = path.join(tmp, "dest")
    try {
      await fsNode.promises.mkdir(src, { recursive: true })
      await fsNode.promises.mkdir(dest, { recursive: true })

      const content = "# My Agent\n\nDoes stuff."
      await fsNode.promises.writeFile(path.join(src, "myagent.md"), content)

      await copyAgentFiles(src, dest)

      const actual = await fsNode.promises.readFile(path.join(dest, "myagent.md"), "utf8")
      expect(actual).toBe(content)
    } finally {
      await rmrf(tmp)
    }
  })
})

describe("setup – default config.json generation", () => {
  it("writes valid JSON with expected top-level keys", async () => {
    const tmp = await makeTmpDir()
    try {
      const wrote = await writeDefaultConfig(tmp, false)
      expect(wrote).toBe(true)

      const raw = await fsNode.promises.readFile(path.join(tmp, "config.json"), "utf8")
      const parsed = JSON.parse(raw) // throws if invalid JSON

      expect(parsed).toHaveProperty("lsp")
      expect(parsed).toHaveProperty("skills")
      expect(parsed).toHaveProperty("mcp")
    } finally {
      await rmrf(tmp)
    }
  })

  it("config.json contains skills.paths pointing at the skills sub-directory", async () => {
    const tmp = await makeTmpDir()
    try {
      await writeDefaultConfig(tmp, false)

      const raw = await fsNode.promises.readFile(path.join(tmp, "config.json"), "utf8")
      const parsed = JSON.parse(raw)

      expect(Array.isArray(parsed.skills?.paths)).toBe(true)
      expect(parsed.skills.paths[0]).toContain("skills")
    } finally {
      await rmrf(tmp)
    }
  })

  it("config.json contains an oh-my-tiny mcp entry", async () => {
    const tmp = await makeTmpDir()
    try {
      await writeDefaultConfig(tmp, false)

      const raw = await fsNode.promises.readFile(path.join(tmp, "config.json"), "utf8")
      const parsed = JSON.parse(raw)

      expect(parsed.mcp).toHaveProperty("oh-my-tiny")
      expect(parsed.mcp["oh-my-tiny"].type).toBe("local")
      expect(Array.isArray(parsed.mcp["oh-my-tiny"].command)).toBe(true)
    } finally {
      await rmrf(tmp)
    }
  })

  it("config.json has lsp: true", async () => {
    const tmp = await makeTmpDir()
    try {
      await writeDefaultConfig(tmp, false)
      const parsed = JSON.parse(await fsNode.promises.readFile(path.join(tmp, "config.json"), "utf8"))
      expect(parsed.lsp).toBe(true)
    } finally {
      await rmrf(tmp)
    }
  })
})

describe("setup – --force flag behaviour", () => {
  it("overwrites existing config.json when force is true", async () => {
    const tmp = await makeTmpDir()
    try {
      // Write an initial config with a sentinel value
      const configFile = path.join(tmp, "config.json")
      await fsNode.promises.writeFile(configFile, JSON.stringify({ custom: "original" }), "utf8")

      const wrote = await writeDefaultConfig(tmp, true)
      expect(wrote).toBe(true)

      const parsed = JSON.parse(await fsNode.promises.readFile(configFile, "utf8"))
      // The original sentinel should be gone; the default key should be present
      expect(parsed.custom).toBeUndefined()
      expect(parsed.lsp).toBe(true)
    } finally {
      await rmrf(tmp)
    }
  })

  it("preserves existing config.json when force is false", async () => {
    const tmp = await makeTmpDir()
    try {
      const configFile = path.join(tmp, "config.json")
      const original = { custom: "should-be-preserved", lsp: false }
      await fsNode.promises.writeFile(configFile, JSON.stringify(original), "utf8")

      const wrote = await writeDefaultConfig(tmp, false)
      expect(wrote).toBe(false)

      const parsed = JSON.parse(await fsNode.promises.readFile(configFile, "utf8"))
      expect(parsed.custom).toBe("should-be-preserved")
      expect(parsed.lsp).toBe(false)
    } finally {
      await rmrf(tmp)
    }
  })
})
