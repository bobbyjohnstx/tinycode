import { describe, expect, test } from "bun:test"
import type {
  Message,
  Part,
  PermissionRequest,
  Project,
  QuestionRequest,
  Session,
  SessionStatus,
  SnapshotFileDiff,
  Todo,
} from "@tinycode/sdk/v2/client"
import { createStore } from "solid-js/store"
import type { State } from "./types"
import { applyDirectoryEvent, applyGlobalEvent } from "./event-reducer"

/**
 * Additional event-reducer tests covering gaps not in event-reducer.test.ts.
 * Covers: session.diff, todo.updated, session.status, message.part.delta,
 * message.updated for first-message sessions, session.updated non-archived
 * upsert, and unrecognized event types.
 */

const rootSession = (input: { id: string; parentID?: string; archived?: number }) =>
  ({
    id: input.id,
    parentID: input.parentID,
    time: {
      created: 1,
      updated: 1,
      archived: input.archived,
    },
  }) as Session

const userMessage = (id: string, sessionID: string) =>
  ({
    id,
    sessionID,
    role: "user",
    time: { created: 1 },
    agent: "assistant",
    model: { providerID: "openai", modelID: "gpt" },
  }) as Message

const textPart = (id: string, sessionID: string, messageID: string) =>
  ({
    id,
    sessionID,
    messageID,
    type: "text",
    text: id,
  }) as Part

const baseState = (input: Partial<State> = {}) =>
  ({
    status: "complete",
    agent: [],
    command: [],
    project: "",
    projectMeta: undefined,
    icon: undefined,
    provider: {} as State["provider"],
    config: {} as State["config"],
    path: { directory: "/tmp" } as State["path"],
    session: [],
    sessionTotal: 0,
    session_status: {},
    session_diff: {},
    todo: {},
    permission: {},
    question: {},
    mcp: {},
    lsp: [],
    vcs: undefined,
    limit: 10,
    message: {},
    part: {},
    part_text_accum_delta: {},
    ...input,
  }) as State

