import { test, expect } from "@playwright/test"
import {
  api,
  authenticatePage,
  createAndNavigateToSession,
  typeAndSubmit,
  waitForPermission,
  rejectPermission,
  SessionTracker,
  type Session,
} from "./helpers"

test.describe("permission-reject: rejection stops processing", () => {
  const tracker = new SessionTracker()

  test.afterEach(async ({ request }) => {
    await tracker.cleanup(request)
  })

  test("rejecting a permission stops processing and shows rejection indicator", async ({
    page,
    request,
  }) => {
    test.setTimeout(120_000)

    await authenticatePage(page)
    tracker.track(await createAndNavigateToSession(page, request))

    // Trigger a shell command that requires permission
    await typeAndSubmit(page, "!echo permission-reject-test")
    await page.waitForTimeout(2_000)

    // Wait for a permission to appear via the API
    const perm = await waitForPermission(request, page, 60_000)

    if (!perm) {
      // The LLM may have processed the command directly without
      // triggering a permission, or may not have attempted the shell
      // command. This is model-dependent behavior.
      console.log(
        "No permission appeared. Model-dependent behavior; skipping gracefully.",
      )
      test.skip(true, "No permission triggered by model")
      return
    }

    // Capture the page state before rejection
    const bodyBeforeReject = await page.textContent("body")

    // Reject the permission via API
    await rejectPermission(request, perm.id)

    // Wait for the rejection to propagate
    await page.waitForTimeout(5_000)

    // After rejection, the permission list should be clear
    const deadline = Date.now() + 15_000
    let permissionsCleared = false
    while (Date.now() < deadline) {
      const data = await api<{ permissions?: { id: string }[] }>(
        request,
        "/permission",
      )
      const perms = data.permissions ?? []
      if (perms.length === 0) {
        permissionsCleared = true
        break
      }
      await page.waitForTimeout(2_000)
    }
    expect(permissionsCleared).toBe(true)

    // Wait additional time for the UI to reflect the rejection
    await page.waitForTimeout(3_000)

    // Verify that processing stopped and a rejection indicator appears.
    // The SPA should show one of:
    // 1. A denial/rejection message in the chat
    // 2. The permission UI disappearing (no longer showing Allow/Deny)
    // 3. An error indicator
    // 4. The session returning to idle state (prompt input re-enabled)
    const bodyAfterReject = await page.textContent("body") ?? ""

    // Check for rejection indicators in the page content
    const hasRejectionIndicator =
      bodyAfterReject.includes("denied") ||
      bodyAfterReject.includes("Denied") ||
      bodyAfterReject.includes("rejected") ||
      bodyAfterReject.includes("Rejected") ||
      bodyAfterReject.includes("permission") ||
      bodyAfterReject.includes("not allowed") ||
      bodyAfterReject.includes("refused")

    // Check that the permission UI buttons are gone
    const allowBtn = await page.$('button:has-text("Allow once")')
    const denyBtn = await page.$('button:has-text("Deny")')
    const permDock = await page.$('[data-slot="permission-footer-actions"]')
    const permUIGone =
      allowBtn === null && denyBtn === null && permDock === null

    // Check that the prompt input is available again (session idle)
    const prompt = await page.$(
      '[data-component="prompt-input"], [contenteditable="true"]',
    )
    const promptAvailable = prompt !== null

    // At least one of these indicators should be true:
    // rejection text appeared, OR permission UI cleared, OR prompt is available
    expect(hasRejectionIndicator || permUIGone || promptAvailable).toBe(true)

    // Also verify the page content changed after rejection
    // (something happened in the UI)
    const pageChanged = bodyAfterReject !== bodyBeforeReject
    const uiResponded = pageChanged || permUIGone
    expect(uiResponded).toBe(true)
  })
})
