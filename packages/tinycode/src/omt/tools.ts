/**
 * All 22 omt tool definitions using the plainTool pattern.
 * Adapted from oh-my-tiny/src/plugin/index.ts.
 */

import { join, resolve } from "path"
import { readFileSync, writeFileSync } from "fs"
import { execSync } from "child_process"
import { homedir } from "os"
import {
  readState,
  writeState,
  clearState,
  listActiveStates,
  getStateDir,
  readNotepad,
  writeNotepadPriority,
  appendNotepadWorking,
  appendNotepadManual,
  pruneNotepadWorking,
  getNotepadStats,
  readProjectMemory,
  writeProjectMemory,
  addProjectMemoryNote,
  addProjectMemoryDirective,
  listWikiPages,
  readWikiPage,
  queryWikiPages,
  ingestWikiPage,
  deleteWikiPage,
  appendWikiLog,
  titleToSlug,
} from "./persistence"
import { getSgModule, SUPPORTED_LANGUAGES, toLangEnum, getFilesForLanguage, formatAstMatch } from "./ast-helpers"

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function plainTool(input: {
  description: string
  args: Record<string, any>
  execute: (args: any, ctx: any) => Promise<any>
}) {
  return input
}

const STATE_MODES = [
  "autopilot",
  "autoresearch",
  "team",
  "ralph",
  "ultrawork",
  "ultraqa",
  "deep-interview",
  "self-improve",
  "ralplan",
  "omc-teams",
  "skill-active",
] as const

function validateRoot(wd: string | undefined, fallback: string): string | null {
  if (!wd) return fallback
  const resolved = resolve(wd)
  if (!resolved.startsWith(homedir())) return null
  return resolved
}

function checkSize(val: string, max: number, label: string): string | null {
  const b = Buffer.byteLength(val, "utf-8")
  return b > max ? `${label} exceeds size limit: ${b} bytes (max ${max})` : null
}

const MAX_STATE = 64 * 1024
const MAX_WIKI = 512 * 1024

// ---------------------------------------------------------------------------
// Tool definitions
// ---------------------------------------------------------------------------