describe("applyDirectoryEvent gap coverage", () => {
  test("session.diff event stores snapshot diffs by session ID", () => {
    const [store, setStore] = createStore(baseState())
    const diffs: SnapshotFileDiff[] = [
      { file: "main.go", status: "modified", patch: "@@...", additions: 1, deletions: 0 } as SnapshotFileDiff,
      { file: "README.md", status: "added", patch: "@@...", additions: 5, deletions: 0 } as SnapshotFileDiff,
    ]

    applyDirectoryEvent({
      event: {
        type: "session.diff",
        properties: { sessionID: "ses_1", diff: diffs },
      },
      store,
      setStore,
      push() {},
      directory: "/tmp",
      loadLsp() {},
    })

    expect(store.session_diff.ses_1).toBeDefined()
    expect(store.session_diff.ses_1!.length).toBeGreaterThanOrEqual(1)
  })

  test("todo.updated event stores todos by session ID", () => {
    const [store, setStore] = createStore(baseState())
    const todos: Todo[] = [
      { id: "todo_1", content: "fix bug", status: "pending" } as unknown as Todo,
      { id: "todo_2", content: "add test", status: "done" } as unknown as Todo,
    ]
    const capturedTodos: Array<{ sessionID: string; todos: Todo[] | undefined }> = []

    applyDirectoryEvent({
      event: {
        type: "todo.updated",
        properties: { sessionID: "ses_1", todos },
      },
      store,
      setStore,
      push() {},
      directory: "/tmp",
      loadLsp() {},
      setSessionTodo(sessionID, value) {
        capturedTodos.push({ sessionID, todos: value })
      },
    })

    expect(store.todo.ses_1).toBeDefined()
    expect(store.todo.ses_1!.length).toBe(2)
    expect(capturedTodos).toEqual([{ sessionID: "ses_1", todos }])
  })

  test("session.status event stores status by session ID", () => {
    const [store, setStore] = createStore(baseState())
    const status: SessionStatus = { type: "busy" }

    applyDirectoryEvent({
      event: {
        type: "session.status",
        properties: { sessionID: "ses_1", status },
      },
      store,
      setStore,
      push() {},
      directory: "/tmp",
      loadLsp() {},
    })

    expect(store.session_status.ses_1).toEqual({ type: "busy" })
  })

  test("session.status event updates existing status", () => {
    const [store, setStore] = createStore(
      baseState({
        session_status: { ses_1: { type: "busy" } },
      }),
    )

    applyDirectoryEvent({
      event: {
        type: "session.status",
        properties: { sessionID: "ses_1", status: { type: "idle" } as SessionStatus },
      },
      store,
      setStore,
      push() {},
      directory: "/tmp",
      loadLsp() {},
    })

    expect(store.session_status.ses_1).toEqual({ type: "idle" })
  })

  test("message.part.delta appends text to existing part", () => {
    const sessionID = "ses_1"
    const messageID = "msg_1"
    const partID = "prt_1"
    const part = { ...textPart(partID, sessionID, messageID), text: "Hello" } as Part
    const [store, setStore] = createStore(
      baseState({
        part: { [messageID]: [part] },
        part_text_accum_delta: {},
      }),
    )

    applyDirectoryEvent({
      event: {
        type: "message.part.delta",
        properties: { messageID, partID, field: "text", delta: " world" },
      },
      store,
      setStore,
      push() {},
      directory: "/tmp",
      loadLsp() {},
    })

    const updated = store.part[messageID]?.find((p) => p.id === partID)
    expect(updated?.type).toBe("text")
    if (updated?.type === "text") {
      expect(updated.text).toBe("Hello world")
    }
    expect(store.part_text_accum_delta[partID]).toBe(" world")
  })

  test("message.part.delta accumulates across multiple deltas", () => {
    const sessionID = "ses_1"
    const messageID = "msg_1"
    const partID = "prt_1"
    const part = { ...textPart(partID, sessionID, messageID), text: "" } as Part
    const [store, setStore] = createStore(
      baseState({
        part: { [messageID]: [part] },
        part_text_accum_delta: {},
      }),
    )

    const deltas = ["one", " two", " three"]
    for (const delta of deltas) {
      applyDirectoryEvent({
        event: {
          type: "message.part.delta",
          properties: { messageID, partID, field: "text", delta },
        },
        store,
        setStore,
        push() {},
        directory: "/tmp",
        loadLsp() {},
      })
    }

    const updated = store.part[messageID]?.find((p) => p.id === partID)
    expect(updated?.type).toBe("text")
    if (updated?.type === "text") {
      expect(updated.text).toBe("one two three")
    }
    expect(store.part_text_accum_delta[partID]).toBe("one two three")
  })

  test("message.part.delta ignores delta for unknown part", () => {
    const [store, setStore] = createStore(
      baseState({
        part: {},
        part_text_accum_delta: {},
      }),
    )

    applyDirectoryEvent({
      event: {
        type: "message.part.delta",
        properties: {
          messageID: "unknown_msg",
          partID: "unknown_prt",
          field: "text",
          delta: "orphan",
        },
      },
      store,
      setStore,
      push() {},
      directory: "/tmp",
      loadLsp() {},
    })

    expect(store.part_text_accum_delta.unknown_prt).toBeUndefined()
  })

  test("message.updated creates first message list for a session", () => {
    const sessionID = "ses_1"
    const [store, setStore] = createStore(baseState())

    applyDirectoryEvent({
      event: {
        type: "message.updated",
        properties: { info: userMessage("msg_1", sessionID) },
      },
      store,
      setStore,
      push() {},
      directory: "/tmp",
      loadLsp() {},
    })

    expect(store.message[sessionID]).toBeDefined()
    expect(store.message[sessionID]!.length).toBe(1)
    expect(store.message[sessionID]![0]!.id).toBe("msg_1")
  })

  test("session.updated for non-archived session not in list inserts it", () => {
    const [store, setStore] = createStore(
      baseState({
        session: [rootSession({ id: "ses_a" }), rootSession({ id: "ses_c" })],
        sessionTotal: 2,
      }),
    )

    applyDirectoryEvent({
      event: {
        type: "session.updated",
        properties: { info: rootSession({ id: "ses_b" }) },
      },
      store,
      setStore,
      push() {},
      directory: "/tmp",
      loadLsp() {},
    })

    expect(store.session.map((s) => s.id)).toEqual(["ses_a", "ses_b", "ses_c"])
  })

  test("session.created reconciles existing session if already present", () => {
    const original = rootSession({ id: "ses_1" })
    const [store, setStore] = createStore(
      baseState({
        session: [original],
        sessionTotal: 1,
      }),
    )

    const updated = {
      ...rootSession({ id: "ses_1" }),
      title: "Updated title",
    } as Session

    applyDirectoryEvent({
      event: { type: "session.created", properties: { info: updated } },
      store,
      setStore,
      push() {},
      directory: "/tmp",
      loadLsp() {},
    })

    expect(store.session.length).toBe(1)
    expect(store.sessionTotal).toBe(1)
  })

  test("message.part.updated clears accumulated delta for the part", () => {
    const sessionID = "ses_1"
    const messageID = "msg_1"
    const partID = "prt_1"
    const part = textPart(partID, sessionID, messageID)
    const [store, setStore] = createStore(
      baseState({
        part: { [messageID]: [part] },
        part_text_accum_delta: { [partID]: "accumulated text" },
      }),
    )

    applyDirectoryEvent({
      event: {
        type: "message.part.updated",
        properties: { part: { ...part, text: "final text" } as Part },
      },
      store,
      setStore,
      push() {},
      directory: "/tmp",
      loadLsp() {},
    })

    expect(store.part_text_accum_delta[partID]).toBeUndefined()
  })

  test("message.part.updated skips patch and step parts", () => {
    const sessionID = "ses_1"
    const messageID = "msg_1"
    const [store, setStore] = createStore(
      baseState({
        part: {},
      }),
    )

    for (const type of ["patch", "step-start", "step-finish"]) {
      applyDirectoryEvent({
        event: {
          type: "message.part.updated",
          properties: {
            part: { id: `prt_${type}`, sessionID, messageID, type } as unknown as Part,
          },
        },
        store,
        setStore,
        push() {},
        directory: "/tmp",
        loadLsp() {},
      })
    }

    expect(store.part[messageID]).toBeUndefined()
  })

  test("permission.asked creates first permission list for a session", () => {
    const [store, setStore] = createStore(baseState())

    const perm = {
      id: "perm_1",
      sessionID: "ses_1",
      permission: "shell",
      patterns: ["*"],
      metadata: {},
      always: [],
    } as PermissionRequest

    applyDirectoryEvent({
      event: { type: "permission.asked", properties: perm },
      store,
      setStore,
      push() {},
      directory: "/tmp",
      loadLsp() {},
    })

    expect(store.permission.ses_1).toBeDefined()
    expect(store.permission.ses_1!.length).toBe(1)
    expect(store.permission.ses_1![0]!.id).toBe("perm_1")
  })

  test("permission.replied ignores reply for unknown session", () => {
    const [store, setStore] = createStore(baseState())

    applyDirectoryEvent({
      event: {
        type: "permission.replied",
        properties: { sessionID: "unknown", requestID: "perm_1" },
      },
      store,
      setStore,
      push() {},
      directory: "/tmp",
      loadLsp() {},
    })

    expect(store.permission.unknown).toBeUndefined()
  })

  test("question.asked creates first question list for a session", () => {
    const [store, setStore] = createStore(baseState())

    const question = {
      id: "q_1",
      sessionID: "ses_1",
      questions: [
        {
          question: "Pick a tool",
          header: "Tool",
          options: [{ label: "A", description: "Option A" }],
        },
      ],
    } as QuestionRequest

    applyDirectoryEvent({
      event: { type: "question.asked", properties: question },
      store,
      setStore,
      push() {},
      directory: "/tmp",
      loadLsp() {},
    })

    expect(store.question.ses_1).toBeDefined()
    expect(store.question.ses_1!.length).toBe(1)
    expect(store.question.ses_1![0]!.id).toBe("q_1")
  })
})

describe("applyGlobalEvent gap coverage", () => {
  test("ignores unrecognized event types without error", () => {
    let refreshCount = 0
    const project: Project[] = [{ id: "a" }] as Project[]

    applyGlobalEvent({
      event: { type: "unknown.event.type", properties: {} },
      project,
      refresh: () => {
        refreshCount += 1
      },
      setGlobalProject() {},
    })

    expect(project.map((x) => x.id)).toEqual(["a"])
    expect(refreshCount).toBe(0)
  })

  test("project.updated merges into existing project entry", () => {
    const project = [{ id: "a", name: "old" }, { id: "c" }] as Project[]

    applyGlobalEvent({
      event: { type: "project.updated", properties: { id: "a", name: "new" } },
      project,
      refresh: () => {},
      setGlobalProject(next) {
        if (typeof next === "function") next(project)
      },
    })

    expect(project[0]!.name).toBe("new")
    expect(project.length).toBe(2)
  })
})
