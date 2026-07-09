import { createUser } from "./user"
import { add } from "./math"

export function formatUserSummary(name: string, score1: number, score2: number): string {
  const user = createUser(name)
  const total = add(score1, score2)
  return `${user.displayName.toUpperCase()}: ${total} points`  // crashes when displayName is undefined
}
