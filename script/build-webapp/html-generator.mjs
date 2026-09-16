import { readFileSync, writeFileSync } from "node:fs"
import { resolve, dirname, join } from "node:path"

const ROOT = resolve(dirname(new URL(import.meta.url).pathname), "../..")

export function generateHTML(outputDir, metafile) {
  const themeJS = readFileSync(
    join(ROOT, "packages/app/public/oc-theme-preload.js"),
    "utf8"
  )

  const outputs = metafile.outputs
  let entryJS = ""
  let entryCSS = ""

  for (const [outPath, info] of Object.entries(outputs)) {
    if (!info.entryPoint) continue
    const isEntry = info.entryPoint.includes("entry.tsx") || info.entryPoint.includes("entry.ts")
    if (!isEntry) continue

    const rel = outPath.startsWith(outputDir)
      ? outPath.slice(outputDir.length)
      : "/" + outPath.replace(/^.*?\/assets\//, "assets/")

    if (outPath.endsWith(".js")) entryJS = rel
    if (outPath.endsWith(".css")) entryCSS = rel
  }

  if (!entryCSS) {
    for (const [outPath, info] of Object.entries(outputs)) {
      if (!outPath.endsWith(".css")) continue
      if (info.cssBundle || outPath.includes("entry")) {
        entryCSS = outPath.startsWith(outputDir)
          ? outPath.slice(outputDir.length)
          : "/" + outPath.replace(/^.*?\/assets\//, "assets/")
        break
      }
    }
  }

  if (!entryCSS) {
    for (const [outPath] of Object.entries(outputs)) {
      if (outPath.endsWith(".css")) {
        entryCSS = outPath.startsWith(outputDir)
          ? outPath.slice(outputDir.length)
          : "/" + outPath.replace(/^.*?\/assets\//, "assets/")
        break
      }
    }
  }

  if (!entryJS || !entryCSS) {
    console.error("Failed to find entry files in metafile:")
    for (const [outPath, info] of Object.entries(outputs)) {
      if (info.entryPoint) {
        console.error(`  ${outPath} -> entryPoint: ${info.entryPoint}`)
      }
    }
  }

  const html = `<!doctype html>
<html lang="en" style="background-color: var(--background-base)">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1, interactive-widget=resizes-content" />
    <title>TinyCode</title>
    <link rel="icon" type="image/png" href="/favicon-96x96-v3.png" sizes="96x96" />
    <link rel="icon" type="image/svg+xml" href="/favicon-v3.svg" />
    <link rel="shortcut icon" href="/favicon-v3.ico" />
    <link rel="apple-touch-icon" sizes="180x180" href="/apple-touch-icon-v3.png" />
    <link rel="manifest" href="/site.webmanifest" />
    <meta name="theme-color" content="#F8F7F7" />
    <meta property="og:image" content="/social-share.png" />
    <meta property="twitter:image" content="/social-share.png" />
    <script id="oc-theme-preload-script">${themeJS}</script>
    <script type="module" crossorigin src="${entryJS}"></script>
    <link rel="stylesheet" crossorigin href="${entryCSS}">
  </head>
  <body class="antialiased overscroll-none text-12-regular overflow-hidden">
    <noscript>You need to enable JavaScript to run this app.</noscript>
    <div id="root" class="flex flex-col h-dvh p-px"></div>
  </body>
</html>
`

  writeFileSync(join(outputDir, "index.html"), html)
}
