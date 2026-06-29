import { app } from "electron"

type Channel = "dev" | "beta" | "prod"
const raw = import.meta.env.TINYCODE_CHANNEL
export const CHANNEL: Channel = raw === "dev" || raw === "beta" || raw === "prod" ? raw : "dev"

export const SETTINGS_STORE = "tinycode.settings"
export const DEFAULT_SERVER_URL_KEY = "defaultServerUrl"
export const WSL_ENABLED_KEY = "wslEnabled"
export const PINCH_ZOOM_ENABLED_KEY = "pinchZoomEnabled"
export const ZOOM_FACTOR_KEY = "zoomFactor"
export const UPDATER_ENABLED = app.isPackaged && CHANNEL !== "dev"
