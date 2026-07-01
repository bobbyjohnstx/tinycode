import type { AgentSideConnection, RequestPermissionRequest, RequestPermissionOutcome } from "@agentclientprotocol/sdk"
import type { TinycodeClient, Permission } from "@tinycode/sdk"
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

  async handle(event: Permission) {
    const sessionId = event.sessionID
    const toolCallId = event.callID ?? ""
    const toolName = event.title
    const args = event.metadata

    const request: RequestPermissionRequest = {
      sessionId,
      toolCall: {
        toolCallId,
        title: toolName,
        kind: "other" as const,
        status: "pending",
        locations: [],
        rawInput: JSON.stringify(args, null, 2),
      },
      options: [
        { label: "Allow Once" } as any,
        { label: "Allow Always (Session)" } as any,
        { label: "Reject" } as any,
      ],
    }

    try {
      const response = await this.input.connection.requestPermission(request)

      let action: "once" | "always" | "reject"
      const outcome = response.outcome as unknown as RequestPermissionOutcome
      if (outcome === ("allow_once" as unknown as RequestPermissionOutcome)) {
        action = "once"
      } else if (outcome === ("allow_always" as unknown as RequestPermissionOutcome)) {
        action = "always"
      } else if (outcome === ("reject_once" as unknown as RequestPermissionOutcome) || outcome === ("cancelled" as unknown as RequestPermissionOutcome)) {
        action = "reject"
      } else {
        action = "reject"
      }

      await this.input.sdk.postSessionIdPermissionsPermissionId({
        path: { id: sessionId, permissionID: event.id },
        body: {
          response: action,
        },
      })
    } catch (error) {
      await this.input.sdk.postSessionIdPermissionsPermissionId({
        path: { id: sessionId, permissionID: event.id },
        body: {
          response: "reject",
        },
      })
    }
  }
}

export * as ACPPermission from "./permission"
