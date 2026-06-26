import { Config } from "@/config/config"
import { Provider } from "@/provider/provider"
import * as InstanceState from "@/effect/instance-state"
import { Effect } from "effect"
import { HttpApiBuilder } from "effect/unstable/httpapi"
import { InstanceHttpApi } from "../api"
import { markInstanceForDisposal } from "../lifecycle"

const SENSITIVE_KEY = /auth|key|token|secret|password/i

function redactKeys(obj: Record<string, any> | undefined) {
  if (!obj) return
  for (const key of Object.keys(obj)) {
    if (SENSITIVE_KEY.test(key)) obj[key] = "***"
  }
}

function redactSecrets(config: any): any {
  const redacted = structuredClone(config)

  if (redacted.provider) {
    for (const provider of Object.values(redacted.provider as Record<string, any>)) {
      if (provider.options?.apiKey) provider.options.apiKey = "***"
      if (provider.options?.enterpriseUrl) provider.options.enterpriseUrl = "***"
      redactKeys(provider.headers)
      if (provider.models) {
        for (const model of Object.values(provider.models as Record<string, any>)) {
          redactKeys(model.headers)
        }
      }
    }
  }

  if (redacted.mcp) {
    for (const mcp of Object.values(redacted.mcp as Record<string, any>)) {
      if (mcp.oauth?.clientSecret) mcp.oauth.clientSecret = "***"
      redactKeys(mcp.headers)
      redactKeys(mcp.environment)
    }
  }

  return redacted
}

export const configHandlers = HttpApiBuilder.group(InstanceHttpApi, "config", (handlers) =>
  Effect.gen(function* () {
    const providerSvc = yield* Provider.Service
    const configSvc = yield* Config.Service

    const get = Effect.fn("ConfigHttpApi.get")(function* () {
      const config = yield* configSvc.get()
      return redactSecrets(config)
    })

    const update = Effect.fn("ConfigHttpApi.update")(function* (ctx) {
      yield* configSvc.update(ctx.payload)
      yield* markInstanceForDisposal(yield* InstanceState.context)
      return ctx.payload
    })

    const providers = Effect.fn("ConfigHttpApi.providers")(function* () {
      const providers = yield* providerSvc.list()
      return {
        providers: Object.values(providers).map(Provider.toPublicInfo),
        default: Provider.defaultModelIDs(providers),
      }
    })

    return handlers.handle("get", get).handle("update", update).handle("providers", providers)
  }),
)
