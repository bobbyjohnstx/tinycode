import * as vscode from "vscode"
import type { ClientSideConnection, SessionNotification } from "@agentclientprotocol/sdk"

let chatParticipant: vscode.ChatParticipant | undefined

export function registerChatProvider(
  connection: ClientSideConnection,
  sessionId: string,
  outputChannel: vscode.OutputChannel,
  setUpdateHandler: (
    handler: ((params: SessionNotification) => void) | undefined
  ) => void
) {
  if (chatParticipant) {
    chatParticipant.dispose()
  }

  chatParticipant = vscode.chat.createChatParticipant(
    "tinycode",
    async (
      request: vscode.ChatRequest,
      _context: vscode.ChatContext,
      stream: vscode.ChatResponseStream,
      token: vscode.CancellationToken
    ) => {
      try {
        outputChannel.appendLine(`User prompt: ${request.prompt}`)

        const onUpdate = (params: SessionNotification) => {
          if (params.sessionId !== sessionId) {
            return
          }
          const update = params.update as {
            sessionUpdate?: string
            content?: { type?: string; text?: string }
            title?: string
            status?: string
          }
          switch (update.sessionUpdate) {
            case "agent_message_chunk":
            case "user_message_chunk":
              if (update.content?.text) {
                stream.markdown(update.content.text)
              }
              break
            case "agent_thought_chunk":
              if (update.content?.text) {
                stream.progress(update.content.text)
              }
              break
            case "tool_call":
              stream.progress(`Running tool: ${update.title ?? "tool"}`)
              break
            case "tool_call_update":
              if (update.status === "completed") {
                stream.progress(`✓ ${update.title ?? "tool"}`)
              } else if (update.status === "failed" || update.status === "error") {
                stream.progress(`✗ ${update.title ?? "tool"}`)
              }
              break
          }
        }

        setUpdateHandler(onUpdate)

        const cancel = token.onCancellationRequested(() => {
          connection.cancel({ sessionId }).catch((err) => {
            outputChannel.appendLine(`Cancel error: ${err}`)
          })
        })

        try {
          const result = await connection.prompt({
            sessionId,
            prompt: [{ type: "text", text: request.prompt }],
          })
          outputChannel.appendLine(`Prompt finished: ${JSON.stringify(result)}`)
        } finally {
          cancel.dispose()
          setUpdateHandler(undefined)
        }

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
