import { describe, test, expect } from "bun:test"
import path from "path"
import fs from "fs/promises"
import { tmpdir } from "../fixture/fixture"
import {
  readNotepad,
  writeNotepadPriority,
  appendNotepadWorking,
  appendNotepadManual,
  pruneNotepadWorking,
  getNotepadStats,
  readState,
  writeState,
  clearState,
  listActiveStates,
  getStateDir,
  readProjectMemory,
  writeProjectMemory,
  readWikiPage,
  ingestWikiPage,
  deleteWikiPage,
  listWikiPages,
  titleToSlug,
} from "../../src/omt/persistence"

// ============================================================================
// Notepad
// ============================================================================

describe("readNotepad", () => {
  test("missing file returns does-not-exist message", async () => {
    await using tmp = await tmpdir()
    const result = readNotepad(tmp.path)
    expect(result).toContain("does not exist")
  })

  test("all sections returns full notepad content", async () => {
    await using tmp = await tmpdir()
    writeNotepadPriority(tmp.path, "critical thing")
    const result = readNotepad(tmp.path, "all")
    expect(result).toContain("## Notepad")
    expect(result).toContain("critical thing")
  })

  test("specific section - priority returns only priority content", async () => {
    await using tmp = await tmpdir()
    writeNotepadPriority(tmp.path, "priority content here")
    const result = readNotepad(tmp.path, "priority")
    expect(result).toContain("priority content here")
    expect(result).toContain("## Priority")
  })

  test("empty priority section returns empty message", async () => {
    await using tmp = await tmpdir()
    // Initialize notepad without writing anything to priority
    appendNotepadWorking(tmp.path, "working entry")
    const result = readNotepad(tmp.path, "priority")
    expect(result).toContain("Empty")
  })

  test("unknown section returns unknown-section message", async () => {
    await using tmp = await tmpdir()
    writeNotepadPriority(tmp.path, "something")
    const result = readNotepad(tmp.path, "bogussection")
    expect(result).toContain("Unknown section")
  })
})

describe("writeNotepadPriority", () => {
  test("creates notepad file if it does not exist", async () => {
    await using tmp = await tmpdir()
    const notepadPath = path.join(tmp.path, ".tinycode", "notepad.md")
    const before = await fs.stat(notepadPath).catch(() => null)
    expect(before).toBeNull()
    writeNotepadPriority(tmp.path, "first write")
    const after = await fs.stat(notepadPath).catch(() => null)
    expect(after).not.toBeNull()
  })

  test("returns success and no warning for content under 500 chars", async () => {
    await using tmp = await tmpdir()
    const result = writeNotepadPriority(tmp.path, "short content")
    expect(result.success).toBe(true)
    expect(result.warning).toBeUndefined()
  })

  test("returns warning for content over 500 chars", async () => {
    await using tmp = await tmpdir()
    const longContent = "x".repeat(501)
    const result = writeNotepadPriority(tmp.path, longContent)
    expect(result.success).toBe(true)
    expect(result.warning).toBeDefined()
    expect(result.warning).toContain("500")
  })

  test("overwrites previous priority content", async () => {
    await using tmp = await tmpdir()
    writeNotepadPriority(tmp.path, "first value")
    writeNotepadPriority(tmp.path, "second value")
    const result = readNotepad(tmp.path, "priority")
    expect(result).toContain("second value")
    expect(result).not.toContain("first value")
  })
})

describe("appendNotepadWorking", () => {
  test("creates entry with timestamp", async () => {
    await using tmp = await tmpdir()
    appendNotepadWorking(tmp.path, "working note content")
    const result = readNotepad(tmp.path, "working")
    expect(result).toContain("working note content")
    // Timestamp format: YYYY-MM-DD HH:MM
    expect(result).toMatch(/\d{4}-\d{2}-\d{2} \d{2}:\d{2}/)
  })

  test("multiple entries accumulate", async () => {
    await using tmp = await tmpdir()
    appendNotepadWorking(tmp.path, "first entry")
    appendNotepadWorking(tmp.path, "second entry")
    const result = readNotepad(tmp.path, "working")
    expect(result).toContain("first entry")
    expect(result).toContain("second entry")
  })

  test("returns true on success", async () => {
    await using tmp = await tmpdir()
    const ok = appendNotepadWorking(tmp.path, "some content")
    expect(ok).toBe(true)
  })
})

