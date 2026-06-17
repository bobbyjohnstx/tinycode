import type { Argv } from "yargs"
import { UI } from "../ui"
import * as prompts from "@clack/prompts"
import { Global } from "@/core/global"
import { InstallationChannel } from "@/core/installation/version"
import fsNode from "fs"
import path from "path"
import os from "os"

interface SetupArgs {
  force: boolean
  "non-interactive": boolean
  provider?: string
  "api-key"?: string
}

export const SetupCommand = {
  command: "setup",
  describe: "set up tinycode configuration, agents, skills, and MCP servers",
  builder: (yargs: Argv) =>
    yargs
      .option("force", {
        alias: "f",
        type: "boolean",
        describe: "overwrite existing config.json",
        default: false,
      })
      .option("non-interactive", {
        type: "boolean",
        describe: "skip all prompts (for CI/containers)",
        default: false,
      })
      .option("provider", {
        type: "string",
        describe: "provider id to configure (used with --non-interactive)",
      })
      .option("api-key", {
        type: "string",
        describe: "API key for the provider (used with --non-interactive)",
      }),

  handler: async (args: SetupArgs) => {
    const nonInteractive = args["non-interactive"]

    if (!nonInteractive) {
      UI.empty()
      UI.println(UI.logo("  "))
      UI.empty()
    }
    prompts.intro("Setup Tinycode")

    const configDir = Global.Path.config
    const agentDir = path.join(configDir, "agent")
    const skillsDir = path.join(configDir, "skills")
    const mcpDir = path.join(configDir, "mcp")

    // Step 1: Create directories
    {
      const spinner = prompts.spinner()
      spinner.start("Creating directories...")
      try {
        await fsNode.promises.mkdir(agentDir, { recursive: true, mode: 0o700 })
        await fsNode.promises.mkdir(skillsDir, { recursive: true, mode: 0o700 })
        await fsNode.promises.mkdir(mcpDir, { recursive: true, mode: 0o700 })
        spinner.stop("Directories created")
      } catch (e: any) {
        spinner.stop("Failed to create directories", 1)
        prompts.log.error(e.message)
        prompts.outro("Setup failed")
        return
      }
    }

    // Step 2: Copy agent .md files
    {
      const spinner = prompts.spinner()
      spinner.start("Copying agent files...")
      try {
        const sourceAgentDir =
          InstallationChannel === "local"
            ? path.resolve(import.meta.dirname, "../../../../.tinycode/agent/")
            : path.resolve(import.meta.dirname, "../../../../.tinycode/agent/")

        const files = await fsNode.promises.readdir(sourceAgentDir).catch(() => [] as string[])
        let copied = 0
        for (const file of files) {
          if (!file.endsWith(".md")) continue
          const src = path.join(sourceAgentDir, file)
          const dest = path.join(agentDir, file)
          await fsNode.promises.copyFile(src, dest)
          copied++
        }
        spinner.stop(`Copied ${copied} agent file${copied !== 1 ? "s" : ""}`)
      } catch (e: any) {
        spinner.stop("Failed to copy agent files", 1)
        prompts.log.warn(e.message)
      }
    }

    // Step 3: Copy skill directories
    {
      const spinner = prompts.spinner()
      spinner.start("Copying skills...")
      try {
        const sourceSkillsDir =
          InstallationChannel === "local"
            ? path.resolve(import.meta.dirname, "../../../../.tinycode/skills/")
            : path.resolve(import.meta.dirname, "../../../../.tinycode/skills/")

        const entries = await fsNode.promises
          .readdir(sourceSkillsDir, { withFileTypes: true })
          .catch(() => [] as fsNode.Dirent[])

        let copied = 0
        for (const entry of entries) {
          if (!entry.isDirectory()) continue
          const skillName = entry.name
          const srcSkillMd = path.join(sourceSkillsDir, skillName, "SKILL.md")
          const destSkillDir = path.join(skillsDir, skillName)
          const destSkillMd = path.join(destSkillDir, "SKILL.md")

          const exists = await fsNode.promises
            .access(srcSkillMd)
            .then(() => true)
            .catch(() => false)
          if (!exists) continue

          await fsNode.promises.mkdir(destSkillDir, { recursive: true, mode: 0o700 })
          await fsNode.promises.copyFile(srcSkillMd, destSkillMd)
          copied++
        }
        spinner.stop(`Copied ${copied} skill${copied !== 1 ? "s" : ""}`)
      } catch (e: any) {
        spinner.stop("Failed to copy skills", 1)
        prompts.log.warn(e.message)
      }
    }

    // Step 4: Copy help.md if it exists
    {
      const helpSrc = path.resolve(import.meta.dirname, "../tui/help.md")
      const helpDest = path.join(configDir, "help.md")
      const exists = await fsNode.promises
        .access(helpSrc)
        .then(() => true)
        .catch(() => false)
      if (exists) {
        const spinner = prompts.spinner()
        spinner.start("Copying help.md...")
        try {
          await fsNode.promises.copyFile(helpSrc, helpDest)
          spinner.stop("Copied help.md")
        } catch (e: any) {
          spinner.stop("Failed to copy help.md", 1)
          prompts.log.warn(e.message)
        }
      }
    }

    // Step 5: Write default config.json if absent (or --force)
    const configFile = path.join(configDir, "config.json")
    const configExists = await fsNode.promises
      .access(configFile)
      .then(() => true)
      .catch(() => false)

    if (!configExists || args.force) {
      const spinner = prompts.spinner()
      spinner.start(configExists ? "Overwriting config.json (--force)..." : "Writing default config.json...")
      try {
        const defaultConfig = {
          lsp: true,
          skills: {
            paths: [skillsDir],
          },
        }
        await fsNode.promises.writeFile(configFile, JSON.stringify(defaultConfig, null, 2) + "\n", {
          encoding: "utf8",
          mode: 0o600,
        })
        spinner.stop("Config written")
      } catch (e: any) {
        spinner.stop("Failed to write config.json", 1)
        prompts.log.error(e.message)
      }
    } else {
      prompts.log.step("Config already exists, skipping (use --force to overwrite)")
    }

    // Non-interactive provider setup
    const authFile = path.join(Global.Path.data, "auth.json")
    if (nonInteractive && args.provider) {
      const apiKey = args["api-key"] || process.env[`${args.provider.toUpperCase()}_API_KEY`]
      if (!apiKey) {
        prompts.log.error(
          `No API key provided. Use --api-key or set ${args.provider.toUpperCase()}_API_KEY environment variable.`,
        )
        process.exitCode = 1
        return
      }
      try {
        let authData: Record<string, unknown> = {}
        try {
          const existing = await fsNode.promises.readFile(authFile, "utf8")
          authData = JSON.parse(existing)
        } catch {}
        authData[args.provider] = { type: "api", key: apiKey }
        await fsNode.promises.writeFile(authFile, JSON.stringify(authData, null, 2), {
          encoding: "utf8",
          mode: 0o600,
        })
        prompts.log.success(`Configured provider: ${args.provider}`)
      } catch (e: any) {
        prompts.log.error(`Failed to configure provider: ${e.message}`)
        process.exitCode = 1
        return
      }
    }

    // Interactive onboarding wizard (first run only, when no config existed)
    if (!nonInteractive && !configExists) {
      const shouldSetupProvider = await prompts.confirm({
        message: "Would you like to set up an AI provider now?",
      })
      if (shouldSetupProvider === true) {
        prompts.log.info("Run `tinycode providers login` to add an AI provider.")
      }
    }

    // Post-setup guidance
    UI.empty()
    const homedir = os.homedir()
    const displayConfigDir = configDir.startsWith(homedir) ? configDir.replace(homedir, "~") : configDir

    const hasAuth = await fsNode.promises
      .access(authFile)
      .then(() => true)
      .catch(() => false)

    if (!hasAuth && !(nonInteractive && args.provider)) {
      prompts.log.message("Next steps:")
      prompts.log.step("Run `tinycode providers login` to add an AI provider")
    }
    if (!configExists || args.force) {
      prompts.log.step(`Edit \`${path.join(displayConfigDir, "tinycode.json")}\` to customize`)
    }
    prompts.log.step("Run `tinycode` to start a session")

    prompts.outro("Setup complete")
  },
}
