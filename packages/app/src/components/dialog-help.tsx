import { Component, createResource, For, Show } from "solid-js"
import { Dialog } from "@tinycode/ui/dialog"
import { Tabs } from "@tinycode/ui/tabs"
import { Icon } from "@tinycode/ui/icon"
import { Spinner } from "@tinycode/ui/spinner"
import { useLanguage } from "@/context/language"
import { useServerSDK } from "@/context/server-sdk"
import { formatKeybind } from "@/context/command"

type HelpKeybinding = {
  key: string
  description: string
  category: string
}

type HelpCommand = {
  name: string
  description: string
  source: string
}

type HelpFeature = {
  name: string
  description: string
}

type HelpResponse = {
  keybindings: HelpKeybinding[]
  commands: HelpCommand[]
  features: HelpFeature[]
}

export const DialogHelp: Component = () => {
  const language = useLanguage()
  const serverSDK = useServerSDK()

  const [data] = createResource(async () => {
    const response = await fetch(`${serverSDK.url}/help`)
    if (!response.ok) throw new Error(language.t("dialog.help.error"))
    return (await response.json()) as HelpResponse
  })

  const groupedKeybindings = () => {
    const items = data()?.keybindings ?? []
    const groups = new Map<string, HelpKeybinding[]>()
    for (const item of items) {
      const category = item.category || "General"
      const list = groups.get(category)
      if (list) list.push(item)
      else groups.set(category, [item])
    }
    return [...groups.entries()]
  }

  return (
    <Dialog title={language.t("dialog.help.title")} size="large" transition>
      <Show
        when={!data.loading}
        fallback={
          <div class="flex items-center justify-center p-8">
            <Spinner class="size-5" />
          </div>
        }
      >
        <Show
          when={!data.error}
          fallback={
            <div class="flex items-center justify-center p-8 text-14-regular text-text-weak">
              {language.t("dialog.help.error")}
            </div>
          }
        >
          <Tabs orientation="horizontal" defaultValue="keybindings" class="h-full">
            <Tabs.List class="px-4">
              <Tabs.Trigger value="keybindings">
                <Icon name="keyboard" size="small" />
                {language.t("dialog.help.tab.keybindings")}
              </Tabs.Trigger>
              <Tabs.Trigger value="commands">
                <Icon name="terminal" size="small" />
                {language.t("dialog.help.tab.commands")}
              </Tabs.Trigger>
              <Tabs.Trigger value="features">
                <Icon name="help" size="small" />
                {language.t("dialog.help.tab.features")}
              </Tabs.Trigger>
            </Tabs.List>

            <Tabs.Content value="keybindings" class="overflow-y-auto px-4 pb-4">
              <div class="flex flex-col gap-4 pt-3">
                <For each={groupedKeybindings()}>
                  {([category, bindings]) => (
                    <div class="flex flex-col gap-1">
                      <h3 class="text-12-medium text-text-weak uppercase tracking-wider pb-1">{category}</h3>
                      <For each={bindings}>
                        {(binding) => (
                          <div class="flex items-center justify-between py-1.5 px-1">
                            <span class="text-13-regular text-text-strong">{binding.description}</span>
                            <kbd class="shrink-0 ml-4 px-2 py-0.5 rounded bg-surface-base text-12-medium text-text-base border border-border-weak-base">
                              {formatKeybind(binding.key)}
                            </kbd>
                          </div>
                        )}
                      </For>
                    </div>
                  )}
                </For>
              </div>
            </Tabs.Content>

            <Tabs.Content value="commands" class="overflow-y-auto px-4 pb-4">
              <div class="flex flex-col gap-1 pt-3">
                <For each={data()?.commands ?? []}>
                  {(cmd) => (
                    <div class="flex items-start gap-3 py-2 px-1">
                      <span class="shrink-0 text-13-medium text-text-strong font-mono">/{cmd.name}</span>
                      <span class="text-13-regular text-text-weak">{cmd.description}</span>
                    </div>
                  )}
                </For>
              </div>
            </Tabs.Content>

            <Tabs.Content value="features" class="overflow-y-auto px-4 pb-4">
              <div class="flex flex-col gap-1 pt-3">
                <For each={data()?.features ?? []}>
                  {(feature) => (
                    <div class="flex flex-col gap-0.5 py-2 px-1">
                      <span class="text-13-medium text-text-strong">{feature.name}</span>
                      <span class="text-13-regular text-text-weak">{feature.description}</span>
                    </div>
                  )}
                </For>
              </div>
            </Tabs.Content>
          </Tabs>
        </Show>
      </Show>
    </Dialog>
  )
}
