import { Context, Duration, Effect, Layer, Ref, Schedule } from "effect"
import { FetchHttpClient, HttpClient, HttpClientRequest, HttpClientResponse } from "effect/unstable/http"
import { Schema } from "effect"
import * as Log from "@/core/util/log"
import { ModelID, ProviderID } from "./schema"
import type { Info, Model } from "./provider"
import { rewriteLocalhostURL } from "@/util/container"
import { readFileSync } from "fs"

const log = Log.create({ service: "local-discovery" })

const PROBE_TIMEOUT = Duration.millis(2_000)
const POLL_INTERVAL = Duration.seconds(30)

// --- Ollama response schema ---

const OllamaModelDetails = Schema.Struct({
  parameter_size: Schema.optional(Schema.String),
  context_length: Schema.optional(Schema.Number),
  family: Schema.optional(Schema.String),
})

const OllamaModel = Schema.Struct({
  name: Schema.String,
  details: Schema.optional(OllamaModelDetails),
  capabilities: Schema.optional(Schema.Array(Schema.String)),
})

const OllamaTagsResponse = Schema.Struct({
  models: Schema.Array(OllamaModel),
})

// --- vLLM response schema ---

const VllmModel = Schema.Struct({
  id: Schema.String,
  max_model_len: Schema.optional(Schema.Number),
  meta: Schema.optional(Schema.Struct({
    n_ctx_train: Schema.optional(Schema.Number),
  })),
})

const VllmModelsResponse = Schema.Struct({
  data: Schema.Array(VllmModel),
})

// --- OpenRouter response schema ---

const OpenRouterModelArchitecture = Schema.Struct({
  input_modalities: Schema.optional(Schema.Array(Schema.String)),
  output_modalities: Schema.optional(Schema.Array(Schema.String)),
})

const OpenRouterModelPricing = Schema.Struct({
  prompt: Schema.optional(Schema.String),
  completion: Schema.optional(Schema.String),
})

const OpenRouterModelTopProvider = Schema.Struct({
  context_length: Schema.optional(Schema.Number),
  max_completion_tokens: Schema.optional(Schema.NullOr(Schema.Number)),
})

const OpenRouterModelReasoning = Schema.Struct({
  mandatory: Schema.optional(Schema.Boolean),
  default_enabled: Schema.optional(Schema.Boolean),
})

const OpenRouterModel = Schema.Struct({
  id: Schema.String,
  name: Schema.String,
  context_length: Schema.optional(Schema.Number),
  architecture: Schema.optional(OpenRouterModelArchitecture),
  pricing: Schema.optional(OpenRouterModelPricing),
  top_provider: Schema.optional(OpenRouterModelTopProvider),
  supported_parameters: Schema.optional(Schema.Array(Schema.String)),
  reasoning: Schema.optional(OpenRouterModelReasoning),
})

const OpenRouterModelsResponse = Schema.Struct({
  data: Schema.Array(OpenRouterModel),
})

const OPENROUTER_API_URL = "https://openrouter.ai/api/v1"
const OPENROUTER_PROBE_TIMEOUT = Duration.millis(5_000)

type OpenRouterModelEntry = Schema.Schema.Type<typeof OpenRouterModel>

