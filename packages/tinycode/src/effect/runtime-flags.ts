import { Config, ConfigProvider, Context, Effect, Layer, Option } from "effect"
import { ConfigService } from "@/effect/config-service"

const bool = (name: string) => Config.boolean(name).pipe(Config.withDefault(false))
const positiveInteger = (name: string) =>
  Config.number(name).pipe(
    Config.map((value) => (Number.isInteger(value) && value > 0 ? value : undefined)),
    Config.orElse(() => Config.succeed(undefined)),
  )
const experimental = bool("TINYCODE_EXPERIMENTAL")
const enabledByExperimental = (name: string) =>
  Config.all({ experimental, enabled: Config.boolean(name).pipe(Config.option) }).pipe(
    Config.map((flags) => Option.getOrElse(flags.enabled, () => flags.experimental)),
  )

export class Service extends ConfigService.Service<Service>()("@tinycode/RuntimeFlags", {
  autoShare: bool("TINYCODE_AUTO_SHARE"),
  pure: bool("TINYCODE_PURE"),
  disableDefaultPlugins: bool("TINYCODE_DISABLE_DEFAULT_PLUGINS"),
  disableChannelDb: bool("TINYCODE_DISABLE_CHANNEL_DB"),
  disableEmbeddedWebUi: bool("TINYCODE_DISABLE_EMBEDDED_WEB_UI"),
  disableExternalSkills: bool("TINYCODE_DISABLE_EXTERNAL_SKILLS"),
  disableLspDownload: bool("TINYCODE_DISABLE_LSP_DOWNLOAD"),
  skipMigrations: bool("TINYCODE_SKIP_MIGRATIONS"),
  disableClaudeCodePrompt: Config.all({
    broad: bool("TINYCODE_DISABLE_CLAUDE_CODE"),
    direct: bool("TINYCODE_DISABLE_CLAUDE_CODE_PROMPT"),
  }).pipe(Config.map((flags) => flags.broad || flags.direct)),
  disableClaudeCodeSkills: Config.all({
    broad: bool("TINYCODE_DISABLE_CLAUDE_CODE"),
    direct: bool("TINYCODE_DISABLE_CLAUDE_CODE_SKILLS"),
  }).pipe(Config.map((flags) => flags.broad || flags.direct)),
  enableExa: Config.all({
    experimental,
    enabled: bool("TINYCODE_ENABLE_EXA"),
    legacy: bool("TINYCODE_EXPERIMENTAL_EXA"),
  }).pipe(Config.map((flags) => flags.experimental || flags.enabled || flags.legacy)),
  enableParallel: Config.all({
    enabled: bool("TINYCODE_ENABLE_PARALLEL"),
    legacy: bool("TINYCODE_EXPERIMENTAL_PARALLEL"),
  }).pipe(Config.map((flags) => flags.enabled || flags.legacy)),
  enableExperimentalModels: bool("TINYCODE_ENABLE_EXPERIMENTAL_MODELS"),
  enableQuestionTool: bool("TINYCODE_ENABLE_QUESTION_TOOL"),
  experimentalScout: enabledByExperimental("TINYCODE_EXPERIMENTAL_SCOUT"),
  experimentalBackgroundSubagents: enabledByExperimental("TINYCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS"),
  experimentalLspTy: bool("TINYCODE_EXPERIMENTAL_LSP_TY"),
  experimentalLspTool: enabledByExperimental("TINYCODE_EXPERIMENTAL_LSP_TOOL"),
  experimentalOxfmt: enabledByExperimental("TINYCODE_EXPERIMENTAL_OXFMT"),
  experimentalPlanMode: enabledByExperimental("TINYCODE_EXPERIMENTAL_PLAN_MODE"),
  experimentalEventSystem: enabledByExperimental("TINYCODE_EXPERIMENTAL_EVENT_SYSTEM"),
  experimentalWorkspaces: enabledByExperimental("TINYCODE_EXPERIMENTAL_WORKSPACES"),
  experimentalIconDiscovery: enabledByExperimental("TINYCODE_EXPERIMENTAL_ICON_DISCOVERY"),
  acpNext: bool("TINYCODE_ACP_NEXT"),
  outputTokenMax: positiveInteger("TINYCODE_EXPERIMENTAL_OUTPUT_TOKEN_MAX"),
  bashDefaultTimeoutMs: positiveInteger("TINYCODE_EXPERIMENTAL_BASH_DEFAULT_TIMEOUT_MS"),
  experimentalNativeLlm: bool("TINYCODE_EXPERIMENTAL_NATIVE_LLM"),
  experimentalWebSockets: bool("TINYCODE_EXPERIMENTAL_WEBSOCKETS"),
  client: Config.string("TINYCODE_CLIENT").pipe(Config.withDefault("cli")),
}) {}

export type Info = Context.Service.Shape<typeof Service>

const emptyConfigLayer = Service.defaultLayer.pipe(
  Layer.provide(ConfigProvider.layer(ConfigProvider.fromUnknown({}))),
  Layer.orDie,
)

export const layer = (overrides: Partial<Info> = {}) =>
  Layer.effect(
    Service,
    Effect.gen(function* () {
      const flags = yield* Service
      return Service.of({ ...flags, ...overrides })
    }),
  ).pipe(Layer.provide(emptyConfigLayer))

export const defaultLayer = Service.defaultLayer.pipe(Layer.orDie)

export * as RuntimeFlags from "./runtime-flags"
