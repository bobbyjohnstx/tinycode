import { readFileSync, existsSync } from "node:fs"
import { dirname, join, resolve } from "node:path"
import { createRequire } from "node:module"

const ROOT = resolve(dirname(new URL(import.meta.url).pathname), "../..")
const VENDORED = join(ROOT, "script/build-deps/node_modules")
const DEPS = existsSync(VENDORED) ? VENDORED : join(ROOT, "node_modules")
const require = createRequire(join(DEPS, "_"))
const postcss = require("postcss")
const tailwindcss = require("@tailwindcss/postcss")

const WORKSPACE_CSS = {
  "@tinycode/ui/styles/tailwind": join(ROOT, "packages/ui/src/styles/tailwind/index.css"),
  "@tinycode/ui/v2/styles/tailwind.css": join(ROOT, "packages/ui/src/v2/styles/tailwind.css"),
  "@tinycode/ui/styles": join(ROOT, "packages/ui/src/styles/index.css"),
}

function resolveWorkspaceImports(css) {
  return css.replace(
    /@import\s+["']([^"']+)["']/g,
    (match, specifier) => {
      if (WORKSPACE_CSS[specifier]) {
        return `@import "${WORKSPACE_CSS[specifier]}"`
      }
      return match
    }
  )
}

export function tailwindPlugin() {
  return {
    name: "tailwind-css",
    setup(build) {
      build.onLoad({ filter: /\.css$/ }, async (args) => {
        let css = readFileSync(args.path, "utf8")
        css = resolveWorkspaceImports(css)
        css = resolveBareModuleImports(css)
        const processor = postcss([tailwindcss()])
        const result = await processor.process(css, { from: args.path })
        return {
          contents: result.css,
          loader: "css",
          resolveDir: dirname(args.path),
        }
      })
    },
  }
}

function resolveBareModuleImports(css) {
  return css.replace(
    /@import\s+["'](tailwindcss\/[^"']+)["']/g,
    (match, specifier) => {
      try {
        const resolved = require.resolve(specifier)
        return `@import "${resolved}"`
      } catch {
        return match
      }
    }
  )
}
