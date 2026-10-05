import type { Message, Part } from "@tinycode/sdk/v2/client"
import { showToast } from "@tinycode/ui/toast"
import { askBtw, lastBtwAnswer } from "@/utils/session-btw"
import {
  downloadBlob,
  formatTranscript,
  formatTranscriptHtml,
  sessionSlug,
  type ExportMessage,
  type ExportPart,
  type ExportSession,
} from "@/utils/session-export"
import {
  buildContinueGoalPrompt,
  buildInitialGoalPrompt,
  clearGoal,
  detectEcosystem,
  getGoal,
  goalStatusText,
  recordGoalOutput,
  resolveGoalCommand,
  setGoal,
} from "@/utils/session-goal"
import { Identifier } from "@/utils/id"

export type ClientSlashKind = "btw" | "goal" | "export" | "export-html"

export function matchClientSlash(text: string): { kind: ClientSlashKind; args: string } | undefined {
  const trimmed = text.trim()
  if (trimmed === "/export html" || trimmed === "/export-html" || trimmed.startsWith("/export-html ")) {
    return { kind: "export-html", args: "" }
  }
  if (trimmed === "/export" || trimmed.startsWith("/export ")) {
    const rest = trimmed.slice("/export".length).trim()
    if (rest === "html") return { kind: "export-html", args: "" }
    return { kind: "export", args: rest }
  }
  if (trimmed === "/btw" || trimmed.startsWith("/btw ")) {
    return { kind: "btw", args: trimmed.slice("/btw".length).trim() }
  }
  if (trimmed === "/goal" || trimmed.startsWith("/goal ")) {
    return { kind: "goal", args: trimmed.slice("/goal".length).trim() }
  }
  return undefined
}

function toExportMessages(
  messages: Message[],
  partsByMessage: Record<string, Part[] | undefined>,
): ExportMessage[] {
  return messages.map((msg) => {
    const parts = (partsByMessage[msg.id] ?? []).flatMap((part): ExportPart[] => {
      if (part.type === "text") return [{ type: "text", text: part.text }]
      if (part.type === "reasoning") return [{ type: "reasoning", text: part.text }]
      if (part.type === "tool") {
        const state = part.state
        if (state.status === "completed") {
          return [{ type: "tool", tool: part.tool, title: state.title, output: state.output, status: state.status }]
        }
        if (state.status === "error") {
          return [{ type: "tool", tool: part.tool, error: state.error, status: state.status }]
        }
        return [{ type: "tool", tool: part.tool, status: state.status }]
      }
      return []
    })
    return {
      role: msg.role,
      agent: "agent" in msg ? msg.agent : undefined,
      modelID: "model" in msg && msg.model ? msg.model.modelID : undefined,
      parts,
    }
  })
}

export function exportSessionTranscript(input: {
  format: "md" | "html"
  session: ExportSession
  messages: Message[]
  parts: Record<string, Part[] | undefined>
}) {
  const exportMessages = toExportMessages(input.messages, input.parts)
  if (exportMessages.length === 0) {
    throw new Error("no messages to export")
  }
  const slug = sessionSlug(input.session)
  if (input.format === "html") {
    const content = formatTranscriptHtml(input.session, exportMessages)
    downloadBlob(`session-${slug}.html`, content, "text/html;charset=utf-8")
    return `session-${slug}.html`
  }
  const content = formatTranscript(input.session, exportMessages)
  downloadBlob(`session-${slug}.md`, content, "text/markdown;charset=utf-8")
  return `session-${slug}.md`
}

type Translate = (key: string, vars?: Record<string, string | number>) => string

export async function handleBtwSlash(input: {
  args: string
  url: string
  directory: string
  sessionID: string
  t: Translate
}) {
  if (!input.args) {
    const last = lastBtwAnswer()
    if (!last) {
      showToast({
        title: input.t("command.session.btw.empty"),
        variant: "error",
      })
      return
    }
    showToast({
      title: input.t("command.session.btw.last.title"),
      description: `Side Q: ${last.question}\n\nSide A: ${last.answer}`,
      duration: 10_000,
    })
    return
  }

  showToast({ title: input.t("command.session.btw.asking") })
  try {
    const answer = await askBtw({
      url: input.url,
      directory: input.directory,
      sessionID: input.sessionID,
      question: input.args,
    })
    showToast({
      title: input.t("command.session.btw.answer.title"),
      description: answer || input.t("command.session.btw.answer.empty"),
      duration: 12_000,
    })
  } catch (err) {
    showToast({
      title: input.t("command.session.btw.failed"),
      description: err instanceof Error ? err.message : String(err),
      variant: "error",
    })
  }
}

