import { Context, Duration, Effect, Layer, Ref, Schedule } from "effect"
import { FetchHttpClient, HttpClient, HttpClientRequest, HttpClientResponse } from "effect/unstable/http"
import { Schema } from "effect"
import * as Log from "@opencode-ai/core/util/log"
import { ModelID, ProviderID } from "./schema"
import type { Info, Model } from "./provider"

const log = Log.create({ service: "local-discovery" })

const PROBE_TIMEOUT = Duration.millis(2_000)
const POLL_INTERVAL = Duration.seconds(30)

// --- Ollama response schema ---

const OllamaModel = Schema.Struct({
  name: Schema.String,
})

const OllamaTagsResponse = Schema.Struct({
  models: Schema.Array(OllamaModel),
})

// --- vLLM response schema ---

const VllmModel = Schema.Struct({
  id: Schema.String,
})

const VllmModelsResponse = Schema.Struct({
  data: Schema.Array(VllmModel),
})

// --- Helpers ---

function makeOllamaProvider(baseURL: string, modelNames: string[]): Info {
  const models: Record<string, Model> = {}
  for (const name of modelNames) {
    const model: Model = {
      id: ModelID.make(name),
      providerID: ProviderID.make("ollama"),
      name,
      family: "",
      api: {
        id: name,
        url: `${baseURL}/v1`,
        npm: "@ai-sdk/openai-compatible",
      },
      status: "active",
      headers: {},
      options: {},
      cost: { input: 0, output: 0, cache: { read: 0, write: 0 } },
      limit: { context: 0, output: 0 },
      capabilities: {
        temperature: true,
        reasoning: false,
        attachment: false,
        toolcall: true,
        input: { text: true, audio: false, image: false, video: false, pdf: false },
        output: { text: true, audio: false, image: false, video: false, pdf: false },
        interleaved: false,
      },
      release_date: "",
      variants: {},
    }
    models[name] = model
  }
  return {
    id: ProviderID.make("ollama"),
    name: "Ollama",
    source: "custom",
    env: [],
    options: { baseURL: `${baseURL}/v1` },
    models,
  }
}

function makeVllmProvider(baseURL: string, modelIds: string[]): Info {
  const models: Record<string, Model> = {}
  for (const id of modelIds) {
    const model: Model = {
      id: ModelID.make(id),
      providerID: ProviderID.make("vllm"),
      name: id,
      family: "",
      api: {
        id,
        url: `${baseURL}/v1`,
        npm: "@ai-sdk/openai-compatible",
      },
      status: "active",
      headers: {},
      options: {},
      cost: { input: 0, output: 0, cache: { read: 0, write: 0 } },
      limit: { context: 0, output: 0 },
      capabilities: {
        temperature: true,
        reasoning: false,
        attachment: false,
        toolcall: true,
        input: { text: true, audio: false, image: false, video: false, pdf: false },
        output: { text: true, audio: false, image: false, video: false, pdf: false },
        interleaved: false,
      },
      release_date: "",
      variants: {},
    }
    models[id] = model
  }
  return {
    id: ProviderID.make("vllm"),
    name: "vLLM",
    source: "custom",
    env: [],
    options: { baseURL: `${baseURL}/v1` },
    models,
  }
}

export interface Interface {
  readonly get: () => Effect.Effect<Record<string, Info>>
}

export class Service extends Context.Service<Service, Interface>()("@opencode/LocalDiscovery") {}

