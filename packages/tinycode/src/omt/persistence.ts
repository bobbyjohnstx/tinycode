/**
 * Persistence helpers for omt tools (merged from oh-my-tiny)
 *
 * Handles all .tinycode/ file I/O for state, notepad, project-memory, and wiki.
 * Falls back to reading from .omc/ for backward compatibility.
 * No Effect dependency -- pure Node.js fs.
 */

import {
  existsSync,
  readFileSync,
  writeFileSync,
  mkdirSync,
  readdirSync,
  unlinkSync,
  renameSync,
} from "fs"
import { join, dirname, sep } from "path"

// ============================================================================
// Shared helpers
// ============================================================================

function dataDir(projectRoot: string): string {
  return join(projectRoot, ".tinycode")
}

function legacyDir(projectRoot: string): string {
  return join(projectRoot, ".omc")
}

function ensureDir(p: string): void {
  if (!existsSync(p)) {
    mkdirSync(p, { recursive: true })
  }
}

type JsonReadResult =
  | { ok: true; data: unknown }
  | { ok: false; reason: "missing" | "corrupt"; error?: string }

function safeReadJson(filePath: string): JsonReadResult {
  if (!existsSync(filePath)) return { ok: false, reason: "missing" }
  try {
    return { ok: true, data: JSON.parse(readFileSync(filePath, "utf-8")) }
  } catch (err) {
    return { ok: false, reason: "corrupt", error: err instanceof Error ? err.message : String(err) }
  }
}

function safeWriteJson(filePath: string, data: unknown): void {
  ensureDir(dirname(filePath))
  const tmp = `${filePath}.tmp.${process.pid}`
  writeFileSync(tmp, JSON.stringify(data, null, 2), "utf-8")
  renameSync(tmp, filePath)
}

/** Read from .tinycode/ first, fall back to .omc/ */
function resolveReadPath(projectRoot: string, relativePath: string): string | null {
  const primary = join(dataDir(projectRoot), relativePath)
  if (existsSync(primary)) return primary
  const fallback = join(legacyDir(projectRoot), relativePath)
  if (existsSync(fallback)) return fallback
  return null
}

/** Read a file from .tinycode/ first, fall back to .omc/ */
function readWithFallback(projectRoot: string, relativePath: string): string | null {
  const resolved = resolveReadPath(projectRoot, relativePath)
  if (!resolved) return null
  try {
    return readFileSync(resolved, "utf-8")
  } catch {
    return null
  }
}

/** Read JSON from .tinycode/ first, fall back to .omc/ */
function readJsonWithFallback(projectRoot: string, relativePath: string): JsonReadResult {
  const resolved = resolveReadPath(projectRoot, relativePath)
  if (!resolved) return { ok: false, reason: "missing" }
  return safeReadJson(resolved)
}

// ============================================================================
// State management
// ============================================================================

function stateRelDir(): string {
  return "state"
}

export function stateFilePath(stateDir: string, mode: string): string {
  return join(stateDir, `${mode}-state.json`)
}

export function readState(stateDir: string, mode: string): unknown {
  const p = stateFilePath(stateDir, mode)
  const result = safeReadJson(p)
  return result.ok ? result.data : null
}

/** Read state with .tinycode/ -> .omc/ fallback */
export function readStateWithFallback(projectRoot: string, mode: string): unknown {
  const result = readJsonWithFallback(projectRoot, join(stateRelDir(), `${mode}-state.json`))
  return result.ok ? result.data : null
}

export function writeState(stateDir: string, mode: string, data: unknown): void {
  safeWriteJson(stateFilePath(stateDir, mode), data)
}

export function clearState(stateDir: string, mode: string): boolean {
  const p = stateFilePath(stateDir, mode)
  if (!existsSync(p)) return false
  try {
    unlinkSync(p)
    return true
  } catch {
    return false
  }
}

export function listActiveStates(stateDir: string): string[] {
  if (!existsSync(stateDir)) return []
  try {
    return readdirSync(stateDir)
      .filter((f) => f.endsWith("-state.json"))
      .map((f) => f.replace(/-state\.json$/, ""))
      .filter((mode) => {
        const result = safeReadJson(join(stateDir, `${mode}-state.json`))
        return result.ok && typeof result.data === "object" && result.data !== null && (result.data as Record<string, unknown>).active === true
      })
  } catch {
    return []
  }
}

