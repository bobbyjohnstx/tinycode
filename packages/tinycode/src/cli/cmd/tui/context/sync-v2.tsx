import { useEvent } from "@tui/context/event"
import { SessionMessage } from "@/core/session-message"
import { createStore, produce, reconcile } from "solid-js/store"
import { createSimpleContext } from "./helper"
import { useSDK } from "./sdk"

// Use any for the store to allow mutations on readonly Effect Schema types
type Message = any
type SessionMessageAssistant = any
type SessionMessageAssistantTool = any
type SessionMessageAssistantText = any
type SessionMessageAssistantReasoning = any

function activeAssistant(messages: Message[]) {
  const index = messages.findIndex((message) => message.type === "assistant" && !message.time.completed)
  if (index < 0) return
  const assistant = messages[index]
  return assistant?.type === "assistant" ? assistant : undefined
}

function activeCompaction(messages: Message[]) {
  const index = messages.findIndex((message) => message.type === "compaction")
  if (index < 0) return
  const compaction = messages[index]
  return compaction?.type === "compaction" ? compaction : undefined
}

function activeShell(messages: Message[], callID: string) {
  const index = messages.findIndex((message) => message.type === "shell" && message.callID === callID)
  if (index < 0) return
  const shell = messages[index]
  return shell?.type === "shell" ? shell : undefined
}

function latestTool(assistant: SessionMessageAssistant | undefined, callID?: string) {
  return assistant?.content.findLast(
    (item: any): item is SessionMessageAssistantTool => item.type === "tool" && (callID === undefined || item.id === callID),
  )
}

function latestText(assistant: SessionMessageAssistant | undefined) {
  return assistant?.content.findLast((item: any): item is SessionMessageAssistantText => item.type === "text")
}

function latestReasoning(assistant: SessionMessageAssistant | undefined, reasoningID: string) {
  return assistant?.content.findLast(
    (item: any): item is SessionMessageAssistantReasoning => item.type === "reasoning" && item.id === reasoningID,
  )
}

