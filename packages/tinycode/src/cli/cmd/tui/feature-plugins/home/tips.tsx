import type { TuiPlugin, TuiPluginApi } from "@opencode-ai/plugin/tui"
import type { InternalTuiPlugin } from "../../plugin/internal"
import { createMemo, Show } from "solid-js"
import { Tips } from "./tips-view"
import { useBindings } from "../../keymap"

const id = "internal:home-tips"

function View(props: { api: TuiPluginApi; hidden: boolean; show: boolean; connected: boolean }) {
  useBindings(() => ({
    commands: [
      {
        name: "tips.toggle",
        title: props.hidden ? "Show tips" : "Hide tips",
        category: "System",
        namespace: "palette",
        run() {
          props.api.kv.set("tips_hidden", !props.api.kv.get("tips_hidden", false))
          props.api.ui.dialog.clear()
        },
      },
    ],
    bindings: props.api.tuiConfig.keybinds.get("tips.toggle"),
  }))

  return (
    <box width="100%" maxWidth={75} alignItems="center" paddingTop={3} flexShrink={1}>
      <Show when={props.show}>
        <Tips api={props.api} connected={props.connected} />
      </Show>
    </box>
  )
}

function FirstTimeTips(props: { api: TuiPluginApi }) {
  const theme = () => props.api.theme.current

  return (
    <box width="100%" maxWidth={75} alignItems="center" paddingTop={3} flexShrink={1} gap={1}>
      <text fg={theme().warning}>● Getting Started</text>
      <text fg={theme().textMuted}>Type @ to mention files or agents</text>
      <text fg={theme().textMuted}>Start with / for commands, ctrl+p for palette</text>
      <text fg={theme().textMuted}>Press tab to switch agents, F1 for help</text>
    </box>
  )
}

const tui: TuiPlugin = async (api) => {
  api.slots.register({
    order: 100,
    slots: {
      home_bottom() {
        const hidden = createMemo(() => api.kv.get("tips_hidden", false))
        const first = createMemo(() => api.state.session.count() === 0)
        const connected = createMemo(() =>
          api.state.provider.some(
            (item) => item.id !== "opencode" || Object.values(item.models).some((model) => model.cost?.input !== 0),
          ),
        )
        const showFirstTimeTips = createMemo(() => first() && connected() && !hidden())
        const show = createMemo(() => (!first() || !connected()) && !hidden())
        return (
          <>
            <Show when={showFirstTimeTips()}>
              <FirstTimeTips api={api} />
            </Show>
            <Show when={!showFirstTimeTips()}>
              <View api={api} hidden={hidden()} show={show()} connected={connected()} />
            </Show>
          </>
        )
      },
    },
  })
}

const plugin: InternalTuiPlugin = {
  id,
  tui,
}

export default plugin
