import { existsSync } from "fs"

let _cached: { isContainer: boolean; runtime: "docker" | "podman" | "none" } | undefined

function detect(): { isContainer: boolean; runtime: "docker" | "podman" | "none" } {
  if (_cached) return _cached

  // Podman creates /run/.containerenv
  if (existsSync("/run/.containerenv")) {
    _cached = { isContainer: true, runtime: "podman" }
    return _cached
  }

  // Docker creates /.dockerenv
  if (existsSync("/.dockerenv")) {
    _cached = { isContainer: true, runtime: "docker" }
    return _cached
  }

  _cached = { isContainer: false, runtime: "none" }
  return _cached
}

/**
 * Returns true if running inside a container (Docker or Podman).
 * Result is cached after the first call.
 */
export function isContainer(): boolean {
  return detect().isContainer
}

/**
 * Returns the appropriate hostname to reach the host from inside a container.
 * - Docker: host.docker.internal
 * - Podman: host.containers.internal
 * - Not in container: localhost
 */
export function containerHostname(): string {
  const { runtime } = detect()
  switch (runtime) {
    case "docker":
      return "host.docker.internal"
    case "podman":
      return "host.containers.internal"
    default:
      return "localhost"
  }
}

/**
 * Rewrites a localhost URL to use the container-appropriate hostname.
 * Only rewrites if running inside a container and the URL targets localhost.
 */
export function rewriteLocalhostURL(url: string): string {
  if (!isContainer()) return url
  const hostname = containerHostname()
  return url
    .replace(/\/\/localhost([:/])/g, `//${hostname}$1`)
    .replace(/\/\/127\.0\.0\.1([:/])/g, `//${hostname}$1`)
}

export * as Container from "./container"