describe("appendNotepadManual", () => {
  test("creates entry with timestamp", async () => {
    await using tmp = await tmpdir()
    appendNotepadManual(tmp.path, "manual note")
    const result = readNotepad(tmp.path, "manual")
    expect(result).toContain("manual note")
    expect(result).toMatch(/\d{4}-\d{2}-\d{2} \d{2}:\d{2}/)
  })

  test("multiple entries accumulate", async () => {
    await using tmp = await tmpdir()
    appendNotepadManual(tmp.path, "first manual")
    appendNotepadManual(tmp.path, "second manual")
    const result = readNotepad(tmp.path, "manual")
    expect(result).toContain("first manual")
    expect(result).toContain("second manual")
  })

  test("returns true on success", async () => {
    await using tmp = await tmpdir()
    const ok = appendNotepadManual(tmp.path, "a note")
    expect(ok).toBe(true)
  })
})

describe("pruneNotepadWorking", () => {
  test("missing file returns zeros", async () => {
    await using tmp = await tmpdir()
    const result = pruneNotepadWorking(tmp.path)
    expect(result.pruned).toBe(0)
    expect(result.remaining).toBe(0)
  })

  test("prunes entries older than cutoff and keeps recent", async () => {
    await using tmp = await tmpdir()
    const notepadPath = path.join(tmp.path, ".tinycode", "notepad.md")

    // Write a notepad with two entries using hardcoded old + recent timestamps
    const oldDate = "2020-01-01 00:00"
    const recentDate = new Date()
    recentDate.setDate(recentDate.getDate() - 1)
    const recentTs = recentDate.toISOString().slice(0, 16).replace("T", " ")

    await fs.mkdir(path.dirname(notepadPath), { recursive: true })
    const content = `# Notepad
<!-- Auto-managed by tinycode. Manual edits preserved in MANUAL section. -->

## Priority Context
<!-- ALWAYS loaded. Keep under 500 chars. Critical discoveries only. -->

## Working Memory
<!-- Session notes. Auto-pruned after 7 days. -->
### ${oldDate}
old entry

### ${recentTs}
recent entry

## MANUAL
<!-- User content. Never auto-pruned. -->

`
    await fs.writeFile(notepadPath, content, "utf-8")

    const result = pruneNotepadWorking(tmp.path, 7)
    expect(result.pruned).toBe(1)
    expect(result.remaining).toBe(1)

    const after = readNotepad(tmp.path, "working")
    expect(after).not.toContain("old entry")
    expect(after).toContain("recent entry")
  })
})

describe("getNotepadStats", () => {
  test("missing file returns exists=false", async () => {
    await using tmp = await tmpdir()
    const stats = getNotepadStats(tmp.path)
    expect(stats.exists).toBe(false)
    expect(stats.totalSize).toBe(0)
    expect(stats.workingMemoryEntries).toBe(0)
    expect(stats.oldestEntry).toBeNull()
  })

  test("populated notepad returns correct counts", async () => {
    await using tmp = await tmpdir()
    writeNotepadPriority(tmp.path, "priority text")
    appendNotepadWorking(tmp.path, "entry one")
    appendNotepadWorking(tmp.path, "entry two")
    const stats = getNotepadStats(tmp.path)
    expect(stats.exists).toBe(true)
    expect(stats.totalSize).toBeGreaterThan(0)
    expect(stats.prioritySize).toBeGreaterThan(0)
    expect(stats.workingMemoryEntries).toBe(2)
    expect(stats.oldestEntry).not.toBeNull()
  })
})

// ============================================================================
// State
// ============================================================================

describe("readState", () => {
  test("missing file returns null", async () => {
    await using tmp = await tmpdir()
    const result = readState(getStateDir(tmp.path), "autopilot")
    expect(result).toBeNull()
  })

  test("valid JSON returns parsed data", async () => {
    await using tmp = await tmpdir()
    const sd = getStateDir(tmp.path)
    writeState(sd, "autopilot", { active: true, iteration: 3 })
    const result = readState(sd, "autopilot") as Record<string, unknown>
    expect(result).not.toBeNull()
    expect(result.active).toBe(true)
    expect(result.iteration).toBe(3)
  })
})

