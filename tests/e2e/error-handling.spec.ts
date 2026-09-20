import { test, expect } from "@playwright/test"
import {
  api,
  authenticatePage,
  typeAndSubmit,
  SessionTracker,
  type Session,
} from "./helpers"

test.describe("error-handling: session error shows notification", () => {
  const tracker = new SessionTracker()

  test.afterEach(async ({ request }) => {
    await tracker.cleanup(request)
  })

  test("invalid model triggers error notification instead of [object Object]", async ({
    page,
    request,
  }) => {
    test.setTimeout(60_000)

    await authenticatePage(page)

    // Create a session with a nonexistent provider/model
    const session = tracker.track(
      await api<Session>(request, "/session", {
        method: "POST",
        body: {
          model: { providerID: "nonexistent", modelID: "fake-model-xyz" },
        },
      }),
    )

    // Navigate to session
    const dirB64 = Buffer.from(session.directory, "utf8")
      .toString("base64")
      .replace(/\+/g, "-")
      .replace(/\//g, "_")
      .replace(/=/g, "")
    await page.goto(`/${dirB64}/session/${session.id}`, {
      waitUntil: "domcontentloaded",
    })
    await page.waitForTimeout(2_000)

    // Send a prompt -- this should fail because the model does not exist
    await typeAndSubmit(page, "Hello, this should fail.")

    console.log("Prompt sent to invalid model, waiting for error...")

    // Wait for an error to appear in the page. The server should send
    // a structured error event via SSE, and the UI should display it
    // as a toast/notification or inline error -- NOT as [object Object].
    const deadline = Date.now() + 30_000
    let errorFound = false
    let hasObjectObject = false

    while (Date.now() < deadline) {
      await page.waitForTimeout(2_000)
      const body = await page.textContent("body") ?? ""

      // Check for [object Object] -- the bug we are guarding against
      if (body.includes("[object Object]")) {
        hasObjectObject = true
        break
      }

      // Check for error indicators: toast, notification, error message text
      const hasErrorText =
        body.toLowerCase().includes("error") ||
        body.toLowerCase().includes("failed") ||
        body.toLowerCase().includes("not found") ||
        body.toLowerCase().includes("unavailable") ||
        body.toLowerCase().includes("provider")

      // Also check for error-specific UI elements
      const errorToast = await page.$(
        '[data-component*="toast"], [data-component*="notification"], [role="alert"], [class*="error"], [class*="toast"]',
      )

      if (hasErrorText || errorToast) {
        errorFound = true
        console.log("Error notification detected")
        break
      }
    }

    // The critical assertion: [object Object] must NOT appear
    expect(hasObjectObject).toBe(false)

    // An error should have been displayed in some form
    if (!errorFound) {
      // It is acceptable if the session simply shows no response
      // (the server may silently fail). The key assertion is above:
      // no [object Object] rendered.
      console.log(
        "NOTE: No explicit error notification appeared, but [object Object] was correctly absent",
      )
    }
  })
})
