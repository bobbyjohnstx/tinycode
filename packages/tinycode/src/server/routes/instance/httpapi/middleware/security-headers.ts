import { Effect } from "effect"
import { HttpRouter, HttpServerResponse } from "effect/unstable/http"

export const securityHeadersLayer = HttpRouter.middleware(
  (effect) =>
    Effect.gen(function* () {
      const response = yield* effect
      return HttpServerResponse.setHeaders(response, {
        "x-frame-options": "DENY",
        "x-content-type-options": "nosniff",
        "referrer-policy": "strict-origin-when-cross-origin",
        "permissions-policy": "camera=(), microphone=(), geolocation=()",
      })
    }),
  { global: true },
)