export const { use: useSyncV2, provider: SyncProviderV2 } = createSimpleContext({
  name: "SyncV2",
  init: () => {
    const [store, setStore] = createStore<{
      messages: {
        [sessionID: string]: Message[]
      }
    }>({
      messages: {},
    })

    const event = useEvent()
    const sdk = useSDK()

    function update(sessionID: string, fn: (messages: Message[]) => void) {
      setStore(
        "messages",
        produce((draft) => {
          fn((draft[sessionID] ??= []))
        }),
      )
    }

    event.subscribe((event) => {
      // TypeScript cannot narrow the union of event types properly, so cast to any
      const evt = event as any
      switch (event.type) {
        case "session.next.prompted": {
          update(evt.properties.sessionID, (draft) => {
            draft.unshift({
              id: event.id,
              type: "user",
              text: evt.properties.prompt.text,
              files: evt.properties.prompt.files,
              agents: evt.properties.prompt.agents,
              time: { created: evt.properties.timestamp },
            })
          })
          break
        }
        case "session.next.synthetic":
          update(evt.properties.sessionID, (draft) => {
            draft.unshift({
              id: event.id,
              type: "synthetic",
              sessionID: evt.properties.sessionID,
              text: evt.properties.text,
              time: { created: evt.properties.timestamp },
            })
          })
          break
        case "session.next.shell.started":
          update(evt.properties.sessionID, (draft) => {
            draft.unshift({
              id: event.id,
              type: "shell",
              callID: evt.properties.callID,
              command: evt.properties.command,
              output: "",
              time: { created: evt.properties.timestamp },
            })
          })
          break
        case "session.next.shell.ended":
          update(evt.properties.sessionID, (draft) => {
            const match = activeShell(draft, evt.properties.callID)
            if (!match) return
            match.output = evt.properties.output
            match.time.completed = evt.properties.timestamp
          })
          break
        case "session.next.step.started":
          update(evt.properties.sessionID, (draft) => {
            const currentAssistant = activeAssistant(draft)
            if (currentAssistant) currentAssistant.time.completed = evt.properties.timestamp
            draft.unshift({
              id: event.id,
              type: "assistant",
              agent: evt.properties.agent,
              model: evt.properties.model,
              content: [],
              snapshot: evt.properties.snapshot ? { start: evt.properties.snapshot } : undefined,
              time: { created: evt.properties.timestamp },
            })
          })
          break
        case "session.next.step.ended":
          update(evt.properties.sessionID, (draft) => {
            const currentAssistant = activeAssistant(draft)
            if (!currentAssistant) return
            currentAssistant.time.completed = evt.properties.timestamp
            currentAssistant.finish = evt.properties.finish
            currentAssistant.cost = evt.properties.cost
            currentAssistant.tokens = evt.properties.tokens
            if (evt.properties.snapshot)
              currentAssistant.snapshot = { ...currentAssistant.snapshot, end: evt.properties.snapshot }
          })
          break
        case "session.next.step.failed":
          update(evt.properties.sessionID, (draft) => {
            const currentAssistant = activeAssistant(draft)
            if (!currentAssistant) return
            currentAssistant.time.completed = evt.properties.timestamp
            currentAssistant.finish = "error"
            currentAssistant.error = evt.properties.error
          })
          break
        case "session.next.text.started":
          update(evt.properties.sessionID, (draft) => {
            activeAssistant(draft)?.content.push({ type: "text", text: "" })
          })
          break
        case "session.next.text.delta":
          update(evt.properties.sessionID, (draft) => {
            const match = latestText(activeAssistant(draft))
            if (match) match.text += evt.properties.delta
          })
          break
        case "session.next.text.ended":
          update(evt.properties.sessionID, (draft) => {
            const match = latestText(activeAssistant(draft))
            if (match) match.text = evt.properties.text
          })
          break
        case "session.next.tool.input.started":
          update(evt.properties.sessionID, (draft) => {
            activeAssistant(draft)?.content.push({
              type: "tool",
              id: evt.properties.callID,
              name: evt.properties.name,
              time: { created: evt.properties.timestamp },
              state: { status: "pending", input: "" },
            })
          })
          break
        case "session.next.tool.input.delta":
          update(evt.properties.sessionID, (draft) => {
            const match = latestTool(activeAssistant(draft), evt.properties.callID)
            if (match?.state.status === "pending") match.state.input += evt.properties.delta
          })
          break
        case "session.next.tool.input.ended":
          break
        case "session.next.tool.called":
          update(evt.properties.sessionID, (draft) => {
            const match = latestTool(activeAssistant(draft), evt.properties.callID)
            if (!match) return
            match.time.ran = evt.properties.timestamp
            match.provider = evt.properties.provider
            match.state = { status: "running", input: evt.properties.input, structured: {}, content: [] }
          })
          break
        case "session.next.tool.progress":
          update(evt.properties.sessionID, (draft) => {
            const match = latestTool(activeAssistant(draft), evt.properties.callID)
            if (match?.state.status !== "running") return
            match.state.structured = evt.properties.structured
            match.state.content = [...evt.properties.content]
          })
          break
        case "session.next.tool.success":
          update(evt.properties.sessionID, (draft) => {
            const match = latestTool(activeAssistant(draft), evt.properties.callID)
            if (match?.state.status !== "running") return
            match.state = {
              status: "completed",
              input: match.state.input,
              structured: evt.properties.structured,
              content: [...evt.properties.content],
            }
            match.provider = evt.properties.provider
            match.time.completed = evt.properties.timestamp
          })
          break
        case "session.next.tool.failed":
          update(evt.properties.sessionID, (draft) => {
            const match = latestTool(activeAssistant(draft), evt.properties.callID)
            if (match?.state.status !== "running") return
            match.state = {
              status: "error",
              error: evt.properties.error,
              input: match.state.input,
              structured: match.state.structured,
              content: match.state.content,
            }
            match.provider = evt.properties.provider
            match.time.completed = evt.properties.timestamp
          })
          break
        case "session.next.reasoning.started":
          update(evt.properties.sessionID, (draft) => {
            activeAssistant(draft)?.content.push({
              type: "reasoning",
              id: evt.properties.reasoningID,
              text: "",
            })
          })
          break
        case "session.next.reasoning.delta":
          update(evt.properties.sessionID, (draft) => {
            const match = latestReasoning(activeAssistant(draft), evt.properties.reasoningID)
            if (match) match.text += evt.properties.delta
          })
          break
        case "session.next.reasoning.ended":
          update(evt.properties.sessionID, (draft) => {
            const match = latestReasoning(activeAssistant(draft), evt.properties.reasoningID)
            if (match) match.text = evt.properties.text
          })
          break
        case "session.next.retried":
          break
        case "session.next.compaction.started":
          update(evt.properties.sessionID, (draft) => {
            draft.unshift({
              id: event.id,
              type: "compaction",
              reason: evt.properties.reason,
              summary: "",
              time: { created: evt.properties.timestamp },
            })
          })
          break
        case "session.next.compaction.delta":
          update(evt.properties.sessionID, (draft) => {
            const match = activeCompaction(draft)
            if (match) match.summary += evt.properties.text
          })
          break
        case "session.next.compaction.ended":
          update(evt.properties.sessionID, (draft) => {
            const match = activeCompaction(draft)
            if (!match) return
            match.summary = evt.properties.text
            match.include = evt.properties.include
          })
          break
      }
    })

    const result = {
      data: store,
      session: {
        message: {
          async sync(sessionID: string) {
            const response = await sdk.client.session.messages({ sessionID })
            // Response is an array of { info: Message, parts: Part[] }
            // We only need the info objects
            const messages = (response.data ?? []).map((m: any) => m.info)
            setStore("messages", sessionID, reconcile(messages))
          },
          fromSession(sessionID: string) {
            const messages = store.messages[sessionID]
            if (!messages) return []
            return messages
          },
        },
      },
    }

    return result
  },
})
