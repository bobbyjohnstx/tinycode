import { test, expect } from "@playwright/test"
import {
  authenticatePage,
  createAndNavigateToSession,
  findPromptInput,
  SessionTracker,
} from "./helpers"

test.describe("slash-commands: command popover and execution", () => {
  const tracker = new SessionTracker()

  test.afterEach(async ({ request }) => {
    await tracker.cleanup(request)
  })

  test("typing / in prompt opens slash command popover", async ({
    page,
    request,
  }) => {
    await authenticatePage(page)
    tracker.track(await createAndNavigateToSession(page, request))

    const prompt = await findPromptInput(page)
    expect(prompt).not.toBeNull()
    await prompt!.click()
    await page.keyboard.type("/", { delay: 80 })
    await page.waitForTimeout(1_000)

    // The slash popover should appear with command buttons
    const slashItems = page.locator("[data-slash-id]")
    const deadline = Date.now() + 5_000
    let count = 0
    while (Date.now() < deadline) {
      count = await slashItems.count()
      if (count > 0) break
      await page.waitForTimeout(500)
    }
    expect(count).toBeGreaterThan(0)
  })

  test("slash popover filters commands as user types", async ({
    page,
    request,
  }) => {
    await authenticatePage(page)
    tracker.track(await createAndNavigateToSession(page, request))

    const prompt = await findPromptInput(page)
    expect(prompt).not.toBeNull()
    await prompt!.click()
    await page.keyboard.type("/mod", { delay: 80 })
    await page.waitForTimeout(1_000)

    // Should filter to show /model command
    const modelItem = page.locator('[data-slash-id="model.choose"]')
    await expect(modelItem).toBeVisible({ timeout: 5_000 })
  })

  test("/model opens model picker dialog", async ({ page, request }) => {
    await authenticatePage(page)
    tracker.track(await createAndNavigateToSession(page, request))

    const prompt = await findPromptInput(page)
    expect(prompt).not.toBeNull()
    await prompt!.click()
    await page.keyboard.type("/model", { delay: 80 })
    await page.waitForTimeout(500)

    // Wait for the slash popover to show the model command
    const modelItem = page.locator('[data-slash-id="model.choose"]')
    await expect(modelItem).toBeVisible({ timeout: 5_000 })

    // Select via Enter (popover auto-highlights the first match)
    await page.keyboard.press("Enter")
    await page.waitForTimeout(1_000)

    // A dialog should open for model selection
    const dialog = page.locator('[data-component="dialog"]')
    await expect(dialog.first()).toBeVisible({ timeout: 5_000 })
  })

  test("/new creates a new session", async ({ page, request }) => {
    await authenticatePage(page)
    tracker.track(await createAndNavigateToSession(page, request))

    const urlBefore = page.url()

    const prompt = await findPromptInput(page)
    expect(prompt).not.toBeNull()
    await prompt!.click()
    await page.keyboard.type("/new", { delay: 80 })
    await page.waitForTimeout(500)

    const newItem = page.locator('[data-slash-id="session.new"]')
    await expect(newItem).toBeVisible({ timeout: 5_000 })

    await page.keyboard.press("Enter")
    await page.waitForTimeout(2_000)

    // URL should change to a new session
    expect(page.url()).not.toBe(urlBefore)
    expect(page.url()).toContain("/session")
  })

  test("/mcp opens MCP picker dialog", async ({ page, request }) => {
    await authenticatePage(page)
    tracker.track(await createAndNavigateToSession(page, request))

    const prompt = await findPromptInput(page)
    expect(prompt).not.toBeNull()
    await prompt!.click()
    await page.keyboard.type("/mcp", { delay: 80 })
    await page.waitForTimeout(500)

    const mcpItem = page.locator('[data-slash-id="mcp.toggle"]')
    await expect(mcpItem).toBeVisible({ timeout: 5_000 })

    await page.keyboard.press("Enter")
    await page.waitForTimeout(1_000)

    const dialog = page.locator('[data-component="dialog"]')
    await expect(dialog.first()).toBeVisible({ timeout: 5_000 })
  })

  test("/open opens file palette dialog", async ({ page, request }) => {
    await authenticatePage(page)
    tracker.track(await createAndNavigateToSession(page, request))

    const prompt = await findPromptInput(page)
    expect(prompt).not.toBeNull()
    await prompt!.click()
    await page.keyboard.type("/open", { delay: 80 })
    await page.waitForTimeout(500)

    const openItem = page.locator('[data-slash-id="file.open"]')
    await expect(openItem).toBeVisible({ timeout: 5_000 })

    await page.keyboard.press("Enter")
    await page.waitForTimeout(1_000)

    const dialog = page.locator('[data-component="dialog"]')
    await expect(dialog.first()).toBeVisible({ timeout: 5_000 })
  })

  test("escape dismisses slash popover", async ({ page, request }) => {
    await authenticatePage(page)
    tracker.track(await createAndNavigateToSession(page, request))

    const prompt = await findPromptInput(page)
    expect(prompt).not.toBeNull()
    await prompt!.click()
    await page.keyboard.type("/", { delay: 80 })
    await page.waitForTimeout(1_000)

    // Verify popover is open
    const slashItems = page.locator("[data-slash-id]")
    await expect(slashItems.first()).toBeVisible({ timeout: 5_000 })

    // Press escape to dismiss
    await page.keyboard.press("Escape")
    await page.waitForTimeout(500)

    // Popover should be gone
    const countAfter = await slashItems.count()
    expect(countAfter).toBe(0)
  })
})