function makeOpenRouterProvider(apiKey: string, entries: OpenRouterModelEntry[]): Info {
  const models: Record<string, Model> = {}
  for (const entry of entries) {
    const params = new Set(entry.supported_parameters ?? [])
    const inputMods = new Set(entry.architecture?.input_modalities ?? ["text"])
    const contextLength = entry.top_provider?.context_length ?? entry.context_length ?? 0
    const maxOutput = entry.top_provider?.max_completion_tokens ?? Math.min(16384, Math.floor(contextLength * 0.2))
    const promptCost = entry.pricing?.prompt ? parseFloat(entry.pricing.prompt) * 1_000_000 : 0
    const completionCost = entry.pricing?.completion ? parseFloat(entry.pricing.completion) * 1_000_000 : 0
    const hasReasoning = entry.reasoning?.mandatory === true || entry.reasoning?.default_enabled === true

    models[entry.id] = {
      id: ModelID.make(entry.id),
      providerID: ProviderID.make("openrouter"),
      name: entry.name,
      family: entry.id.split("/")[0] ?? "",
      api: {
        id: entry.id,
        url: OPENROUTER_API_URL,
        npm: "@openrouter/ai-sdk-provider",
      },
      status: "active",
      headers: {},
      options: {},
      cost: { input: promptCost, output: completionCost, cache: { read: 0, write: 0 } },
      limit: { context: contextLength, output: maxOutput },
      capabilities: {
        temperature: params.has("temperature"),
        reasoning: hasReasoning,
        attachment: inputMods.has("image") || inputMods.has("file"),
        toolcall: params.has("tools"),
        input: {
          text: inputMods.has("text"),
          audio: inputMods.has("audio"),
          image: inputMods.has("image"),
          video: inputMods.has("video"),
          pdf: inputMods.has("file"),
        },
        output: { text: true, audio: false, image: false, video: false, pdf: false },
        interleaved: false,
      },
      release_date: "",
      variants: {},
    }
  }
  return {
    id: ProviderID.make("openrouter"),
    name: "OpenRouter",
    source: "custom",
    env: ["OPENROUTER_API_KEY"],
    options: { apiKey },
    models,
  }
}

// --- Kubernetes API schemas ---

const K8sServicePort = Schema.Struct({
  name: Schema.optional(Schema.String),
  port: Schema.Number,
  protocol: Schema.optional(Schema.String),
})

const K8sServiceSpec = Schema.Struct({
  clusterIP: Schema.optional(Schema.String),
  ports: Schema.optional(Schema.Array(K8sServicePort)),
})

const K8sObjectMeta = Schema.Struct({
  name: Schema.String,
  namespace: Schema.optional(Schema.String),
  labels: Schema.optional(Schema.Record(Schema.String, Schema.String)),
  annotations: Schema.optional(Schema.Record(Schema.String, Schema.String)),
})

const K8sService = Schema.Struct({
  metadata: K8sObjectMeta,
  spec: K8sServiceSpec,
})

const K8sServiceList = Schema.Struct({
  items: Schema.Array(K8sService),
})

// --- Helpers ---

type OllamaModelEntry = Schema.Schema.Type<typeof OllamaModel>