export const layer: Layer.Layer<Service, never, HttpClient.HttpClient> = Layer.effect(
  Service,
  Effect.gen(function* () {
    const httpClient = yield* HttpClient.HttpClient

    const discovered = yield* Ref.make<Record<string, Info>>({})

    function probeOllama(baseURL: string): Effect.Effect<Info | null> {
      return HttpClientRequest.get(`${baseURL}/api/tags`).pipe(
        httpClient.execute,
        Effect.timeout(PROBE_TIMEOUT),
        Effect.flatMap((res) => HttpClientResponse.schemaBodyJson(OllamaTagsResponse)(res)),
        Effect.map((data) => {
          const names = data.models.map((m) => m.name).filter((n) => n.length > 0)
          if (names.length === 0) return null
          log.info("ollama discovered", { count: names.length, models: names.slice(0, 5) })
          return makeOllamaProvider(baseURL, names)
        }),
        Effect.catchAll((err) => {
          log.info("ollama not available", { baseURL, error: String(err) })
          return Effect.succeed(null)
        }),
      )
    }

    function probeVllm(baseURL: string): Effect.Effect<Info | null> {
      return HttpClientRequest.get(`${baseURL}/v1/models`).pipe(
        httpClient.execute,
        Effect.timeout(PROBE_TIMEOUT),
        Effect.flatMap((res) => HttpClientResponse.schemaBodyJson(VllmModelsResponse)(res)),
        Effect.map((data) => {
          const ids = data.data.map((m) => m.id).filter((id) => id.length > 0)
          if (ids.length === 0) return null
          log.info("vllm discovered", { count: ids.length, models: ids.slice(0, 5) })
          return makeVllmProvider(baseURL, ids)
        }),
        Effect.catchAll((err) => {
          log.info("vllm not available", { baseURL, error: String(err) })
          return Effect.succeed(null)
        }),
      )
    }

    // Probe a LiteMaaS/LiteLLM or any OpenAI-compatible MaaS server.
    // Requires TINYCODE_MAAS_HOST and TINYCODE_MAAS_API_KEY env vars.
    // Filters out embedding models (ids containing "embed").
    function probeMaas(baseURL: string, apiKey: string): Effect.Effect<Info | null> {
      return HttpClientRequest.get(`${baseURL}/v1/models`).pipe(
        HttpClientRequest.setHeader("Authorization", `Bearer ${apiKey}`),
        httpClient.execute,
        Effect.timeout(PROBE_TIMEOUT),
        Effect.flatMap((res) => HttpClientResponse.schemaBodyJson(VllmModelsResponse)(res)),
        Effect.map((data) => {
          const ids = data.data
            .map((m) => m.id)
            .filter((id) => id.length > 0 && !id.toLowerCase().includes("embed"))
          if (ids.length === 0) return null
          log.info("maas discovered", { host: baseURL, count: ids.length, models: ids.slice(0, 5) })
          const models: Record<string, Model> = {}
          for (const id of ids) {
            models[id] = {
              id: ModelID.make(id),
              providerID: ProviderID.make("maas"),
              name: id,
              family: "",
              api: { id, url: `${baseURL}/v1`, npm: "@ai-sdk/openai-compatible" },
              status: "active",
              headers: { Authorization: `Bearer ${apiKey}` },
              options: { apiKey },
              cost: { input: 0, output: 0, cache: { read: 0, write: 0 } },
              limit: { context: 0, output: 0 },
              capabilities: {
                temperature: true, reasoning: false, attachment: false, toolcall: true,
                input: { text: true, audio: false, image: false, video: false, pdf: false },
                output: { text: true, audio: false, image: false, video: false, pdf: false },
                interleaved: false,
              },
              release_date: "",
              variants: {},
            }
          }
          return {
            id: ProviderID.make("maas"),
            name: "MaaS",
            source: "custom" as const,
            env: [],
            options: { baseURL: `${baseURL}/v1`, apiKey },
            models,
          }
        }),
        Effect.catchAll((err) => {
          log.info("maas not available", { baseURL, error: String(err) })
          return Effect.succeed(null)
        }),
      )
    }

    function runDiscovery(): Effect.Effect<void> {
      // Strip trailing slashes — a common user mistake that produces double-slash URLs
      const ollamaHost = (process.env["TINYCODE_OLLAMA_HOST"] ?? "http://localhost:11434").replace(/\/+$/, "")
      const vllmHost = (process.env["TINYCODE_VLLM_HOST"] ?? "http://localhost:8000").replace(/\/+$/, "")
      const maasHost = process.env["TINYCODE_MAAS_HOST"]?.replace(/\/+$/, "")
      const maasKey = process.env["TINYCODE_MAAS_API_KEY"]

      return Effect.gen(function* () {
        const probes: Effect.Effect<Info | null>[] = [
          probeOllama(ollamaHost),
          probeVllm(vllmHost),
        ]
        if (maasHost && maasKey) probes.push(probeMaas(maasHost, maasKey))

        const results = yield* Effect.all(probes, { concurrency: probes.length })
        const [ollamaResult, vllmResult, maasResult] = results

        const next: Record<string, Info> = {}
        if (ollamaResult) next["ollama"] = ollamaResult
        if (vllmResult) next["vllm"] = vllmResult
        if (maasResult) next["maas"] = maasResult

        yield* Ref.set(discovered, next)
      })
    }

    // Run first discovery synchronously so providers are available immediately,
    // then poll every 30 seconds in the background for changes.
    yield* runDiscovery()
    yield* Effect.sleep(POLL_INTERVAL).pipe(
      Effect.andThen(runDiscovery()),
      Effect.repeat(Schedule.fixed(POLL_INTERVAL)),
      Effect.forkScoped,
    )

    const get = () => Ref.get(discovered)

    return Service.of({ get })
  }),
)

export const defaultLayer: Layer.Layer<Service> = layer.pipe(
  Layer.provide(FetchHttpClient.layer),
)

export * as LocalDiscovery from "./local-discovery"
