import { test, expect } from "@playwright/test"
import { authenticatePage, createAndNavigateToSession } from "./helpers"

const TOKEN =
  process.env.TINYCODE_AUTH_TOKEN ?? "1f3a89333398837197e267aee1952196c5989cc94b6bbc69443a71ca863c0931"

test.describe("provider discovery", () => {
  test("server discovers LM Studio and returns it in provider list", async ({ request }) => {
    const resp = await request.get("http://127.0.0.1:4096/provider", {
      headers: { Authorization: `Bearer ${TOKEN}` },
    })
    expect(resp.ok()).toBeTruthy()
    const data = await resp.json()

    const connected: string[] = data.connected
    expect(connected).toContain("lm-studio")

    const providers = data.all
    const lmStudio = providers.find((p: any) => p.id === "lm-studio")
    expect(lmStudio).toBeTruthy()
    expect(Object.keys(lmStudio.models).length).toBeGreaterThan(0)

    // ornith model should be among the discovered models
    const hasOrnith = "ornith-1.0-9b-mlx" in lmStudio.models
    expect(hasOrnith).toBe(true)
  })

  test("server returns correct default model from discovered providers", async ({ request }) => {
    const resp = await request.get("http://127.0.0.1:4096/provider", {
      headers: { Authorization: `Bearer ${TOKEN}` },
    })
    const data = await resp.json()

    // Default should include lm-studio with a model
    const defaults: Record<string, string> = data.default
    expect(defaults["lm-studio"]).toBeTruthy()
  })

  test("config endpoint resolves unavailable model to discovered default", async ({ request }) => {
    const resp = await request.get("http://127.0.0.1:4096/global/config", {
      headers: { Authorization: `Bearer ${TOKEN}` },
    })
    const config = await resp.json()

    // Config model should be resolved to an available model, not ollama
    expect(config.model).toBeTruthy()
    expect(config.model).not.toContain("ollama")
    expect(config.model).toContain("lm-studio")
  })

  test("session page displays discovered model in prompt bar", async ({ page, request }) => {
    test.setTimeout(30_000)
    await authenticatePage(page)
    const session = await createAndNavigateToSession(page, request)

    // Wait for model to appear in prompt bar
    const deadline = Date.now() + 15_000
    let found = false
    while (Date.now() < deadline) {
      const body = await page.textContent("body")
      if (body?.includes("LM Studio") || body?.includes("ornith")) {
        found = true
        break
      }
      await page.waitForTimeout(1000)
    }
    expect(found).toBe(true)

    // Clean up
    await request.delete(`http://127.0.0.1:4096/session/${session.id}`, {
      headers: { Authorization: `Bearer ${TOKEN}` },
    })
  })
})
