import type { ToolCall, ToolCallContent, ToolCallLocation, ToolCallUpdate, ToolKind } from "@agentclientprotocol/sdk"

export type ToolInput = Record<string, unknown>

export type ToolAttachment = {
  readonly mime?: string
  readonly url?: string
  readonly [key: string]: unknown
}

export type CompletedToolState = {
  readonly status: "completed"
  readonly input: ToolInput
  readonly output: string
  readonly metadata?: unknown
  readonly attachments?: ReadonlyArray<ToolAttachment>
}

export type RunningToolState = {
  readonly status: "running"
  readonly input: ToolInput
  readonly title?: string
}

export type ErrorToolState = {
  readonly status: "error"
  readonly input: ToolInput
  readonly error: string
  readonly metadata?: unknown
}

export function toToolKind(toolName: string): ToolKind {
  const tool = toolName.toLowerCase()

  switch (tool) {
    case "bash":
    case "shell":
      return "terminal"

    case "webfetch":
    case "websearch":
      return "search"

    case "edit":
    case "write":
      return "file"

    case "read":
      return "file"

    case "grep":
    case "glob":
      return "search"

    case "agent":
    case "task":
      return "other"

    default:
      if (tool.startsWith("mcp_")) return "mcp"
      return "other"
  }
}

export function toLocations(toolName: string, input: ToolInput, cwd?: string): ToolCallLocation[] {
  const tool = toolName.toLowerCase()

  switch (tool) {
    case "bash":
    case "shell":
      return cwd ? [{ path: cwd }] : []

    case "read":
    case "edit":
    case "write":
      return locationFrom(input.filePath ?? input.file_path ?? input.filepath)

    case "grep":
    case "glob":
      return locationFrom(input.path ?? cwd)

    default:
      return []
  }
}

function locationFrom(...values: unknown[]): ToolCallLocation[] {
  for (const value of values) {
    if (typeof value === "string" && value) {
      return [{ path: value }]
    }
  }
  return []
}

export function pendingToolCall(input: {
  readonly toolCallId: string
  readonly toolName: string
  readonly state: { readonly input: ToolInput; readonly title?: string }
  readonly cwd?: string
}): ToolCall {
  return {
    toolCallId: input.toolCallId,
    title: input.state.title ?? input.toolName,
    kind: toToolKind(input.toolName),
    status: "pending",
    locations: toLocations(input.toolName, input.state.input, input.cwd),
    rawInput: JSON.stringify(input.state.input, null, 2),
  }
}

export function runningToolUpdate(input: {
  readonly toolCallId: string
  readonly toolName: string
  readonly state: RunningToolState
  readonly output?: string
  readonly cwd?: string
}): ToolCallUpdate {
  const content = input.output
    ? [
        {
          type: "content" as const,
          content: {
            type: "text" as const,
            text: input.output,
          },
        },
      ]
    : []

  return {
    toolCallId: input.toolCallId,
    title: input.state.title ?? input.toolName,
    kind: toToolKind(input.toolName),
    status: "running",
    locations: toLocations(input.toolName, input.state.input, input.cwd),
    rawInput: JSON.stringify(input.state.input, null, 2),
    content,
  }
}

export function completedToolUpdate(input: {
  readonly toolCallId: string
  readonly toolName: string
  readonly state: CompletedToolState
  readonly cwd?: string
}): ToolCallUpdate {
  const content: ToolCallContent[] = [
    {
      type: "content",
      content: {
        type: "text",
        text: input.state.output,
      },
    },
  ]

  return {
    toolCallId: input.toolCallId,
    title: input.toolName,
    kind: toToolKind(input.toolName),
    status: "completed",
    locations: toLocations(input.toolName, input.state.input, input.cwd),
    rawInput: JSON.stringify(input.state.input, null, 2),
    content,
  }
}

export function errorToolUpdate(input: {
  readonly toolCallId: string
  readonly toolName: string
  readonly state: ErrorToolState
  readonly cwd?: string
}): ToolCallUpdate {
  const content: ToolCallContent[] = [
    {
      type: "content",
      content: {
        type: "text",
        text: input.state.error,
      },
    },
  ]

  return {
    toolCallId: input.toolCallId,
    title: input.toolName,
    kind: toToolKind(input.toolName),
    status: "error",
    locations: toLocations(input.toolName, input.state.input, input.cwd),
    rawInput: JSON.stringify(input.state.input, null, 2),
    content,
  }
}

export * as ACPTool from "./tool"
