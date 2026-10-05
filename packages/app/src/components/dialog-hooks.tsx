import { Component, createResource, For, Show } from "solid-js"
import { Dialog } from "@tinycode/ui/dialog"
import { useLanguage } from "@/context/language"
import { useSDK } from "@/context/sdk"
import { useSync } from "@/context/sync"

type HookEntry = {
  command: string
  match?: Record<string, string>
  timeout?: number
}

type HooksConfig = Record<string, HookEntry[] | undefined>

type PluginInfo = {
  id?: string
  name?: string
}

function Row(props: { label: string; children: any }) {
  return (
    <div class="flex items-start gap-3 py-2 border-b border-border-base last:border-b-0">
      <span class="text-text-weak text-13-regular w-[120px] shrink-0">{props.label}</span>
      <span class="text-text-base text-13-regular min-w-0 break-all">{props.children}</span>
    </div>
  )
}

function shellHooksFromConfig(config: unknown): HooksConfig {
  if (!config || typeof config !== "object") return {}
  const hooks = (config as { hooks?: unknown }).hooks
  if (!hooks || typeof hooks !== "object") return {}
  return hooks as HooksConfig
}

export const DialogHooks: Component = () => {
  const language = useLanguage()
  const sdk = useSDK()
  const sync = useSync()

  const [plugins] = createResource(async (): Promise<PluginInfo[]> => {
    try {
      const res = await fetch(`${sdk.url.replace(/\/$/, "")}/plugin`, {
        credentials: "include",
        headers: {
          "x-tinycode-directory": encodeURIComponent(sdk.directory),
        },
      })
      if (!res.ok) return []
      const data = (await res.json()) as PluginInfo[] | { plugins?: PluginInfo[] }
      if (Array.isArray(data)) return data
      return data.plugins ?? []
    } catch {
      return []
    }
  })

  const shellHooks = () => shellHooksFromConfig(sync.data.config)
  const shellEvents = () =>
    Object.entries(shellHooks())
      .filter(([, hooks]) => Array.isArray(hooks) && hooks.length > 0)
      .sort(([a], [b]) => a.localeCompare(b))

  return (
    <Dialog title={language.t("dialog.hooks.title")} size="large">
      <div class="flex flex-col gap-1 p-4 overflow-y-auto max-h-[60vh]">
        <Row label={language.t("dialog.hooks.plugins")}>
          <Show
            when={(plugins() ?? []).length > 0}
            fallback={
              <Show when={!plugins.loading} fallback="...">
                {language.t("dialog.hooks.plugins.none")}
              </Show>
            }
          >
            <div class="flex flex-col gap-0.5">
              <For each={plugins() ?? []}>{(p) => <div>{p.name || p.id || "?"}</div>}</For>
            </div>
          </Show>
        </Row>
        <Row label={language.t("dialog.hooks.shell")}>
          <Show
            when={shellEvents().length > 0}
            fallback={
              <div class="flex flex-col gap-2">
                <span>{language.t("dialog.hooks.shell.none")}</span>
                <pre class="text-[12px] font-mono text-text-weak whitespace-pre-wrap">
                  {language.t("dialog.hooks.shell.hint")}
                </pre>
              </div>
            }
          >
            <div class="flex flex-col gap-2 font-mono text-[12px]">
              <For each={shellEvents()}>
                {([event, hooks]) => (
                  <div>
                    <div class="text-text-base mb-0.5">{event}</div>
                    <For each={hooks ?? []}>
                      {(h) => {
                        let cmd = h.command
                        if (cmd.length > 50) cmd = cmd.slice(0, 47) + "..."
                        const bits = [cmd]
                        if (h.match && Object.keys(h.match).length > 0) {
                          bits.push(`(match: ${JSON.stringify(h.match)})`)
                        }
                        if (h.timeout) bits.push(`[${h.timeout}s]`)
                        return <div class="text-text-weak pl-2">{bits.join(" ")}</div>
                      }}
                    </For>
                  </div>
                )}
              </For>
            </div>
          </Show>
        </Row>
      </div>
    </Dialog>
  )
}
