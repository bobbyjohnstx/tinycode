/**
 * Mask an API key or secret for display purposes.
 * Shows first 4 and last 4 characters with asterisks in the middle.
 * Keys shorter than 8 characters are fully masked.
 */
export function maskKey(key: string): string {
  if (key.length < 8) return "****"
  return key.slice(0, 4) + "****" + key.slice(-4)
}

export * as Mask from "./mask"
