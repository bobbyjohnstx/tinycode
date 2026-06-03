import path from "path"
import fsNode from "fs/promises"
import { existsSync } from "fs"
import { Effect } from "effect"
import { Global } from "@opencode-ai/core/global"
import * as Log from "@opencode-ai/core/util/log"

const log = Log.create({ service: "migration" })

const CONFIG_FILES = ["config.json", "auth.json", "tui.json", "opencode.json", "opencode.jsonc"]

async function copyIfMissing(src: string, dest: string): Promise<boolean> {
  if (!existsSync(src) || existsSync(dest)) return false
  await fsNode.mkdir(path.dirname(dest), { recursive: true })
  await fsNode.copyFile(src, dest)
  return true
}

/** Core migration logic — exported for testing with custom paths. */
export async function migrateFiles(oldConfig: string, newConfig: string, oldData: string, newData: string) {
  let migrated = false
  for (const file of CONFIG_FILES) {
    if (await copyIfMissing(path.join(oldConfig, file), path.join(newConfig, file))) {
      migrated = true
      log.info(`migrated config file: ${file}`)
    }
  }
  if (await copyIfMissing(path.join(oldData, "db.sqlite"), path.join(newData, "db.sqlite"))) {
    migrated = true
    log.info("migrated db.sqlite")
  }
  if (migrated) log.info(`Migrated config from ${oldConfig} to ${newConfig}`)
}

// One-time migration from ~/.config/opencode/ to ~/.config/tinycode/
// Called at startup before config loading. Does NOT delete old files.
export const migrateConfig = Effect.promise(async () => {
  try {
    const oldConfig = Global.Path.config.replace(/tinycode$/, "opencode")
    const newConfig = Global.Path.config
    const oldData = Global.Path.data.replace(/tinycode$/, "opencode")
    const newData = Global.Path.data
    if (oldConfig === newConfig && oldData === newData) return
    await migrateFiles(oldConfig, newConfig, oldData, newData)
  } catch (err) {
    log.warn("config migration failed, continuing", { error: String(err) })
  }
})
