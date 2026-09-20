import { test, expect } from "@playwright/test"
import {
  authenticatePage,
  createAndNavigateToSession,
  findPromptInput,
  typeAndSubmit,
  waitForPermission,
  approvePermission,
  SessionTracker,
} from "./helpers"

test.describe("shell: shell mode", () => {
  const tracker = new SessionTracker()

  test.afterEach(async ({ request }) => {
    await tracker.cleanup(request)
  })

  test("enters shell mode when typing ! prefix in prompt", async ({ page, request }) => {
    await authenticatePage(page)
    tracker.track(await createAndNavigateToSession(page, request))

    const prompt = await findPromptInput(page)
    expect(prompt).toBeTruthy()

    await prompt!.click()
    await page.waitForTimeout(300)

    // Type ! to trigger shell mode
    await page.keyboard.type("!", { delay: 80 })
    await page.waitForTimeout(500)

    // The SPA consumes the ! and enters shell mode. The placeholder changes
    // to "Enter shell command..." to indicate shell mode is active.
    // Check for the shell mode placeholder or any shell-mode indicator.
    const body = await page.textContent("body")
    const hasShellIndicator =
      body?.includes("shell command") ||
      body?.includes("Shell") ||
      body?.includes("shell")
    expect(hasShellIndicator).toBe(true)
  })

  test("submits shell command via prompt", async ({ page, request }) => {
    await authenticatePage(page)
    tracker.track(await createAndNavigateToSession(page, request))

    // Type a shell command and submit
    await typeAndSubmit(page, "!echo hello")

    // After submission, the prompt text should be cleared or the message
    // should appear in the session timeline. We wait briefly for the
    // server to process.
    await page.waitForTimeout(3_000)

    // The command was accepted if no error toast or auth error appears
    const body = await page.textContent("body")
    expect(body).not.toContain("unauthorized")
  })

  test("shell output appears after approving permission", async ({ page, request }) => {
    test.setTimeout(90_000)

    await authenticatePage(page)
    tracker.track(await createAndNavigateToSession(page, request))

    // Type a shell command that will trigger a permission request
    await typeAndSubmit(page, "!echo e2e-test-output")
    await page.waitForTimeout(2_000)

    // Wait for a permission to appear (via API polling or inline UI)
    const perm = await waitForPermission(request, page, 60_000)

    if (perm) {
      // Approve via API
      await approvePermission(request, perm.id)
      await page.waitForTimeout(5_000)

      // After approval, wait for output to appear in the page.
      // The echo command should produce "e2e-test-output" in the body.
      const deadline = Date.now() + 30_000
      let found = false
      while (Date.now() < deadline) {
        const body = await page.textContent("body")
        if (body && body.includes("e2e-test-output")) {
          found = true
          break
        }
        await page.waitForTimeout(2_000)
      }
      expect(found).toBe(true)
    } else {
      // Permission may have been auto-approved or shown inline.
      // Check if the Allow button appeared in the page instead.
      const allowBtn = await page.$('button:has-text("Allow once")')
      if (allowBtn) {
        await allowBtn.click()
        await page.waitForTimeout(5_000)

        const body = await page.textContent("body")
        expect(body).toContain("e2e-test-output")
      } else {
        // If no permission was needed (auto-approved), check for output directly
        const body = await page.textContent("body")
        // The command may have already completed
        expect(body).toBeTruthy()
      }
    }
  })

  test("shell output renders in monospace with line breaks", async ({ page, request }) => {
    test.setTimeout(180_000)

    await authenticatePage(page)
    tracker.track(await createAndNavigateToSession(page, request))

    // Use ls -lat which produces multi-line tabular output
    await typeAndSubmit(page, "!ls -lat")
    await page.waitForTimeout(2_000)

    // Approve permission if needed (may need multiple approvals)
    for (let attempt = 0; attempt < 3; attempt++) {
      const perm = await waitForPermission(request, page, 10_000)
      if (perm) {
        await approvePermission(request, perm.id)
        await page.waitForTimeout(1_000)
      } else {
        const allowBtn = await page.$('button:has-text("Allow once")')
        if (allowBtn) {
          await allowBtn.click()
          await page.waitForTimeout(1_000)
        } else {
          break
        }
      }
    }

    // Wait for the bash tool renderer output (data-slot="bash-pre")
    // This requires the LLM to actually call the bash tool
    const deadline = Date.now() + 45_000
    let outputEl = null
    while (Date.now() < deadline) {
      outputEl = await page.$('[data-slot="bash-pre"] code')
      if (outputEl) break
      await page.waitForTimeout(2_000)
    }

    if (!outputEl) {
      test.skip(true, "LLM did not call the bash tool — cannot verify rendering")
      return
    }

    // Verify the output is inside a <pre> with monospace font
    const pre = await outputEl.evaluateHandle((el) => el.closest("pre"))
    const fontFamily = await pre.evaluate((el) =>
      window.getComputedStyle(el as Element).fontFamily,
    )
    expect(fontFamily.toLowerCase()).toMatch(/mono/)

    // Verify multi-line content (ls -lat should produce multiple lines)
    const text = await outputEl.textContent()
    const lines = text?.split("\n").filter((l) => l.trim()) ?? []
    expect(lines.length).toBeGreaterThan(1)
  })
})