/** List active states with .tinycode/ -> .omc/ fallback */
export function listActiveStatesWithFallback(projectRoot: string): string[] {
  const primary = join(dataDir(projectRoot), stateRelDir())
  if (existsSync(primary)) return listActiveStates(primary)
  const fallback = join(legacyDir(projectRoot), stateRelDir())
  if (existsSync(fallback)) return listActiveStates(fallback)
  return []
}

// ============================================================================
// Notepad
// ============================================================================

const PRIORITY_HEADER = "## Priority Context"
const WORKING_MEMORY_HEADER = "## Working Memory"
const MANUAL_HEADER = "## MANUAL"

function escapeRegex(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")
}

// Boundary pattern that only matches the exact known section headers, not arbitrary ## in user content
const SECTION_BOUNDARIES = [PRIORITY_HEADER, WORKING_MEMORY_HEADER, MANUAL_HEADER]
  .map(escapeRegex)
  .join("|")

const NOTEPAD_TEMPLATE = `# Notepad
<!-- Auto-managed by tinycode. Manual edits preserved in MANUAL section. -->

## Priority Context
<!-- ALWAYS loaded. Keep under 500 chars. Critical discoveries only. -->

## Working Memory
<!-- Session notes. Auto-pruned after 7 days. -->

## MANUAL
<!-- User content. Never auto-pruned. -->

`

function getNotepadPath(projectRoot: string): string {
  return join(dataDir(projectRoot), "notepad.md")
}

function initNotepadIfNeeded(projectRoot: string): void {
  const p = getNotepadPath(projectRoot)
  if (!existsSync(p)) {
    ensureDir(dirname(p))
    writeFileSync(p, NOTEPAD_TEMPLATE, "utf-8")
  }
}

function readNotepadRaw(projectRoot: string): string | null {
  // Try .tinycode/ first, fall back to .omc/
  return readWithFallback(projectRoot, "notepad.md")
}

function extractSection(content: string, header: string): string | null {
  const pattern = new RegExp(`${escapeRegex(header)}\\n([\\s\\S]*?)(?=\\n(?:${SECTION_BOUNDARIES})|$)`)
  const match = content.match(pattern)
  if (!match) return null
  const section = match[1].replace(/<!--[\s\S]*?-->/g, "").trim()
  return section || null
}

function replaceSection(content: string, header: string, newContent: string): string {
  const commentPattern = new RegExp(`${escapeRegex(header)}\\n(<!--[\\s\\S]*?-->)`)
  const replacePattern = new RegExp(`(${escapeRegex(header)}\\n)([\\s\\S]*?)(?=\\n(?:${SECTION_BOUNDARIES})|$)`)
  const commentMatch = content.match(commentPattern)
  const preservedComment = commentMatch ? commentMatch[1] + "\n" : ""
  return content.replace(replacePattern, `$1${preservedComment}${newContent}\n\n`)
}

export function readNotepad(projectRoot: string, section?: string): string {
  const content = readNotepadRaw(projectRoot)
  const sec = section || "all"

  if (!content) {
    return "Notepad does not exist. Use notepad_write_* tools to create it."
  }

  if (sec === "all") {
    return `## Notepad\n\nPath: ${getNotepadPath(projectRoot)}\n\n${content}`
  }

  const headerMap: Record<string, string> = {
    priority: PRIORITY_HEADER,
    working: WORKING_MEMORY_HEADER,
    manual: MANUAL_HEADER,
  }
  const header = headerMap[sec]
  if (!header) return `Unknown section: ${sec}`

  const extracted = extractSection(content, header)
  const title = sec.charAt(0).toUpperCase() + sec.slice(1)
  if (!extracted) return `## ${title}\n\n(Empty or notepad does not exist)`
  return `## ${title}\n\n${extracted}`
}

