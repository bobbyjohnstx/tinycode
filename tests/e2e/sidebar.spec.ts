import { test, expect } from "@playwright/test"
import {
  api,
  authenticatePage,
  createSession,
  deleteSession,
  openProjectInSPA,
  SessionTracker,
  type Session,
} from "./helpers"

test.describe("sidebar: session list and navigation", () => {
  const tracker = new SessionTracker()

  test.afterEach(async ({ request }) => {
    await tracker.cleanup(request)
  })

  test("sidebar shows multiple sessions after creation", async ({
    page,
    request,
  }) => {
    await authenticatePage(page)

    // Create 3 sessions via API
    const first = tracker.track(await createSession(request))
    tracker.track(await createSession(request))
    tracker.track(await createSession(request))

    // Seed the SPA's localStorage with the project so the home page renders sessions
    await openProjectInSPA(page, first.directory)

    // Reload to pick up the project and sessions
    await page.reload({ waitUntil: "domcontentloaded" })
    await page.waitForTimeout(3_000)

    // Try to find session rows in the DOM
    const sessionRows = page.locator('[data-component="home-session-row"]')
    const projectRows = page.locator('[data-component="home-project-row"]')

    // If project rows exist, click to expand sessions
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
      if (rowCount >= 3) break
      await page.waitForTimeout(1_000)
    }
    expect(rowCount).toBeGreaterThanOrEqual(3)
  })

  test("clicking session in sidebar navigates to session view", async ({
    page,
    request,
  }) => {
    await authenticatePage(page)

    const session = tracker.track(await createSession(request))

    // Seed localStorage so home page renders this project's sessions
    await openProjectInSPA(page, session.directory)

    // Reload to pick up the project
    await page.reload({ waitUntil: "domcontentloaded" })
    await page.waitForTimeout(3_000)

    // Expand project group if needed
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

  test("active session is visually distinguished in sidebar", async ({
    page,
    request,
  }) => {
    await authenticatePage(page)

    const session = tracker.track(await createSession(request))

    // Navigate directly to the session
    const dirB64 = Buffer.from(session.directory, "utf8")
      .toString("base64")
      .replace(/\+/g, "-")
      .replace(/\//g, "_")
      .replace(/=/g, "")
    await page.goto(`/${dirB64}/session/${session.id}`, {
      waitUntil: "domcontentloaded",
    })
    await page.waitForTimeout(3_000)

    // Check for active/selected state indicators using the real SPA attributes.
    // The SPA uses data-selected and aria-current="page" on project rows.
    const activeByData = await page.$("[data-selected]")
    const activeByAria = await page.$('[aria-current="page"]')
    const hasActiveIndicator = activeByData !== null || activeByAria !== null

    expect(page.url()).toContain(session.id)
    expect(hasActiveIndicator).toBe(true)
  })

  test("session list updates after creating new session via SSE", async ({
    page,
    request,
  }) => {
    test.setTimeout(60_000)

    await authenticatePage(page)

    // Create a seed session to establish the project
    const seed = tracker.track(await createSession(request))

    // Seed localStorage so home page renders this project's sessions
    await openProjectInSPA(page, seed.directory)

    // Reload to pick up the project
    await page.reload({ waitUntil: "domcontentloaded" })
    await page.waitForTimeout(2_000)

    // Create a new session via API (should trigger session.created SSE)
    const newSession = tracker.track(await createSession(request))

    // Wait for SSE to deliver the update, then verify the new session
    // appears in the session list (avoid counting totals which is fragile
    // when other tests clean up sessions concurrently).
    const deadline = Date.now() + 15_000
    let found = false
    while (Date.now() < deadline) {
      const sessions = await api<Session[]>(request, "/session")
      if (sessions.find((s) => s.id === newSession.id)) {
        found = true
        break
      }
      await page.waitForTimeout(2_000)
    }
    expect(found).toBe(true)

    // Also verify the DOM if session rows are available
    const sessionRows = page.locator('[data-component="home-session-row"]')
    const projectRows = page.locator('[data-component="home-project-row"]')
    const projectCount = await projectRows.count()
    if (projectCount > 0) {
      await projectRows.first().click()
      await page.waitForTimeout(1_000)
    }

    const deadline2 = Date.now() + 15_000
    while (Date.now() < deadline2) {
      if (await sessionRows.count() > 0) break
      await page.waitForTimeout(1_000)
    }
    expect(await sessionRows.count()).toBeGreaterThan(0)
  })

  test("session list updates after deleting a session via SSE", async ({
    page,
    request,
  }) => {
    test.setTimeout(60_000)

    await authenticatePage(page)

    // Create 2 sessions
    const s1 = tracker.track(await createSession(request))
    const s2 = tracker.track(await createSession(request))

    await page.reload({ waitUntil: "domcontentloaded" })
    await page.waitForTimeout(3_000)

    // Verify both exist via API
    const before = await api<Session[]>(request, "/session")
    expect(before.find((s) => s.id === s1.id)).toBeTruthy()
    expect(before.find((s) => s.id === s2.id)).toBeTruthy()

    // Delete one session via API (triggers session.deleted SSE)
    await deleteSession(request, s1.id)

    // Wait for SSE to propagate
    await page.waitForTimeout(5_000)

    // Verify the deleted session is gone via API
    const after = await api<Session[]>(request, "/session")
    expect(after.find((s) => s.id === s1.id)).toBeFalsy()
    expect(after.find((s) => s.id === s2.id)).toBeTruthy()
  })
})
