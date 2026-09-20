import { test, expect } from "@playwright/test"
import {
  api,
  authenticatePage,
  createSession,
  createAndNavigateToSession,
  deleteSession,
  findPromptInput,
  typeAndSubmit,
  SessionTracker,
  type Session,
} from "./helpers"

test.describe("session: lifecycle", () => {
  const tracker = new SessionTracker()

  test.afterEach(async ({ request }) => {
    await tracker.cleanup(request)
  })

  test("creates session via API and retrieves it", async ({ request }) => {
    const session = tracker.track(await createSession(request))

    expect(session.id).toBeTruthy()
    expect(session.directory).toBeTruthy()

    // Verify the session appears in the session list
    const sessions = await api<Session[]>(request, "/session")
    const found = sessions.find((s) => s.id === session.id)
    expect(found).toBeTruthy()
  })

  test("shows session view after navigating to session URL", async ({ page, request }) => {
    await authenticatePage(page)

    const session = tracker.track(
      await createAndNavigateToSession(page, request),
    )

    // The session page should load without auth errors
    const body = await page.textContent("body")
    expect(body).not.toContain("unauthorized")
    expect(body).not.toContain("Unauthorized")

    // URL should contain the session ID
    expect(page.url()).toContain(session.id)
  })

  test("deletes session via API and confirms removal", async ({ request }) => {
    const session = tracker.track(await createSession(request))

    // Verify it exists
    const before = await api<Session[]>(request, "/session")
    expect(before.find((s) => s.id === session.id)).toBeTruthy()

    // Delete it
    await deleteSession(request, session.id)

    // Verify it no longer appears
    const after = await api<Session[]>(request, "/session")
    expect(after.find((s) => s.id === session.id)).toBeFalsy()

    // Remove from tracker since we already deleted it
    tracker.track({ id: "__already_deleted__", directory: "" } as Session)
  })

  test("session prompt input is present in session view", async ({ page, request }) => {
    await authenticatePage(page)

    tracker.track(await createAndNavigateToSession(page, request))

    // The session view should have a prompt input
    const prompt = await findPromptInput(page)
    expect(prompt).toBeTruthy()
  })
})