export function writeNotepadPriority(projectRoot: string, content: string): { success: boolean; warning?: string } {
  initNotepadIfNeeded(projectRoot)
  try {
    const p = getNotepadPath(projectRoot)
    let notepad = readFileSync(p, "utf-8")
    notepad = replaceSection(notepad, PRIORITY_HEADER, content)
    writeFileSync(p, notepad, "utf-8")
    const warning = content.length > 500
      ? `Priority Context exceeds 500 chars (${content.length} chars). Consider condensing.`
      : undefined
    return { success: true, warning }
  } catch {
    return { success: false }
  }
}

export function appendNotepadWorking(projectRoot: string, content: string): boolean {
  initNotepadIfNeeded(projectRoot)
  try {
    const p = getNotepadPath(projectRoot)
    let notepad = readFileSync(p, "utf-8")
    const current = extractSection(notepad, WORKING_MEMORY_HEADER) || ""
    const timestamp = new Date().toISOString().slice(0, 16).replace("T", " ")
    const newEntry = `### ${timestamp}\n${content}\n`
    const updated = current ? current + "\n" + newEntry : newEntry
    notepad = replaceSection(notepad, WORKING_MEMORY_HEADER, updated)
    writeFileSync(p, notepad, "utf-8")
    return true
  } catch {
    return false
  }
}

export function appendNotepadManual(projectRoot: string, content: string): boolean {
  initNotepadIfNeeded(projectRoot)
  try {
    const p = getNotepadPath(projectRoot)
    let notepad = readFileSync(p, "utf-8")
    const current = extractSection(notepad, MANUAL_HEADER) || ""
    const timestamp = new Date().toISOString().slice(0, 16).replace("T", " ")
    const newEntry = `### ${timestamp}\n${content}\n`
    const updated = current ? current + "\n" + newEntry : newEntry
    notepad = replaceSection(notepad, MANUAL_HEADER, updated)
    writeFileSync(p, notepad, "utf-8")
    return true
  } catch {
    return false
  }
}

export function pruneNotepadWorking(projectRoot: string, daysOld = 7): { pruned: number; remaining: number } {
  const p = getNotepadPath(projectRoot)
  if (!existsSync(p)) return { pruned: 0, remaining: 0 }
  try {
    let notepad = readFileSync(p, "utf-8")
    const workingMemory = extractSection(notepad, WORKING_MEMORY_HEADER)
    if (!workingMemory) return { pruned: 0, remaining: 0 }

    const entryRegex = /### (\d{4}-\d{2}-\d{2} \d{2}:\d{2})\n([\s\S]*?)(?=### |$)/g
    const entries: Array<{ timestamp: string; content: string }> = []
    let match: RegExpExecArray | null
    while ((match = entryRegex.exec(workingMemory)) !== null) {
      entries.push({ timestamp: match[1], content: match[2].trim() })
    }

    const cutoff = new Date()
    cutoff.setDate(cutoff.getDate() - daysOld)
    const kept = entries.filter((e) => new Date(e.timestamp) >= cutoff)
    const pruned = entries.length - kept.length

    const newContent = kept.map((e) => `### ${e.timestamp}\n${e.content}`).join("\n\n")
    notepad = replaceSection(notepad, WORKING_MEMORY_HEADER, newContent)
    writeFileSync(p, notepad, "utf-8")
    return { pruned, remaining: kept.length }
  } catch {
    return { pruned: 0, remaining: 0 }
  }
}

