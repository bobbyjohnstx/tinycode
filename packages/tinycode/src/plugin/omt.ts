/**
 * Internal omt plugin -- registers all 22 omt tools + hooks natively.
 * Replaces the external oh-my-tiny plugin dependency.
 */

import type { Plugin as PluginInstance } from "@tinycode/plugin"
import { createTools, readNotepad, readProjectMemory } from "../omt"

export const OmtPlugin: PluginInstance = async (input) => {
  const dir = input.directory

  return {
    tool: createTools(dir),

    // Auto-approve omt_* tool calls -- they only read/write local .tinycode/ files
    "permission.ask": async (input: any, output: any) => {
      const title = input.title ?? ""
      const pattern = Array.isArray(input.pattern) ? input.pattern.join(" ") : (input.pattern ?? "")
      if (title.startsWith("omt_") || pattern.startsWith("omt_")) {
        output.status = "allow"
      }
    },

    // Inject notepad priority context and project memory into every system prompt
    "experimental.chat.system.transform": async (_input: any, output: any) => {
      try {
        const priority = readNotepad(dir, "priority")
        const mem = readProjectMemory(dir)
        // Only inject if content is non-trivial
        if (priority && !priority.includes("(Empty or notepad does not exist)") && !priority.includes("Notepad does not exist")) {
          output.system.push(`<omt-priority-context>\n${priority}\n</omt-priority-context>`)
        }
        if (mem && typeof mem === "object" && Object.keys(mem).length > 0) {
          const memStr = JSON.stringify(mem, null, 2)
          output.system.push(`<omt-project-memory>\n${memStr}\n</omt-project-memory>`)
        }
      } catch {
        // silently skip on any error -- hooks must not crash the session
      }
    },
  }
}
