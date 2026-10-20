import { Show, createMemo, type JSX } from "solid-js"
import { WordmarkV2 } from "@tinycode/ui/v2/components/wordmark-v2.jsx"
import { Button } from "@tinycode/ui/button"
import { useDialog } from "@tinycode/ui/context/dialog"
import { useLanguage } from "@/context/language"
import { useProviders } from "@/hooks/use-providers"

export function NewSessionDesignView(props: { children: JSX.Element }) {
  const language = useLanguage()
  const providers = useProviders()
  const dialog = useDialog()
  const needsConnect = createMemo(() => providers.connected().length === 0)

  return (
    <div data-component="session-new-design" class="relative size-full overflow-hidden bg-v2-background-bg-deep">
      <div class="absolute inset-x-0 top-[25.375%] flex justify-center px-6">
        <div class="w-full max-w-[720px]">
          <WordmarkV2 class="h-auto w-full text-v2-icon-icon-base" />
          <Show when={needsConnect()}>
            <div class="mt-6 flex flex-col items-center gap-3 text-center" data-component="session-empty-connect">
              <div class="text-14-medium text-text-strong">{language.t("session.empty.connect.title")}</div>
              <div class="text-14-regular text-text-base max-w-100">
                {language.t("session.empty.connect.description")}
              </div>
              <Button
                size="large"
                icon="plus-small"
                onClick={() => {
                  void import("@/components/dialog-select-provider").then((x) => {
                    dialog.show(() => <x.DialogSelectProvider />)
                  })
                }}
              >
                {language.t("session.empty.connect.action")}
              </Button>
            </div>
          </Show>
          <div class="mt-8">{props.children}</div>
        </div>
      </div>
    </div>
  )
}
