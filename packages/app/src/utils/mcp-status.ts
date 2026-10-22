/** Go MCP statuses from internal/mcp (plus legacy SPA values). */
export type McpStatusValue =
  | "connected"
  | "disconnected"
  | "connecting"
  | "reconnecting"
  | "error"
  | "failed"
  | "disabled"
  | "needs_auth"
  | "needs_client_registration"
  | string

const STATUS_LABEL_KEYS = {
  connected: "mcp.status.connected",
  disconnected: "mcp.status.disconnected",
  connecting: "mcp.status.connecting",
  reconnecting: "mcp.status.reconnecting",
  error: "mcp.status.error",
  failed: "mcp.status.failed",
  needs_auth: "mcp.status.needs_auth",
  needs_client_registration: "mcp.status.needs_client_registration",
  disabled: "mcp.status.disabled",
} as const

/** Go `error` and legacy `failed` / client-registration failures. */
export function isMcpFailure(status: McpStatusValue | undefined): boolean {
  return status === "error" || status === "failed" || status === "needs_client_registration"
}

export function isMcpWarning(status: McpStatusValue | undefined): boolean {
  return (
    status === "needs_auth" ||
    status === "connecting" ||
    status === "reconnecting" ||
    status === "disconnected"
  )
}

export function mcpIssueLevel(
  statuses: Iterable<{ status?: McpStatusValue }>,
): "critical" | "warning" | undefined {
  let warn = false
  for (const item of statuses) {
    if (isMcpFailure(item.status)) return "critical"
    if (isMcpWarning(item.status)) warn = true
  }
  if (warn) return "warning"
  return undefined
}

export function mcpStatusLabelKey(status: McpStatusValue | undefined): string | undefined {
  if (!status) return undefined
  return STATUS_LABEL_KEYS[status as keyof typeof STATUS_LABEL_KEYS]
}

/** Tailwind classList bits for status dots (success / critical / warning / weak). */
export function mcpStatusDotClass(status: McpStatusValue | undefined): Record<string, boolean> {
  return {
    "bg-icon-success-base": status === "connected",
    "bg-icon-critical-base": isMcpFailure(status),
    "bg-border-weak-base": status === "disabled",
    "bg-icon-warning-base":
      status === "needs_auth" ||
      status === "connecting" ||
      status === "reconnecting" ||
      status === "disconnected",
  }
}