describe("writeState + readState round-trip", () => {
  test("data survives a write-then-read cycle", async () => {
    await using tmp = await tmpdir()
    const sd = getStateDir(tmp.path)
    const payload = { active: false, current_phase: "planning", iteration: 7 }
    writeState(sd, "team", payload)
    const back = readState(sd, "team") as typeof payload
    expect(back.active).toBe(false)
    expect(back.current_phase).toBe("planning")
    expect(back.iteration).toBe(7)
  })
})

describe("clearState", () => {
  test("removes existing state file and returns true", async () => {
    await using tmp = await tmpdir()
    const sd = getStateDir(tmp.path)
    writeState(sd, "autopilot", { active: true })
    const removed = clearState(sd, "autopilot")
    expect(removed).toBe(true)
    expect(readState(sd, "autopilot")).toBeNull()
  })

  test("returns false when state file is missing", async () => {
    await using tmp = await tmpdir()
    const removed = clearState(getStateDir(tmp.path), "autopilot")
    expect(removed).toBe(false)
  })
})

describe("listActiveStates", () => {
  test("missing directory returns empty array", async () => {
    await using tmp = await tmpdir()
    const result = listActiveStates(getStateDir(tmp.path))
    expect(result).toEqual([])
  })

  test("filters by active:true and excludes inactive", async () => {
    await using tmp = await tmpdir()
    const sd = getStateDir(tmp.path)
    writeState(sd, "autopilot", { active: true })
    writeState(sd, "team", { active: false })
    writeState(sd, "ralph", { active: true })
    const result = listActiveStates(sd)
    expect(result).toContain("autopilot")
    expect(result).toContain("ralph")
    expect(result).not.toContain("team")
  })
})

// ============================================================================
// Project memory
// ============================================================================

describe("readProjectMemory", () => {
  test("missing file returns null", async () => {
    await using tmp = await tmpdir()
    const result = readProjectMemory(tmp.path)
    expect(result).toBeNull()
  })

  test("parses full memory correctly", async () => {
    await using tmp = await tmpdir()
    writeProjectMemory(tmp.path, { techStack: "TypeScript", build: "bun" })
    const result = readProjectMemory(tmp.path) as Record<string, unknown>
    expect(result).not.toBeNull()
    expect(result.techStack).toBe("TypeScript")
    expect(result.build).toBe("bun")
  })

  test("section filtering returns only requested field", async () => {
    await using tmp = await tmpdir()
    writeProjectMemory(tmp.path, { techStack: "Go", build: "make" })
    const result = readProjectMemory(tmp.path, "techStack")
    expect(result).toBe("Go")
  })
})

describe("writeProjectMemory", () => {
  test("creates file with defaults injected", async () => {
    await using tmp = await tmpdir()
    writeProjectMemory(tmp.path, { techStack: "Rust" })
    const result = readProjectMemory(tmp.path) as Record<string, unknown>
    expect(result.version).toBeDefined()
    expect(result.lastScanned).toBeDefined()
    expect(result.projectRoot).toBeDefined()
  })

  test("merge=true merges with existing memory", async () => {
    await using tmp = await tmpdir()
    writeProjectMemory(tmp.path, { techStack: "Python", build: "pip" })
    writeProjectMemory(tmp.path, { conventions: "pep8" }, true)
    const result = readProjectMemory(tmp.path) as Record<string, unknown>
    expect(result.techStack).toBe("Python")
    expect(result.conventions).toBe("pep8")
  })

  test("preserves customNotes and userDirectives on merge", async () => {
    await using tmp = await tmpdir()
    const notes = [{ category: "env", content: "use .env.local", timestamp: Date.now() }]
    writeProjectMemory(tmp.path, { techStack: "Node", customNotes: notes })
    writeProjectMemory(tmp.path, { conventions: "eslint" }, true)
    const result = readProjectMemory(tmp.path) as Record<string, unknown>
    expect(Array.isArray(result.customNotes)).toBe(true)
    expect((result.customNotes as unknown[]).length).toBe(1)
  })
})

// ============================================================================
// Wiki
// ============================================================================

