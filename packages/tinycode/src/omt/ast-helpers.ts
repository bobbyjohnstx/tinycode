/**
 * Shared AST grep helpers used by omt tools (merged from oh-my-tiny).
 * Uses optional @ast-grep/napi with graceful fallback.
 */

import { statSync, readdirSync } from "fs"
import { join, extname, resolve } from "path"
import { createRequire } from "module"

// ============================================================================
// File scan cache -- avoids re-walking the tree on repeated searches
// ============================================================================

const FILE_SCAN_CACHE = new Map<string, { files: string[]; truncated: boolean; expires: number }>()
const FILE_SCAN_TTL_MS = 10_000 // 10 seconds

export function getCachedFiles(dirPath: string, language: string): { files: string[]; truncated: boolean } | null {
  const key = `${dirPath}:${language}`
  const entry = FILE_SCAN_CACHE.get(key)
  if (entry && Date.now() < entry.expires) return { files: entry.files, truncated: entry.truncated }
  FILE_SCAN_CACHE.delete(key)
  return null
}

export function setCachedFiles(dirPath: string, language: string, files: string[], truncated: boolean): void {
  FILE_SCAN_CACHE.set(`${dirPath}:${language}`, { files, truncated, expires: Date.now() + FILE_SCAN_TTL_MS })
}

// ============================================================================
// AST grep -- graceful degradation
// ============================================================================

let sgModule: typeof import("@ast-grep/napi") | null = null

export async function getSgModule(): Promise<typeof import("@ast-grep/napi") | null> {
  if (sgModule) return sgModule
  try {
    const require = createRequire(import.meta.url)
    sgModule = require("@ast-grep/napi") as typeof import("@ast-grep/napi")
    return sgModule
  } catch {
    try {
      sgModule = await import("@ast-grep/napi")
      return sgModule
    } catch {
      return null
    }
  }
}

export const SUPPORTED_LANGUAGES = [
  "javascript",
  "typescript",
  "tsx",
  "python",
  "ruby",
  "go",
  "rust",
  "java",
  "kotlin",
  "swift",
  "c",
  "cpp",
  "csharp",
  "html",
  "css",
  "json",
  "yaml",
] as const

export const EXT_TO_LANG: Record<string, string> = {
  ".js": "javascript",
  ".mjs": "javascript",
  ".cjs": "javascript",
  ".jsx": "javascript",
  ".ts": "typescript",
  ".mts": "typescript",
  ".cts": "typescript",
  ".tsx": "tsx",
  ".py": "python",
  ".rb": "ruby",
  ".go": "go",
  ".rs": "rust",
  ".java": "java",
  ".kt": "kotlin",
  ".kts": "kotlin",
  ".swift": "swift",
  ".c": "c",
  ".h": "c",
  ".cpp": "cpp",
  ".cc": "cpp",
  ".cxx": "cpp",
  ".hpp": "cpp",
  ".cs": "csharp",
  ".html": "html",
  ".htm": "html",
  ".css": "css",
  ".json": "json",
  ".yaml": "yaml",
  ".yml": "yaml",
}

/**
 * Map a language name to the value expected by `sg.parse()`.
 * In @ast-grep/napi v0.43+ only a handful of languages ship as enum members;
 * the rest are passed as plain capitalized strings (NapiLang = Lang | string).
 */
export function toLangEnum(_sg: typeof import("@ast-grep/napi"), language: string): string {
  const langMap: Record<string, string> = {
    javascript: "JavaScript",
    typescript: "TypeScript",
    tsx: "Tsx",
    python: "Python",
    ruby: "Ruby",
    go: "Go",
    rust: "Rust",
    java: "Java",
    kotlin: "Kotlin",
    swift: "Swift",
    c: "C",
    cpp: "Cpp",
    csharp: "CSharp",
    html: "Html",
    css: "Css",
    json: "Json",
    yaml: "Yaml",
  }
  const lang = langMap[language]
  if (!lang) throw new Error(`Unsupported language: ${language}`)
  return lang
}

const SKIP_DIRS = new Set(["node_modules", ".git", "dist", "build", "__pycache__", ".venv", "venv"])

export function getFilesForLanguage(
  dirPath: string,
  language: string,
  maxFiles = 1000,
): { files: string[]; truncated: boolean } {
  const resolved = resolve(dirPath)
  try {
    const stat = statSync(resolved)
    if (stat.isFile()) return { files: [resolved], truncated: false }
  } catch (err) {
    throw new Error(`Cannot access path "${resolved}": ${(err as Error).message}`)
  }

  const cached = getCachedFiles(resolved, language)
  if (cached) return cached

  const extensions = Object.entries(EXT_TO_LANG)
    .filter(([, lang]) => lang === language)
    .map(([ext]) => ext)

  const files: string[] = []

  function walk(dir: string) {
    if (files.length >= maxFiles) return
    try {
      const entries = readdirSync(dir, { withFileTypes: true })
      for (const entry of entries) {
        if (files.length >= maxFiles) break
        const fullPath = join(dir, entry.name)
        if (entry.isDirectory()) {
          if (!SKIP_DIRS.has(entry.name)) walk(fullPath)
        } else if (entry.isFile()) {
          if (extensions.includes(extname(entry.name).toLowerCase())) files.push(fullPath)
        }
      }
    } catch {
      // ignore permission errors
    }
  }

  walk(resolved)
  const truncated = files.length >= maxFiles
  setCachedFiles(resolved, language, files, truncated)
  return { files, truncated }
}

export function formatAstMatch(
  filePath: string,
  startLine: number,
  endLine: number,
  context: number,
  fileContent: string,
): string {
  const lines = fileContent.split("\n")
  const contextStart = Math.max(0, startLine - context - 1)
  const contextEnd = Math.min(lines.length, endLine + context)
  const contextLines = lines.slice(contextStart, contextEnd)
  const numbered = contextLines.map((line, i) => {
    const lineNum = contextStart + i + 1
    const isMatch = lineNum >= startLine && lineNum <= endLine
    return `${isMatch ? ">" : " "} ${lineNum.toString().padStart(4)}: ${line}`
  })
  return `${filePath}:${startLine}\n${numbered.join("\n")}`
}