export async function detectProjectEcosystem(listRoot: () => Promise<string[]>): Promise<string> {
  try {
    const names = await listRoot()
    return detectEcosystem(names)
  } catch {
    return ""
  }
}

export async function handleGoalSlash(input: {
  args: string
  sessionID: string
  agent: string
  model: { providerID: string; modelID: string }
  variant?: string
  listRoot: () => Promise<string[]>
  promptAsync: (text: string) => Promise<void>
  t: (key: string, vars?: Record<string, string | number>) => string
}) {
  const arg = input.args.trim()
  if (!arg) {
    const goal = getGoal(input.sessionID)
    showToast({
      title: goal ? goalStatusText(goal) : input.t("command.session.goal.none"),
    })
    return
  }

  if (["clear", "stop", "off", "cancel"].includes(arg)) {
    const had = clearGoal(input.sessionID)
    showToast({
      title: had ? input.t("command.session.goal.cancelled") : input.t("command.session.goal.noneCancel"),
    })
    return
  }

  const eco = await detectProjectEcosystem(input.listRoot)
  const command = resolveGoalCommand(arg, eco)
  setGoal(input.sessionID, arg, command)
  const promptText = buildInitialGoalPrompt(arg, command)
  showToast({ title: input.t("command.session.goal.set", { goal: arg }) })
  await input.promptAsync(promptText)
}

export function lastBashToolResult(partsByMessage: Record<string, Part[] | undefined>, messageIDs: string[]) {
  for (let i = messageIDs.length - 1; i >= 0; i--) {
    const id = messageIDs[i]
    if (!id) continue
    const parts = partsByMessage[id] ?? []
    for (let j = parts.length - 1; j >= 0; j--) {
      const part = parts[j]
      if (!part || part.type !== "tool" || part.tool !== "bash") continue
      if (part.state.status === "completed") {
        return { met: true, output: part.state.output ?? "" }
      }
      if (part.state.status === "error") {
        return { met: false, output: part.state.error ?? "" }
      }
    }
  }
  return undefined
}

export async function continueGoalOnIdle(input: {
  sessionID: string
  messages: Message[]
  parts: Record<string, Part[] | undefined>
  agent: string
  model: { providerID: string; modelID: string }
  shell: (command: string) => Promise<void>
  promptAsync: (text: string) => Promise<void>
  t: (key: string, vars?: Record<string, string | number>) => string
}) {
  const goal = getGoal(input.sessionID)
  if (!goal) return

  if (goal.phase === "verify") {
    const result = lastBashToolResult(
      input.parts,
      input.messages.map((m) => m.id),
    )
    const output = result?.output ?? ""
    const met = result?.met === true

    if (met) {
      const text = goal.text
      const iterations = goal.iteration
      clearGoal(input.sessionID)
      showToast({
        title: input.t("command.session.goal.met", { goal: text, count: iterations }),
        variant: "success",
      })
      return
    }

    if (recordGoalOutput(goal, output)) {
      clearGoal(input.sessionID)
      showToast({
        title: input.t("command.session.goal.stuck", { goal: goal.text }),
        variant: "error",
      })
      return
    }

    if (goal.iteration >= goal.maxIterations) {
      clearGoal(input.sessionID)
      showToast({
        title: input.t("command.session.goal.max", { count: goal.maxIterations, goal: goal.text }),
        variant: "error",
      })
      return
    }

    goal.phase = "prompt"
    await input.promptAsync(buildContinueGoalPrompt(goal, output))
    return
  }

  // phase === "prompt": agent just went idle
  goal.iteration += 1

  if (!goal.command) {
    if (goal.iteration >= goal.maxIterations) {
      clearGoal(input.sessionID)
      showToast({
        title: input.t("command.session.goal.max", { count: goal.maxIterations, goal: goal.text }),
        variant: "error",
      })
    }
    return
  }

  if (goal.iteration > goal.maxIterations) {
    clearGoal(input.sessionID)
    showToast({
      title: input.t("command.session.goal.max", { count: goal.maxIterations, goal: goal.text }),
      variant: "error",
    })
    return
  }

  goal.phase = "verify"
  await input.shell(goal.command)
}

export function goalPromptParts(text: string) {
  return [
    {
      id: Identifier.ascending("part"),
      type: "text" as const,
      text,
    },
  ]
}
