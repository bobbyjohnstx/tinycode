import type { AgentSideConnection, RequestPermissionRequest } from "@agentclientprotocol/sdk"
import type { TinycodeClient, EventPermissionAsked } from "@tinycode/sdk"
import type { ACPSession } from "./session"

type Connection = Pick<AgentSideConnection, "requestPermission">

export class Handler {
  constructor(
    private readonly input: {
      sdk: TinycodeClient
      connection: Connection
      session: ACPSession.Interface
    },
  ) {}

  async handle(event: EventPermissionAsked) {
    const sessionId = event.properties.sessionID
    const toolCallId = event.properties.callID
    const toolName = event.properties.name
    const args = event.properties.args

    const request: RequestPermissionRequest = {
      sessionId,
      toolCall: {
        toolCallId,
        title: toolName,
        kind: "other",
        status: "pending",
        locations: [],
        rawInput: JSON.stringify(args, null, 2),
      },
      options: [
        { id: "allow_once", label: "Allow Once" },
        { id: "allow_always", label: "Allow Always (Session)" },
        { id: "reject_once", label: "Reject" },
      ],
    }

    try {
      const response = await this.input.connection.requestPermission(request)

      let action: "once" | "session" | "reject"
      switch (response.outcome) {
        case "allow_once":
          action = "once"
          break
        case "allow_always":
          action = "session"
          break
        case "reject_once":
        case "cancelled":
          action = "reject"
          break
        default:
          action = "reject"
      }

      await this.input.sdk.session.permission({
        path: { id: sessionId },
        body: {
          action,
          callID: toolCallId,
        },
      })
    } catch (error) {
      await this.input.sdk.session.permission({
        path: { id: sessionId },
        body: {
          action: "reject",
          callID: toolCallId,
        },
      })
    }
  }
}

export * as ACPPermission from "./permission"
