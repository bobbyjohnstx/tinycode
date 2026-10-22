import { For, Show, createMemo } from "solid-js"
import type { PermissionRequest } from "@tinycode/sdk/v2"
import { Button } from "@tinycode/ui/button"
import { DockPrompt } from "@tinycode/ui/dock-prompt"
import { Icon } from "@tinycode/ui/icon"
import { useLanguage } from "@/context/language"
import {
  isShellPermission,
  permissionAllowAlwaysScope,
  permissionDockTitle,
  permissionToolKey,
} from "./permission-tool-label"

export function SessionPermissionDock(props: {
  request: PermissionRequest
  responding: boolean
  onDecide: (response: "once" | "always" | "reject") => void
}) {
  const language = useLanguage()

  const toolKey = () => permissionToolKey(props.request.permission)

  const headerTitle = createMemo(() =>
    permissionDockTitle({
      permission: props.request.permission,
      patterns: props.request.patterns,
      fallback: language.t("notification.permission.title"),
      toolTitle: (key) => language.t(`settings.permissions.tool.${key}.title` as Parameters<typeof language.t>[0]),
    }),
  )

  const allowAlwaysLabel = createMemo(() => {
    const scope = permissionAllowAlwaysScope(props.request.always, props.request.patterns)
    if (!scope) return language.t("ui.permission.allowAlways")
    return language.t("ui.permission.allowAlwaysScoped", { scope })
  })

  const toolDescription = () => {
    const key = `settings.permissions.tool.${toolKey()}.description`
    const value = language.t(key as Parameters<typeof language.t>[0])
    if (value === key) return ""
    return value
  }

  return (
    <DockPrompt
      kind="permission"
      header={
        <div data-slot="permission-row" data-variant="header">
          <span data-slot="permission-icon">
            <Icon name="warning" size="normal" />
          </span>
          <div data-slot="permission-header-title">{headerTitle()}</div>
        </div>
      }
      footer={
        <>
          <div />
          <div data-slot="permission-footer-actions">
            <Button variant="ghost" size="normal" onClick={() => props.onDecide("reject")} disabled={props.responding}>
              {language.t("ui.permission.deny")}
            </Button>
            <Button
              variant="secondary"
              size="normal"
              onClick={() => props.onDecide("always")}
              disabled={props.responding}
            >
              {allowAlwaysLabel()}
            </Button>
            <Button variant="primary" size="normal" onClick={() => props.onDecide("once")} disabled={props.responding}>
              {language.t("ui.permission.allowOnce")}
            </Button>
          </div>
        </>
      }
    >
      <Show when={toolDescription()}>
        <div data-slot="permission-row">
          <span data-slot="permission-spacer" aria-hidden="true" />
          <div data-slot="permission-hint">{toolDescription()}</div>
        </div>
      </Show>

      <Show when={props.request.patterns.length > 0}>
        <div data-slot="permission-row">
          <span data-slot="permission-spacer" aria-hidden="true" />
          <div data-slot="permission-patterns">
            <For each={props.request.patterns}>
              {(pattern) => <code class="text-12-regular text-text-base break-all">{pattern}</code>}
            </For>
          </div>
        </div>
      </Show>

      <Show when={isShellPermission(props.request.permission)}>
        <div data-slot="permission-row">
          <span data-slot="permission-spacer" aria-hidden="true" />
          <div data-slot="permission-hint">{language.t("ui.permission.shellAlwaysSibling")}</div>
        </div>
      </Show>
    </DockPrompt>
  )
}
