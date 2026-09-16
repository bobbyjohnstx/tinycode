import { readFileSync, existsSync, statSync } from "node:fs"
import { resolve, join, dirname } from "node:path"

const ROOT = resolve(dirname(new URL(import.meta.url).pathname), "../..")

const PACKAGES = {
  "@tinycode/ui": join(ROOT, "packages/ui"),
  "@tinycode/sdk": join(ROOT, "packages/sdk/js"),
  tinycode: join(ROOT, "packages/tinycode"),
}

function loadExports(pkgDir) {
  const pkgPath = join(pkgDir, "package.json")
  const pkg = JSON.parse(readFileSync(pkgPath, "utf8"))
  return pkg.exports || {}
}

function getTarget(target) {
  if (typeof target === "string") return target
  if (typeof target === "object") return target.bun || target.import || target.default
  return null
}

function resolveExport(pkgDir, exports, subpath) {
  for (const [pattern, target] of Object.entries(exports)) {
    const resolved = getTarget(target)
    if (!resolved) continue
    if (pattern === subpath) {
      return join(pkgDir, resolved)
    }
  }

  const globs = Object.entries(exports)
    .filter(([p]) => p.includes("*"))
    .map(([p, t]) => [p, getTarget(t)])
    .filter(([, t]) => t)
    .sort((a, b) => b[0].length - a[0].length)

  for (const [pattern, resolved] of globs) {
    const regex = new RegExp("^" + pattern.replace("*", "(.+)") + "$")
    const match = subpath.match(regex)
    if (match) {
      return join(pkgDir, resolved.replace("*", match[1]))
    }
  }
  return null
}

function tryExtensions(p) {
  if (existsSync(p) && statSync(p).isFile()) return p
  if (p.endsWith(".jsx") && existsSync(p.replace(/\.jsx$/, ".tsx"))) {
    return p.replace(/\.jsx$/, ".tsx")
  }
  for (const ext of [".ts", ".tsx", ".js", ".jsx", ".css"]) {
    if (existsSync(p + ext)) return p + ext
  }
  if (existsSync(join(p, "index.ts"))) return join(p, "index.ts")
  if (existsSync(join(p, "index.tsx"))) return join(p, "index.tsx")
  return p
}

export function workspaceResolverPlugin() {
  const exportsMaps = {}
  for (const [name, dir] of Object.entries(PACKAGES)) {
    exportsMaps[name] = loadExports(dir)
  }

  return {
    name: "workspace-resolver",
    setup(build) {
      for (const [pkgName, pkgDir] of Object.entries(PACKAGES)) {
        const filter = new RegExp(`^${pkgName.replace("/", "\\/")}(\\/.*)?$`)

        build.onResolve({ filter }, (args) => {
          const subpath = args.path === pkgName ? "." : "./" + args.path.slice(pkgName.length + 1)
          const exports = exportsMaps[pkgName]
          const resolved = resolveExport(pkgDir, exports, subpath)
          if (resolved) {
            return { path: tryExtensions(resolved) }
          }
          const fallback = join(pkgDir, "src", subpath === "." ? "index.ts" : subpath.slice(2))
          return { path: tryExtensions(fallback) }
        })
      }

      build.onResolve({ filter: /^@\// }, (args) => {
        const rel = args.path.slice(2)
        const full = join(ROOT, "packages/app/src", rel)
        return { path: tryExtensions(full) }
      })

      build.onResolve({ filter: /\?worker&url$/ }, (args) => {
        const clean = args.path.replace(/\?worker&url$/, "")
        return { path: clean, namespace: "worker-url" }
      })

      build.onLoad({ filter: /.*/, namespace: "worker-url" }, async (args) => {
        const resolved = await build.resolve(args.path, {
          kind: "import-statement",
          resolveDir: ROOT,
        })
        if (resolved.errors.length > 0) {
          return { errors: resolved.errors }
        }
        const workerSrc = readFileSync(resolved.path, "utf8")
        return {
          contents: `export default URL.createObjectURL(new Blob([${JSON.stringify(workerSrc)}], { type: "text/javascript" }));`,
          loader: "js",
        }
      })

      build.onResolve({ filter: /^\/assets\// }, (args) => {
        const rel = args.path.slice(1)
        const full = join(ROOT, "packages/app/public", rel)
        if (existsSync(full)) {
          return { path: full }
        }
        return { path: args.path, external: true }
      })
    },
  }
}
