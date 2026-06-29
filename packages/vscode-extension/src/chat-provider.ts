import * as vscode from "vscode"
import type { ClientSideConnection } from "@agentclientprotocol/sdk"
import type { AgentClientProtocol } from "@agentclientprotocol/sdk"

let chatParticipant: vscode.ChatParticipant | undefined

export function registerChatProvider(
  connection: ClientSideConnection,
  sessionId: string,
  outputChannel: vscode.OutputChannel
) {
  // Unregister existing participant if any
  if (chatParticipant) {
    chatParticipant.dispose()
  }

  chatParticipant = vscode.chat.createChatParticipant(
    "tinycode",
    async (
      request: vscode.ChatRequest,
      context: vscode.ChatContext,
      stream: vscode.ChatResponseStream,
      token: vscode.CancellationToken
    ) => {
      try {
        outputChannel.appendLine(
          `User prompt: ${request.prompt}`
        )

        // Listen for session updates
        const unsubscribe = connection.onNotification(
          "sessionUpdate",
          (params: AgentClientProtocol.SessionUpdateNotification) => {
            if (params.sessionId !== sessionId) {
              return
            }

            for (const event of params.events) {
              switch (event.type) {
                case "message-part": {
                  if (event.part.type === "text") {
                    stream.markdown(event.part.text)
                  }
                  break
                }

                case "tool-call-started": {
                  stream.progress(`Running tool: ${event.name}`)
                  break
                }

                case "tool-call-completed": {
                  if (event.status === "success") {
                    stream.progress(`✓ ${event.name}`)
                  } else {
                    stream.progress(`✗ ${event.name}: ${event.error}`)
                  }
                  break
                }

                case "request-permission": {
                  handlePermissionRequest(
                    connection,
                    sessionId,
                    event,
                    outputChannel
                  ).catch((error) => {
                    outputChannel.appendLine(
                      `Permission request error: ${error}`
                    )
                  })
                  break
                }

                default:
                  // Ignore other event types
                  break
              }
            }
          }
        )

        // Send prompt
        await connection.prompt({
          sessionId,
          prompt: [
            {
              type: "text",
              text: request.prompt,
            },
          ],
        })

        // Keep the subscription active until the response completes
        // In a real implementation, we'd want to track response completion
        // and unsubscribe appropriately
        return {
          metadata: {
            command: "tinycode",
          },
        }
      } catch (error) {
        outputChannel.appendLine(`Chat error: ${error}`)
        stream.markdown(
          `Error: ${error instanceof Error ? error.message : String(error)}`
        )
        throw error
      }
    }
  )

  chatParticipant.iconPath = vscode.Uri.parse(
    "data:image/svg+xml;base64,PHN2ZyB3aWR0aD0iMTYiIGhlaWdodD0iMTYiIHZpZXdCb3g9IjAgMCAxNiAxNiIgZmlsbD0ibm9uZSIgeG1sbnM9Imh0dHA6Ly93d3cudzMub3JnLzIwMDAvc3ZnIj4KICA8cmVjdCB3aWR0aD0iMTYiIGhlaWdodD0iMTYiIGZpbGw9IiMwMDdiZmYiLz4KICA8cGF0aCBkPSJNNCAzaDhMOCA4IDQgM3oiIGZpbGw9IndoaXRlIi8+Cjwvc3ZnPgo="
  )
}

async function handlePermissionRequest(
  connection: ClientSideConnection,
  sessionId: string,
  event: Extract<
    AgentClientProtocol.SessionEvent,
    { type: "request-permission" }
  >,
  outputChannel: vscode.OutputChannel
) {
  const toolName = event.tool
  const description =
    event.description || `Allow tinycode to use ${toolName}?`

  outputChannel.appendLine(`Permission request: ${toolName}`)
  outputChannel.appendLine(`Description: ${description}`)

  const items = [
    { label: "Allow Once", value: "allow-once" as const },
    { label: "Allow Always", value: "allow-always" as const },
    { label: "Deny", value: "deny" as const },
  ]

  const selected = await vscode.window.showQuickPick(items, {
    placeHolder: description,
    title: `tinycode: ${toolName}`,
  })

  if (!selected) {
    // User cancelled, treat as deny
    await connection.respondToPermissionRequest({
      sessionId,
      requestId: event.requestId,
      decision: "deny",
    })
    return
  }

  await connection.respondToPermissionRequest({
    sessionId,
    requestId: event.requestId,
    decision: selected.value,
  })

  outputChannel.appendLine(`Permission decision: ${selected.value}`)
}
