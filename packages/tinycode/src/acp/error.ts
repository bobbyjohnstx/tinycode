import { RequestError } from "@agentclientprotocol/sdk"
import { Schema } from "effect"

export class SessionNotFoundError extends Schema.TaggedErrorClass<SessionNotFoundError>()("ACPSessionNotFoundError", {
  sessionId: Schema.String,
}) {}

export class InvalidModelError extends Schema.TaggedErrorClass<InvalidModelError>()("ACPInvalidModelError", {
  modelId: Schema.String,
  providerId: Schema.optional(Schema.String),
}) {}

export class InvalidModeError extends Schema.TaggedErrorClass<InvalidModeError>()("ACPInvalidModeError", {
  mode: Schema.String,
}) {}

export class AuthRequiredError extends Schema.TaggedErrorClass<AuthRequiredError>()("ACPAuthRequiredError", {
  providerId: Schema.optional(Schema.String),
}) {}

export class UnsupportedOperationError extends Schema.TaggedErrorClass<UnsupportedOperationError>()(
  "ACPUnsupportedOperationError",
  {
    method: Schema.String,
  },
) {}

export class ServiceFailureError extends Schema.TaggedErrorClass<ServiceFailureError>()("ACPServiceFailureError", {
  safeMessage: Schema.String,
  service: Schema.optional(Schema.String),
  errorName: Schema.optional(Schema.String),
}) {}

export type Error =
  | SessionNotFoundError
  | InvalidModelError
  | InvalidModeError
  | AuthRequiredError
  | UnsupportedOperationError
  | ServiceFailureError

export function toRequestError(error: Error) {
  switch (error._tag) {
    case "ACPSessionNotFoundError":
      return RequestError.invalidParams({ sessionId: error.sessionId }, `session not found: ${error.sessionId}`)
    case "ACPInvalidModelError":
      return RequestError.invalidParams(
        { providerId: error.providerId, modelId: error.modelId },
        `model not found: ${error.modelId}`,
      )
    case "ACPInvalidModeError":
      return RequestError.invalidParams({ mode: error.mode }, `mode not found: ${error.mode}`)
    case "ACPAuthRequiredError":
      return RequestError.authRequired({ providerId: error.providerId }, "provider authentication required")
    case "ACPUnsupportedOperationError":
      return RequestError.methodNotFound(error.method)
    case "ACPServiceFailureError":
      return RequestError.internalError(
        {
          ...(error.service ? { service: error.service } : {}),
          ...(error.errorName ? { errorName: error.errorName } : {}),
        },
        error.safeMessage,
      )
  }
}

export function fromUnknownDefect(_defect: unknown, safeMessage = "Internal service failure") {
  return new ServiceFailureError({ safeMessage })
}

export * as ACPError from "./error"
