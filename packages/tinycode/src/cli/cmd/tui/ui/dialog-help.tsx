import { TextAttributes, type ScrollBoxRenderable } from "@opentui/core"
import { useTerminalDimensions } from "@opentui/solid"
import { createMemo } from "solid-js"
import { useTheme } from "@tui/context/theme"
import { useDialog } from "./dialog"
import { useBindings } from "../keymap"
import { getScrollAcceleration } from "../util/scroll"
import { useTuiConfig } from "../context/tui-config"
import HELP_CONTENT from "../help.md" with { type: "text" }

export function DialogHelp() {
  const dialog = useDialog()
  const { theme, syntax } = useTheme()
  const tuiConfig = useTuiConfig()
  const dimensions = useTerminalDimensions()
  const scrollAcceleration = createMemo(() => getScrollAcceleration(tuiConfig))
  const maxHeight = createMemo(() => Math.max(10, Math.floor(dimensions().height * 0.75)))

  dialog.setSize("full")

  let scroll: ScrollBoxRenderable | undefined

  useBindings(() => ({
    bindings: [
      { key: "escape", desc: "Close help", group: "Dialog", cmd: () => dialog.clear() },
      { key: "return", desc: "Close help", group: "Dialog", cmd: () => dialog.clear() },
      {
        key: "up",
        desc: "Scroll up",
        group: "Dialog",
        cmd: () => scroll?.scrollBy(-1),
      },
      {
        key: "down",
        desc: "Scroll down",
        group: "Dialog",
        cmd: () => scroll?.scrollBy(1),
      },
      {
        key: "pageup",
        desc: "Page up",
        group: "Dialog",
        cmd: () => scroll?.scrollBy(-10),
      },
      {
        key: "pagedown",
        desc: "Page down",
        group: "Dialog",
        cmd: () => scroll?.scrollBy(10),
      },
    ],
  }))

  return (
    <box paddingLeft={2} paddingRight={2} gap={1}>
      <box flexDirection="row" justifyContent="space-between">
        <text attributes={TextAttributes.BOLD} fg={theme.text}>
          Help
        </text>
        <text fg={theme.textMuted} onMouseUp={() => dialog.clear()}>
          esc/enter
        </text>
      </box>
      <scrollbox
        ref={(r: ScrollBoxRenderable) => (scroll = r)}
        maxHeight={maxHeight()}
        scrollAcceleration={scrollAcceleration()}
        scrollbarOptions={{ visible: false }}
        paddingLeft={1}
        paddingRight={1}
      >
        <markdown content={HELP_CONTENT} syntaxStyle={syntax()} fg={theme.textMuted} bg={theme.background} />
      </scrollbox>
    </box>
  )
}
