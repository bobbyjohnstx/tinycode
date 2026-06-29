import type { AgentSideConnection } from "@agentclientprotocol/sdk"
import type { TinycodeClient, Event, EventMessagePartUpdated, Part, ToolPart } from "@tinycode/sdk"
import { Effect } from "effect"
import { ACPSession } from "./session"
import { ACPPermission } from "./permission"
import { partsToContentChunks, type ReplayPart } from "./content"
import { pendingToolCall, runningToolUpdate, completedToolUpdate, errorToolUpdate } from "./tool"

type Connection = Pick<AgentSideConnection, "sessionUpdate"> &
  Partial<Pick<AgentSideConnection, "requestPermission" | "writeTextFile">>
type GlobalEventEnvelope = {
  payload?: Event
}
type GlobalEventStream = {
  stream: AsyncIterable<GlobalEventEnvelope>
}

export function start(input: { sdk: TinycodeClient; connection: Connection; session: ACPSession.Interface }) {
  const subscription = new Subscription(input)
  subscription.start()
  return subscription
}

export class Subscription {
  private readonly abort = new AbortController()
  private readonly toolStarts = new Set<string>()
  private readonly permission: ACPPermission.Handler
  private started = false

  constructor(
    private readonly input: {
      sdk: TinycodeClient
      connection: Connection
      session: ACPSession.Interface
    },
  ) {
    this.permission = new ACPPermission.Handler(input)
  }

  start() {
    if (this.started) return
    this.started = true
    this.run().catch(() => {
      if (this.abort.signal.aborted) return
    })
  }

  stop() {
    this.abort.abort()
  }

  async handle(event: Event) {
    switch (event.type) {
      case "permission.asked":
        this.permission.handle(event)
        return
      case "message.part.updated":
        return this.handlePartUpdated(event)
    }
  }

  async replayMessage(message: { info: { sessionID: string; id: string; role: string }; parts: Part[] }) {
    if (message.info.role !== "assistant" && message.info.role !== "user") return

    for (const part of message.parts) {
      await this.recordFetchedPart(message.info.sessionID, message, part)
      if (part.type === "tool") {
        await this.handleToolPart(message.info.sessionID, part, process.cwd())
        continue
      }
      await this.replayContentPart(message, part as ReplayPart)
    }
  }

  private async replayContentPart(
    message: { info: { sessionID: string; id: string; role: string } },
    part: ReplayPart,
  ) {
    if (part.type !== "text" && part.type !== "file" && part.type !== "reasoning") return

    const sessionUpdate =
      part.type === "reasoning" ? "agent_thought_chunk" : message.info.role === "user" ? "user_message_chunk" : "agent_message_chunk"

    for (const chunk of partsToContentChunks([part])) {
      await this.input.connection.sessionUpdate({
        sessionId: message.info.sessionID,
        update: {
          sessionUpdate,
          messageId: message.info.id,
          ...chunk,
        },
      })
    }
  }

  private async run() {
    while (!this.abort.signal.aborted) {
      const events = (await this.input.sdk.global.event({
        signal: this.abort.signal,
      })) as GlobalEventStream

      for await (const event of events.stream) {
        if (this.abort.signal.aborted) return
        if (!event.payload) continue
        await this.handle(event.payload).catch(() => {})
      }
      if (!this.abort.signal.aborted) await new Promise((resolve) => setTimeout(resolve, 1000))
    }
  }

  private async handlePartUpdated(event: EventMessagePartUpdated) {
    const part = event.properties.part
    const sessionId = part.sessionID || ""
    const session = await Effect.runPromise(this.input.session.tryGet(sessionId))
    if (!session) return

    await Effect.runPromise(
      this.input.session.recordPartMetadata({
        sessionId: session.id,
        messageId: part.messageID,
        partId: part.id,
        partType: part.type,
        role: part.type === "reasoning" ? "assistant" : undefined,
        ignored: part.type === "text" ? part.ignored : undefined,
        toolCallId: part.type === "tool" ? part.callID : undefined,
      }),
    )
    if (part.type === "tool") {
      await this.handleToolPart(session.id, part, session.cwd)
    }

    if (part.type === "text" && !part.ignored) {
      await this.input.connection.sessionUpdate({
        sessionId: session.id,
        update: {
          sessionUpdate: "agent_message_chunk",
          messageId: part.messageID,
          content: {
            type: "text",
            text: event.properties.delta ?? part.text,
          },
        },
      })
    }

    if (part.type === "reasoning") {
      await this.input.connection.sessionUpdate({
        sessionId: session.id,
        update: {
          sessionUpdate: "agent_thought_chunk",
          messageId: part.messageID,
          content: {
            type: "text",
            text: event.properties.delta ?? part.text,
          },
        },
      })
    }
  }

  private async handleToolPart(sessionId: string, part: ToolPart, cwd: string) {
    const key = `${sessionId}:${part.callID}`

    if (part.status === "pending" && !this.toolStarts.has(key)) {
      this.toolStarts.add(key)
      const toolCall = pendingToolCall({
        toolCallId: part.callID,
        toolName: part.name,
        state: { input: part.args as Record<string, unknown> },
        cwd,
      })
      await this.input.connection.sessionUpdate({
        sessionId,
        update: {
          sessionUpdate: "tool_call",
          messageId: part.messageID,
          toolCall,
        },
      })
    }

    if (part.status === "running") {
      const toolUpdate = runningToolUpdate({
        toolCallId: part.callID,
        toolName: part.name,
        state: { status: "running", input: part.args as Record<string, unknown> },
        output: part.result?.output,
        cwd,
      })
      await this.input.connection.sessionUpdate({
        sessionId,
        update: {
          sessionUpdate: "tool_call_update",
          messageId: part.messageID,
          toolCallUpdate: toolUpdate,
        },
      })
    }

    if (part.status === "completed") {
      const toolUpdate = completedToolUpdate({
        toolCallId: part.callID,
        toolName: part.name,
        state: {
          status: "completed",
          input: part.args as Record<string, unknown>,
          output: part.result?.output ?? "",
        },
        cwd,
      })
      await this.input.connection.sessionUpdate({
        sessionId,
        update: {
          sessionUpdate: "tool_call_update",
          messageId: part.messageID,
          toolCallUpdate: toolUpdate,
        },
      })
      this.toolStarts.delete(key)
    }

    if (part.status === "error") {
      const toolUpdate = errorToolUpdate({
        toolCallId: part.callID,
        toolName: part.name,
        state: {
          status: "error",
          input: part.args as Record<string, unknown>,
          error: part.result?.output ?? "Unknown error",
        },
        cwd,
      })
      await this.input.connection.sessionUpdate({
        sessionId,
        update: {
          sessionUpdate: "tool_call_update",
          messageId: part.messageID,
          toolCallUpdate: toolUpdate,
        },
      })
      this.toolStarts.delete(key)
    }
  }

  private async recordFetchedPart(
    sessionId: string,
    message: { info: { id: string; role: string } },
    part: Part,
  ) {
    await Effect.runPromise(
      this.input.session.recordPartMetadata({
        sessionId,
        messageId: message.info.id,
        partId: part.id,
        partType: part.type,
        role: message.info.role,
        ignored: part.type === "text" ? part.ignored : undefined,
        toolCallId: part.type === "tool" ? part.callID : undefined,
      }),
    )
  }
}

export * as ACPEvent from "./event"