function makeOllamaProvider(baseURL: string, entries: OllamaModelEntry[]): Info {
  const models: Record<string, Model> = {}
  for (const entry of entries) {
    const caps = new Set(entry.capabilities ?? [])
    const contextLimit = entry.details?.context_length ?? 0
    const effectiveContext = contextLimit > 0 ? Math.floor(contextLimit * 0.8) : 0
    const outputLimit = contextLimit > 0 ? Math.min(4096, Math.floor(contextLimit * 0.2)) : 0
    const model: Model = {
      id: ModelID.make(entry.name),
      providerID: ProviderID.make("ollama"),
      name: entry.name,
      family: entry.details?.family ?? "",
      api: {
        id: entry.name,
        url: `${baseURL}/v1`,
        npm: "@ai-sdk/openai-compatible",
      },
      status: "active",
      headers: {},
      options: {},
      cost: { input: 0, output: 0, cache: { read: 0, write: 0 } },
      limit: { context: effectiveContext, output: outputLimit },
      capabilities: {
        temperature: true,
        reasoning: caps.has("thinking"),
        attachment: caps.has("vision"),
        toolcall: caps.has("tools"),
        input: { text: true, audio: false, image: caps.has("vision"), video: false, pdf: false },
        output: { text: true, audio: false, image: false, video: false, pdf: false },
        interleaved: false,
      },
      release_date: "",
      variants: {},
    }
    models[entry.name] = model
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

type VllmModelEntry = { id: string; max_model_len?: number | undefined; meta?: { n_ctx_train?: number } }

function makeVllmProvider(baseURL: string, modelIds: string[]): Info
function makeVllmProvider(baseURL: string, models: VllmModelEntry[], providerID?: string, providerName?: string): Info
function makeVllmProvider(
  baseURL: string,
  modelsOrIds: string[] | VllmModelEntry[],
  providerID = "vllm",
  providerName = "vLLM",
): Info {
  const entries: VllmModelEntry[] =
    typeof modelsOrIds[0] === "string"
      ? (modelsOrIds as string[]).map((id) => ({ id }))
      : (modelsOrIds as VllmModelEntry[])

  const models: Record<string, Model> = {}
  for (const entry of entries) {
    const contextLimit = entry.max_model_len ?? entry.meta?.n_ctx_train ?? 0
    // Reserve ~20% for system prompt + tools; remainder for conversation
    const effectiveContext = contextLimit > 0 ? Math.floor(contextLimit * 0.8) : 0
    const outputLimit = contextLimit > 0 ? Math.min(4096, Math.floor(contextLimit * 0.2)) : 0
    const model: Model = {
      id: ModelID.make(entry.id),
      providerID: ProviderID.make(providerID),
      name: entry.id,
      family: "",
      api: {
        id: entry.id,
        url: `${baseURL}/v1`,
        npm: "@ai-sdk/openai-compatible",
      },
      status: "active",
      headers: {},
      options: {},
      cost: { input: 0, output: 0, cache: { read: 0, write: 0 } },
      limit: { context: effectiveContext, output: outputLimit },
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
    models[entry.id] = model
  }
  return {
    id: ProviderID.make(providerID),
    name: providerName,
    source: "custom",
    env: [],
    options: { baseURL: `${baseURL}/v1` },
    models,
  }
}

// --- Kubernetes in-cluster helpers ---

function isInCluster(): boolean {
  return !!process.env["KUBERNETES_SERVICE_HOST"]
}

function readInClusterConfig(): { token: string; namespace: string; apiBase: string } | null {
  try {
    const token = readFileSync("/var/run/secrets/kubernetes.io/serviceaccount/token", "utf8").trim()
    const namespace = readFileSync("/var/run/secrets/kubernetes.io/serviceaccount/namespace", "utf8").trim()
    const host = process.env["KUBERNETES_SERVICE_HOST"]!
    const port = process.env["KUBERNETES_SERVICE_PORT"] ?? "443"
    return { token, namespace, apiBase: `https://${host}:${port}` }
  } catch {
    return null
  }
}

// Ports to probe on each discovered service, in priority order.
const VLLM_CANDIDATE_PORTS = [8080, 8000, 80]

// Labels that identify a KServe/RHOAI InferenceService serving pod.
const KSERVE_LABEL = "serving.kserve.io/inferenceservice"
// Annotation that explicitly opts a service into tinycode discovery.
const TINYCODE_DISCOVER_ANNOTATION = "tinycode.dev/discover"

export interface Interface {
  readonly get: () => Effect.Effect<Record<string, Info>>
}

export class Service extends Context.Service<Service, Interface>()("@tinycode/LocalDiscovery") {}

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
          const entries = data.models.filter((m) => m.name.length > 0)
          if (entries.length === 0) return null
          log.info("ollama discovered", { count: entries.length, models: entries.slice(0, 5).map((m) => m.name) })
          return makeOllamaProvider(baseURL, entries)
        }),
        Effect.catch((err) => {
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
          const entries = data.data.filter((m) => m.id.length > 0)
          if (entries.length === 0) return null
          log.info("vllm discovered", { count: entries.length, models: entries.slice(0, 5).map((m) => m.id) })
          return makeVllmProvider(baseURL, entries)
        }),
        Effect.catch((err) => {
          log.info("vllm not available", { baseURL, error: String(err) })
          return Effect.succeed(null)
        }),
      )
    }

    // Probe a single URL for a vLLM-compatible /v1/models endpoint.
    // Returns a provider keyed by providerID if successful, null otherwise.
    function probeVllmUrl(
      url: string,
      providerID: string,
      providerName: string,
      token?: string,
    ): Effect.Effect<[string, Info] | null> {
      const req = token
        ? HttpClientRequest.get(`${url}/v1/models`).pipe(
            HttpClientRequest.setHeader("Authorization", `Bearer ${token}`),
          )
        : HttpClientRequest.get(`${url}/v1/models`)
      return req.pipe(
        httpClient.execute,
        Effect.timeout(PROBE_TIMEOUT),
        Effect.flatMap((res) => HttpClientResponse.schemaBodyJson(VllmModelsResponse)(res)),
        Effect.map((data) => {
          const entries = data.data.filter((m) => m.id.length > 0 && !m.id.toLowerCase().includes("embed"))
          if (entries.length === 0) return null
          log.info("kubernetes vllm discovered", { service: providerID, models: entries.map((m) => m.id) })
          return [providerID, makeVllmProvider(url, entries, providerID, providerName)] as [string, Info]
        }),
        Effect.catch(() => Effect.succeed(null)),
      )
    }

    // Discover vLLM services running in the same Kubernetes namespace.
    //
    // Priority order:
    // 1. Services with annotation tinycode.dev/discover=vllm  (explicit opt-in)
    // 2. Services with label serving.kserve.io/inferenceservice (RHOAI/KServe)
    // 3. TINYCODE_VLLM_URLS env var (comma-separated explicit URLs)
    //
    // Each discovered service becomes its own provider keyed as "vllm-<service-name>",
    // so multiple models can coexist in the model picker.
    function probeKubernetesVllm(): Effect.Effect<Record<string, Info>, never, never> {
      return Effect.gen(function* () {
        const result: Record<string, Info> = {}

        // Explicit multi-URL override — always checked regardless of cluster detection
        const explicitUrls = process.env["TINYCODE_VLLM_URLS"]
        if (explicitUrls) {
          const urls = explicitUrls
            .split(",")
            .map((u) => u.trim())
            .filter(Boolean)
          const probes = yield* Effect.all(
            urls.map((url, i) => probeVllmUrl(url, `vllm-${i}`, `vLLM (${url})`)),
            { concurrency: urls.length },
          )
          for (const entry of probes) {
            if (entry) result[entry[0]] = entry[1]
          }
        }

        // In-cluster discovery via Kubernetes API
        if (!isInCluster()) return result
        const clusterConfig = readInClusterConfig()
        if (!clusterConfig) return result

        const { token, namespace, apiBase } = clusterConfig

        // Fetch services in the current namespace
        const svcList = yield* HttpClientRequest.get(`${apiBase}/api/v1/namespaces/${namespace}/services`).pipe(
          HttpClientRequest.setHeader("Authorization", `Bearer ${token}`),
          httpClient.execute,
          Effect.timeout(Duration.millis(5_000)),
          Effect.flatMap((res) => HttpClientResponse.schemaBodyJson(K8sServiceList)(res)),
          Effect.catch(() => Effect.succeed({ items: [] as (typeof K8sServiceList.Type)["items"] })),
        )

        // Score and filter services: explicit annotation > KServe label > probe-all
        const candidates: Array<{ name: string; clusterIP: string; ports: number[]; priority: number }> = []

        for (const svc of svcList.items) {
          const clusterIP = svc.spec.clusterIP
          if (!clusterIP || clusterIP === "None") continue

          const annotations = svc.metadata.annotations ?? {}
          const labels = svc.metadata.labels ?? {}
          const name = svc.metadata.name

          // Skip Kubernetes internal services
          if (name.startsWith("kubernetes") || name.endsWith("-metrics")) continue

          const isExplicit = annotations[TINYCODE_DISCOVER_ANNOTATION] === "vllm"
          const isKServe = KSERVE_LABEL in labels

          // Collect candidate ports: prefer http-named ports, then known vLLM ports
          const svcPorts = (svc.spec.ports ?? [])
            .filter((p: { protocol?: string; port: number }) => p.protocol === "TCP" || !p.protocol)
            .map((p: { port: number }) => p.port)
          const ports = svcPorts.length > 0 ? svcPorts : VLLM_CANDIDATE_PORTS

          if (isExplicit) {
            candidates.push({ name, clusterIP, ports, priority: 0 })
          } else if (isKServe) {
            candidates.push({ name, clusterIP, ports, priority: 1 })
          } else {
            // Probe-all: only try known vLLM ports to limit noise
            candidates.push({ name, clusterIP, ports: VLLM_CANDIDATE_PORTS, priority: 2 })
          }
        }

        // Sort by priority, probe all concurrently
        candidates.sort((a, b) => a.priority - b.priority)

        const probes = candidates.flatMap((c) =>
          c.ports.map((port) => probeVllmUrl(`http://${c.clusterIP}:${port}`, `vllm-${c.name}`, c.name)),
        )

        const probeResults = yield* Effect.all(probes, { concurrency: 10 })
        for (const entry of probeResults) {
          // First successful result per provider ID wins
          if (entry && !(entry[0] in result)) result[entry[0]] = entry[1]
        }

        if (Object.keys(result).length > 0) {
          log.info("kubernetes vllm discovery complete", {
            namespace,
            found: Object.keys(result),
          })
        }

        return result
      })
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
          const entries = data.data.filter((m) => m.id.length > 0 && !m.id.toLowerCase().includes("embed"))
          if (entries.length === 0) return null
          log.info("maas discovered", { host: baseURL, count: entries.length, models: entries.slice(0, 5).map((m) => m.id) })
          const models: Record<string, Model> = {}
          for (const entry of entries) {
            const contextLimit = entry.max_model_len ?? 0
            const effectiveContext = contextLimit > 0 ? Math.floor(contextLimit * 0.8) : 0
            const outputLimit = contextLimit > 0 ? Math.min(4096, Math.floor(contextLimit * 0.2)) : 0
            models[entry.id] = {
              id: ModelID.make(entry.id),
              providerID: ProviderID.make("maas"),
              name: entry.id,
              family: "",
              api: { id: entry.id, url: `${baseURL}/v1`, npm: "@ai-sdk/openai-compatible" },
              status: "active",
              headers: { Authorization: `Bearer ${apiKey}` },
              options: { apiKey },
              cost: { input: 0, output: 0, cache: { read: 0, write: 0 } },
              limit: { context: effectiveContext, output: outputLimit },
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
        Effect.catch((err) => {
          log.info("maas not available", { baseURL, error: String(err) })
          return Effect.succeed(null)
        }),
      )
    }

    function probeRamalama(baseURL: string): Effect.Effect<Info | null> {
      return HttpClientRequest.get(`${baseURL}/v1/models`).pipe(
        httpClient.execute,
        Effect.timeout(PROBE_TIMEOUT),
        Effect.flatMap((res) => HttpClientResponse.schemaBodyJson(VllmModelsResponse)(res)),
        Effect.map((data) => {
          const entries = data.data.filter((m) => m.id.length > 0)
          if (entries.length === 0) return null
          log.info("ramalama discovered", { count: entries.length, models: entries.slice(0, 5).map((m) => m.id) })
          return makeVllmProvider(baseURL, entries, "ramalama", "RamaLama")
        }),
        Effect.catch((err) => {
          log.info("ramalama not available", { baseURL, error: String(err) })
          return Effect.succeed(null)
        }),
      )
    }

    function probeOpenRouter(apiKey: string): Effect.Effect<Info | null> {
      return HttpClientRequest.get(`${OPENROUTER_API_URL}/models`).pipe(
        HttpClientRequest.setHeader("Authorization", `Bearer ${apiKey}`),
        httpClient.execute,
        Effect.timeout(OPENROUTER_PROBE_TIMEOUT),
        Effect.flatMap((res) => HttpClientResponse.schemaBodyJson(OpenRouterModelsResponse)(res)),
        Effect.map((data) => {
          const entries = data.data.filter((m) => {
            if (!m.id || m.id.length === 0) return false
            if (m.id.includes(":free") || m.id.includes(":beta")) return false
            const params = new Set(m.supported_parameters ?? [])
            return params.has("tools")
          })
          if (entries.length === 0) return null
          log.info("openrouter discovered", { count: entries.length })
          return makeOpenRouterProvider(apiKey, entries)
        }),
        Effect.catch((err) => {
          log.info("openrouter not available", { error: String(err) })
          return Effect.succeed(null)
        }),
      )
    }

    function runDiscovery(): Effect.Effect<void> {
      // Strip trailing slashes — a common user mistake that produces double-slash URLs
      const ollamaHostEnv = process.env["TINYCODE_OLLAMA_HOST"]
      const vllmHostEnv = process.env["TINYCODE_VLLM_HOST"]
      const ramalamaHostEnv = process.env["TINYCODE_RAMALAMA_HOST"]
      // Only rewrite to container hostname if the user didn't explicitly set the host
      const ollamaHost = ollamaHostEnv
        ? ollamaHostEnv.replace(/\/+$/, "")
        : rewriteLocalhostURL("http://localhost:11434")
      const vllmHost = vllmHostEnv ? vllmHostEnv.replace(/\/+$/, "") : rewriteLocalhostURL("http://localhost:8000")
      const ramalamaHost = ramalamaHostEnv ? ramalamaHostEnv.replace(/\/+$/, "") : undefined
      const maasHost = process.env["TINYCODE_MAAS_HOST"]?.replace(/\/+$/, "")
      const maasKey = process.env["TINYCODE_MAAS_API_KEY"]
      const openRouterKey = process.env["OPENROUTER_API_KEY"]

      return Effect.gen(function* () {
        const probes: Effect.Effect<Info | null>[] = [probeOllama(ollamaHost), probeVllm(vllmHost)]
        if (ramalamaHost) probes.push(probeRamalama(ramalamaHost))
        if (maasHost && maasKey) probes.push(probeMaas(maasHost, maasKey))

        const cloudProbes: Effect.Effect<Info | null>[] = []
        if (openRouterKey) cloudProbes.push(probeOpenRouter(openRouterKey))

        const [results, k8sProviders, ...cloudResults] = yield* Effect.all(
          [
            Effect.all(probes, { concurrency: probes.length }),
            probeKubernetesVllm(),
            ...cloudProbes,
          ],
          { concurrency: "unbounded" },
        )
        const [ollamaResult, vllmResult, ...rest] = results
        const ramalamaResult = ramalamaHost ? rest[0] : null
        const maasResult = ramalamaHost ? rest[1] : rest[0]
        const openRouterResult = openRouterKey ? cloudResults[0] : null

        const next: Record<string, Info> = {}
        if (ollamaResult) next["ollama"] = ollamaResult
        // localhost vllm only added if no k8s vllm services found
        if (vllmResult && Object.keys(k8sProviders).length === 0) next["vllm"] = vllmResult
        if (ramalamaResult) next["ramalama"] = ramalamaResult
        if (maasResult) next["maas"] = maasResult
        if (openRouterResult) next["openrouter"] = openRouterResult
        // Merge k8s-discovered vllm providers (each keyed as "vllm-<service-name>")
        Object.assign(next, k8sProviders)

        yield* Ref.set(discovered, next)
      })
    }

    // Run discovery in the background — forked so startup never hangs.
    yield* runDiscovery().pipe(Effect.repeat(Schedule.fixed(POLL_INTERVAL)), Effect.forkScoped)

    const get = () => Ref.get(discovered)

    return Service.of({ get })
  }),
)

export const defaultLayer: Layer.Layer<Service> = layer.pipe(Layer.provide(FetchHttpClient.layer))

export * as LocalDiscovery from "./local-discovery"
