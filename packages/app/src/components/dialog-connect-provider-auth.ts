import type { ProviderAuthMethod } from "@tinycode/sdk/v2/client"

// Provider OAuth routes are not implemented. Drop those methods so the dialog
// cannot offer a login the server will reject. See issue #676.
export function usableAuthMethods(
  raw: ProviderAuthMethod[] | undefined,
  fallback: ProviderAuthMethod[],
): ProviderAuthMethod[] {
  const list = raw ?? fallback
  const usable = list.filter((method) => method.type !== "oauth")
  if (usable.length > 0) return usable
  if (list.some((method) => method.type === "oauth")) return fallback
  return list
}
