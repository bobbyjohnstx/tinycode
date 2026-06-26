import { Config } from "@/config/config"
import { Provider } from "@/provider/provider"
import * as InstanceState from "@/effect/instance-state"
import { Effect } from "effect"
import { HttpApiBuilder } from "effect/unstable/httpapi"
import { InstanceHttpApi } from "../api"
import { markInstanceForDisposal } from "../lifecycle"

function redactSecrets(config: any): any {
  const redacted = structuredClone(config)

  // Redact provider API keys and headers
  if (redacted.provider) {
    for (const [, provider] of Object.entries(redacted.provider as Record<string, any>)) {
      if (provider.options?.apiKey) provider.options.apiKey = "***"
      if (provider.options?.enterpriseUrl) provider.options.enterpriseUrl = "***"
      if (provider.headers) {
        for (const key of Object.keys(provider.headers)) {
          if (/auth|key|token|secret/i.test(key)) {
            provider.headers[key] = "***"
          }
        }
      }
      // Redact model-level headers
      if (provider.models) {
        for (const [, model] of Object.entries(provider.models as Record<string, any>)) {
          if (model.headers) {
            for (const key of Object.keys(model.headers)) {
              if (/auth|key|token|secret/i.test(key)) {
                model.headers[key] = "***"
              }
            }
          }
        }
      }
    }
  }

  // Redact MCP OAuth secrets
  if (redacted.mcp) {
    for (const [, mcp] of Object.entries(redacted.mcp as Record<string, any>)) {
      if (mcp.oauth?.clientSecret) mcp.oauth.clientSecret = "***"
      if (mcp.headers) {
        for (const key of Object.keys(mcp.headers)) {
          if (/auth|key|token|secret/i.test(key)) {
            mcp.headers[key] = "***"
          }
        }
      }
      if (mcp.environment) {
        for (const key of Object.keys(mcp.environment)) {
          if (/auth|key|token|secret|password/i.test(key)) {
            mcp.environment[key] = "***"
          }
        }
      }
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
