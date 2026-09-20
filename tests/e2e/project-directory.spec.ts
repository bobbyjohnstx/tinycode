import { test, expect } from "@playwright/test"

const AUTH_TOKEN = "1f3a89333398837197e267aee1952196c5989cc94b6bbc69443a71ca863c0931"
const BASE = "http://127.0.0.1:4096"
const COFFEE_DIR = "/Users/bjohns/projects/coffee"

function base64Encode(value: string): string {
  return Buffer.from(value).toString("base64").replace(/\+/g, "-").replace(/\//g, "_").replace(/=/g, "")
}

const COFFEE_B64 = base64Encode(COFFEE_DIR)
const AUTH_PARAM = Buffer.from(`tinycode:${AUTH_TOKEN}`).toString("base64")

test.describe("project directory", () => {
  test("direct URL navigation creates session with correct directory", async ({ page, request }) => {
    test.setTimeout(60_000)

    // Navigate directly to coffee project session page
    await page.goto(`${BASE}/${COFFEE_B64}/session?auth_token=${AUTH_PARAM}`)
    await page.waitForTimeout(5000)

    // Type a shell command to trigger session creation
    const prompt = page.locator("[contenteditable], textarea").first()
    await prompt.waitFor({ timeout: 10000 })
    await prompt.click()
    await page.keyboard.type("! pwd", { delay: 50 })
    await page.keyboard.press("Enter")
    await page.waitForTimeout(5000)

    // URL should have coffee base64
    expect(page.url()).toContain(COFFEE_B64)

    // Check the database — session should have coffee directory
    const resp = await request.get(`${BASE}/session`, {
      headers: {
        Authorization: `Bearer ${AUTH_TOKEN}`,
        "x-tinycode-directory": encodeURIComponent(COFFEE_DIR),
      },
    })
    const sessions = await resp.json()
    expect(sessions.length).toBeGreaterThan(0)
    expect(sessions[0].directory).toBe(COFFEE_DIR)
  })

  test("home page add project then navigate via URL works", async ({ page, request }) => {
    test.setTimeout(60_000)

    // Start fresh
    await page.goto(`${BASE}?auth_token=${AUTH_PARAM}`)
    await page.waitForTimeout(3000)
    await page.evaluate(() => localStorage.clear())
    await page.reload()
    await page.waitForTimeout(3000)

    // Add coffee project via dialog
    await page.getByText("Add project").first().click()
    await page.waitForTimeout(1500)
    await page.locator("input").first().fill(COFFEE_DIR)
    await page.waitForTimeout(2000)
    await page.getByText("/Users/bjohns/projects/coffee/").first().click()
    await page.waitForTimeout(2000)

    // Coffee should be in sidebar
    await expect(page.getByText("coffee")).toBeVisible()

    // Navigate directly to coffee project session page via URL
    await page.goto(`${BASE}/${COFFEE_B64}/session?auth_token=${AUTH_PARAM}`)
    await page.waitForTimeout(5000)

    // Should see empty session page for coffee project
    const url = page.url()
    expect(url).toContain(COFFEE_B64)

    // Type and submit to create session
    const prompt = page.locator("[contenteditable], textarea").first()
    await prompt.waitFor({ timeout: 10000 })
    await prompt.click()
    await page.keyboard.type("! pwd", { delay: 50 })
    await page.keyboard.press("Enter")
    await page.waitForTimeout(5000)

    // Verify shell output shows coffee directory
    const body = await page.textContent("body")
    expect(body).toContain("/Users/bjohns/projects/coffee")
  })
})
