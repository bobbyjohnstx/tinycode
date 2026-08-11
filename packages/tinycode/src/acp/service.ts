import {
  type AgentSideConnection,
  type AuthenticateRequest,
  type AuthenticateResponse,
  type CancelNotification,
  type CloseSessionRequest,
  type CloseSessionResponse,
  type ForkSessionRequest,
  type ForkSessionResponse,
  type InitializeRequest,
  type InitializeResponse,
  type ListSessionsRequest,
  type ListSessionsResponse,
  type LoadSessionRequest,
  type LoadSessionResponse,
  type NewSessionRequest,
  type NewSessionResponse,
  type PromptRequest,
  type PromptResponse,
  type ResumeSessionRequest,
  type ResumeSessionResponse,
  type SessionInfo,
  type SetSessionConfigOptionRequest,
  type SetSessionConfigOptionResponse,
  type SetSessionModelRequest,
  type SetSessionModelResponse,
  type SetSessionModeRequest,
  type SetSessionModeResponse,
} from "@agentclientprotocol/sdk"
import type { TinycodeClient } from "@tinycode/sdk"
import { Effect } from "effect"
import * as ACPError from "./error"
import { promptContentToParts } from "./content"
import { ACPEvent } from "./event"
import { ACPSession } from "./session"

export type Error = ACPError.Error
type ServiceConnection = Pick<AgentSideConnection, "sessionUpdate"> &
  Partial<Pick<AgentSideConnection, "requestPermission" | "writeTextFile">>

export type Interface = {
  readonly initialize: (input: InitializeRequest) => Effect.Effect<InitializeResponse, Error>
  readonly authenticate: (input: AuthenticateRequest) => Effect.Effect<AuthenticateResponse, Error>
  readonly newSession: (input: NewSessionRequest) => Effect.Effect<NewSessionResponse, Error>
  readonly loadSession: (input: LoadSessionRequest) => Effect.Effect<LoadSessionResponse, Error>
  readonly listSessions: (input: ListSessionsRequest) => Effect.Effect<ListSessionsResponse, Error>
  readonly resumeSession: (input: ResumeSessionRequest) => Effect.Effect<ResumeSessionResponse, Error>
  readonly closeSession: (input: CloseSessionRequest) => Effect.Effect<CloseSessionResponse, Error>
  readonly forkSession: (input: ForkSessionRequest) => Effect.Effect<ForkSessionResponse, Error>
  readonly setSessionConfigOption: (
    input: SetSessionConfigOptionRequest,
  ) => Effect.Effect<SetSessionConfigOptionResponse, Error>
  readonly setSessionMode: (input: SetSessionModeRequest) => Effect.Effect<SetSessionModeResponse, Error>
  readonly setSessionModel: (input: SetSessionModelRequest) => Effect.Effect<SetSessionModelResponse, Error>
  readonly prompt: (input: PromptRequest) => Effect.Effect<PromptResponse, Error>
  readonly cancel: (input: CancelNotification) => Effect.Effect<void, Error>
}

