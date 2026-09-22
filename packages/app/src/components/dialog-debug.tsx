import { Component, createResource, For, Show } from "solid-js"
import { Dialog } from "@tinycode/ui/dialog"
import { Button } from "@tinycode/ui/button"
import { showToast } from "@tinycode/ui/toast"
import { useLanguage } from "@/context/language"
import { useSDK } from "@/context/sdk"
import { useSync } from "@/context/sync"
import { useLocal } from "@/context/local"

type HealthData = { healthy?: boolean; version?: string }
type McpEntry = { status: string }

function Row(props: { label: string; children: any }) {
  return (
    <div class="flex items-start gap-3 py-2 border-b border-border-base last:border-b-0">
      <span class="text-text-weak text-13-regular w-[120px] shrink-0">{props.label}</span>
      <span class="text-text-base text-13-regular min-w-0 break-all">{props.children}</span>
    </div>
  )
}

export const DialogDebug: Component = () => {
  const language = useLanguage()
  const sdk = useSDK()
  const sync = useSync()
  const local = useLocal()

  const [health] = createResource(async (): Promise<HealthData> => {
    const res = await sdk.client.global.health().catch(() => ({ data: undefined }))
    return (res.data ?? {}) as HealthData
  })

  const [paths] = createResource(async () => {
    const res = await sdk.client.path.get().catch(() => ({ data: undefined }))
    return res.data as Record<string, string> | undefined
  })

  const model = () => {
    const m = local.model.current()
    if (!m) return undefined
    return m
  }

  const mcpEntries = () => {
    const mcp = sync.data.mcp ?? {}
    return Object.entries(mcp).sort(([a], [b]) => a.localeCompare(b))
  }

  const statusText = () => {
    const h = health()
    if (!h) return language.t("dialog.debug.unknown")
    return h.healthy ? language.t("dialog.debug.healthy") : language.t("dialog.debug.unhealthy")
  }

  const buildDiagnosticsText = () => {
    const lines: string[] = []
    lines.push("# Diagnostics")
    lines.push("")

    const h = health()
    if (h?.version) lines.push(`Version: ${h.version}`)
    lines.push(`Status: ${statusText()}`)
    lines.push(`Directory: ${sdk.directory}`)

    const m = model()
    if (m) lines.push(`Model: ${m.provider.id}/${m.id}`)

    const mcpItems = mcpEntries()
    if (mcpItems.length > 0) {
      lines.push("")
      lines.push("MCP Servers:")
      for (const [name, status] of mcpItems) {
        lines.push(`  ${name}: ${(status as McpEntry).status}`)
      }
    }

    const p = paths()
    if (p) {
      lines.push("")
      lines.push("Paths:")
      for (const [key, value] of Object.entries(p)) {
        if (value) lines.push(`  ${key}: ${value}`)
      }
    }

    return lines.join("\n")
  }

  const copyDiagnostics = async () => {
    const text = buildDiagnosticsText()
    try {
      await navigator.clipboard.writeText(text)
      showToast({
        title: language.t("dialog.debug.copied"),
        variant: "success",
      })
    } catch {
      const textarea = document.createElement("textarea")
      textarea.value = text
      textarea.style.position = "fixed"
      textarea.style.opacity = "0"
      document.body.appendChild(textarea)
      textarea.select()
      document.execCommand("copy")
      document.body.removeChild(textarea)
      showToast({
        title: language.t("dialog.debug.copied"),
        variant: "success",
      })
    }
  }

  return (
    <Dialog title={language.t("dialog.debug.title")} size="large">
      <div class="flex flex-col gap-1 p-4 overflow-y-auto max-h-[60vh]">
        <Row label={language.t("dialog.debug.version")}>
          {health()?.version ?? "..."}
        </Row>
        <Row label={language.t("dialog.debug.status")}>
          <span
            classList={{
              "text-icon-success-base": health()?.healthy === true,
              "text-icon-critical-base": health()?.healthy === false,
            }}
          >
            {statusText()}
          </span>
        </Row>
        <Row label={language.t("dialog.debug.provider")}>
          <Show when={model()} fallback="--">
            {(m) => `${m().provider.id} / ${m().id}`}
          </Show>
        </Row>
        <Row label={language.t("dialog.debug.mcp")}>
          <Show
            when={mcpEntries().length > 0}
            fallback={language.t("dialog.debug.mcp.none")}
          >
            <div class="flex flex-col gap-0.5">
              <For each={mcpEntries()}>
                {([name, status]) => (
                  <div class="flex items-center gap-2">
                    <div
                      classList={{
                        "size-1.5 rounded-full shrink-0": true,
                        "bg-icon-success-base": (status as McpEntry).status === "connected",
                        "bg-icon-critical-base": (status as McpEntry).status === "failed",
                        "bg-border-weak-base": (status as McpEntry).status === "disabled",
                        "bg-icon-warning-base": (status as McpEntry).status === "needs_auth",
                      }}
                    />
                    <span>{name}: {(status as McpEntry).status}</span>
                  </div>
                )}
              </For>
            </div>
          </Show>
        </Row>
        <Row label={language.t("dialog.debug.paths")}>
          <Show when={paths()} fallback="...">
            {(p) => (
              <div class="flex flex-col gap-0.5 font-mono text-[12px]">
                <For each={Object.entries(p()).filter(([, v]) => !!v)}>
                  {([key, value]) => (
                    <div>
                      <span class="text-text-weak">{key}:</span> {value}
                    </div>
                  )}
                </For>
              </div>
            )}
          </Show>
        </Row>
      </div>
      <div class="flex justify-end p-4 pt-0">
        <Button variant="secondary" onClick={copyDiagnostics}>
          {language.t("dialog.debug.copy")}
        </Button>
      </div>
    </Dialog>
  )
}
