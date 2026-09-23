import { Match, Show, Switch, createMemo, createSignal, createEffect, onCleanup } from "solid-js"
import { Tooltip, type TooltipProps } from "@tinycode/ui/tooltip"
import { ProgressCircle } from "@tinycode/ui/progress-circle"
import { Button } from "@tinycode/ui/button"

import { useFile } from "@/context/file"
import { useLayout } from "@/context/layout"
import { useSync } from "@/context/sync"
import { useLanguage } from "@/context/language"
import { useLocal } from "@/context/local"
import { useSDK } from "@/context/sdk"
import { useProviders } from "@/hooks/use-providers"
import { getSessionContextMetrics } from "@/components/session/session-context-metrics"
import { useSessionLayout } from "@/pages/session/session-layout"
import { createSessionTabs } from "@/pages/session/helpers"

interface SessionContextUsageProps {
  variant?: "button" | "indicator"
  placement?: TooltipProps["placement"]
}

function openSessionContext(args: {
  view: ReturnType<ReturnType<typeof useLayout>["view"]>
  layout: ReturnType<typeof useLayout>
  tabs: ReturnType<ReturnType<typeof useLayout>["tabs"]>
}) {
  if (!args.view.reviewPanel.opened()) args.view.reviewPanel.open()
  if (args.layout.fileTree.opened() && args.layout.fileTree.tab() !== "all") args.layout.fileTree.setTab("all")
  void args.tabs.open("context")
  args.tabs.setActive("context")
}

type BalanceData = { remaining?: number | null; usage?: number; provider?: string }

function formatTokenCount(total: number): string {
  if (total >= 1_000_000) return `${(total / 1_000_000).toFixed(1)}m`
  if (total >= 1_000) return `${Math.round(total / 1_000)}k`
  return String(total)
}

export function SessionContextUsage(props: SessionContextUsageProps) {
  const sync = useSync()
  const file = useFile()
  const layout = useLayout()
  const language = useLanguage()
  const local = useLocal()
  const sdk = useSDK()
  const providers = useProviders()
  const { params, tabs, view } = useSessionLayout()

  const variant = createMemo(() => props.variant ?? "button")
  const tabState = createSessionTabs({
    tabs,
    pathFromTab: file.pathFromTab,
    normalizeTab: (tab) => (tab.startsWith("file://") ? file.tab(tab) : tab),
  })
  const messages = createMemo(() => (params.id ? (sync.data.message[params.id] ?? []) : []))

  const usd = createMemo(
    () =>
      new Intl.NumberFormat(language.intl(), {
        style: "currency",
        currency: "USD",
      }),
  )

  const metrics = createMemo(() => getSessionContextMetrics(messages(), [...providers.all().values()]))
  const context = createMemo(() => metrics().context)
  const cost = createMemo(() => {
    return usd().format(metrics().totalCost)
  })

  const [balance, setBalance] = createSignal<BalanceData | undefined>()

  createEffect(() => {
    const model = local.model.current()
    if (!model) {
      setBalance(undefined)
      return
    }
    const providerID = model.provider.id
    let cancelled = false
    const url = `${sdk.url}/provider/${encodeURIComponent(providerID)}/balance`
    fetch(url)
      .then((res) => (res.ok ? res.json() : undefined))
      .then((data: BalanceData | undefined) => {
        if (!cancelled) setBalance(data)
      })
      .catch(() => {
        if (!cancelled) setBalance(undefined)
      })
    onCleanup(() => {
      cancelled = true
    })
  })

  const balanceText = createMemo(() => {
    const b = balance()
    if (!b) return undefined
    if (b.remaining != null) return `${usd().format(b.remaining)} remaining`
    if (b.usage != null && b.usage > 0) return `${usd().format(b.usage)} used`
    return undefined
  })

  const tokenText = createMemo(() => {
    const ctx = context()
    if (!ctx) return undefined
    return `${formatTokenCount(ctx.total)} tok`
  })

  const mcpStatus = createMemo(() => {
    if (!sync.data.mcp_ready) return undefined
    const servers = sync.data.mcp
    const entries = Object.entries(servers)
    if (entries.length === 0) return undefined
    const connected = entries.filter(([, s]) => s.status === "connected").length
    const hasErrors = entries.some(([, s]) => s.status === "failed")
    return { total: entries.length, connected, hasErrors }
  })

  const openContext = () => {
    if (!params.id) return

    if (tabState.activeTab() === "context") {
      tabs().close("context")
      return
    }
    openSessionContext({
      view: view(),
      layout,
      tabs: tabs(),
    })
  }

  const circle = () => (
    <div class="flex items-center justify-center">
      <ProgressCircle size={16} strokeWidth={2} percentage={context()?.usage ?? 0} />
    </div>
  )

  const tooltipValue = () => (
    <div>
      <Show when={context()}>
        {(ctx) => (
          <>
            <div class="flex items-center gap-2">
              <span class="text-text-invert-strong">{ctx().total.toLocaleString(language.intl())}</span>
              <span class="text-text-invert-base">{language.t("context.usage.tokens")}</span>
            </div>
            <div class="flex items-center gap-2">
              <span class="text-text-invert-strong">{ctx().usage ?? 0}%</span>
              <span class="text-text-invert-base">{language.t("context.usage.usage")}</span>
            </div>
          </>
        )}
      </Show>
      <div class="flex items-center gap-2">
        <span class="text-text-invert-strong">{cost()}</span>
        <span class="text-text-invert-base">{language.t("context.usage.cost")}</span>
      </div>
      <Show when={balanceText()}>
        {(text) => (
          <div class="flex items-center gap-2">
            <span class="text-text-invert-strong">{text()}</span>
            <span class="text-text-invert-base">{language.t("context.usage.balance")}</span>
          </div>
        )}
      </Show>
    </div>
  )

  return (
    <Show when={params.id}>
      <Tooltip value={tooltipValue()} placement={props.placement ?? "top"}>
        <Switch>
          <Match when={variant() === "indicator"}>{circle()}</Match>
          <Match when={true}>
            <Button
              type="button"
              variant="ghost"
              class="h-6 px-1.5 gap-1.5"
              onClick={openContext}
              aria-label={language.t("context.usage.view")}
            >
              {circle()}
              <span class="flex items-center gap-1.5 text-11-regular text-text-weak">
                <Show when={tokenText()}>
                  {(text) => <span>{text()}</span>}
                </Show>
                <Show when={cost()}>
                  {(c) => <span>{c()}</span>}
                </Show>
                <Show when={balanceText()}>
                  {(text) => <span>{text()}</span>}
                </Show>
                <Show when={mcpStatus()}>
                  {(mcp) => (
                    <span class="flex items-center gap-0.5">
                      <span>MCP</span>
                      <span
                        class="inline-block size-1.5 rounded-full"
                        classList={{
                          "bg-icon-success-base": !mcp().hasErrors,
                          "bg-icon-critical-base": mcp().hasErrors,
                        }}
                      />
                      <span>{mcp().connected}</span>
                    </span>
                  )}
                </Show>
              </span>
            </Button>
          </Match>
        </Switch>
      </Tooltip>
    </Show>
  )
}
