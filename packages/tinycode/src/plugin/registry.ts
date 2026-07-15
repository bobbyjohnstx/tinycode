import registryData from "./registry.json"

export type RegistryPlugin = {
  name: string
  npm: string
  description: string
  author: string
  tags: string[]
  builtin?: boolean
}

export function all(): RegistryPlugin[] {
  return registryData.plugins
}

export function search(query: string): RegistryPlugin[] {
  if (!query) return all()
  const q = query.toLowerCase()
  return registryData.plugins.filter(
    (p) =>
      p.name.toLowerCase().includes(q) ||
      p.description.toLowerCase().includes(q) ||
      p.tags.some((t) => t.toLowerCase().includes(q)),
  )
}

export function resolve(name: string): RegistryPlugin | undefined {
  return registryData.plugins.find((p) => p.name === name)
}

export * as PluginRegistry from "./registry"
