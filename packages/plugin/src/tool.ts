import { z } from "zod"

export type ToolContext = {
  sessionID: string
  messageID: string
  agent: string
  /**
   * Current project directory for this session.
   * Prefer this over process.cwd() when resolving relative paths.
   */
  directory: string
  /**
   * Project worktree root for this session.
   * Useful for generating stable relative paths (e.g. path.relative(worktree, absPath)).
   */
  worktree: string
  abort: AbortSignal
  metadata(input: { title?: string; metadata?: { [key: string]: any } }): void
  ask(input: AskInput): Promise<void>
  /**
   * Emit a progress message during tool execution.
   * Updates the tool call's metadata with the latest progress text.
   * Safe to call even if no listener exists (no-op by default).
   */
  progress: (message: string) => void
  /**
   * Read-only snapshot of the conversation history for this session.
   * Returns a simplified view of messages with role and text content.
   */
  messages: () => Promise<ReadonlyArray<{ role: string; content: string }>>
  /**
   * Read-only session metadata: session ID, current model, and active agent.
   */
  sessionInfo: () => Promise<{ id: string; model: string; agent: string }>
}

type AskInput = {
  permission: string
  patterns: string[]
  always: string[]
  metadata: { [key: string]: any }
}

export type ToolAttachment = {
  type: "file"
  mime: string
  url: string
  filename?: string
}

export type ToolResult =
  | string
  | {
      title?: string
      output: string
      metadata?: { [key: string]: any }
      attachments?: ToolAttachment[]
    }

export function tool<Args extends z.ZodRawShape>(input: {
  description: string
  args: Args
  execute(args: z.infer<z.ZodObject<Args>>, context: ToolContext): Promise<ToolResult>
}) {
  return input
}
tool.schema = z

export type ToolDefinition = ReturnType<typeof tool>
