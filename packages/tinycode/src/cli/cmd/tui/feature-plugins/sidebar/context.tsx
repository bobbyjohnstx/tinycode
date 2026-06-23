import type { AssistantMessage } from "@tinycode/sdk/v2"
import type { TuiPlugin, TuiPluginApi } from "@tinycode/plugin/tui"
import type { InternalTuiPlugin } from "../../plugin/internal"
import { createMemo, createResource, Show } from "solid-js"
import { isRecord } from "@/util/record"

const id = "internal:sidebar-context"

const money = new Intl.NumberFormat("en-US", {
  style: "currency",
  currency: "USD",
})

function View(props: { api: TuiPluginApi; session_id: string }) {
  const theme = () => props.api.theme.current
  const msg = createMemo(() => props.api.state.session.messages(props.session_id))
  const session = createMemo(() => props.api.state.session.get(props.session_id))
  const cost = createMemo(() => session()?.cost ?? 0)
  const openRouterSource = createMemo(() => ({
    cost: cost(),
    sessionID: props.session_id,
    enabled: hasOpenRouter(props.api),
  }))
  const [openRouter] = createResource(openRouterSource, (source) => openRouterAccount(props.api, source.enabled))

  const state = createMemo(() => {
    const last = msg().findLast((item): item is AssistantMessage => item.role === "assistant" && item.tokens.output > 0)
    if (!last) {
      return {
        tokens: 0,
        percent: null,
      }
    }

    const tokens =
      last.tokens.input + last.tokens.output + last.tokens.reasoning + last.tokens.cache.read + last.tokens.cache.write
    const model = props.api.state.provider.find((item) => item.id === last.providerID)?.models[last.modelID]
    return {
      tokens,
      percent: model?.limit.context ? Math.round((tokens / model.limit.context) * 100) : null,
    }
  })

  return (
    <box>
      <text fg={theme().text}>
        <b>Context</b>
      </text>
      <text fg={theme().textMuted}>{state().tokens.toLocaleString()} tokens</text>
      <text fg={theme().textMuted}>{state().percent ?? 0}% used</text>
      <text fg={theme().textMuted}>{money.format(cost())} spent</text>
      <Show when={openRouter()}>
        {(account) => (
          <>
            <text fg={theme().textMuted}>{money.format(account().remaining)} OpenRouter left</text>
            <text fg={theme().textMuted}>OpenRouter used {money.format(account().totalUsage)} total</text>
            <Show when={account().monthly !== undefined || account().yearly !== undefined}>
              <text fg={theme().textMuted}>tinycode month {money.format(account().monthly ?? 0)}</text>
              <text fg={theme().textMuted}>tinycode year {money.format(account().yearly ?? 0)}</text>
            </Show>
          </>
        )}
      </Show>
    </box>
  )
}

function hasOpenRouter(api: TuiPluginApi) {
  if (process.env.OPENROUTER_API_KEY) return true
  return api.state.provider.some((provider) => provider.id === "openrouter")
}

async function openRouterAccount(api: TuiPluginApi, enabled: boolean) {
  const apiKey = process.env.OPENROUTER_API_KEY
  if (!enabled || !apiKey) return undefined

  const credits = await fetch("https://openrouter.ai/api/v1/credits", {
    headers: {
      Authorization: `Bearer ${apiKey}`,
    },
  }).then((response) => (response.ok ? response.json() : undefined))

  const data = isRecord(credits) && isRecord(credits.data) ? credits.data : undefined
  const totalCredits = number(data?.total_credits)
  const totalUsage = number(data?.total_usage)
  if (totalCredits === undefined || totalUsage === undefined) return undefined

  const sessions = await api.client.session
    .list({
      start: new Date(new Date().getFullYear(), 0, 1).getTime(),
      limit: 10_000,
    })
    .then((response) => response.data ?? [])
    .catch(() => [])
  const local = localOpenRouterSpend(sessions)

  return {
    remaining: Math.max(0, totalCredits - totalUsage),
    totalUsage,
    monthly: local.monthly,
    yearly: local.yearly,
  }
}

function localOpenRouterSpend(sessions: ReadonlyArray<ReturnType<TuiPluginApi["state"]["session"]["get"]>>) {
  const monthStart = new Date()
  monthStart.setDate(1)
  monthStart.setHours(0, 0, 0, 0)

  const sum = (start: number) =>
    sessions
      .filter((session) => session?.model?.providerID === "openrouter" && session.time.updated >= start)
      .reduce((total, session) => total + (session?.cost ?? 0), 0)

  return {
    monthly: sum(monthStart.getTime()),
    yearly: sum(new Date(new Date().getFullYear(), 0, 1).getTime()),
  }
}

function number(value: unknown) {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined
}

const tui: TuiPlugin = async (api) => {
  api.slots.register({
    order: 100,
    slots: {
      sidebar_content(_ctx, props) {
        return <View api={api} session_id={props.session_id} />
      },
    },
  })
}

const plugin: InternalTuiPlugin = {
  id,
  tui,
}

export default plugin