export function make(input: { sdk: TinycodeClient; connection?: ServiceConnection }): Interface {
  const session = ACPSession.make()
  const events = input.connection ? ACPEvent.start({ sdk: input.sdk, connection: input.connection, session }) : undefined

  const initialize = Effect.fn("ACP.initialize")(function* (_params: InitializeRequest) {
    const response: InitializeResponse = {
      protocolVersion: 1,
      agentCapabilities: {
        loadSession: true,
        promptCapabilities: {
          embeddedContext: true,
          image: true,
        },
        sessionCapabilities: {
          close: {},
          fork: {},
          list: {},
          resume: {},
        },
      },
      authMethods: [],
      agentInfo: {
        name: "tinycode",
        version: "0.1.0",
      },
    }
    return response
  })

  const authenticate = Effect.fn("ACP.authenticate")(function* (_params: AuthenticateRequest) {
    return {}
  })

  const newSession = Effect.fn("ACP.newSession")(function* (params: NewSessionRequest) {
    const created = yield* Effect.tryPromise({
      try: async () => {
        const result = await input.sdk.session.create({ query: { directory: params.cwd } })
        if (!result.data) throw new Error("Failed to create session")
        return result.data
      },
      catch: (error) => new ACPError.ServiceFailureError({ safeMessage: String(error) }),
    })

    const state = yield* session.create({
      id: created.id,
      cwd: params.cwd,
      mcpServers: params.mcpServers,
    })

    const response: NewSessionResponse = {
      sessionId: state.id,
      configOptions: [],
    }
    return response
  })

  const loadSession = Effect.fn("ACP.loadSession")(function* (params: LoadSessionRequest) {
    yield* Effect.tryPromise({
      try: async () => {
        const result = await input.sdk.session.get({ path: { id: params.sessionId }, query: { directory: params.cwd } })
        if (!result.data) throw new Error("Session not found")
        return result.data
      },
      catch: () => new ACPError.SessionNotFoundError({ sessionId: params.sessionId }),
    })

    const messages = yield* Effect.tryPromise({
      try: async () => {
        const result = await input.sdk.session.messages({ path: { id: params.sessionId }, query: { directory: params.cwd } })
        if (!result.data) throw new Error("Failed to fetch messages")
        return result.data
      },
      catch: (error) => new ACPError.ServiceFailureError({ safeMessage: String(error) }),
    })

    const state = yield* session.load({
      id: params.sessionId,
      cwd: params.cwd,
      mcpServers: params.mcpServers,
    })

    if (events) {
      yield* Effect.promise(async () => {
        for (const msg of messages) {
          await events.replayMessage(msg)
        }
      })
    }

    return {
      configOptions: [],
    }
  })

  const listSessions = Effect.fn("ACP.listSessions")(function* (params: ListSessionsRequest) {
    const sessions = yield* Effect.tryPromise({
      try: async () => {
        const result = await input.sdk.session.list({
          query: params.cwd ? { directory: params.cwd } : undefined,
        })
        if (!result.data) throw new Error("Failed to list sessions")
        return result.data
      },
      catch: (error) => new ACPError.ServiceFailureError({ safeMessage: String(error) }),
    })

    const serverEntries = sessions.map(
      (item: any): SessionInfo => ({
        sessionId: item.id,
        cwd: item.directory,
        title: item.title,
        updatedAt: new Date(item.time.updated).toISOString(),
      }),
    )

    const liveEntries = (yield* session.list(params.cwd ?? undefined))
      .filter((item) => !serverEntries.some((entry: any) => entry.sessionId === item.id))
      .map(
        (item): SessionInfo => ({
          sessionId: item.id,
          cwd: item.cwd,
          updatedAt: item.createdAt.toISOString(),
        }),
      )

    const sorted = [...liveEntries, ...serverEntries].toSorted(
      (a, b) => new Date(b.updatedAt ?? 0).getTime() - new Date(a.updatedAt ?? 0).getTime(),
    )

    return {
      sessions: sorted,
    }
  })

  const resumeSession = Effect.fn("ACP.resumeSession")(function* (params: ResumeSessionRequest) {
    yield* Effect.tryPromise({
      try: async () => {
        const result = await input.sdk.session.get({ path: { id: params.sessionId }, query: { directory: params.cwd } })
        if (!result.data) throw new Error("Session not found")
        return result.data
      },
      catch: () => new ACPError.SessionNotFoundError({ sessionId: params.sessionId }),
    })

    const messages = yield* Effect.tryPromise({
      try: async () => {
        const result = await input.sdk.session.messages({ path: { id: params.sessionId }, query: { directory: params.cwd, limit: 20 } })
        if (!result.data) throw new Error("Failed to fetch messages")
        return result.data
      },
      catch: (error) => new ACPError.ServiceFailureError({ safeMessage: String(error) }),
    })

    const state = yield* session.load({
      id: params.sessionId,
      cwd: params.cwd,
      mcpServers: params.mcpServers,
    })

    if (events) {
      yield* Effect.promise(async () => {
        for (const msg of messages) {
          await events.replayMessage(msg)
        }
      })
    }

    return {
      configOptions: [],
    }
  })

  const closeSession = Effect.fn("ACP.closeSession")(function* (params: CloseSessionRequest) {
    yield* Effect.tryPromise({
      try: async () => {
        const result = await input.sdk.session.delete({ path: { id: params.sessionId } })
        if (result.error) throw new Error("Failed to delete session")
        return result.data
      },
      catch: (error) => new ACPError.ServiceFailureError({ safeMessage: String(error) }),
    })

    yield* session.remove(params.sessionId)

    return {}
  })

  const forkSession = Effect.fn("ACP.forkSession")(function* (params: ForkSessionRequest) {
    const forked = yield* Effect.tryPromise({
      try: async () => {
        const result = await input.sdk.session.fork({ path: { id: params.sessionId } })
        if (!result.data) throw new Error("Failed to fork session")
        return result.data
      },
      catch: (error) => new ACPError.ServiceFailureError({ safeMessage: String(error) }),
    })

    const state = yield* session.tryGet(params.sessionId)
    if (state) {
      yield* session.create({
        id: forked.id,
        cwd: state.cwd,
        mcpServers: state.mcpServers,
        model: state.model,
        variant: state.variant,
        modeId: state.modeId,
      })
    }

    return {
      sessionId: forked.id,
    }
  })

  const setSessionConfigOption: Interface["setSessionConfigOption"] = Effect.fn("ACP.setSessionConfigOption")(
    function* (_params: SetSessionConfigOptionRequest) {
      return {} as SetSessionConfigOptionResponse
    },
  )

  const setSessionMode = Effect.fn("ACP.setSessionMode")(function* (params: SetSessionModeRequest) {
    yield* session.setMode(params.sessionId, (params as any).mode?.id)
    return {}
  })

  const setSessionModel = Effect.fn("ACP.setSessionModel")(function* (params: SetSessionModelRequest) {
    yield* session.setModel(params.sessionId, {
      providerID: (params as any).modelId || "",
      modelID: (params as any).modelId || "",
    })
    return {}
  })

  const runUntilIdle = <A>(sessionId: string, fn: () => Promise<A>): Promise<A> =>
    events ? events.runUntilIdle(sessionId, fn) : fn()

  const prompt: Interface["prompt"] = Effect.fn("ACP.prompt")(function* (params: PromptRequest) {
    const parts = promptContentToParts((params as any).content)

    yield* Effect.tryPromise({
      try: () =>
        runUntilIdle(params.sessionId, async () => {
          const result = await input.sdk.session.prompt({
            path: { id: params.sessionId },
            body: {
              parts,
            },
          })
          if (result.error) throw new Error("Failed to send prompt")
          return result.data
        }),
      catch: (error) => new ACPError.ServiceFailureError({ safeMessage: String(error) }),
    })

    return {} as PromptResponse
  })

  const cancel = Effect.fn("ACP.cancel")(function* (params: CancelNotification) {
    yield* Effect.tryPromise({
      try: () => input.sdk.session.abort({ path: { id: params.sessionId } }),
      catch: (error) => new ACPError.ServiceFailureError({ safeMessage: String(error) }),
    })
  })

  return {
    initialize,
    authenticate,
    newSession,
    loadSession,
    listSessions,
    resumeSession,
    closeSession,
    forkSession,
    setSessionConfigOption,
    setSessionMode,
    setSessionModel,
    prompt,
    cancel,
  }
}

export * as ACPService from "./service"
