import { useMutation, useQueryClient } from "@tanstack/solid-query"
import { Component, createMemo, Show } from "solid-js"
import { useSync } from "@/context/sync"
import { useSDK } from "@/context/sdk"
import { Dialog } from "@tinycode/ui/dialog"
import { List } from "@tinycode/ui/list"
import { Button } from "@tinycode/ui/button"
import { useLanguage } from "@/context/language"
import { useQueryOptions } from "@/context/server-sync"
import { pathKey } from "@/utils/path-key"
import { isMcpFailure, mcpStatusLabelKey } from "@/utils/mcp-status"

export const DialogSelectMcp: Component = () => {
  const sync = useSync()
  const sdk = useSDK()
  const language = useLanguage()
  const queryClient = useQueryClient()
  const queryOptions = useQueryOptions()

  const items = createMemo(() =>
    Object.entries(sync.data.mcp ?? {})
      .map(([name, status]) => ({ name, status: status.status }))
      .sort((a, b) => a.name.localeCompare(b.name)),
  )

  // Reconnect-only (TUI parity). No connect/disconnect Switch; auth stays CLI.
  const reconnect = useMutation(() => ({
    mutationFn: async (name: string) => {
      const status = sync.data.mcp[name]
      if (status?.status === "needs_auth") {
        return
      }
      const base = sdk.url.replace(/\/$/, "")
      const res = await fetch(`${base}/mcp/${encodeURIComponent(name)}/reconnect`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
      })
      if (!res.ok) {
        throw new Error(`MCP reconnect failed (${res.status})`)
      }
    },
    onSuccess: () => queryClient.refetchQueries(queryOptions.mcp(pathKey(sync.directory))),
  }))

  const connectedCount = createMemo(() => items().filter((i) => i.status === "connected").length)
  const totalCount = createMemo(() => items().length)

  return (
    <Dialog
      title={language.t("dialog.mcp.title")}
      description={language.t("dialog.mcp.description", { enabled: connectedCount(), total: totalCount() })}
    >
      <List
        search={{ placeholder: language.t("common.search.placeholder"), autofocus: true }}
        emptyMessage={language.t("dialog.mcp.empty")}
        key={(x) => x?.name ?? ""}
        items={items}
        filterKeys={["name", "status"]}
        sortBy={(a, b) => a.name.localeCompare(b.name)}
        onSelect={(x) => {
          if (!x || reconnect.isPending) return
          if (x.status === "needs_auth") return
          reconnect.mutate(x.name)
        }}
      >
        {(i) => {
          const mcpStatus = () => sync.data.mcp[i.name]
          const status = () => mcpStatus()?.status
          const statusLabel = () => {
            const key = mcpStatusLabelKey(status())
            if (!key) return
            return language.t(key)
          }
          const error = () => {
            const s = mcpStatus()
            if (isMcpFailure(s?.status)) return s?.error
            if (s?.status === "needs_auth") {
              return "Authenticate via CLI: tinycode mcp auth " + i.name
            }
          }
          const canReconnect = () => status() !== "needs_auth"
          return (
            <div class="w-full flex items-center justify-between gap-x-3">
              <div class="flex flex-col gap-0.5 min-w-0">
                <div class="flex items-center gap-2">
                  <span class="truncate">{i.name}</span>
                  <Show when={statusLabel()}>
                    <span class="text-11-regular text-text-weaker">{statusLabel()}</span>
                  </Show>
                </div>
                <Show when={error()}>
                  <span class="text-11-regular text-text-weaker truncate">{error()}</span>
                </Show>
              </div>
              <div onClick={(e) => e.stopPropagation()}>
                <Button
                  size="small"
                  variant="ghost"
                  disabled={!canReconnect() || (reconnect.isPending && reconnect.variables === i.name)}
                  onClick={() => {
                    if (!canReconnect() || reconnect.isPending) return
                    reconnect.mutate(i.name)
                  }}
                >
                  {language.t("dialog.mcp.reconnect")}
                </Button>
              </div>
            </div>
          )
        }}
      </List>
    </Dialog>
  )
}
