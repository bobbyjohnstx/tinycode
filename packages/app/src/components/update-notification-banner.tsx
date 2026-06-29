import { createSignal, onCleanup, onMount, Show } from "solid-js"
import { Button } from "@tinycode/ui/button"
import { Icon } from "@tinycode/ui/icon"
import { useLanguage } from "@/context/language"

export function UpdateNotificationBanner() {
  const language = useLanguage()
  const [updateInfo, setUpdateInfo] = createSignal<{ version: string } | null>(null)
  const [visible, setVisible] = createSignal(false)
  const [animating, setAnimating] = createSignal(false)

  // Only render in Electron desktop app
  const isDesktop = () => typeof window !== "undefined" && !!window.api

  const handleRestart = async () => {
    if (!window.api?.installUpdate) return
    await window.api.installUpdate()
  }

  const handleDismiss = () => {
    setAnimating(true)
    setTimeout(() => {
      setVisible(false)
      setAnimating(false)
    }, 200)
  }

  onMount(() => {
    if (!isDesktop()) return

    // Listen for update-ready event from Electron main process
    const cleanup = window.api?.onUpdateReady?.((version: string) => {
      setUpdateInfo({ version })
      setVisible(true)
    })

    onCleanup(() => {
      cleanup?.()
    })
  })

  return (
    <Show when={isDesktop() && visible()}>
      <div
        class="fixed top-4 left-1/2 -translate-x-1/2 z-[100] pointer-events-auto"
        classList={{
          "animate-[slide-in-from-top_200ms_ease-out]": !animating(),
          "animate-[slide-out-to-top_200ms_ease-in]": animating(),
        }}
        role="alert"
        aria-live="polite"
      >
        <div class="flex items-center gap-3 px-4 py-3 rounded-lg border border-border-base bg-surface-raised-stronger-non-alpha shadow-[var(--shadow-lg-border-base)] min-w-[320px] max-w-[min(calc(100vw-2rem),480px)]">
          {/* Icon */}
          <div class="flex-shrink-0 w-5 h-5 rounded-full bg-surface-positive flex items-center justify-center">
            <Icon name="check-small" size="small" class="text-icon-on-positive" />
          </div>

          {/* Content */}
          <div class="flex-1 min-w-0">
            <p class="text-13-medium text-text-strong">
              {language.t("updateBanner.title", { version: updateInfo()?.version ?? "" })}
            </p>
          </div>

          {/* Actions */}
          <div class="flex items-center gap-2">
            <Button variant="primary" size="small" onClick={handleRestart}>
              {language.t("updateBanner.action.restart")}
            </Button>
            <button
              type="button"
              onClick={handleDismiss}
              class="flex items-center justify-center w-6 h-6 rounded-md hover:bg-surface-raised-base-hover transition-colors"
              aria-label={language.t("updateBanner.action.dismiss")}
            >
              <Icon name="close" size="small" class="text-icon-weak" />
            </button>
          </div>
        </div>
      </div>
    </Show>
  )
}
