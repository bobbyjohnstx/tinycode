import * as vscode from "vscode"
import { spawn, type ChildProcess } from "child_process"
import { ClientSideConnection, ndJsonStream } from "@agentclientprotocol/sdk"
import type { AgentClientProtocol } from "@agentclientprotocol/sdk"
import { registerChatProvider } from "./chat-provider"

let childProcess: ChildProcess | undefined
let connection: ClientSideConnection | undefined
let outputChannel: vscode.OutputChannel | undefined
let sessionId: string | undefined

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

  // Auto-start if workspace is open
  const workspaceFolder = vscode.workspace.workspaceFolders?.[0]
  if (workspaceFolder) {
    startAgent(workspaceFolder.uri.fsPath)
  }
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

    // Log stderr to output channel
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

    // Create ACP connection
    const stream = ndJsonStream(childProcess.stdout, childProcess.stdin)
    connection = new ClientSideConnection(stream)

    // Initialize the connection
    const initResult = await connection.initialize({
      protocolVersion: "0.1.0",
      capabilities: {
        supportsPermissions: true,
      },
      clientInfo: {
        name: "vscode-tinycode",
        version: "0.1.0",
      },
    })

    outputChannel?.appendLine(
      `Connected to tinycode server: ${JSON.stringify(initResult.serverInfo)}`
    )

    // Create a new session
    const sessionResult = await connection.newSession({
      cwd,
    })

    sessionId = sessionResult.sessionId
    outputChannel?.appendLine(`Session created: ${sessionId}`)

    // Register chat provider
    registerChatProvider(connection, sessionId, outputChannel)

    vscode.window.showInformationMessage("tinycode AI assistant started")
  } catch (error) {
    outputChannel?.appendLine(`Error: ${error}`)
    vscode.window.showErrorMessage(
      `Failed to start tinycode: ${error instanceof Error ? error.message : String(error)}`
    )
    cleanup()
  }
}

function stopAgent() {
  if (!childProcess) {
    vscode.window.showInformationMessage("tinycode is not running")
    return
  }

  outputChannel?.appendLine("Stopping tinycode")
  cleanup()
  vscode.window.showInformationMessage("tinycode AI assistant stopped")
}

function cleanup() {
  if (childProcess) {
    childProcess.kill()
    childProcess = undefined
  }
  connection = undefined
  sessionId = undefined
}

export function deactivate() {
  cleanup()
  outputChannel?.dispose()
}
