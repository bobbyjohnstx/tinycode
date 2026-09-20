import { test, expect } from "@playwright/test"
import {
  authenticatePage,
  createSession,
  openProjectInSPA,
  SessionTracker,
} from "./helpers"

test.describe("navigation: basic layout", () => {
  const tracker = new SessionTracker()

  test.afterEach(async ({ request }) => {
    await tracker.cleanup(request)
  })

  test("loads welcome screen with app shell elements", async ({ page }) => {
    await authenticatePage(page)

    // The app shell should render with at least a sidebar and a prompt area
    // or a welcome/home view. We verify the body has meaningful content
    // (not a blank page or error).
    const body = await page.textContent("body")
    expect(body).toBeTruthy()
    expect(body!.length).toBeGreaterThan(0)

    // The page should not show an auth error
    expect(body).not.toContain("unauthorized")
    expect(body).not.toContain("Unauthorized")
  })

  test("shows session in sidebar after creation via API", async ({ page, request }) => {
    await authenticatePage(page)

    // Create two sessions via API
    const first = tracker.track(await createSession(request))
    tracker.track(await createSession(request))

    // Seed localStorage so home page renders this project's sessions
    await openProjectInSPA(page, first.directory)

    // Reload to pick up the project and sessions
    await page.reload({ waitUntil: "domcontentloaded" })
    await page.waitForTimeout(3_000)

    // The page body should contain references to the created sessions.
    // Sessions may appear as sidebar items or in a session list.
    const body = await page.textContent("body")
    expect(body).toBeTruthy()

    // At minimum, verify the page rendered something beyond a blank state.
    // The exact selectors depend on the SPA's DOM, so we check for session
    // row components or fallback to verifying at least 2 sessions exist in
    // the session list API response.
    const sessionRows = page.locator('[data-component="home-session-row"]')
    const projectRows = page.locator('[data-component="home-project-row"]')

    // If project rows exist, click the first to expand sessions
    const projectCount = await projectRows.count()
    if (projectCount > 0) {
      await projectRows.first().click()
      await page.waitForTimeout(1_000)
    }

    // Wait for session rows to appear (handles SSE propagation delay)
    const deadline = Date.now() + 15_000
    let rowCount = 0
    while (Date.now() < deadline) {
      rowCount = await sessionRows.count()
      if (rowCount >= 2) break
      await page.waitForTimeout(1_000)
    }
    expect(rowCount).toBeGreaterThanOrEqual(2)
  })

  test("navigates to session URL when session is selected", async ({ page, request }) => {
    await authenticatePage(page)

    const session = tracker.track(await createSession(request))

    // Seed localStorage so home page renders this project's sessions
    await openProjectInSPA(page, session.directory)

    // Reload to pick up the project
    await page.reload({ waitUntil: "domcontentloaded" })
    await page.waitForTimeout(3_000)

    // Try to find and click the session in the sidebar
    const projectRows = page.locator('[data-component="home-project-row"]')
    const projectCount = await projectRows.count()
    if (projectCount > 0) {
      await projectRows.first().click()
      await page.waitForTimeout(1_000)
    }

    const sessionRows = page.locator('[data-component="home-session-row"]')
    const deadline = Date.now() + 10_000
    while (Date.now() < deadline) {
      if (await sessionRows.count() > 0) break
      await page.waitForTimeout(1_000)
    }
    expect(await sessionRows.count()).toBeGreaterThan(0)
    await sessionRows.first().click()
    await page.waitForTimeout(2_000)
    expect(page.url()).toContain("/session/")
  })
})
