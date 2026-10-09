export type FormatterRow = {
  name: string
  extensions: string[]
  enabled: boolean
}

// undefined means the status request has not settled. An empty array means
// formatting is off. A disabled row stays in the list so the UI can label it.
export function visibleFormatters(
  ready: boolean,
  items: FormatterRow[] | undefined,
): FormatterRow[] | undefined {
  if (!ready) return undefined
  return items ?? []
}

export type FormatterView = "loading" | "empty" | "list"

// "loading" stays a separate state so the empty sentence does not appear
// before the status request settles.
export function formatterView(ready: boolean, items: FormatterRow[] | undefined): FormatterView {
  const visible = visibleFormatters(ready, items)
  if (!visible) return "loading"
  if (visible.length === 0) return "empty"
  return "list"
}
