import { describe, expect, test } from "bun:test"

/**
 * Security helpers and CSP validation tests.
 * Tests CSP header presence, URL validation, and navigation blocking.
 */
describe("CSP header management", () => {
  type Headers = Record<string, string[]>

  function addRendererHeaders(url: string, headers: Headers, isRendererUrl: (url: string, html?: boolean) => boolean) {
    // Simplified version of addRendererHeaders logic
    if (isRendererUrl(url, true)) {
      headers["Content-Security-Policy"] = [
        "default-src 'self' oc://renderer; script-src 'self' oc://renderer; style-src 'self' 'unsafe-inline' oc://renderer; connect-src 'self' oc://renderer http://localhost:* http://127.0.0.1:* ws://localhost:* ws://127.0.0.1:*; img-src 'self' oc://renderer data: blob:; font-src 'self' oc://renderer data:;",
      ]
      headers["Document-Policy"] = ["include-js-call-stacks-in-crash-reports"]
    }
  }

  const isRendererUrl = (url: string, html = false): boolean => {
    if (!url || !URL.canParse(url)) return false
    const parsed = new URL(url)
    if (html && !parsed.pathname.endsWith(".html")) return false
    return parsed.protocol === "oc:" && parsed.host === "renderer"
  }

  test("CSP header is present for renderer HTML URLs", () => {
    const headers: Headers = {}
    addRendererHeaders("oc://renderer/index.html", headers, isRendererUrl)

    expect(headers["Content-Security-Policy"]).toBeDefined()
    expect(headers["Content-Security-Policy"][0]).toContain("default-src 'self' oc://renderer")
  })

  test("Document-Policy header is present for renderer HTML URLs", () => {
    const headers: Headers = {}
    addRendererHeaders("oc://renderer/index.html", headers, isRendererUrl)

    expect(headers["Document-Policy"]).toBeDefined()
    expect(headers["Document-Policy"][0]).toBe("include-js-call-stacks-in-crash-reports")
  })

  test("CSP header is NOT present for non-renderer URLs", () => {
    const headers: Headers = {}
    addRendererHeaders("https://example.com/index.html", headers, isRendererUrl)

    expect(headers["Content-Security-Policy"]).toBeUndefined()
  })

  test("CSP header is NOT present for non-HTML renderer URLs", () => {
    const headers: Headers = {}
    addRendererHeaders("oc://renderer/script.js", headers, isRendererUrl)

    expect(headers["Content-Security-Policy"]).toBeUndefined()
  })
})

describe("renderer URL validation", () => {
  const rendererProtocol = "oc"
  const rendererHost = "renderer"

  function isRendererUrl(value?: string, html = false): boolean {
    if (!value || !URL.canParse(value)) return false
    const url = new URL(value)
    if (html && !url.pathname.endsWith(".html")) return false
    if (url.protocol === `${rendererProtocol}:` && url.host === rendererHost) return true
    return false
  }

  test("valid renderer URL is accepted", () => {
    expect(isRendererUrl("oc://renderer/index.html")).toBe(true)
  })

  test("valid renderer HTML URL is accepted when html=true", () => {
    expect(isRendererUrl("oc://renderer/index.html", true)).toBe(true)
  })

  test("non-HTML renderer URL is rejected when html=true", () => {
    expect(isRendererUrl("oc://renderer/script.js", true)).toBe(false)
  })

  test("wrong protocol is rejected", () => {
    expect(isRendererUrl("https://renderer/index.html")).toBe(false)
  })

  test("wrong host is rejected", () => {
    expect(isRendererUrl("oc://wrong/index.html")).toBe(false)
  })

  test("undefined URL is rejected", () => {
    expect(isRendererUrl(undefined)).toBe(false)
  })

  test("invalid URL string is rejected", () => {
    expect(isRendererUrl("not a url")).toBe(false)
  })
})

describe("navigation blocking", () => {
  const rendererProtocol = "oc"
  const rendererHost = "renderer"

  function isRendererUrl(value?: string): boolean {
    if (!value || !URL.canParse(value)) return false
    const url = new URL(value)
    return url.protocol === `${rendererProtocol}:` && url.host === rendererHost
  }

  function shouldBlockNavigation(url: string): boolean {
    return !isRendererUrl(url)
  }

  test("navigation to renderer URLs is allowed", () => {
    expect(shouldBlockNavigation("oc://renderer/index.html")).toBe(false)
    expect(shouldBlockNavigation("oc://renderer/settings.html")).toBe(false)
  })

  test("navigation to external URLs is blocked", () => {
    expect(shouldBlockNavigation("https://evil.com")).toBe(true)
    expect(shouldBlockNavigation("http://localhost:3000")).toBe(true)
  })

  test("navigation to file URLs is blocked", () => {
    expect(shouldBlockNavigation("file:///etc/passwd")).toBe(true)
  })

  test("navigation to invalid URLs is blocked", () => {
    expect(shouldBlockNavigation("javascript:alert(1)")).toBe(true)
    expect(shouldBlockNavigation("not a url")).toBe(true)
  })
})

describe("window.open handler", () => {
  function handleWindowOpen(url: string): { action: "allow" | "deny"; openExternal: boolean } {
    const shouldOpenExternal = url.startsWith("http:") || url.startsWith("https:")
    return {
      action: "deny", // Always deny window.open
      openExternal: shouldOpenExternal,
    }
  }

  test("http URLs are denied but marked for external opening", () => {
    const result = handleWindowOpen("http://example.com")
    expect(result.action).toBe("deny")
    expect(result.openExternal).toBe(true)
  })

  test("https URLs are denied but marked for external opening", () => {
    const result = handleWindowOpen("https://example.com")
    expect(result.action).toBe("deny")
    expect(result.openExternal).toBe(true)
  })

  test("non-http(s) URLs are denied without external opening", () => {
    const result = handleWindowOpen("file:///path")
    expect(result.action).toBe("deny")
    expect(result.openExternal).toBe(false)
  })

  test("renderer URLs are denied without external opening", () => {
    const result = handleWindowOpen("oc://renderer/index.html")
    expect(result.action).toBe("deny")
    expect(result.openExternal).toBe(false)
  })
})

describe("zoom level clamping", () => {
  const maxZoomLevel = 10
  const minZoomLevel = 0.2

  function clampZoom(value: number): number {
    return Math.min(Math.max(value, minZoomLevel), maxZoomLevel)
  }

  test("values within range are unchanged", () => {
    expect(clampZoom(1)).toBe(1)
    expect(clampZoom(2)).toBe(2)
    expect(clampZoom(0.5)).toBe(0.5)
  })

  test("values below minimum are clamped to minimum", () => {
    expect(clampZoom(0.1)).toBe(minZoomLevel)
    expect(clampZoom(0)).toBe(minZoomLevel)
    expect(clampZoom(-1)).toBe(minZoomLevel)
  })

  test("values above maximum are clamped to maximum", () => {
    expect(clampZoom(11)).toBe(maxZoomLevel)
    expect(clampZoom(20)).toBe(maxZoomLevel)
    expect(clampZoom(100)).toBe(maxZoomLevel)
  })
})
