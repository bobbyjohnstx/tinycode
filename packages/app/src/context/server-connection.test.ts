import { describe, expect, test } from "bun:test"
import { normalizeServerUrl, serverName, ServerConnection } from "./server"

describe("normalizeServerUrl", () => {
  test("returns undefined for empty input", () => {
    expect(normalizeServerUrl("")).toBeUndefined()
    expect(normalizeServerUrl("   ")).toBeUndefined()
  })

  test("adds http:// when no protocol is specified", () => {
    expect(normalizeServerUrl("example.com")).toBe("http://example.com")
    expect(normalizeServerUrl("192.168.1.1:3000")).toBe("http://192.168.1.1:3000")
  })

  test("preserves https:// when already present", () => {
    expect(normalizeServerUrl("https://example.com")).toBe("https://example.com")
  })

  test("preserves http:// when already present", () => {
    expect(normalizeServerUrl("http://example.com")).toBe("http://example.com")
  })

  test("strips trailing slashes", () => {
    expect(normalizeServerUrl("https://example.com/")).toBe("https://example.com")
    expect(normalizeServerUrl("https://example.com///")).toBe("https://example.com")
  })

  test("trims whitespace from input", () => {
    expect(normalizeServerUrl("  https://example.com  ")).toBe("https://example.com")
  })
})

describe("serverName", () => {
  test("returns URL host without protocol for http connection", () => {
    const conn: ServerConnection.Http = {
      type: "http",
      http: { url: "https://my-server.example.com" },
    }
    expect(serverName(conn)).toBe("my-server.example.com")
  })

  test("returns display name when set", () => {
    const conn: ServerConnection.Http = {
      type: "http",
      displayName: "Production",
      http: { url: "https://my-server.example.com" },
    }
    expect(serverName(conn)).toBe("Production")
  })

  test("ignores display name when flag is true", () => {
    const conn: ServerConnection.Http = {
      type: "http",
      displayName: "Production",
      http: { url: "https://my-server.example.com" },
    }
    expect(serverName(conn, true)).toBe("my-server.example.com")
  })

  test("returns empty string for undefined connection", () => {
    expect(serverName(undefined)).toBe("")
  })

  test("strips trailing slashes from URL host", () => {
    const conn: ServerConnection.Http = {
      type: "http",
      http: { url: "https://my-server.example.com/" },
    }
    expect(serverName(conn)).toBe("my-server.example.com")
  })
})

describe("ServerConnection.key", () => {
  test("returns URL for http connections", () => {
    const conn: ServerConnection.Http = {
      type: "http",
      http: { url: "https://example.com" },
    }
    expect(ServerConnection.key(conn) as string).toBe("https://example.com")
  })

  test("returns 'sidecar' for base sidecar connections", () => {
    const conn: ServerConnection.Sidecar = {
      type: "sidecar",
      variant: "base",
      http: { url: "http://localhost:3000" },
    }
    expect(ServerConnection.key(conn) as string).toBe("sidecar")
  })

  test("returns wsl:distro for WSL sidecar connections", () => {
    const conn: ServerConnection.Sidecar = {
      type: "sidecar",
      variant: "wsl",
      distro: "Ubuntu",
      http: { url: "http://localhost:3000" },
    }
    expect(ServerConnection.key(conn) as string).toBe("wsl:Ubuntu")
  })

  test("returns ssh:host for SSH connections", () => {
    const conn: ServerConnection.Ssh = {
      type: "ssh",
      host: "dev-machine",
      http: { url: "http://localhost:3001" },
    }
    expect(ServerConnection.key(conn) as string).toBe("ssh:dev-machine")
  })
})
