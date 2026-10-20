import { createMemo, Show } from "solid-js"
import { Icon } from "@tinycode/ui/icon"
import { useLanguage } from "@/context/language"
import { useServerSDK, type SseConnectionState } from "@/context/server-sdk"

export function SseConnectionBanner() {
  const language = useLanguage()
  const serverSDK = useServerSDK()
  const state = createMemo(() => serverSDK.connectionState())

  const copy = (status: Exclude<SseConnectionState, "connected">) => {
    if (status === "offline") {
      return {
        title: language.t("sseBanner.offline.title"),
        description: language.t("sseBanner.offline.description"),
      }
    }
    return {
      title: language.t("sseBanner.reconnecting.title"),
      description: language.t("sseBanner.reconnecting.description"),
    }
  }

  return (
    <Show when={state() !== "connected" ? state() : false}>
      {(status) => {
        const text = copy(status() as Exclude<SseConnectionState, "connected">)
        return (
          <div
            class="fixed top-4 left-1/2 -translate-x-1/2 z-[100] pointer-events-none"
            role="status"
            aria-live="polite"
          >
            <div class="flex items-center gap-3 px-4 py-3 rounded-lg border border-border-base bg-surface-raised-stronger-non-alpha shadow-[var(--shadow-lg-border-base)] min-w-[320px] max-w-[min(calc(100vw-2rem),480px)]">
              <div class="flex-shrink-0 w-5 h-5 rounded-full bg-surface-raised-base flex items-center justify-center">
                <Icon name={status() === "offline" ? "circle-x" : "status"} size="small" class="text-icon-strong" />
              </div>
              <div class="flex-1 min-w-0">
                <p class="text-13-medium text-text-strong">{text.title}</p>
                <p class="text-12-regular text-text-weak">{text.description}</p>
              </div>
            </div>
          </div>
        )
      }}
    </Show>
  )
}
