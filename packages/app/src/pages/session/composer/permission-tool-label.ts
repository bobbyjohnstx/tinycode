/** Go tool ids → SPA i18n permission tool keys (settings.permissions.tool.<key>.*). */
const TOOL_ALIASES: Record<string, string> = {
  shell: "bash",
  "destructive-shell": "bash",
  "secret-shell": "bash",
}

export function permissionToolKey(permission: string): string {
  return TOOL_ALIASES[permission] ?? permission
}

export function permissionDockTitle(input: {
  permission: string
  patterns: string[]
  toolTitle: (key: string) => string
  fallback: string
}): string {
  const key = permissionToolKey(input.permission)
  const title = input.toolTitle(key)
  const resolved = title && title !== `settings.permissions.tool.${key}.title` ? title : input.permission || input.fallback
  const pattern = input.patterns[0]?.trim()
  if (!pattern) return resolved
  const snippet = pattern.length > 48 ? `${pattern.slice(0, 45)}…` : pattern
  return `${resolved}: ${snippet}`
}