export function getNotepadStats(projectRoot: string): {
  exists: boolean
  totalSize: number
  prioritySize: number
  workingMemoryEntries: number
  oldestEntry: string | null
} {
  // Check .tinycode/ first, then .omc/
  const primary = getNotepadPath(projectRoot)
  const fallbackPath = join(legacyDir(projectRoot), "notepad.md")
  const p = existsSync(primary) ? primary : existsSync(fallbackPath) ? fallbackPath : null

  if (!p) {
    return { exists: false, totalSize: 0, prioritySize: 0, workingMemoryEntries: 0, oldestEntry: null }
  }
  const content = readFileSync(p, "utf-8")
  const priority = extractSection(content, PRIORITY_HEADER) || ""
  const working = extractSection(content, WORKING_MEMORY_HEADER) || ""
  const matches = working.match(/### \d{4}-\d{2}-\d{2} \d{2}:\d{2}/g)
  const entryCount = matches ? matches.length : 0
  let oldestEntry: string | null = null
  if (matches && matches.length > 0) {
    const timestamps = matches.map((m) => m.replace("### ", ""))
    timestamps.sort()
    oldestEntry = timestamps[0]
  }
  return {
    exists: true,
    totalSize: Buffer.byteLength(content, "utf-8"),
    prioritySize: Buffer.byteLength(priority, "utf-8"),
    workingMemoryEntries: entryCount,
    oldestEntry,
  }
}

// ============================================================================
// Project memory
// ============================================================================

function getProjectMemoryPath(projectRoot: string): string {
  return join(dataDir(projectRoot), "project-memory.json")
}

export function readProjectMemory(projectRoot: string, section?: string): unknown {
  // Try .tinycode/ first, fall back to .omc/
  const result = readJsonWithFallback(projectRoot, "project-memory.json")
  if (!result.ok) return null
  try {
    const memory = result.data as Record<string, unknown>
    if (!section || section === "all") return memory

    const sectionMap: Record<string, string> = {
      techStack: "techStack",
      build: "build",
      conventions: "conventions",
      structure: "structure",
      notes: "customNotes",
      directives: "userDirectives",
    }
    const key = sectionMap[section] || section
    return memory[key] ?? null
  } catch {
    return null
  }
}

export function writeProjectMemory(
  projectRoot: string,
  memory: Record<string, unknown>,
  merge = false,
): void {
  const p = getProjectMemoryPath(projectRoot)
  ensureDir(dirname(p))

  let finalMemory = { ...memory }
  if (merge && existsSync(p)) {
    try {
      const existing = JSON.parse(readFileSync(p, "utf-8")) as Record<string, unknown>
      finalMemory = { ...existing, ...memory }
      // Preserve user-contributed arrays from existing if not in incoming
      for (const key of ["customNotes", "userDirectives", "hotPaths"]) {
        if (!(key in memory) && key in existing) {
          finalMemory[key] = existing[key]
        }
      }
    } catch {
      // use incoming as-is
    }
  }

  if (!finalMemory.version) finalMemory.version = "1.0.0"
  if (!finalMemory.lastScanned) finalMemory.lastScanned = Date.now()
  if (!finalMemory.projectRoot) finalMemory.projectRoot = projectRoot

  safeWriteJson(p, finalMemory)
}

export function addProjectMemoryNote(
  projectRoot: string,
  category: string,
  content: string,
): void {
  const existing = (readProjectMemory(projectRoot) as Record<string, unknown> | null) || {}
  const notes = Array.isArray(existing.customNotes) ? [...existing.customNotes] : []
  notes.push({ category, content, timestamp: Date.now() })
  writeProjectMemory(projectRoot, { ...existing, customNotes: notes }, false)
}

export function addProjectMemoryDirective(
  projectRoot: string,
  directive: string,
  priority = "normal",
  context = "",
): void {
  const existing = (readProjectMemory(projectRoot) as Record<string, unknown> | null) || {}
  const directives = Array.isArray(existing.userDirectives) ? [...existing.userDirectives] : []
  directives.push({ directive, priority, context, timestamp: Date.now(), source: "explicit" })
  writeProjectMemory(projectRoot, { ...existing, userDirectives: directives }, false)
}

// ============================================================================
// Wiki
// ============================================================================

function getWikiDir(projectRoot: string): string {
  return join(dataDir(projectRoot), "wiki")
}

/** Get the wiki dir, checking .tinycode/ first then .omc/ for reads */
function getWikiDirForRead(projectRoot: string): string | null {
  const primary = getWikiDir(projectRoot)
  if (existsSync(primary)) return primary
  const fallback = join(legacyDir(projectRoot), "wiki")
  if (existsSync(fallback)) return fallback
  return null
}

function ensureWikiDir(projectRoot: string): string {
  const d = getWikiDir(projectRoot)
  ensureDir(d)
  return d
}

interface WikiFrontmatter {
  title: string
  tags: string[]
  created: string
  updated: string
  sources: string[]
  links: string[]
  category: string
  confidence: string
}

function parseYamlArray(val: unknown): string[] {
  if (!val) return []
  if (Array.isArray(val)) return val.map(String)
  if (typeof val === "string") {
    // inline array: [a, b, c]
    const m = val.match(/^\[(.*)\]$/)
    if (m) return m[1].split(",").map((s) => s.trim()).filter(Boolean)
    return val ? [val] : []
  }
  return []
}

function parseSimpleYaml(yaml: string): Record<string, unknown> {
  const result: Record<string, unknown> = {}
  let currentKey = ""
  let inList = false
  const list: string[] = []

  for (const rawLine of yaml.split("\n")) {
    const line = rawLine

    // List item
    if (/^\s+-\s+(.*)/.test(line) && inList) {
      const m = line.match(/^\s+-\s+(.*)/)
      if (m) list.push(m[1].trim())
      continue
    }

    // Flush list
    if (inList) {
      result[currentKey] = list.splice(0)
      inList = false
    }

    // Key-value
    const kv = line.match(/^(\w+):\s*(.*)/)
    if (!kv) continue
    const [, key, value] = kv
    currentKey = key

    if (value.trim() === "") {
      // Possible list follows
      inList = true
    } else {
      result[key] = value.trim().replace(/^["']|["']$/g, "")
    }
  }

  if (inList && currentKey) {
    result[currentKey] = list
  }

  return result
}

function parseFrontmatter(raw: string): { frontmatter: WikiFrontmatter; content: string } | null {
  const normalized = raw.replace(/\r\n/g, "\n")
  const match = normalized.match(/^---\n([\s\S]*?)\n---\n([\s\S]*)$/)
  if (!match) return null

  try {
    const fm = parseSimpleYaml(match[1])
    return {
      frontmatter: {
        title: String(fm.title || ""),
        tags: parseYamlArray(fm.tags),
        created: String(fm.created || new Date().toISOString()),
        updated: String(fm.updated || new Date().toISOString()),
        sources: parseYamlArray(fm.sources),
        links: parseYamlArray(fm.links),
        category: String(fm.category || "reference"),
        confidence: String(fm.confidence || "medium"),
      },
      content: match[2],
    }
  } catch {
    return null
  }
}

function serializePage(frontmatter: WikiFrontmatter, content: string): string {
  const yamlLines = [
    `title: "${frontmatter.title}"`,
    `category: ${frontmatter.category}`,
    `confidence: ${frontmatter.confidence}`,
    `created: ${frontmatter.created}`,
    `updated: ${frontmatter.updated}`,
    `tags:`,
    ...frontmatter.tags.map((t) => `  - ${t}`),
    `sources:`,
    ...frontmatter.sources.map((s) => `  - ${s}`),
    `links:`,
    ...frontmatter.links.map((l) => `  - ${l}`),
  ]
  return `---\n${yamlLines.join("\n")}\n---\n${content}`
}

export function titleToSlug(title: string): string {
  return title
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "")
    .slice(0, 80) + ".md"
}

export function listWikiPages(projectRoot: string): string {
  const wikiDir = getWikiDirForRead(projectRoot)
  if (!wikiDir) return "Wiki is empty. Use wiki_add or wiki_ingest to create pages."

  // Try index.md first
  const indexPath = join(wikiDir, "index.md")
  if (existsSync(indexPath)) {
    try {
      return readFileSync(indexPath, "utf-8")
    } catch {
      // fall through
    }
  }

  // Fallback: list files
  try {
    const reserved = new Set(["index.md", "log.md", "environment.md"])
    const pages = readdirSync(wikiDir).filter((f) => f.endsWith(".md") && !reserved.has(f))
    if (pages.length === 0) return "Wiki is empty. Use wiki_add or wiki_ingest to create pages."
    return `Wiki has ${pages.length} pages but no index:\n${pages.map((p) => `- ${p}`).join("\n")}`
  } catch {
    return "Error listing wiki pages."
  }
}

export function readWikiPage(projectRoot: string, page: string): { found: boolean; text: string } {
  const filename = page.endsWith(".md") ? page : `${page}.md`
  const wikiDir = getWikiDirForRead(projectRoot)
  if (!wikiDir) return { found: false, text: `Wiki page not found: ${filename}` }

  const filePath = join(wikiDir, filename)

  if (!filePath.startsWith(wikiDir + sep)) {
    return { found: false, text: `Invalid page path: ${filename}` }
  }

  if (!existsSync(filePath)) {
    return { found: false, text: `Wiki page not found: ${filename}` }
  }

  try {
    const raw = readFileSync(filePath, "utf-8")
    const parsed = parseFrontmatter(raw)
    if (!parsed) return { found: true, text: raw }

    const { frontmatter: fm, content } = parsed
    const header = [
      `## ${fm.title}`,
      `**Category:** ${fm.category} | **Confidence:** ${fm.confidence} | **Updated:** ${fm.updated}`,
      `**Tags:** ${fm.tags.join(", ")}`,
      fm.links.length > 0 ? `**Links:** ${fm.links.join(", ")}` : "",
      fm.sources.length > 0 ? `**Sources:** ${fm.sources.join(", ")}` : "",
      "",
    ].filter(Boolean).join("\n")

    return { found: true, text: `${header}\n${content}` }
  } catch {
    return { found: false, text: `Error reading wiki page: ${filename}` }
  }
}

export function queryWikiPages(
  projectRoot: string,
  query: string,
  tags?: string[],
  category?: string,
  limit = 20,
): string {
  const wikiDir = getWikiDirForRead(projectRoot)
  if (!wikiDir) return `No wiki pages match "${query}".`

  const reserved = new Set(["index.md", "log.md", "environment.md"])
  let files: string[]
  try {
    files = readdirSync(wikiDir).filter((f) => f.endsWith(".md") && !reserved.has(f))
  } catch {
    return "Error reading wiki directory."
  }

  const queryTokens = query.toLowerCase().split(/\s+/).filter(Boolean)

  interface Match {
    filename: string
    fm: WikiFrontmatter
    score: number
    snippet: string
  }

  const matches: Match[] = []

  for (const file of files) {
    try {
      const raw = readFileSync(join(wikiDir, file), "utf-8")
      const parsed = parseFrontmatter(raw)
      if (!parsed) continue

      const { frontmatter: fm, content } = parsed

      // Category filter
      if (category && fm.category !== category) continue

      // Tag filter (OR)
      if (tags && tags.length > 0) {
        const hasTag = tags.some((t) => fm.tags.includes(t))
        if (!hasTag) continue
      }

      // Score: title + tags + content
      const titleText = fm.title.toLowerCase()
      const tagText = fm.tags.join(" ").toLowerCase()
      const bodyText = content.toLowerCase()

      let score = 0
      for (const token of queryTokens) {
        if (titleText.includes(token)) score += 3
        if (tagText.includes(token)) score += 2
        if (bodyText.includes(token)) score += 1
      }

      if (score === 0) continue

      // Snippet: first matching line
      const lines = content.split("\n")
      const snippetLine = lines.find((l) => queryTokens.some((t) => l.toLowerCase().includes(t)))
      const snippet = snippetLine ? snippetLine.trim().slice(0, 120) : content.trim().slice(0, 120)

      matches.push({ filename: file, fm, score, snippet })
    } catch {
      // skip
    }
  }

  matches.sort((a, b) => b.score - a.score)
  const top = matches.slice(0, limit)

  if (top.length === 0) return `No wiki pages match "${query}".`

  const results = top.map((m, i) =>
    `### ${i + 1}. ${m.fm.title} (${m.fm.category}, ${m.fm.confidence})\n` +
    `**File:** ${m.filename} | **Tags:** ${m.fm.tags.join(", ")} | **Score:** ${m.score}\n` +
    `**Snippet:** ${m.snippet}`
  )

  return `## Wiki Query: "${query}"\n\n${top.length} results:\n\n${results.join("\n\n")}`
}

export function ingestWikiPage(
  projectRoot: string,
  title: string,
  content: string,
  tags: string[],
  category: string,
  confidence = "medium",
  sources: string[] = [],
): { created: string[]; updated: string[]; totalAffected: number } {
  const wikiDir = ensureWikiDir(projectRoot)
  const slug = titleToSlug(title)
  const filePath = join(wikiDir, slug)
  const now = new Date().toISOString()

  const created: string[] = []
  const updated: string[] = []

  if (existsSync(filePath)) {
    // Merge: append to existing content
    const raw = readFileSync(filePath, "utf-8")
    const parsed = parseFrontmatter(raw)
    if (parsed) {
      const { frontmatter: fm, content: existing } = parsed
      fm.updated = now
      fm.tags = [...new Set([...fm.tags, ...tags])]
      if (sources.length > 0) fm.sources = [...new Set([...fm.sources, ...sources])]

      const separator = `\n\n---\n*Updated ${now.slice(0, 10)}*\n\n`
      const merged = existing.trimEnd() + separator + content
      writeFileSync(filePath, serializePage(fm, merged), "utf-8")
      updated.push(slug)
    } else {
      // Can't parse frontmatter -- append raw
      const raw2 = readFileSync(filePath, "utf-8")
      writeFileSync(filePath, raw2.trimEnd() + "\n\n" + content, "utf-8")
      updated.push(slug)
    }
  } else {
    // Create new page
    const fm: WikiFrontmatter = {
      title,
      tags,
      created: now,
      updated: now,
      sources,
      links: [],
      category,
      confidence,
    }
    writeFileSync(filePath, serializePage(fm, content), "utf-8")
    created.push(slug)
  }

  // Update index
  updateWikiIndex(projectRoot)

  return { created, updated, totalAffected: created.length + updated.length }
}

export function deleteWikiPage(projectRoot: string, page: string): boolean {
  const filename = page.endsWith(".md") ? page : `${page}.md`
  const wikiDir = getWikiDir(projectRoot)
  const filePath = join(wikiDir, filename)
  if (!filePath.startsWith(wikiDir + sep)) return false
  if (!existsSync(filePath)) return false
  try {
    unlinkSync(filePath)
    updateWikiIndex(projectRoot)
    return true
  } catch {
    return false
  }
}

function updateWikiIndex(projectRoot: string): void {
  const wikiDir = getWikiDir(projectRoot)
  if (!existsSync(wikiDir)) return
  const reserved = new Set(["index.md", "log.md", "environment.md"])
  try {
    const files = readdirSync(wikiDir).filter((f) => f.endsWith(".md") && !reserved.has(f))
    const entries: string[] = []

    for (const file of files.sort()) {
      try {
        const raw = readFileSync(join(wikiDir, file), "utf-8")
        const parsed = parseFrontmatter(raw)
        if (parsed) {
          const { frontmatter: fm } = parsed
          entries.push(`- **[${fm.title}](${file})** (${fm.category}) -- *${fm.tags.slice(0, 3).join(", ")}*`)
        } else {
          entries.push(`- ${file}`)
        }
      } catch {
        entries.push(`- ${file}`)
      }
    }

    const index = `# Wiki Index\n\n*${files.length} pages -- last updated ${new Date().toISOString().slice(0, 10)}*\n\n${entries.join("\n")}\n`
    writeFileSync(join(wikiDir, "index.md"), index, "utf-8")
  } catch {
    // best effort
  }
}

export function appendWikiLog(
  projectRoot: string,
  operation: string,
  pagesAffected: string[],
  summary: string,
): void {
  const logPath = join(getWikiDir(projectRoot), "log.md")
  const entry = `\n## ${new Date().toISOString()}\n- **Operation:** ${operation}\n- **Pages:** ${pagesAffected.join(", ")}\n- **Summary:** ${summary}\n`
  try {
    ensureDir(dirname(logPath))
    if (existsSync(logPath)) {
      const existing = readFileSync(logPath, "utf-8")
      writeFileSync(logPath, existing + entry, "utf-8")
    } else {
      writeFileSync(logPath, `# Wiki Log\n${entry}`, "utf-8")
    }
  } catch {
    // best effort
  }
}
