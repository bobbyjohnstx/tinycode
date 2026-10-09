import type { ProviderAuthMethod } from "@tinycode/sdk/v2/client"
import { Button } from "@tinycode/ui/button"
import { useDialog } from "@tinycode/ui/context/dialog"
import { Dialog } from "@tinycode/ui/dialog"
import { IconButton } from "@tinycode/ui/icon-button"
import { List, type ListRef } from "@tinycode/ui/list"
import { ProviderIcon } from "@tinycode/ui/provider-icon"
import { Spinner } from "@tinycode/ui/spinner"
import { TextField } from "@tinycode/ui/text-field"
import { showToast } from "@tinycode/ui/toast"
import { createEffect, createMemo, createResource, Match, onCleanup, Switch } from "solid-js"
import { createStore, produce } from "solid-js/store"
import { Link } from "@/components/link"
import { useServerSDK } from "@/context/server-sdk"
import { useServerSync } from "@/context/server-sync"
import { useLanguage } from "@/context/language"
import { useProviders } from "@/hooks/use-providers"

export function DialogConnectProvider(props: { provider: string }) {
  const dialog = useDialog()
  const serverSync = useServerSync()
  const serverSDK = useServerSDK()
  const language = useLanguage()
  const providers = useProviders()

  const all = () => {
    void import("./dialog-select-provider").then((x) => {
      dialog.show(() => <x.DialogSelectProvider />)
    })
  }

  const alive = { value: true }

  onCleanup(() => {
    alive.value = false
  })

  const provider = createMemo(
    () => providers.all().get(props.provider) ?? serverSync.data.provider.all.get(props.provider)!,
  )
  const fallback = createMemo<ProviderAuthMethod[]>(() => [
    {
      type: "api" as const,
      label: language.t("provider.connect.method.apiKey"),
    },
  ])
  const [auth] = createResource(
    () => props.provider,
    async () => {
      const cached = serverSync.data.provider_auth[props.provider]
      if (cached) return cached
      const res = await serverSDK.client.provider.auth()
      if (!alive.value) return fallback()
      serverSync.set("provider_auth", res.data ?? {})
      return res.data?.[props.provider] ?? fallback()
    },
  )
  const loading = createMemo(() => auth.loading && !serverSync.data.provider_auth[props.provider])
  // Provider OAuth routes are not implemented. Drop those methods so the dialog
  // cannot offer a login the server will reject. See issue #674.
  const methods = createMemo(() => {
    const raw = auth.latest ?? serverSync.data.provider_auth[props.provider] ?? fallback()
    const usable = raw.filter((method) => method.type !== "oauth")
    if (usable.length > 0) return usable
    if (raw.some((method) => method.type === "oauth")) return fallback()
    return raw
  })
  const [store, setStore] = createStore({
    methodIndex: undefined as undefined | number,
  })

  type Action = { type: "method.select"; index: number } | { type: "method.reset" }

  function dispatch(action: Action) {
    setStore(
      produce((draft) => {
        if (action.type === "method.select") {
          draft.methodIndex = action.index
          return
        }
        draft.methodIndex = undefined
      }),
    )
  }

  const method = createMemo(() => (store.methodIndex !== undefined ? methods().at(store.methodIndex!) : undefined))

  const methodLabel = (value?: { type?: string; label?: string }) => {
    if (!value) return ""
    if (value.type === "api") return language.t("provider.connect.method.apiKey")
    return value.label ?? ""
  }

  function selectMethod(index: number) {
    dispatch({ type: "method.select", index })
  }

  let listRef: ListRef | undefined
  function handleKey(e: KeyboardEvent) {
    if (e.key === "Enter" && e.target instanceof HTMLInputElement) {
      return
    }
    if (e.key === "Escape") return
    listRef?.onKeyDown(e)
  }

  let auto = false
  createEffect(() => {
    if (auto) return
    if (loading()) return
    if (methods().length === 0) {
      auto = true
      void complete()
      return
    }
    if (methods().length === 1) {
      auto = true
      void selectMethod(0)
    }
  })

  async function complete() {
    await serverSDK.client.global.dispose()
    dialog.close()
    showToast({
      variant: "success",
      icon: "circle-check",
      title: language.t("provider.connect.toast.connected.title", { provider: provider().name }),
      description: language.t("provider.connect.toast.connected.description", { provider: provider().name }),
    })
  }

  function goBack() {
    if (methods().length === 1) {
      all()
      return
    }
    if (store.methodIndex !== undefined) {
      dispatch({ type: "method.reset" })
      return
    }
    all()
  }

  function MethodSelection() {
    return (
      <>
        <div class="text-14-regular text-text-base">
          {language.t("provider.connect.selectMethod", { provider: provider().name })}
        </div>
        <div>
          <List
            ref={(ref) => {
              listRef = ref
            }}
            items={methods}
            key={(m) => m?.label}
            onSelect={async (selected, index) => {
              if (!selected) return
              void selectMethod(index)
            }}
          >
            {(i) => (
              <div class="w-full flex items-center gap-x-2">
                <div class="w-4 h-2 rounded-[1px] bg-input-base shadow-xs-border-base flex items-center justify-center">
                  <div class="w-2.5 h-0.5 ml-0 bg-icon-strong-base hidden" data-slot="list-item-extra-icon" />
                </div>
                <span>{methodLabel(i)}</span>
              </div>
            )}
          </List>
        </div>
      </>
    )
  }

  function ApiAuthView() {
    const [formStore, setFormStore] = createStore({
      value: "",
      error: undefined as string | undefined,
    })

    async function handleSubmit(e: SubmitEvent) {
      e.preventDefault()

      const form = e.currentTarget as HTMLFormElement
      const formData = new FormData(form)
      const apiKey = formData.get("apiKey") as string

      if (!apiKey?.trim()) {
        setFormStore("error", language.t("provider.connect.apiKey.required"))
        return
      }

      setFormStore("error", undefined)
      await serverSDK.client.auth.set({
        providerID: props.provider,
        auth: {
          type: "api",
          key: apiKey,
        },
      })
      await complete()
    }

    return (
      <div class="flex flex-col gap-6">
        <Switch>
          <Match when={provider().id === "tinycode"}>
            <div class="flex flex-col gap-4">
              <div class="text-14-regular text-text-base">{language.t("provider.connect.tinycodeZen.line1")}</div>
              <div class="text-14-regular text-text-base">{language.t("provider.connect.tinycodeZen.line2")}</div>
              <div class="text-14-regular text-text-base">
                {language.t("provider.connect.tinycodeZen.visit.prefix")}
                <Link href="https://tinycode.dev/zen" tabIndex={-1}>
                  {language.t("provider.connect.tinycodeZen.visit.link")}
                </Link>
                {language.t("provider.connect.tinycodeZen.visit.suffix")}
              </div>
            </div>
          </Match>
          <Match when={true}>
            <div class="text-14-regular text-text-base">
              {language.t("provider.connect.apiKey.description", { provider: provider().name })}
            </div>
          </Match>
        </Switch>
        <form onSubmit={handleSubmit} class="flex flex-col items-start gap-4">
          <TextField
            autofocus
            type="text"
            label={language.t("provider.connect.apiKey.label", { provider: provider().name })}
            placeholder={language.t("provider.connect.apiKey.placeholder")}
            name="apiKey"
            value={formStore.value}
            onChange={(v) => setFormStore("value", v)}
            validationState={formStore.error ? "invalid" : undefined}
            error={formStore.error}
          />
          <Button class="w-auto" type="submit" size="large" variant="primary">
            {language.t("common.continue")}
          </Button>
        </form>
      </div>
    )
  }

  return (
    <Dialog
      title={
        <IconButton
          tabIndex={-1}
          icon="arrow-left"
          variant="ghost"
          onClick={goBack}
          aria-label={language.t("common.goBack")}
        />
      }
    >
      <div class="flex flex-col gap-6 px-2.5 pb-3">
        <div class="px-2.5 flex gap-4 items-center">
          <ProviderIcon id={props.provider} class="size-5 shrink-0 icon-strong-base" />
          <div class="text-16-medium text-text-strong">
            <Switch>
              <Match when={props.provider === "anthropic" && method()?.label?.toLowerCase().includes("max")}>
                {language.t("provider.connect.title.anthropicProMax")}
              </Match>
              <Match when={true}>{language.t("provider.connect.title", { provider: provider().name })}</Match>
            </Switch>
          </div>
        </div>
        <div class="px-2.5 pb-10 flex flex-col gap-6">
          <div onKeyDown={handleKey} tabIndex={0} autofocus={store.methodIndex === undefined ? true : undefined}>
            <Switch>
              <Match when={loading()}>
                <div class="text-14-regular text-text-base">
                  <div class="flex items-center gap-x-2">
                    <Spinner />
                    <span>{language.t("provider.connect.status.inProgress")}</span>
                  </div>
                </div>
              </Match>
              <Match when={store.methodIndex === undefined}>
                <MethodSelection />
              </Match>
              <Match when={method()?.type === "api"}>
                <ApiAuthView />
              </Match>
            </Switch>
          </div>
        </div>
      </div>
    </Dialog>
  )
}
