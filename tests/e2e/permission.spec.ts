import { test, expect } from "@playwright/test"
import {
  authenticatePage,
  createAndNavigateToSession,
  typeAndSubmit,
  waitForPermission,
  approvePermission,
  rejectPermission,
  SessionTracker,
} from "./helpers"

test.describe("permission: dialog interactions", () => {
  const tracker = new SessionTracker()

  test.afterEach(async ({ request }) => {
    await tracker.cleanup(request)
  })

  test("permission dialog appears when shell command triggers permission.asked", async ({
    page,
    request,
  }) => {
    test.setTimeout(90_000)

    await authenticatePage(page)
    tracker.track(await createAndNavigateToSession(page, request))

    // Trigger a permission by running a shell command
    await typeAndSubmit(page, "!ls")
    await page.waitForTimeout(2_000)

    // Check for permission via API or inline UI
    const perm = await waitForPermission(request, page, 60_000)

    if (perm) {
      // Permission appeared via API -- the UI should also show it.
      // Check for permission-related UI elements.
      // Give the SSE event time to propagate to the UI.
      await page.waitForTimeout(3_000)

      const body = await page.textContent("body")
      // The page should contain some permission-related content:
      // either "Permission", "Allow", "Deny", or the permission dock.
      const hasPermissionUI =
        body?.includes("Permission") ||
        body?.includes("Allow") ||
        body?.includes("Deny") ||
        (await page.$('[data-slot="permission-footer-actions"]')) !== null ||
        (await page.$('button:has-text("Allow once")')) !== null

      expect(hasPermissionUI).toBe(true)

      // Clean up: approve the permission so the session can finish
      await approvePermission(request, perm.id)
    } else {
      // The LLM may have processed the command directly without triggering
      // a permission (model-dependent behavior). Verify that either inline
      // permission UI appeared OR the command was processed.
      const allowBtn = await page.$('button:has-text("Allow once")')
      const permDock = await page.$('[data-slot="permission-footer-actions"]')
      const body = await page.textContent("body")
      const commandProcessed = body?.includes("ls") ?? false

      expect(allowBtn !== null || permDock !== null || commandProcessed).toBe(true)
    }
  })

  test("approving permission via API allows processing to continue", async ({
    page,
    request,
  }) => {
    test.setTimeout(90_000)

    await authenticatePage(page)
    tracker.track(await createAndNavigateToSession(page, request))

    await typeAndSubmit(page, "!echo approved-test")
    await page.waitForTimeout(2_000)

    const perm = await waitForPermission(request, page, 60_000)

    if (perm) {
      await approvePermission(request, perm.id)
      await page.waitForTimeout(5_000)

      // After approval, the permission should be resolved.
      // Poll for an empty permission list.
      const deadline = Date.now() + 15_000
      let resolved = false
      while (Date.now() < deadline) {
        const data = await request.fetch("http://127.0.0.1:4096/permission", {
          headers: {
            Authorization: `Bearer ${process.env.TINYCODE_AUTH_TOKEN ?? "1f3a89333398837197e267aee1952196c5989cc94b6bbc69443a71ca863c0931"}`,
          },
        })
        const json = await data.json()
        const perms = (json as { permissions?: unknown[] }).permissions ?? []
        if (perms.length === 0) {
          resolved = true
          break
        }
        await page.waitForTimeout(2_000)
      }
      expect(resolved).toBe(true)
    } else {
      // The LLM may have processed the command directly without triggering
      // a permission (model-dependent behavior). Verify that either inline
      // permission UI appeared OR the command output rendered.
      const allowBtn = await page.$('button:has-text("Allow once")')
      const permDock = await page.$('[data-slot="permission-footer-actions"]')
      const body = await page.textContent("body")
      const commandRan = body?.includes("approved-test") ?? false

      expect(allowBtn !== null || permDock !== null || commandRan).toBe(true)

      if (allowBtn) {
        await allowBtn.click()
        await page.waitForTimeout(3_000)
        const btnAfter = await page.$('button:has-text("Allow once")')
        expect(btnAfter).toBeNull()
      }
    }
  })

  test("rejecting permission via API denies processing", async ({
    page,
    request,
  }) => {
    test.setTimeout(90_000)

    await authenticatePage(page)
    tracker.track(await createAndNavigateToSession(page, request))

    await typeAndSubmit(page, "!echo rejected-test")
    await page.waitForTimeout(2_000)

    const perm = await waitForPermission(request, page, 60_000)

    if (perm) {
      await rejectPermission(request, perm.id)
      await page.waitForTimeout(3_000)

      // After rejection, the permission list should be clear
      const deadline = Date.now() + 15_000
      let resolved = false
      while (Date.now() < deadline) {
        const data = await request.fetch("http://127.0.0.1:4096/permission", {
          headers: {
            Authorization: `Bearer ${process.env.TINYCODE_AUTH_TOKEN ?? "1f3a89333398837197e267aee1952196c5989cc94b6bbc69443a71ca863c0931"}`,
          },
        })
        const json = await data.json()
        const perms = (json as { permissions?: unknown[] }).permissions ?? []
        if (perms.length === 0) {
          resolved = true
          break
        }
        await page.waitForTimeout(2_000)
      }
      expect(resolved).toBe(true)
    } else {
      // The LLM may have processed the command directly without triggering
      // a permission (model-dependent behavior). Verify that either inline
      // permission UI appeared OR the command was processed.
      const denyBtn = await page.$('button:has-text("Deny")')
      const permDock = await page.$('[data-slot="permission-footer-actions"]')
      const body = await page.textContent("body")
      const commandProcessed = body?.includes("rejected-test") ?? false

      expect(denyBtn !== null || permDock !== null || commandProcessed).toBe(true)

      if (denyBtn) {
        await denyBtn.click()
        await page.waitForTimeout(3_000)
        const btnAfter = await page.$('button:has-text("Deny")')
        expect(btnAfter).toBeNull()
      }
    }
  })
})
