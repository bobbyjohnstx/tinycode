import { defineConfig } from "@playwright/test"
import { existsSync } from "fs"
import { join } from "path"

// Resolve a working chromium executable. The Playwright version (1.59.1)
// expects revision 1217, but the download may be incomplete. Fall back to
// a known-good revision (1243) if the expected one is missing.
function resolveChromiumPath(): string | undefined {
  const cacheDir =
    process.env.PLAYWRIGHT_BROWSERS_PATH ??
    join(require("os").homedir(), "Library/Caches/ms-playwright")

  // Prefer the headless shell matching this Playwright version
  const candidates = [
    join(cacheDir, "chromium_headless_shell-1217/chrome-headless-shell-mac-arm64/chrome-headless-shell"),
    join(cacheDir, "chromium_headless_shell-1243/chrome-headless-shell-mac-arm64/chrome-headless-shell"),
    join(cacheDir, "chromium_headless_shell-1208/chrome-headless-shell-mac-arm64/chrome-headless-shell"),
  ]

  for (const candidate of candidates) {
    if (existsSync(candidate)) return candidate
  }
  return undefined
}

const executablePath = resolveChromiumPath()

export default defineConfig({
  testDir: ".",
  outputDir: "./test-results",
  timeout: 30_000,
  expect: {
    timeout: 10_000,
  },
  retries: 0,
  workers: 1,
  reporter: [["line"]],
  use: {
    baseURL: process.env.TINYCODE_BASE_URL ?? "http://127.0.0.1:4096",
    headless: true,
    trace: "on-first-retry",
    screenshot: "only-on-failure",
    launchOptions: {
      ...(executablePath ? { executablePath } : {}),
    },
  },
  projects: [
    {
      name: "chromium",
    },
  ],
})
