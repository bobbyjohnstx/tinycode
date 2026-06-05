import type { Argv } from "yargs"
import { UI } from "../ui"
import * as prompts from "@clack/prompts"
import { Global } from "@opencode-ai/core/global"
import { InstallationChannel } from "@opencode-ai/core/installation/version"
import fsNode from "fs"
import path from "path"

interface SetupArgs {
  force: boolean
  skipMcp: boolean
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
      .option("skip-mcp", {
        type: "boolean",
        describe: "skip installing MCP servers",
        default: false,
      }),

  handler: async (args: SetupArgs) => {
    UI.empty()
    UI.println(UI.logo("  "))
    UI.empty()
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
        await fsNode.promises.mkdir(agentDir, { recursive: true })
        await fsNode.promises.mkdir(skillsDir, { recursive: true })
        await fsNode.promises.mkdir(mcpDir, { recursive: true })
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

          await fsNode.promises.mkdir(destSkillDir, { recursive: true })
          await fsNode.promises.copyFile(srcSkillMd, destSkillMd)
          copied++
        }
        spinner.stop(`Copied ${copied} skill${copied !== 1 ? "s" : ""}`)
      } catch (e: any) {
        spinner.stop("Failed to copy skills", 1)
        prompts.log.warn(e.message)
      }
    }

    // Step 4: Install oh-my-tiny MCP server
    if (!args.skipMcp) {
      const spinner = prompts.spinner()
      spinner.start("Installing oh-my-tiny MCP server...")
      try {
        // Create package.json if it doesn't exist
        const pkgJson = path.join(mcpDir, "package.json")
        const pkgExists = await fsNode.promises
          .access(pkgJson)
          .then(() => true)
          .catch(() => false)
        if (!pkgExists) {
          await fsNode.promises.writeFile(pkgJson, "{}\n", "utf8")
        }

        // Create .npmrc pointing at Gitea registry
        const npmrc = path.join(mcpDir, ".npmrc")
        await fsNode.promises.writeFile(
          npmrc,
          "//localhost:3000/api/packages/bjohns/npm/:_authToken=c0112b78b112703d8b5fb740f99b6cc3f4f57215\n",
          "utf8",
        )

        const proc = Bun.spawn(
          ["npm", "install", "oh-my-tiny", "--registry", "http://localhost:3000/api/packages/bjohns/npm/"],
          {
            cwd: mcpDir,
            stdout: "pipe",
            stderr: "pipe",
          },
        )

        await proc.exited
        if (proc.exitCode !== 0) {
          const stderr = await new Response(proc.stderr).text()
          spinner.stop("Failed to install oh-my-tiny", 1)
          prompts.log.warn(stderr.trim() || "npm install exited with code " + proc.exitCode)
        } else {
          spinner.stop("Installed oh-my-tiny MCP server")
        }
      } catch (e: any) {
        spinner.stop("Failed to install MCP server", 1)
        prompts.log.warn(e.message)
      }
    } else {
      prompts.log.step("Skipping MCP install (--skip-mcp)")
    }

    // Step 5: Copy help.md if it exists
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

    // Step 6: Write default config.json if absent (or --force)
    {
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
            mcp: {
              "oh-my-tiny": {
                type: "local",
                command: ["node", path.join(mcpDir, "node_modules/oh-my-tiny/dist/mcp/server.js")],
              },
            },
          }
          await fsNode.promises.writeFile(configFile, JSON.stringify(defaultConfig, null, 2) + "\n", "utf8")
          spinner.stop("Config written")
        } catch (e: any) {
          spinner.stop("Failed to write config.json", 1)
          prompts.log.error(e.message)
        }
      } else {
        prompts.log.step("Config already exists, skipping (use --force to overwrite)")
      }
    }

    UI.empty()
    prompts.outro("Setup complete")
  },
}