export function createTools(dir: string): Record<string, ReturnType<typeof plainTool>> {
  return {
    // ---- State ----------------------------------------------------------------

    omt_state_read: plainTool({
      description: "Read the current state for a specific mode. Returns JSON state data or indicates no state exists.",
      args: {
        mode: { type: "string", enum: STATE_MODES, description: "The mode to read state for" },
        workingDirectory: { type: "string", description: "Override project directory" },
        session_id: { type: "string", description: "Session ID (optional)" },
      },
      async execute(args) {
        const root = validateRoot(args.workingDirectory, dir)
        if (!root) return `workingDirectory must be within your home directory`
        const data = readState(getStateDir(root), args.mode)
        if (data === null) return `No state found for mode: ${args.mode}`
        return `## State for ${args.mode}\n\n\`\`\`json\n${JSON.stringify(data, null, 2)}\n\`\`\``
      },
    }),

    omt_state_write: plainTool({
      description: "Write/update state for a specific mode. Creates state file and directories if needed.",
      args: {
        mode: { type: "string", enum: STATE_MODES, description: "The mode to write state for" },
        workingDirectory: { type: "string" },
        session_id: { type: "string" },
        active: { type: "boolean" },
        iteration: { type: "integer", minimum: 0 },
        max_iterations: { type: "integer", minimum: 1 },
        current_phase: { type: "string" },
        task_description: { type: "string" },
        plan_path: { type: "string" },
        started_at: { type: "string" },
        completed_at: { type: "string" },
        error: { type: "string" },
        state: { type: "object", additionalProperties: true },
      },
      async execute(args) {
        const root = validateRoot(args.workingDirectory, dir)
        if (!root) return `workingDirectory must be within your home directory`
        const sd = getStateDir(root)
        const built: Record<string, unknown> = {}
        if (args.active !== undefined) built.active = args.active
        if (args.iteration !== undefined) built.iteration = args.iteration
        if (args.max_iterations !== undefined) built.max_iterations = args.max_iterations
        if (args.current_phase !== undefined) built.current_phase = args.current_phase
        if (args.task_description !== undefined) built.task_description = args.task_description
        if (args.plan_path !== undefined) built.plan_path = args.plan_path
        if (args.started_at !== undefined) built.started_at = args.started_at
        if (args.completed_at !== undefined) built.completed_at = args.completed_at
        if (args.error !== undefined) built.error = args.error
        if (args.state) for (const [k, v] of Object.entries(args.state)) if (!(k in built)) built[k] = v
        const withMeta = {
          ...built,
          _meta: { mode: args.mode, updatedAt: new Date().toISOString(), updatedBy: "omt_state_write" },
        }
        const err = checkSize(JSON.stringify(withMeta), MAX_STATE, "State payload")
        if (err) return err
        writeState(sd, args.mode, withMeta)
        return `Successfully wrote state for ${args.mode}\n\n\`\`\`json\n${JSON.stringify(withMeta, null, 2)}\n\`\`\``
      },
    }),

    omt_state_clear: plainTool({
      description: "Clear/delete state for a specific mode.",
      args: {
        mode: { type: "string", enum: [...STATE_MODES] },
        workingDirectory: { type: "string" },
        session_id: { type: "string" },
      },
      async execute(args) {
        const root = validateRoot(args.workingDirectory, dir)
        if (!root) return `workingDirectory must be within your home directory`
        const sd = getStateDir(root)
        const cleared = clearState(sd, args.mode)
        return cleared
          ? `Successfully cleared state for mode: ${args.mode}`
          : `No state found to clear for mode: ${args.mode}`
      },
    }),

    omt_state_list_active: plainTool({
      description: "List all currently active modes.",
      args: { workingDirectory: { type: "string" }, session_id: { type: "string" } },
      async execute(args) {
        const root = validateRoot(args.workingDirectory, dir)
        if (!root) return `workingDirectory must be within your home directory`
        const active = listActiveStates(getStateDir(root))
        if (active.length === 0) return "## Active Modes\n\nNo modes are currently active."
        return `## Active Modes (${active.length})\n\n${active.map((m) => `- **${m}**`).join("\n")}`
      },
    }),

    omt_state_get_status: plainTool({
      description: "Get detailed status for a specific mode or all modes.",
      args: {
        mode: { type: "string", enum: [...STATE_MODES] },
        workingDirectory: { type: "string" },
        session_id: { type: "string" },
      },
      async execute(args) {
        const root = validateRoot(args.workingDirectory, dir)
        if (!root) return `workingDirectory must be within your home directory`
        const sd = getStateDir(root)
        if (args.mode) {
          const data = readState(sd, args.mode)
          const active = data !== null && typeof data === "object" && (data as Record<string, unknown>).active === true
          const preview = data ? JSON.stringify(data, null, 2).slice(0, 500) : "No state file"
          return `## Status: ${args.mode}\n\n- **Active:** ${active ? "Yes" : "No"}\n\n\`\`\`json\n${preview}\n\`\`\``
        }
        const lines = ["## All Mode Statuses\n"]
        for (const m of STATE_MODES) {
          const data = readState(sd, m)
          const active = data !== null && typeof data === "object" && (data as Record<string, unknown>).active === true
          lines.push(`${active ? "[ACTIVE]" : "[INACTIVE]"} **${m}**`)
        }
        return lines.join("\n")
      },
    }),

    // ---- Notepad --------------------------------------------------------------

    omt_notepad_read: plainTool({
      description:
        "Read the notepad content. Can read the full notepad or a specific section (priority, working, manual).",
      args: {
        section: { type: "string", enum: [["all", "priority", "working", "manual"]] },
        workingDirectory: { type: "string" },
      },
      async execute(args) {
        const root = validateRoot(args.workingDirectory, dir)
        if (!root) return `workingDirectory must be within your home directory`
        return readNotepad(root, args.section)
      },
    }),

    omt_notepad_write_priority: plainTool({
      description: "Write to the Priority Context section. This REPLACES the existing content. Keep under 500 chars.",
      args: {
        content: { type: "string", description: "Content to write (recommend under 500 chars)" },
        workingDirectory: { type: "string" },
      },
      async execute(args) {
        const root = validateRoot(args.workingDirectory, dir)
        if (!root) return `workingDirectory must be within your home directory`
        const result = writeNotepadPriority(root, args.content)
        if (!result.success) return "Failed to write to Priority Context. Check file permissions."
        let msg = `Successfully wrote to Priority Context (${args.content.length} chars)`
        if (result.warning) msg += `\n\n**Warning:** ${result.warning}`
        return msg
      },
    }),

    omt_notepad_write_working: plainTool({
      description: "Add an entry to Working Memory section. Entries are timestamped and auto-pruned after 7 days.",
      args: {
        content: { type: "string", description: "Content to add as a new entry" },
        workingDirectory: { type: "string" },
      },
      async execute(args) {
        const root = validateRoot(args.workingDirectory, dir)
        if (!root) return `workingDirectory must be within your home directory`
        const ok = appendNotepadWorking(root, args.content)
        return ok
          ? `Successfully added entry to Working Memory (${args.content.length} chars)`
          : "Failed to add entry to Working Memory."
      },
    }),

    omt_notepad_write_manual: plainTool({
      description: "Add a custom note to the MANUAL section. Content in this section is never auto-pruned.",
      args: {
        content: { type: "string", description: "Content to add as a new entry" },
        workingDirectory: { type: "string" },
      },
      async execute(args) {
        const root = validateRoot(args.workingDirectory, dir)
        if (!root) return `workingDirectory must be within your home directory`
        const ok = appendNotepadManual(root, args.content)
        return ok
          ? `Successfully added entry to MANUAL section (${args.content.length} chars)`
          : "Failed to add entry to MANUAL section."
      },
    }),

    omt_notepad_prune: plainTool({
      description: "Prune Working Memory entries older than N days (default: 7 days).",
      args: {
        daysOld: { type: "integer", minimum: 1 },
        workingDirectory: { type: "string" },
      },
      async execute(args) {
        const root = validateRoot(args.workingDirectory, dir)
        if (!root) return `workingDirectory must be within your home directory`
        const result = pruneNotepadWorking(root, args.daysOld ?? 7)
        return `## Prune Results\n\n- Pruned: ${result.pruned} entries\n- Remaining: ${result.remaining} entries\n- Threshold: ${args.daysOld ?? 7} days`
      },
    }),

    omt_notepad_stats: plainTool({
      description: "Get statistics about the notepad (size, entry count, oldest entry).",
      args: { workingDirectory: { type: "string" } },
      async execute(args) {
        const root = validateRoot(args.workingDirectory, dir)
        if (!root) return `workingDirectory must be within your home directory`
        const stats = getNotepadStats(root)
        if (!stats.exists) return "## Notepad Statistics\n\nNotepad does not exist yet."
        return [
          "## Notepad Statistics\n",
          `- **Total Size:** ${stats.totalSize} bytes`,
          `- **Priority Context Size:** ${stats.prioritySize} bytes`,
          `- **Working Memory Entries:** ${stats.workingMemoryEntries}`,
          `- **Oldest Entry:** ${stats.oldestEntry || "None"}`,
          `- **Path:** ${join(root, ".tinycode", "notepad.md")}`,
        ].join("\n")
      },
    }),

    // ---- Project Memory -------------------------------------------------------

    omt_project_memory_read: plainTool({
      description: "Read the project memory. Can read the full memory or a specific section.",
      args: {
        section: {
          type: "string",
          enum: [["all", "techStack", "build", "conventions", "structure", "notes", "directives"]],
        },
        workingDirectory: { type: "string" },
      },
      async execute(args) {
        const root = validateRoot(args.workingDirectory, dir)
        if (!root) return `workingDirectory must be within your home directory`
        const data = readProjectMemory(root, args.section)
        if (data === null)
          return `Project memory does not exist.\nExpected path: ${join(root, ".tinycode", "project-memory.json")}\n\nUse omt_project_memory_write to create it.`
        return `## Project Memory\n\nPath: ${join(root, ".tinycode", "project-memory.json")}\n\n\`\`\`json\n${JSON.stringify(data, null, 2)}\n\`\`\``
      },
    }),

    omt_project_memory_write: plainTool({
      description: "Write/update project memory. Can replace entirely or merge with existing memory.",
      args: {
        memory: { type: "object", additionalProperties: { type: "string" }, description: "The memory object to write" },
        merge: { type: "boolean" },
        workingDirectory: { type: "string" },
      },
      async execute(args) {
        const root = validateRoot(args.workingDirectory, dir)
        if (!root) return `workingDirectory must be within your home directory`
        writeProjectMemory(root, args.memory as Record<string, unknown>, args.merge ?? false)
        return `Successfully ${args.merge ? "merged" : "wrote"} project memory.\nPath: ${join(root, ".tinycode", "project-memory.json")}`
      },
    }),

    omt_project_memory_add_note: plainTool({
      description: "Add a custom note to project memory. Notes are categorized and persisted across sessions.",
      args: {
        category: { type: "string", description: "Note category (e.g., build, test, deploy, env, architecture)" },
        content: { type: "string", description: "Note content" },
        workingDirectory: { type: "string" },
      },
      async execute(args) {
        const root = validateRoot(args.workingDirectory, dir)
        if (!root) return `workingDirectory must be within your home directory`
        const existing = readProjectMemory(root)
        if (existing === null) return "Project memory does not exist. Use omt_project_memory_write to create it first."
        addProjectMemoryNote(root, args.category, args.content)
        return `Successfully added note to project memory.\n\n- **Category:** ${args.category}\n- **Content:** ${args.content}`
      },
    }),

    omt_project_memory_add_directive: plainTool({
      description: "Add a user directive to project memory. Directives are instructions that persist across sessions.",
      args: {
        directive: { type: "string", description: "The directive (e.g., Always use TypeScript strict mode)" },
        priority: { type: "string", enum: [["high", "normal"]] },
        context: { type: "string" },
        workingDirectory: { type: "string" },
      },
      async execute(args) {
        const root = validateRoot(args.workingDirectory, dir)
        if (!root) return `workingDirectory must be within your home directory`
        const existing = readProjectMemory(root)
        if (existing === null) return "Project memory does not exist. Use omt_project_memory_write to create it first."
        addProjectMemoryDirective(root, args.directive, args.priority ?? "normal", args.context ?? "")
        return `Successfully added directive to project memory.\n\n- **Directive:** ${args.directive}\n- **Priority:** ${args.priority ?? "normal"}`
      },
    }),

    // ---- Wiki -----------------------------------------------------------------

    omt_wiki_list: plainTool({
      description: "List all wiki pages with summaries.",
      args: { workingDirectory: { type: "string" } },
      async execute(args) {
        const root = validateRoot(args.workingDirectory, dir)
        if (!root) return `workingDirectory must be within your home directory`
        return listWikiPages(root)
      },
    }),

    omt_wiki_read: plainTool({
      description: "Read a specific wiki page by filename (without .md extension is OK).",
      args: {
        page: { type: "string", description: "Page filename or slug" },
        workingDirectory: { type: "string" },
      },
      async execute(args) {
        const root = validateRoot(args.workingDirectory, dir)
        if (!root) return `workingDirectory must be within your home directory`
        const result = readWikiPage(root, args.page)
        return result.text
      },
    }),

    omt_wiki_query: plainTool({
      description: "Search across all wiki pages by keywords and tags. Returns matching pages with relevance snippets.",
      args: {
        query: { type: "string", description: "Search text (matched against title, tags, and content)" },
        tags: { type: "array", items: { type: "string" } },
        category: {
          type: "string",
          enum: [
            [
              "architecture",
              "decision",
              "pattern",
              "debugging",
              "environment",
              "session-log",
              "reference",
              "convention",
            ],
          ],
        },
        limit: { type: "integer", minimum: 1, maximum: 100 },
        workingDirectory: { type: "string" },
      },
      async execute(args) {
        const root = validateRoot(args.workingDirectory, dir)
        if (!root) return `workingDirectory must be within your home directory`
        return queryWikiPages(root, args.query, args.tags, args.category, args.limit ?? 20)
      },
    }),

    omt_wiki_add: plainTool({
      description: "Quick-add a wiki page. Simpler than wiki_ingest -- creates a single page directly.",
      args: {
        title: { type: "string", description: "Page title (max 200 chars)" },
        content: { type: "string", description: "Page content in markdown (max 50KB)" },
        category: {
          type: "string",
          enum: [
            [
              "architecture",
              "decision",
              "pattern",
              "debugging",
              "environment",
              "session-log",
              "reference",
              "convention",
            ],
          ],
        },
        tags: { type: "array", items: { type: "string" } },
        workingDirectory: { type: "string" },
      },
      async execute(args) {
        const root = validateRoot(args.workingDirectory, dir)
        if (!root) return `workingDirectory must be within your home directory`
        const sizeErr = checkSize(args.content, MAX_WIKI, "Wiki page content")
        if (sizeErr) return sizeErr
        const slug = titleToSlug(args.title)
        const existing = readWikiPage(root, slug)
        if (existing.found)
          return `Page "${slug}" already exists. Use omt_wiki_ingest to merge content, or omt_wiki_delete to remove it first.`
        const result = ingestWikiPage(root, args.title, args.content, args.tags ?? [], args.category ?? "reference")
        return `Wiki page created: ${result.created[0]}\nPath: .tinycode/wiki/${result.created[0]}`
      },
    }),

    omt_wiki_ingest: plainTool({
      description: "Process knowledge into wiki pages. Creates new pages or merges into existing ones.",
      args: {
        title: { type: "string", description: "Page title" },
        content: { type: "string", description: "Markdown content to ingest (max 50KB)" },
        tags: { type: "array", items: { type: "string" }, description: "Searchable tags" },
        category: {
          type: "string",
          enum: [
            [
              "architecture",
              "decision",
              "pattern",
              "debugging",
              "environment",
              "session-log",
              "reference",
              "convention",
            ],
          ],
        },
        confidence: { type: "string", enum: [["high", "medium", "low"]] },
        sources: { type: "array", items: { type: "string" } },
        workingDirectory: { type: "string" },
      },
      async execute(args) {
        const root = validateRoot(args.workingDirectory, dir)
        if (!root) return `workingDirectory must be within your home directory`
        const sizeErr = checkSize(args.content, MAX_WIKI, "Wiki page content")
        if (sizeErr) return sizeErr
        const result = ingestWikiPage(
          root,
          args.title,
          args.content,
          args.tags,
          args.category,
          args.confidence,
          args.sources,
        )
        return `Wiki ingest complete.\n- Created: ${result.created.join(", ") || "none"}\n- Updated: ${result.updated.join(", ") || "none"}\n- Total affected: ${result.totalAffected}`
      },
    }),

    omt_wiki_delete: plainTool({
      description: "Delete a wiki page by filename.",
      args: {
        page: { type: "string", description: "Page filename or slug to delete" },
        workingDirectory: { type: "string" },
      },
      async execute(args) {
        const root = validateRoot(args.workingDirectory, dir)
        if (!root) return `workingDirectory must be within your home directory`
        const filename = args.page.endsWith(".md") ? args.page : `${args.page}.md`
        const deleted = deleteWikiPage(root, args.page)
        if (!deleted) return `Wiki page not found: ${filename}`
        appendWikiLog(root, "delete", [filename], `Deleted page "${filename}"`)
        return `Deleted wiki page: ${filename}`
      },
    }),

    // ---- LSP (stubbed -- uses tinycode's native LSP tool instead) -------------

    omt_lsp_diagnostics: plainTool({
      description:
        "Get language server diagnostics (errors, warnings, hints) for a file. Note: Use tinycode's native 'lsp' tool for full LSP functionality.",
      args: {
        file: { type: "string", description: "Path to the source file" },
        severity: { type: "string", enum: [["error", "warning", "info", "hint"]] },
      },
      async execute(_args) {
        return `omt_lsp_diagnostics is not available as a standalone tool. Use tinycode's native 'lsp' tool instead, which provides full LSP integration including diagnostics, hover, go-to-definition, and more.\n\nExample: Use the 'lsp' tool with operation 'hover' on the file you want to inspect.`
      },
    }),

    omt_lsp_servers: plainTool({
      description: "List all known language servers and their installation status.",
      args: {},
      async execute() {
        const servers = [
          {
            name: "typescript-language-server",
            langs: "TypeScript, JavaScript",
            install: "npm install -g typescript-language-server typescript",
          },
          { name: "pyright", langs: "Python", install: "npm install -g pyright" },
          { name: "gopls", langs: "Go", install: "go install golang.org/x/tools/gopls@latest" },
          { name: "rust-analyzer", langs: "Rust", install: "rustup component add rust-analyzer" },
          { name: "clangd", langs: "C, C++", install: "brew install llvm  OR  apt install clangd" },
        ]
        const rows = servers.map((s) => {
          let found = false
          try {
            execSync(`which ${s.name}`, { stdio: "pipe" })
            found = true
          } catch {
            /* not installed */
          }
          return `${found ? "[OK]" : "[--]"}  ${s.name.padEnd(32)} ${s.langs}${found ? "" : `\n      install: ${s.install}`}`
        })
        return `Language Server Status (${rows.filter((r) => r.startsWith("[OK]")).length}/${servers.length} installed)\n\n${rows.join("\n\n")}`
      },
    }),

    // ---- AST ------------------------------------------------------------------

    omt_ast_grep_search: plainTool({
      description: `Search for code patterns using AST matching. More precise than text search.

Use meta-variables in patterns:
- $NAME - matches any single AST node
- $$$ARGS - matches multiple nodes

Examples: "function $NAME($$$ARGS)", "console.log($MSG)", "$X === null"`,
      args: {
        pattern: { type: "string", description: "AST pattern with meta-variables ($VAR, $$$VARS)" },
        language: { type: "string", enum: [...SUPPORTED_LANGUAGES], description: "Programming language" },
        path: { type: "string", description: "Directory or file to search (default: project root)" },
        context: { type: "integer", minimum: 0, description: "Lines of context around matches (default: 2)" },
        maxResults: { type: "integer", minimum: 1, description: "Maximum results to return (default: 20)" },
      },
      async execute(args) {
        const sg = await getSgModule()
        if (!sg) return "@ast-grep/napi is not available in this environment."
        const searchPath = args.path ?? dir
        const { files, truncated } = getFilesForLanguage(resolve(searchPath), args.language)
        if (files.length === 0) return `No ${args.language} files found in ${searchPath}`
        const results: string[] = []
        let total = 0
        const max = args.maxResults ?? 20
        const ctx = args.context ?? 2
        for (const fp of files) {
          if (total >= max) break
          try {
            const content = readFileSync(fp, "utf-8")
            const matches = sg.parse(toLangEnum(sg, args.language), content).root().findAll(args.pattern)
            for (const match of matches) {
              if (total >= max) break
              const r = match.range()
              results.push(formatAstMatch(fp, r.start.line + 1, r.end.line + 1, ctx, content))
              total++
            }
          } catch {
            /* skip unparseable */
          }
        }
        if (results.length === 0)
          return `No matches found for pattern: ${args.pattern}\n\nSearched ${files.length} ${args.language} file(s) in ${searchPath}`
        const out = `Found ${total} match(es) in ${files.length} file(s)\nPattern: ${args.pattern}\n\n${results.join("\n\n---\n\n")}`
        return truncated ? `${out}\n\nSearch limited to first 1000 files.` : out
      },
    }),

    omt_ast_grep_replace: plainTool({
      description: `Replace code patterns using AST matching. Preserves matched content via meta-variables.

IMPORTANT: dryRun=true (default) only previews changes. Set dryRun=false to apply.`,
      args: {
        pattern: { type: "string", description: "Pattern to match" },
        replacement: { type: "string", description: "Replacement pattern (use same meta-variables)" },
        language: { type: "string", enum: [...SUPPORTED_LANGUAGES], description: "Programming language" },
        path: { type: "string", description: "Directory or file to search (default: project root)" },
        dryRun: { type: "boolean", description: "Preview only, don't apply changes (default: true)" },
      },
      async execute(args) {
        const sg = await getSgModule()
        if (!sg) return "@ast-grep/napi is not available in this environment."
        const searchPath = args.path ?? dir
        const dryRun = args.dryRun !== false
        const { files, truncated } = getFilesForLanguage(resolve(searchPath), args.language)
        if (files.length === 0) return `No ${args.language} files found in ${searchPath}`
        const changes: Array<{ file: string; before: string; after: string; line: number }> = []
        for (const fp of files) {
          try {
            const content = readFileSync(fp, "utf-8")
            const matches = sg.parse(toLangEnum(sg, args.language), content).root().findAll(args.pattern)
            if (matches.length === 0) continue
            const edits: Array<{ start: number; end: number; replacement: string; line: number; before: string }> = []
            for (const match of matches) {
              const r = match.range()
              let rep = args.replacement
              for (const mv of args.replacement.match(/\$\$?\$?[A-Z_][A-Z0-9_]*/g) ?? []) {
                const cap = match.getMatch(mv.replace(/^\$+/, ""))
                if (cap) rep = rep.replaceAll(mv, cap.text().replace(/\$/g, "$$$$"))
              }
              edits.push({
                start: r.start.index,
                end: r.end.index,
                replacement: rep,
                line: r.start.line + 1,
                before: match.text(),
              })
            }
            edits.sort((a, b) => b.start - a.start)
            let newContent = content
            for (const e of edits) {
              newContent = newContent.slice(0, e.start) + e.replacement + newContent.slice(e.end)
              changes.push({ file: fp, before: e.before, after: e.replacement, line: e.line })
            }
            if (!dryRun && edits.length > 0) writeFileSync(fp, newContent, "utf-8")
          } catch {
            /* skip */
          }
        }
        if (changes.length === 0) return `No matches found for pattern: ${args.pattern}`
        const mode = dryRun ? "DRY RUN (no changes applied)" : "CHANGES APPLIED"
        const header = `${mode}\n\nFound ${changes.length} replacement(s)\nPattern: ${args.pattern}\nReplacement: ${args.replacement}\n\n`
        const list = changes
          .slice(0, 50)
          .map((c) => `${c.file}:${c.line}\n  - ${c.before}\n  + ${c.after}`)
          .join("\n\n")
        return (
          header +
          list +
          (changes.length > 50 ? `\n\n... and ${changes.length - 50} more` : "") +
          (dryRun ? "\n\nTo apply changes, set dryRun: false" : "") +
          (truncated ? "\n\nSearch limited to first 1000 files." : "")
        )
      },
    }),
  }
}
