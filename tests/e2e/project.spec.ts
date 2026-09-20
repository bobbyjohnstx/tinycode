import { test, expect } from "@playwright/test"
import {
  api,
  authenticatePage,
  createSession,
  openProjectInSPA,
  SessionTracker,
  type Session,
} from "./helpers"

test.describe("project: directory management", () => {
  const tracker = new SessionTracker()

  test.afterEach(async ({ request }) => {
    await tracker.cleanup(request)
  })

  test("current project directory is visible in the UI", async ({
    page,
    request,
  }) => {
    await authenticatePage(page)

    // Verify the project API returns the expected directory
    const project = await api<{ id: string; worktree: string }>(
      request,
      "/project/current",
    )
    expect(project.worktree).toBe("/Users/bjohns/projects/tinycode-go")

    // Create a session and seed localStorage with the project
    const session = tracker.track(await createSession(request))
    await openProjectInSPA(page, session.directory)

    // Reload so the home page picks up the project
    await page.reload({ waitUntil: "domcontentloaded" })
    await page.waitForTimeout(3_000)

    // Check if the project name or directory appears anywhere in the page
    const body = await page.textContent("body")
    expect(body).toBeTruthy()

    const hasProjectRef =
      body!.includes("tinycode-go") ||
      body!.includes("/Users/bjohns/projects/tinycode-go")
    expect(hasProjectRef).toBe(true)
  })

  test("session created with specific directory is associated correctly", async ({
    request,
  }) => {
    // Create a session with a specific directory via query param
    const session = tracker.track(
      await api<Session>(request, "/session?directory=/tmp/e2e-project-test", {
        method: "POST",
        body: {},
      }),
    )

    expect(session.id).toBeTruthy()
    expect(session.directory).toBe("/tmp/e2e-project-test")

    // Verify via GET that the directory is persisted
    const fetched = await api<Session>(request, `/session/${session.id}`)
    expect(fetched.directory).toBe("/tmp/e2e-project-test")
  })

  test("sessions are filtered by project directory", async ({ request }) => {
    // Create sessions in two different directories
    const sA1 = tracker.track(
      await api<Session>(
        request,
        "/session?directory=/tmp/e2e-project-a",
        { method: "POST", body: {} },
      ),
    )
    const sA2 = tracker.track(
      await api<Session>(
        request,
        "/session?directory=/tmp/e2e-project-a",
        { method: "POST", body: {} },
      ),
    )
    const sB1 = tracker.track(
      await api<Session>(
        request,
        "/session?directory=/tmp/e2e-project-b",
        { method: "POST", body: {} },
      ),
    )
    const sB2 = tracker.track(
      await api<Session>(
        request,
        "/session?directory=/tmp/e2e-project-b",
        { method: "POST", body: {} },
      ),
    )

    // List sessions filtered to project A
    const sessionsA = await api<Session[]>(
      request,
      "/session?directory=/tmp/e2e-project-a",
    )
    const idsA = sessionsA.map((s) => s.id)
    expect(idsA).toContain(sA1.id)
    expect(idsA).toContain(sA2.id)
    expect(idsA).not.toContain(sB1.id)
    expect(idsA).not.toContain(sB2.id)

    // List sessions filtered to project B
    const sessionsB = await api<Session[]>(
      request,
      "/session?directory=/tmp/e2e-project-b",
    )
    const idsB = sessionsB.map((s) => s.id)
    expect(idsB).toContain(sB1.id)
    expect(idsB).toContain(sB2.id)
    expect(idsB).not.toContain(sA1.id)
    expect(idsB).not.toContain(sA2.id)
  })
})
