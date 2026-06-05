import type { WorkspaceID } from "@/control-plane/schema"
import { Session } from "@/session/session"
import { SessionID } from "@/session/schema"
import { NotFoundError } from "@/storage/storage"
import { Context, Effect, Layer, Schema } from "effect"
import { HttpServerRequest } from "effect/unstable/http"
import { HttpApiMiddleware } from "effect/unstable/httpapi"

// Query fields this middleware reads from the URL. Spread into every
// endpoint query schema in groups that apply WorkspaceRoutingMiddleware,
// otherwise HttpApi rejects requests carrying these params with 400.
// HttpApiMiddleware in effect-smol cannot declare query params today -
// remove this once upstream supports middleware-declared query schemas.
export const WorkspaceRoutingQueryFields = {
  directory: Schema.optional(Schema.String),
  workspace: Schema.optional(Schema.String),
}

export const WorkspaceRoutingQuery = Schema.Struct(WorkspaceRoutingQueryFields)

export class WorkspaceRouteContext extends Context.Service<
  WorkspaceRouteContext,
  {
    readonly directory: string
    readonly workspaceID?: WorkspaceID
  }
>()("@tinycode/ExperimentalHttpApiWorkspaceRouteContext") {}

export class WorkspaceRoutingMiddleware extends HttpApiMiddleware.Service<
  WorkspaceRoutingMiddleware,
  {
    provides: WorkspaceRouteContext
    requires: Session.Service
  }
>()("@tinycode/ExperimentalHttpApiWorkspaceRouting") {}

function requestURL(request: HttpServerRequest.HttpServerRequest): URL {
  return new URL(request.url, "http://localhost")
}

function getWorkspaceRouteSessionID(url: URL): SessionID | undefined {
  const id = url.searchParams.get("sessionID")
  return id ? SessionID.make(id) : undefined
}

export const workspaceRoutingLayer = Layer.effect(
  WorkspaceRoutingMiddleware,
  Effect.gen(function* () {
    return WorkspaceRoutingMiddleware.of((effect) =>
      Effect.gen(function* () {
        const request = yield* HttpServerRequest.HttpServerRequest
        const url = requestURL(request)
        const sessionID = getWorkspaceRouteSessionID(url)
        const session = sessionID
          ? yield* Session.Service.use((svc) => svc.get(sessionID)).pipe(
              Effect.catchIf(
                (error): error is NotFoundError => NotFoundError.isInstance(error),
                () => Effect.succeed(undefined),
              ),
              Effect.catchDefect(() => Effect.succeed(undefined)),
            )
          : undefined
        const directory =
          session?.directory || url.searchParams.get("directory") || request.headers["x-opencode-directory"] || process.cwd()
        return yield* effect.pipe(
          Effect.provideService(
            WorkspaceRouteContext,
            WorkspaceRouteContext.of({ directory }),
          ),
        )
      }),
    )
  }),
)
