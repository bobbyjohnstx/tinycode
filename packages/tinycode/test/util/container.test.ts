import { describe, expect, test } from "bun:test"
import { rewriteLocalhostURL } from "../../src/util/container"

describe("util.container", () => {
  describe("rewriteLocalhostURL", () => {
    // These tests run on the host (not in a container), so rewriteLocalhostURL
    // should return the URL unchanged since isContainer() returns false.
    test("returns URL unchanged when not in a container", () => {
      expect(rewriteLocalhostURL("http://localhost:11434")).toBe("http://localhost:11434")
    })

    test("returns URL unchanged for non-localhost URLs when not in a container", () => {
      expect(rewriteLocalhostURL("http://example.com:8080")).toBe("http://example.com:8080")
    })

    test("returns 127.0.0.1 URL unchanged when not in a container", () => {
      expect(rewriteLocalhostURL("http://127.0.0.1:8000")).toBe("http://127.0.0.1:8000")
    })
  })
})
