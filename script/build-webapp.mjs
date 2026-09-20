#!/usr/bin/env node

import { resolve, dirname, join, relative } from "node:path"
import { cpSync, rmSync, mkdirSync, existsSync, readdirSync, readFileSync } from "node:fs"
import { createRequire } from "node:module"
import { workspaceResolverPlugin } from "./build-webapp/workspace-resolver.mjs"
import { tailwindPlugin } from "./build-webapp/tailwind-plugin.mjs"
import { generateHTML } from "./build-webapp/html-generator.mjs"

const ROOT = resolve(dirname(new URL(import.meta.url).pathname), "..")

// Resolve deps from vendored dir if it exists, else workspace node_modules
const VENDORED = join(ROOT, "script/build-deps/node_modules")
const DEPS = existsSync(VENDORED) ? VENDORED : join(ROOT, "node_modules")

const require = createRequire(join(DEPS, "_"))
const esbuild = require("esbuild")
const { solidPlugin } = require("esbuild-plugin-solid")
const { transformAsync } = require("@babel/core")
const babelSolid = require("babel-preset-solid")
const babelTS = require("@babel/preset-typescript")
const APP_DIR = join(ROOT, "packages/app")

function importMetaGlobPlugin() {
  return {
    name: "import-meta-glob",
    setup(build) {
      build.onLoad({ filter: /\.(t|j)sx?$/ }, async (args) => {
        const src = readFileSync(args.path, "utf8")
        if (!src.includes("import.meta.glob")) return undefined

        const replaced = src.replace(
          /import\.meta\.glob(?:<[^>]*>)?\(["']([^"']+)["'](?:\s*,\s*(\{[^}]*\}))?\)/g,
          (_match, pattern, optsStr) => {
            const dir = dirname(args.path)
            const starIdx = pattern.indexOf("*")
            const prefix = pattern.substring(0, starIdx)
            const suffix = pattern.substring(starIdx + 1)
            const searchDir = resolve(dir, prefix)
            if (!existsSync(searchDir)) return "{}"
            const extractDefault = optsStr && /import\s*:\s*["']default["']/.test(optsStr)
            const entries = readdirSync(searchDir)
              .filter((f) => f.endsWith(suffix))
              .sort()
              .map((f) => {
                const key = prefix + f
                const abs = join(searchDir, f)
                const rel = "./" + relative(dir, abs)
                const loader = extractDefault
                  ? `() => import(${JSON.stringify(rel)}).then(m => m.default)`
                  : `() => import(${JSON.stringify(rel)})`
                return `${JSON.stringify(key)}: ${loader}`
              })
            return `{${entries.join(", ")}}`
          }
        )

        if (replaced === src) return undefined

        const isTSX = args.path.endsWith(".tsx") || args.path.endsWith(".jsx")
        const result = await transformAsync(replaced, {
          presets: [
            [babelSolid, { generate: "dom", hydratable: false }],
            [babelTS, {}],
          ],
          filename: args.path,
          sourceMaps: "inline",
        })
        return { contents: result.code, loader: "js" }
      })
    },
  }
}
const OUT_DIR = join(APP_DIR, "dist")

const channel = (() => {
  const raw = process.env.TINYCODE_CHANNEL
  if (raw === "dev" || raw === "beta" || raw === "prod") return raw
  if (raw === "latest") return "prod"
  return "dev"
})()

async function build() {
  if (existsSync(OUT_DIR)) {
    for (const entry of readdirSync(OUT_DIR)) {
      if (entry === "assets") {
        rmSync(join(OUT_DIR, entry), { recursive: true })
      }
    }
  }
  mkdirSync(join(OUT_DIR, "assets"), { recursive: true })

  const result = await esbuild.build({
    entryPoints: [join(APP_DIR, "src/entry.tsx")],
    bundle: true,
    splitting: true,
    format: "esm",
    outdir: join(OUT_DIR, "assets"),
    metafile: true,
    minify: true,
    sourcemap: false,
    target: ["es2022", "chrome100", "firefox100", "safari16"],
    define: {
      "import.meta.env.DEV": "false",
      "import.meta.env.PROD": "true",
      "import.meta.env.VITE_TINYCODE_CHANNEL": JSON.stringify(channel),
      "import.meta.env.VITE_TINYCODE_SERVER_HOST": "undefined",
      "import.meta.env.VITE_TINYCODE_SERVER_PORT": "undefined",
    },
    loader: {
      ".woff2": "file",
      ".woff": "file",
      ".ttf": "file",
      ".eot": "file",
      ".svg": "file",
      ".png": "file",
      ".jpg": "file",
      ".jpeg": "file",
      ".gif": "file",
      ".aac": "file",
      ".mp3": "file",
      ".wav": "file",
      ".ogg": "file",
    },
    plugins: [
      workspaceResolverPlugin(),
      importMetaGlobPlugin(),
      tailwindPlugin(),
      solidPlugin({ solid: { generate: "dom", hydratable: false } }),
    ],
    jsx: "preserve",
    conditions: ["bun", "import"],
    mainFields: ["module", "main"],
    nodePaths: [DEPS],
    logLevel: "info",
    entryNames: "[name]-[hash]",
    chunkNames: "[name]-[hash]",
    assetNames: "[name]-[hash]",
  })

  cpSync(join(APP_DIR, "public"), OUT_DIR, { recursive: true })

  generateHTML(OUT_DIR, result.metafile)

  console.log("\nBuild complete:", OUT_DIR)
}

build().catch((err) => {
  console.error("Build failed:", err)
  process.exit(1)
})
