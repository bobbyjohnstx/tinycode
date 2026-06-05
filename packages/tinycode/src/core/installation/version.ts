declare global {
  const TINYCODE_VERSION: string
  const TINYCODE_CHANNEL: string
}

export const InstallationVersion = typeof TINYCODE_VERSION === "string" ? TINYCODE_VERSION : "local"
export const InstallationChannel = typeof TINYCODE_CHANNEL === "string" ? TINYCODE_CHANNEL : "local"
export const InstallationLocal = InstallationChannel === "local"
