import { Component, createResource, Show } from "solid-js"
import { Dialog } from "@tinycode/ui/dialog"
import { useLanguage } from "@/context/language"
import { useSDK } from "@/context/sdk"

export const DialogDiff: Component = () => {
  const language = useLanguage()
  const sdk = useSDK()

  const [diff] = createResource(async () => {
    const res = await sdk.client.vcs.diff2.raw().catch(() => ({ data: undefined }))
    const data = res.data
    if (typeof data === "string") return data
    return ""
  })

  return (
    <Dialog title={language.t("dialog.diff.title")} size="x-large">
      <div class="p-4 overflow-auto max-h-[70vh]">
        <Show
          when={diff() && diff()!.length > 0}
          fallback={
            <div class="text-text-weak text-14-regular text-center py-8">
              {language.t("dialog.diff.empty")}
            </div>
          }
        >
          <pre class="text-[12px] leading-relaxed font-mono whitespace-pre-wrap break-all text-text-base bg-surface-raised-base p-4 rounded-lg overflow-x-auto">
            {diff()}
          </pre>
        </Show>
      </div>
    </Dialog>
  )
}
