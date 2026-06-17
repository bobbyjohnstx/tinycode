import fs from "fs/promises"
import path from "path"
import { existsSync, readFileSync } from "fs"
import { xdgConfig, xdgData } from "xdg-basedir"

const log = {
  info: (msg: string) => console.error(`[migrate] ${msg}`),
  warn: (msg: string) => console.error(`[migrate] WARNING: ${msg}`),
}

/**
 * Legacy env var mappings from opencode -> tinycode.
 */
const LEGACY_ENV_VARS: Record<string, string> = {
  OPENCODE_API_KEY: "TINYCODE_API_KEY",
  OPENCODE_CONFIG: "TINYCODE_CONFIG",
  OPENCODE_CONFIG_DIR: "TINYCODE_CONFIG_DIR",
  OPENCODE_OLLAMA_HOST: "TINYCODE_OLLAMA_HOST",
  OPENCODE_VLLM_HOST: "TINYCODE_VLLM_HOST",
}

/**
 * Check for legacy opencode env vars and log deprecation warnings.
 */
export function checkLegacyEnvVars(): void {
  for (const [oldVar, newVar] of Object.entries(LEGACY_ENV_VARS)) {
    if (process.env[oldVar]) {
      log.warn(`${oldVar} is deprecated. Use ${newVar} instead.`)
    }
  }
}

/**
 * Migrate legacy opencode config directory to tinycode.
 * Only migrates if the old directory exists and the new one does not.
 */
export async function migrateConfigDirectory(): Promise<void> {
  if (!xdgConfig) return

  const oldConfigDir = path.join(xdgConfig, "opencode")
  const newConfigDir = path.join(xdgConfig, "tinycode")

  if (!existsSync(oldConfigDir)) return

  if (existsSync(newConfigDir)) {
    log.warn(
      `Both ~/.config/opencode/ and ~/.config/tinycode/ exist. Skipping migration. Remove the old directory manually if no longer needed.`,
    )
    return
  }

  try {
    await fs.rename(oldConfigDir, newConfigDir)
    log.info(`Migrated config directory: opencode -> tinycode`)
  } catch (err: any) {
    log.warn(`Failed to migrate config directory: ${err.message}`)
  }
}

/**
 * Migrate legacy opencode auth.json to tinycode.
 * Merges legacy auth data rather than overwriting.
 */
export async function migrateAuthData(): Promise<void> {
  if (!xdgData) return

  const oldAuthFile = path.join(xdgData, "opencode", "auth.json")
  const newAuthFile = path.join(xdgData, "tinycode", "auth.json")

  if (!existsSync(oldAuthFile)) return

  try {
    const oldData = JSON.parse(readFileSync(oldAuthFile, "utf8"))

    let newData: Record<string, unknown> = {}
    if (existsSync(newAuthFile)) {
      try {
        newData = JSON.parse(readFileSync(newAuthFile, "utf8"))
      } catch {}
    }

    // Merge: only add keys that don't already exist in new data
    let merged = false
    for (const [key, value] of Object.entries(oldData)) {
      if (!(key in newData)) {
        newData[key] = value
        merged = true
      }
    }

    if (merged) {
      await fs.mkdir(path.dirname(newAuthFile), { recursive: true, mode: 0o700 })
      await fs.writeFile(newAuthFile, JSON.stringify(newData, null, 2), {
        encoding: "utf8",
        mode: 0o600,
      })
      log.info(`Merged legacy auth data from opencode to tinycode`)
    }
  } catch (err: any) {
    log.warn(`Failed to migrate auth data: ${err.message}`)
  }
}

/**
 * Run all migration checks. Safe to call multiple times (idempotent).
 */
export async function runMigrations(): Promise<void> {
  checkLegacyEnvVars()
  await migrateConfigDirectory()
  await migrateAuthData()
}

export * as ConfigMigrate from "./migrate"
