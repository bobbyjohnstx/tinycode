export interface User {
  name: string
  displayName: string
}

export function createUser(name: string): User {
  if (name === "") {
    return { name, displayName: undefined as any }  // BUG: empty string yields undefined displayName
  }
  return { name, displayName: name.charAt(0).toUpperCase() + name.slice(1) }
}
