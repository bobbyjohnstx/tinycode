import { Config } from "effect"

function truthy(key: string) {
  const value = process.env[key]?.toLowerCase()
  return value === "true" || value === "1"
}

const TINYCODE_EXPERIMENTAL = truthy("TINYCODE_EXPERIMENTAL")
const copy = process.env["TINYCODE_EXPERIMENTAL_DISABLE_COPY_ON_SELECT"]

function enabledByExperimental(key: string) {
  if (process.env[key] !== undefined) return truthy(key)
  return TINYCODE_EXPERIMENTAL
}

export const Flag = {
  OTEL_EXPORTER_OTLP_ENDPOINT: process.env["OTEL_EXPORTER_OTLP_ENDPOINT"],
  OTEL_EXPORTER_OTLP_HEADERS: process.env["OTEL_EXPORTER_OTLP_HEADERS"],

  TINYCODE_AUTO_HEAP_SNAPSHOT: truthy("TINYCODE_AUTO_HEAP_SNAPSHOT"),
  TINYCODE_GIT_BASH_PATH: process.env["TINYCODE_GIT_BASH_PATH"],
  TINYCODE_CONFIG: process.env["TINYCODE_CONFIG"],
  TINYCODE_CONFIG_CONTENT: process.env["TINYCODE_CONFIG_CONTENT"],
  TINYCODE_DISABLE_AUTOUPDATE: truthy("TINYCODE_DISABLE_AUTOUPDATE"),
  TINYCODE_ALWAYS_NOTIFY_UPDATE: truthy("TINYCODE_ALWAYS_NOTIFY_UPDATE"),
  TINYCODE_DISABLE_PRUNE: truthy("TINYCODE_DISABLE_PRUNE"),
  TINYCODE_DISABLE_TERMINAL_TITLE: truthy("TINYCODE_DISABLE_TERMINAL_TITLE"),
  TINYCODE_SHOW_TTFD: truthy("TINYCODE_SHOW_TTFD"),
  TINYCODE_DISABLE_AUTOCOMPACT: truthy("TINYCODE_DISABLE_AUTOCOMPACT"),
  TINYCODE_DISABLE_MODELS_FETCH: truthy("TINYCODE_DISABLE_MODELS_FETCH"),
  TINYCODE_DISABLE_MOUSE: truthy("TINYCODE_DISABLE_MOUSE"),
  TINYCODE_FAKE_VCS: process.env["TINYCODE_FAKE_VCS"],
  TINYCODE_SERVER_PASSWORD: process.env["TINYCODE_SERVER_PASSWORD"],
  TINYCODE_SERVER_USERNAME: process.env["TINYCODE_SERVER_USERNAME"],

  // New tinycode-specific env vars
  TINYCODE_OLLAMA_HOST: process.env["TINYCODE_OLLAMA_HOST"] ?? "http://localhost:11434",
  TINYCODE_VLLM_HOST: process.env["TINYCODE_VLLM_HOST"] ?? "http://localhost:8000",
  TINYCODE_RAMALAMA_HOST: process.env["TINYCODE_RAMALAMA_HOST"],

  // Experimental
  TINYCODE_EXPERIMENTAL_FILEWATCHER: Config.boolean("TINYCODE_EXPERIMENTAL_FILEWATCHER").pipe(
    Config.withDefault(false),
  ),
  TINYCODE_EXPERIMENTAL_DISABLE_FILEWATCHER: Config.boolean("TINYCODE_EXPERIMENTAL_DISABLE_FILEWATCHER").pipe(
    Config.withDefault(false),
  ),
  TINYCODE_EXPERIMENTAL_DISABLE_COPY_ON_SELECT:
    copy === undefined ? process.platform === "win32" : truthy("TINYCODE_EXPERIMENTAL_DISABLE_COPY_ON_SELECT"),
  TINYCODE_MODELS_URL: process.env["TINYCODE_MODELS_URL"],
  TINYCODE_MODELS_PATH: process.env["TINYCODE_MODELS_PATH"],
  TINYCODE_DB: process.env["TINYCODE_DB"],

  TINYCODE_WORKSPACE_ID: process.env["TINYCODE_WORKSPACE_ID"],
  TINYCODE_EXPERIMENTAL_WORKSPACES: enabledByExperimental("TINYCODE_EXPERIMENTAL_WORKSPACES"),
  TINYCODE_EXPERIMENTAL_SESSION_SWITCHER: enabledByExperimental("TINYCODE_EXPERIMENTAL_SESSION_SWITCHER"),

  // Evaluated at access time (not module load) because tests, the CLI, and
  // external tooling set these env vars at runtime.
  get TINYCODE_DISABLE_PROJECT_CONFIG() {
    return truthy("TINYCODE_DISABLE_PROJECT_CONFIG")
  },
  get TINYCODE_TUI_CONFIG() {
    return process.env["TINYCODE_TUI_CONFIG"]
  },
  get TINYCODE_CONFIG_DIR() {
    return process.env["TINYCODE_CONFIG_DIR"]
  },
  get TINYCODE_PURE() {
    return truthy("TINYCODE_PURE")
  },
  get TINYCODE_PERMISSION() {
    return process.env["TINYCODE_PERMISSION"]
  },
  get TINYCODE_PLUGIN_META_FILE() {
    return process.env["TINYCODE_PLUGIN_META_FILE"]
  },
  get TINYCODE_CLIENT() {
    return process.env["TINYCODE_CLIENT"] ?? "cli"
  },
}
