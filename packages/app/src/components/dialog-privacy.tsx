import { Component, createResource, For, Show } from "solid-js"
import { Dialog } from "@tinycode/ui/dialog"
import { useLanguage } from "@/context/language"
import { useSDK } from "@/context/sdk"

function Row(props: { label: string; children: any }) {
  return (
    <div class="flex items-start gap-3 py-2 border-b border-border-base last:border-b-0">
      <span class="text-text-weak text-13-regular w-[120px] shrink-0">{props.label}</span>
      <span class="text-text-base text-13-regular min-w-0 break-all">{props.children}</span>
    </div>
  )
}

export const DialogPrivacy: Component = () => {
  const language = useLanguage()
  const sdk = useSDK()

  const [paths] = createResource(async () => {
    const res = await sdk.client.path.get().catch(() => ({ data: undefined }))
    return res.data as Record<string, string> | undefined
  })

  return (
    <Dialog title={language.t("dialog.privacy.title")} size="large">
      <div class="flex flex-col gap-3 p-4 overflow-y-auto max-h-[60vh] text-13-regular">
        <section class="flex flex-col gap-1">
          <h3 class="text-14-medium text-text-strong">{language.t("dialog.privacy.local.title")}</h3>
          <p class="text-text-weak">{language.t("dialog.privacy.local.body")}</p>
          <Show when={paths()}>
            {(p) => (
              <div class="mt-2 flex flex-col gap-0.5 font-mono text-[12px]">
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
        </section>
        <Row label={language.t("dialog.privacy.leaves.label")}>
          {language.t("dialog.privacy.leaves.body")}
        </Row>
        <Row label={language.t("dialog.privacy.notCollected.label")}>
          {language.t("dialog.privacy.notCollected.body")}
        </Row>
      </div>
    </Dialog>
  )
}
