import * as vscode from "vscode"
import { spawn, type ChildProcess } from "child_process"
import { ClientSideConnection, ndJsonStream } from "@agentclientprotocol/sdk"
import type {
  RequestPermissionRequest,
  RequestPermissionResponse,
  SessionNotification,
} from "@agentclientprotocol/sdk"
import { registerChatProvider } from "./chat-provider"

let childProcess: ChildProcess | undefined
let connection: ClientSideConnection | undefined
let outputChannel: vscode.OutputChannel | undefined
let sessionId: string | undefined
let latestUpdateHandler: ((params: SessionNotification) => void) | undefined

export function activate(context: vscode.ExtensionContext) {
  outputChannel = vscode.window.createOutputChannel("tinycode")
  outputChannel.appendLine("tinycode extension activated")

  context.subscriptions.push(
    vscode.commands.registerCommand("tinycode.start", () => {
      const workspaceFolder = vscode.workspace.workspaceFolders?.[0]
      if (!workspaceFolder) {
        vscode.window.showErrorMessage(
          "No workspace folder open. Please open a folder first."
        )
        return
      }
      startAgent(workspaceFolder.uri.fsPath)
    })
  )

  context.subscriptions.push(
    vscode.commands.registerCommand("tinycode.stop", () => {
      stopAgent()
    })
  )

  const workspaceFolder = vscode.workspace.workspaceFolders?.[0]
  if (workspaceFolder) {
    startAgent(workspaceFolder.uri.fsPath)
  }
}

export function setSessionUpdateHandler(
  handler: ((params: SessionNotification) => void) | undefined
) {
  latestUpdateHandler = handler
}

async function startAgent(cwd: string) {
  if (childProcess) {
    outputChannel?.appendLine("tinycode is already running")
    return
  }

  const config = vscode.workspace.getConfiguration("tinycode")
  const tinycodePath = config.get<string>("path") || "tinycode"

  outputChannel?.appendLine(`Starting tinycode from ${tinycodePath}`)
  outputChannel?.appendLine(`Working directory: ${cwd}`)

  try {
    childProcess = spawn(tinycodePath, ["acp", "--cwd", cwd], {
      stdio: ["pipe", "pipe", "pipe"],
    })

    if (!childProcess.stdout || !childProcess.stdin) {
      throw new Error("Failed to create stdio streams")
    }

    childProcess.stderr?.on("data", (data) => {
      outputChannel?.appendLine(`[stderr] ${data.toString()}`)
    })

    childProcess.on("error", (error) => {
      outputChannel?.appendLine(`Process error: ${error.message}`)
      vscode.window.showErrorMessage(
        `Failed to start tinycode: ${error.message}`
      )
      cleanup()
    })

    childProcess.on("exit", (code, signal) => {
      outputChannel?.appendLine(
        `Process exited with code ${code} and signal ${signal}`
      )
      cleanup()
    })

    const stream = ndJsonStream(childProcess.stdout, childProcess.stdin)
    connection = new ClientSideConnection((_agent) => {
      return {
        async sessionUpdate(params: SessionNotification) {
          latestUpdateHandler?.(params)
        },
        async requestPermission(
          params: RequestPermissionRequest
        ): Promise<RequestPermissionResponse> {
          return handlePermissionRequest(params)
        },
      }
    }, stream)

    const initResult = await connection.initialize({
      protocolVersion: 1,
      clientCapabilities: {},
      clientInfo: {
        name: "vscode-tinycode",
        version: "0.1.0",
      },
    })

    outputChannel?.appendLine(
      `Connected to tinycode: ${JSON.stringify(initResult.agentInfo ?? initResult)}`
    )

    const sessionResult = await connection.newSession({
      cwd,
      mcpServers: [],
    })

    sessionId = sessionResult.sessionId
    outputChannel?.appendLine(`Session created: ${sessionId}`)

    registerChatProvider(connection, sessionId, outputChannel, setSessionUpdateHandler)

    vscode.window.showInformationMessage("tinycode AI assistant started")
  } catch (error) {
    outputChannel?.appendLine(`Error: ${error}`)
    vscode.window.showErrorMessage(
      `Failed to start tinycode: ${error instanceof Error ? error.message : String(error)}`
    )
    cleanup()
  }
}

async function handlePermissionRequest(
  params: RequestPermissionRequest
): Promise<RequestPermissionResponse> {
  const title = params.toolCall?.title || "tool"
  outputChannel?.appendLine(`Permission request: ${title}`)

  const items = [
    { label: "Allow Once", optionId: "allow_once" },
    { label: "Allow Always", optionId: "allow_always" },
    { label: "Reject", optionId: "reject_once" },
  ]

  const selected = await vscode.window.showQuickPick(items, {
    placeHolder: `Allow tinycode to use ${title}?`,
    title: `tinycode: ${title}`,
  })

  if (!selected) {
    return { outcome: { outcome: "cancelled" } }
  }

  outputChannel?.appendLine(`Permission decision: ${selected.optionId}`)
  return {
    outcome: {
      outcome: "selected",
      optionId: selected.optionId,
    },
  }
}

function stopAgent() {
  if (childProcess) {
    childProcess.kill()
    cleanup()
    vscode.window.showInformationMessage("tinycode AI assistant stopped")
  }
}

function cleanup() {
  childProcess = undefined
  connection = undefined
  sessionId = undefined
  latestUpdateHandler = undefined
}

export function deactivate() {
  stopAgent()
}
