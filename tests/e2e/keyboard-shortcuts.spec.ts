import { test, expect } from "@playwright/test"
import {
  authenticatePage,
  createAndNavigateToSession,
  findPromptInput,
  SessionTracker,
} from "./helpers"

const MOD = process.platform === "darwin" ? "Meta" : "Control"

test.describe("keyboard-shortcuts: global and session shortcuts", () => {
  const tracker = new SessionTracker()

  test.afterEach(async ({ request }) => {
    await tracker.cleanup(request)
  })

  test("Cmd+Shift+P opens command palette", async ({ page, request }) => {
    await authenticatePage(page)
    tracker.track(await createAndNavigateToSession(page, request))

    await page.keyboard.press(`${MOD}+Shift+p`)
    await page.waitForTimeout(1_000)

    const dialog = page.locator('[data-component="dialog"]')
    await expect(dialog.first()).toBeVisible({ timeout: 5_000 })
  })

  test("command palette lists available commands", async ({
    page,
    request,
  }) => {
    await authenticatePage(page)
    tracker.track(await createAndNavigateToSession(page, request))

    await page.keyboard.press(`${MOD}+Shift+p`)
    await page.waitForTimeout(1_000)

    const dialog = page.locator('[data-component="dialog"]')
    await expect(dialog.first()).toBeVisible({ timeout: 5_000 })

    // The palette dialog body should contain command items
    const body = await dialog.first().textContent()
    expect(body).toBeTruthy()
    expect(body!.length).toBeGreaterThan(0)
  })

  test("Escape closes command palette", async ({ page, request }) => {
    await authenticatePage(page)
    tracker.track(await createAndNavigateToSession(page, request))

    await page.keyboard.press(`${MOD}+Shift+p`)
    await page.waitForTimeout(1_000)

    const dialog = page.locator('[data-component="dialog"]')
    await expect(dialog.first()).toBeVisible({ timeout: 5_000 })

    await page.keyboard.press("Escape")
    await page.waitForTimeout(500)

    // Dialog should close
    const dialogCount = await dialog.count()
    const anyVisible = dialogCount > 0 && (await dialog.first().isVisible().catch(() => false))
    expect(anyVisible).toBe(false)
  })

  test("Cmd+Shift+T cycles theme", async ({ page, request }) => {
    await authenticatePage(page)
    tracker.track(await createAndNavigateToSession(page, request))

    // Read initial color scheme
    const schemeBefore = await page.evaluate(() =>
      document.documentElement.getAttribute("data-color-scheme"),
    )

    await page.keyboard.press(`${MOD}+Shift+t`)
    await page.waitForTimeout(500)

    const schemeAfter = await page.evaluate(() =>
      document.documentElement.getAttribute("data-color-scheme"),
    )

    // Theme should have changed (light→dark or dark→light, or theme name changed)
    const themeBefore = await page.evaluate(() =>
      document.documentElement.getAttribute("data-theme"),
    )
    await page.keyboard.press(`${MOD}+Shift+t`)
    await page.waitForTimeout(500)

    const themeAfter = await page.evaluate(() =>
      document.documentElement.getAttribute("data-theme"),
    )

    // At least one of color-scheme or theme should have changed across cycles
    const changed =
      schemeBefore !== schemeAfter || themeBefore !== themeAfter
    expect(changed).toBe(true)
  })

  test("Cmd+, opens settings dialog", async ({ page, request }) => {
    await authenticatePage(page)
    tracker.track(await createAndNavigateToSession(page, request))

    await page.keyboard.press(`${MOD}+,`)
    await page.waitForTimeout(1_000)

    const dialog = page.locator('[data-component="dialog"]')
    await expect(dialog.first()).toBeVisible({ timeout: 5_000 })

    // Settings dialog should contain settings-related content
    const dialogText = await dialog.first().textContent()
    const hasSettings =
      dialogText?.includes("Settings") ||
      dialogText?.includes("General") ||
      dialogText?.includes("Theme") ||
      dialogText?.includes("Color")
    expect(hasSettings).toBe(true)
  })

  test("Cmd+' opens model picker", async ({ page, request }) => {
    await authenticatePage(page)
    tracker.track(await createAndNavigateToSession(page, request))

    await page.keyboard.press(`${MOD}+'`)
    await page.waitForTimeout(1_000)

    const dialog = page.locator('[data-component="dialog"]')
    await expect(dialog.first()).toBeVisible({ timeout: 5_000 })
  })

  test("Ctrl+L focuses prompt input", async ({ page, request }) => {
    await authenticatePage(page)
    tracker.track(await createAndNavigateToSession(page, request))

    // Click somewhere else first to unfocus the prompt
    await page.click("body")
    await page.waitForTimeout(500)

    await page.keyboard.press("Control+l")
    await page.waitForTimeout(500)

    // The prompt input should be focused
    const prompt = await findPromptInput(page)
    expect(prompt).not.toBeNull()
    const isFocused = await prompt!.evaluate(
      (el) => document.activeElement === el || el.contains(document.activeElement),
    )
    expect(isFocused).toBe(true)
  })

  test("Cmd+Shift+S creates new session", async ({ page, request }) => {
    await authenticatePage(page)
    tracker.track(await createAndNavigateToSession(page, request))

    const urlBefore = page.url()

    await page.keyboard.press(`${MOD}+Shift+s`)
    await page.waitForTimeout(2_000)

    // Should navigate to a new session URL
    expect(page.url()).not.toBe(urlBefore)
    expect(page.url()).toContain("/session")
  })

  test("/terminal slash command toggles terminal panel", async ({
    page,
    request,
  }) => {
    await authenticatePage(page)
    tracker.track(await createAndNavigateToSession(page, request))

    // The terminal panel is always in the DOM; aria-hidden="true" when closed
    const hiddenBefore = await page.evaluate(() =>
      document.getElementById("terminal-panel")?.getAttribute("aria-hidden"),
    )
    expect(hiddenBefore).toBe("true")

    // Use /terminal slash command to toggle the terminal panel
    const prompt = await findPromptInput(page)
    expect(prompt).not.toBeNull()
    await prompt!.click()
    await page.keyboard.type("/terminal", { delay: 80 })
    await page.waitForTimeout(500)

    const termItem = page.locator('[data-slash-id="terminal.toggle"]')
    await expect(termItem).toBeVisible({ timeout: 5_000 })
    await page.keyboard.press("Enter")
    await page.waitForTimeout(1_000)

    // After toggling, aria-hidden should be "false"
    const hiddenAfter = await page.evaluate(() =>
      document.getElementById("terminal-panel")?.getAttribute("aria-hidden"),
    )
    expect(hiddenAfter).toBe("false")
  })
})
