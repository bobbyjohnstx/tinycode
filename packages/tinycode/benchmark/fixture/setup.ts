import * as fs from "fs/promises"
import os from "os"
import path from "path"
import { $ } from "bun"

export interface FixtureDir {
  path: string
  cleanup: () => Promise<void>
}

export async function createFixtureDir(): Promise<FixtureDir> {
  const dirpath = path.join(
    os.tmpdir(),
    "tinycode-bench-" + Math.random().toString(36).slice(2)
  )

  await fs.mkdir(dirpath, { recursive: true })

  // Copy template files to fixture directory
  const templateDir = path.join(import.meta.dir, "template")
  await copyDirectory(templateDir, dirpath)

  // Initialize git repository
  await $`git init`.cwd(dirpath).quiet()
  await $`git config core.fsmonitor false`.cwd(dirpath).quiet()
  await $`git config commit.gpgsign false`.cwd(dirpath).quiet()
  await $`git config user.email "bench@tinycode.test"`.cwd(dirpath).quiet()
  await $`git config user.name "Benchmark"`.cwd(dirpath).quiet()
  await $`git add -A`.cwd(dirpath).quiet()
  await $`git commit -m "initial"`.cwd(dirpath).quiet()

  const realpath = await fs.realpath(dirpath)

  return {
    path: realpath,
    cleanup: async () => {
      await fs.rm(realpath, { recursive: true, force: true })
    },
  }
}

async function copyDirectory(src: string, dest: string): Promise<void> {
  const entries = await fs.readdir(src, { withFileTypes: true })

  for (const entry of entries) {
    const srcPath = path.join(src, entry.name)
    const destPath = path.join(dest, entry.name)

    if (entry.isDirectory()) {
      await fs.mkdir(destPath, { recursive: true })
      await copyDirectory(srcPath, destPath)
    } else {
      await fs.copyFile(srcPath, destPath)
    }
  }
}
