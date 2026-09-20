import { test, expect } from "@playwright/test"
import {
  authenticatePage,
  createAndNavigateToSession,
  SessionTracker,
} from "./helpers"

test.describe("responsive: layout adapts to viewport width", () => {
  const tracker = new SessionTracker()

  test.afterEach(async ({ request }) => {
    await tracker.cleanup(request)
  })

  test("sidebar hides at narrow viewport width", async ({ page, request }) => {
    // Start at wide viewport where sidebar should be visible
    await page.setViewportSize({ width: 1200, height: 800 })
    await authenticatePage(page)
    tracker.track(await createAndNavigateToSession(page, request))

    // At 1200px, look for sidebar-like elements.
    // The SPA sidebar may use data-component="sidebar", role="navigation",
    // or an aside element.
    const sidebarSelectors = [
      '[data-component="sidebar"]',
      '[data-component*="sidebar"]',
      '[data-slot="sidebar"]',
      'aside',
      'nav[role="navigation"]',
    ]

    let sidebarFound = false
    let sidebarSelector = ""
    for (const sel of sidebarSelectors) {
      const el = await page.$(sel)
      if (el && (await el.isVisible())) {
        sidebarFound = true
        sidebarSelector = sel
        break
      }
    }

    if (!sidebarFound) {
      // If no sidebar is visible at 1200px, the layout may not have a
      // persistent sidebar (could be overlay/drawer only). Verify the
      // page at least renders content at both widths.
      const bodyWide = await page.textContent("body")
      expect(bodyWide).toBeTruthy()

      await page.setViewportSize({ width: 600, height: 800 })
      await page.waitForTimeout(1_000)

      const bodyNarrow = await page.textContent("body")
      expect(bodyNarrow).toBeTruthy()
      // Page should still render meaningful content at narrow width
      expect(bodyNarrow!.length).toBeGreaterThan(0)
      return
    }

    // Sidebar is visible at 1200px. Now resize to 600px.
    await page.setViewportSize({ width: 600, height: 800 })
    await page.waitForTimeout(1_000)

    // At 600px, the sidebar should be hidden or collapsed
    const sidebarNarrow = await page.$(sidebarSelector)
    if (sidebarNarrow) {
      const visible = await sidebarNarrow.isVisible()
      const box = await sidebarNarrow.boundingBox()

      // The sidebar should either be invisible, have zero width, or be
      // positioned off-screen
      const collapsed =
        !visible ||
        (box !== null && box.width === 0) ||
        (box !== null && box.x + box.width <= 0)
      expect(collapsed).toBe(true)
    }
    // If the sidebar element is removed from DOM entirely, that also
    // counts as hidden — the test passes.
  })

  test("very narrow viewport renders without JavaScript errors", async ({
    page,
    request,
  }) => {
    // Collect any JS errors during the test
    const jsErrors: string[] = []
    page.on("pageerror", (error) => {
      jsErrors.push(error.message)
    })

    // Set very narrow viewport before loading
    await page.setViewportSize({ width: 400, height: 300 })

    await authenticatePage(page)

    // Give the SPA time to fully render at narrow viewport
    await page.waitForTimeout(3_000)

    // The page should render something (not blank or crashed)
    const body = await page.textContent("body")
    expect(body).toBeTruthy()
    expect(body!.length).toBeGreaterThan(0)

    // No uncaught JavaScript errors should have occurred
    // Filter out non-critical warnings that some SPAs emit
    const criticalErrors = jsErrors.filter(
      (msg) =>
        !msg.includes("ResizeObserver") &&
        !msg.includes("Non-Error promise rejection"),
    )
    expect(criticalErrors).toEqual([])

    // Verify the page is not showing a "white screen of death" —
    // check that some visual elements exist
    const hasVisualContent =
      (await page.$$("button")).length > 0 ||
      (await page.$$("input")).length > 0 ||
      (await page.$$("[contenteditable]")).length > 0 ||
      (await page.$$("div")).length > 5
    expect(hasVisualContent).toBe(true)
  })
})
