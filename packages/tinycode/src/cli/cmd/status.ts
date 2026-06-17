import { Effect } from "effect"
import { effectCmd } from "../effect-cmd"
import { UI } from "../ui"
import * as Prompt from "../effect/prompt"
import { Auth } from "../../auth"
import { ModelsDev } from "@/core/models-dev"
import { Global } from "@/core/global"
import { InstallationVersion, InstallationChannel } from "@/core/installation/version"
import { existsSync } from "fs"
import path from "path"
import os from "os"

export const StatusCommand = effectCmd({
  command: "status",
  describe: "show tinycode status and provider health",
  instance: false,
  builder: (yargs) =>
    yargs.option("json", {
      type: "boolean",
      describe: "output machine-readable JSON",
      default: false,
    }),
  handler: Effect.fn("Cli.status")(function* (args) {
    const authSvc = yield* Auth.Service
    const modelsDev = yield* ModelsDev.Service

    const credentials = Object.entries(yield* Effect.orDie(authSvc.all()))
    const database = yield* modelsDev.get()

    const homedir = os.homedir()
    const configPath = Global.Path.config
    const displayConfig = configPath.startsWith(homedir) ? configPath.replace(homedir, "~") : configPath

    // Check env var providers
    const envProviders: Array<{ provider: string; envVar: string }> = []
    for (const [providerID, provider] of Object.entries(database)) {
      for (const envVar of provider.env) {
        if (process.env[envVar]) {
          envProviders.push({ provider: provider.name || providerID, envVar })
        }
      }
    }

    const providers: Array<{
      name: string
      id: string
      auth: string
      models: number
      reachable: boolean
    }> = []

    for (const [providerID, provider] of Object.entries(database)) {
      const credential = credentials.find(([key]) => key === providerID)
      const envAuth = provider.env.find((e) => process.env[e])
      const authMethod = credential ? credential[1].type : envAuth ? "env" : "none"
      const modelCount = Object.keys(provider.models).length
      const reachable = authMethod !== "none"

      providers.push({
        name: provider.name || providerID,
        id: providerID,
        auth: authMethod,
        models: modelCount,
        reachable,
      })
    }

    // JSON output
    if (args.json) {
      const output = {
        version: InstallationVersion,
        channel: InstallationChannel,
        config: configPath,
        providers: providers.map((p) => ({
          id: p.id,
          name: p.name,
          auth: p.auth,
          models: p.models,
          reachable: p.reachable,
        })),
        warnings: [] as string[],
      }
      process.stdout.write(JSON.stringify(output, null, 2) + "\n")
      if (!providers.some((p) => p.reachable)) {
        process.exitCode = 1
      }
      return
    }

    // Human-readable output
    UI.empty()
    yield* Prompt.intro("Tinycode Status")

    yield* Prompt.log.info(`Version: ${InstallationVersion} (${InstallationChannel})`)
    yield* Prompt.log.info(`Config:  ${displayConfig}`)

    if (providers.length === 0) {
      yield* Prompt.log.warn("No providers found")
    } else {
      UI.empty()
      for (const p of providers) {
        const status = p.reachable
          ? UI.Style.TEXT_SUCCESS_BOLD + "ready" + UI.Style.TEXT_NORMAL
          : UI.Style.TEXT_DIM + "no auth" + UI.Style.TEXT_NORMAL
        yield* Prompt.log.info(
          `${p.name} ${UI.Style.TEXT_DIM}(${p.id})${UI.Style.TEXT_NORMAL} ${status} - ${p.models} model${p.models !== 1 ? "s" : ""} - ${p.auth}`,
        )
      }
    }

    if (envProviders.length > 0) {
      UI.empty()
      for (const { provider, envVar } of envProviders) {
        yield* Prompt.log.info(`${provider} via ${UI.Style.TEXT_DIM}${envVar}${UI.Style.TEXT_NORMAL}`)
      }
    }

    const warnings: string[] = []

    // Check for legacy config
    const legacyConfig = path.join(os.homedir(), ".config", "opencode")
    if (existsSync(legacyConfig)) {
      warnings.push("Legacy opencode config directory detected at " + legacyConfig)
    }

    if (warnings.length > 0) {
      UI.empty()
      for (const w of warnings) {
        yield* Prompt.log.warn(w)
      }
    }

    const reachableCount = providers.filter((p) => p.reachable).length
    yield* Prompt.outro(`${reachableCount}/${providers.length} providers ready`)

    if (reachableCount === 0) {
      process.exitCode = 1
    }
  }),
})
