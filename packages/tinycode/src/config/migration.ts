import path from "path"
import fsNode from "fs/promises"
import { existsSync } from "fs"
import { Effect } from "effect"
import { Global } from "@opencode-ai/core/global"
import * as Log from "@opencode-ai/core/util/log"

const log = Log.create({ service: "migration" })

// One-time migration from ~/.config/opencode/ to ~/.config/tinycode/
// and from ~/.local/share/opencode/ to ~/.local/share/tinycode/
// Called at startup before config loading.
// Does NOT delete old files — users may have opencode installed alongside.
export const migrateConfig = Effect.promise(async () => {
  try {
    const oldConfig = Global.Path.config.replace(/tinycode$/, "opencode")
    const oldData = Global.Path.data.replace(/tinycode$/, "opencode")
    const newConfig = Global.Path.config
    const newData = Global.Path.data

    if (oldConfig === newConfig && oldData === newData) return

    let migrated = false

    async function copyIfMissing(src: string, dest: string): Promise<boolean> {
      if (!existsSync(src) || existsSync(dest)) return false
      await fsNode.copyFile(src, dest)
      return true
    }

    for (const file of ["config.json", "auth.json", "tui.json", "opencode.json", "opencode.jsonc"]) {
      const src = path.join(oldConfig, file)
      const dest = path.join(newConfig, file)
      if (await copyIfMissing(src, dest)) {
        migrated = true
        log.info(`migrated config file: ${file}`)
      }
    }

    const oldDb = path.join(oldData, "db.sqlite")
    const newDb = path.join(newData, "db.sqlite")
    if (await copyIfMissing(oldDb, newDb)) {
      migrated = true
      log.info("migrated db.sqlite")
    }

    if (migrated) {
      log.info(`Migrated config from ${oldConfig} to ${newConfig}`)
    }
  } catch (err) {
    log.warn("config migration failed, continuing", { error: String(err) })
  }
})
