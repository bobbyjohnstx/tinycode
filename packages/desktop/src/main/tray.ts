import { app, Menu, nativeImage, Tray } from "electron"
import { join, dirname } from "node:path"
import { fileURLToPath } from "node:url"

const root = dirname(fileURLToPath(import.meta.url))

let tray: Tray | null = null

export function createTray(deps: { showWindow: () => void; quit: () => void }) {
  const iconDir = app.isPackaged
    ? join(process.resourcesPath, "icons")
    : join(root, "../../resources/icons")

  const iconFile = process.platform === "win32" ? "icon.ico" : "32x32.png"
  const icon = nativeImage.createFromPath(join(iconDir, iconFile))

  tray = new Tray(icon.resize({ width: 16, height: 16 }))
  tray.setToolTip("TinyCode")

  const contextMenu = Menu.buildFromTemplate([
    { label: "Show Window", click: deps.showWindow },
    { type: "separator" },
    { label: "Quit", click: deps.quit },
  ])

  tray.setContextMenu(contextMenu)
  tray.on("click", deps.showWindow)
}

export function destroyTray() {
  tray?.destroy()
  tray = null
}