describe("readWikiPage", () => {
  test("missing wiki dir returns not-found", async () => {
    await using tmp = await tmpdir()
    const result = readWikiPage(tmp.path, "some-page")
    expect(result.found).toBe(false)
    expect(result.text).toContain("not found")
  })

  test("missing page returns not-found", async () => {
    await using tmp = await tmpdir()
    // Create wiki dir by ingesting a page
    ingestWikiPage(tmp.path, "Setup", "setup content", ["setup"], "reference")
    const result = readWikiPage(tmp.path, "nonexistent-page")
    expect(result.found).toBe(false)
    expect(result.text).toContain("not found")
  })

  test("path traversal is blocked", async () => {
    await using tmp = await tmpdir()
    ingestWikiPage(tmp.path, "Setup", "setup content", ["setup"], "reference")
    const result = readWikiPage(tmp.path, "../outside")
    expect(result.found).toBe(false)
  })
})

describe("ingestWikiPage", () => {
  test("creates new page", async () => {
    await using tmp = await tmpdir()
    const result = ingestWikiPage(tmp.path, "My Title", "body content", ["tag1"], "reference")
    expect(result.created.length).toBe(1)
    expect(result.updated.length).toBe(0)
    expect(result.totalAffected).toBe(1)
  })

  test("updates existing page and merges tags", async () => {
    await using tmp = await tmpdir()
    ingestWikiPage(tmp.path, "My Title", "original content", ["tag1"], "reference")
    const result = ingestWikiPage(tmp.path, "My Title", "new content", ["tag2"], "reference")
    expect(result.created.length).toBe(0)
    expect(result.updated.length).toBe(1)

    const page = readWikiPage(tmp.path, "my-title")
    expect(page.found).toBe(true)
    expect(page.text).toContain("tag1")
    expect(page.text).toContain("tag2")
  })

  test("page is readable after ingest", async () => {
    await using tmp = await tmpdir()
    ingestWikiPage(tmp.path, "Test Page", "test body", ["test"], "reference")
    const slug = titleToSlug("Test Page")
    const page = readWikiPage(tmp.path, slug)
    expect(page.found).toBe(true)
    expect(page.text).toContain("Test Page")
  })
})

describe("deleteWikiPage", () => {
  test("removes existing page and returns true", async () => {
    await using tmp = await tmpdir()
    ingestWikiPage(tmp.path, "Delete Me", "content", ["x"], "reference")
    const slug = titleToSlug("Delete Me")
    const deleted = deleteWikiPage(tmp.path, slug)
    expect(deleted).toBe(true)
    expect(readWikiPage(tmp.path, slug).found).toBe(false)
  })

  test("returns false for missing page", async () => {
    await using tmp = await tmpdir()
    ingestWikiPage(tmp.path, "Other Page", "content", [], "reference")
    const deleted = deleteWikiPage(tmp.path, "does-not-exist")
    expect(deleted).toBe(false)
  })

  test("blocks path traversal", async () => {
    await using tmp = await tmpdir()
    ingestWikiPage(tmp.path, "Safe Page", "content", [], "reference")
    const deleted = deleteWikiPage(tmp.path, "../outside")
    expect(deleted).toBe(false)
  })
})

describe("listWikiPages", () => {
  test("missing wiki dir returns empty message", async () => {
    await using tmp = await tmpdir()
    const result = listWikiPages(tmp.path)
    expect(result).toContain("empty")
  })

  test("returns index content when wiki has pages", async () => {
    await using tmp = await tmpdir()
    ingestWikiPage(tmp.path, "Architecture Overview", "details here", ["arch"], "architecture")
    const result = listWikiPages(tmp.path)
    // After ingest, updateWikiIndex writes index.md which listWikiPages reads
    expect(typeof result).toBe("string")
    expect(result.length).toBeGreaterThan(0)
  })
})

describe("titleToSlug", () => {
  test("converts title to lowercase hyphenated slug with .md extension", () => {
    expect(titleToSlug("Hello World")).toBe("hello-world.md")
  })

  test("strips special characters", () => {
    expect(titleToSlug("My API (v2)")).toBe("my-api-v2.md")
  })

  test("collapses multiple separators", () => {
    expect(titleToSlug("one  --  two")).toBe("one-two.md")
  })
})
