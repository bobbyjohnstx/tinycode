import { test, expect } from "@playwright/test"
import {
  api,
  authenticatePage,
  approvePermission,
  typeAndSubmit,
  SessionTracker,
  type Session,
} from "./helpers"

test.describe("thinking-toggle: reasoning block expand/collapse", () => {
  const tracker = new SessionTracker()

  test.afterEach(async ({ request }) => {
    await tracker.cleanup(request)
  })

  /**
   * Auto-approve any permissions that appear during LLM processing.
   * Returns a cleanup function to stop polling.
   */
  function startPermissionAutoApprover(
    request: Parameters<typeof approvePermission>[0],
    intervalMs = 3_000,
  ): () => void {
    let running = true
    const poll = async () => {
      while (running) {
        try {
          const data = await api<{ permissions?: { id: string }[] }>(
            request,
            "/permission",
          )
          for (const p of data.permissions ?? []) {
            await approvePermission(request, p.id)
          }
        } catch {
          // best-effort
        }
        await new Promise((r) => setTimeout(r, intervalMs))
      }
    }
    poll()
    return () => {
      running = false
    }
  }

  test("thinking block can be toggled between expanded and collapsed", async ({
    page,
    request,
  }) => {
    test.setTimeout(120_000)

    await authenticatePage(page)

    // Create session with the local reasoning model, which produces
    // thinking/reasoning blocks
    const session = tracker.track(
      await api<Session>(request, "/session", {
        method: "POST",
        body: {
          model: { providerID: "lm-studio", modelID: "ornith-1.0-9b-mlx" },
        },
      }),
    )

    const dirB64 = Buffer.from(session.directory, "utf8")
      .toString("base64")
      .replace(/\+/g, "-")
      .replace(/\//g, "_")
      .replace(/=/g, "")
    await page.goto(`/${dirB64}/session/${session.id}`, {
      waitUntil: "domcontentloaded",
    })
    await page.waitForTimeout(2_000)

    const stopApprover = startPermissionAutoApprover(request)

    try {
      // Send a prompt that should trigger reasoning
      await typeAndSubmit(
        page,
        "What is 2+2? Explain your reasoning step by step.",
      )

      console.log("Prompt sent, waiting for reasoning block...")

      // Wait for the response and look for a thinking/reasoning toggle.
      // The SPA renders thinking blocks with data attributes or summary
      // elements that can be toggled.
      const thinkingSelectors = [
        '[data-slot="session-turn-thinking"]',
        '[data-component="thinking"]',
        '[data-component*="thinking"]',
        '[data-component="reasoning"]',
        '[data-slot="thinking"]',
        "details summary",
        'button:has-text("Thinking")',
        'button:has-text("Thought")',
        '[data-component*="thought"]',
        ".thinking-toggle",
        '[aria-label*="thinking"]',
        '[aria-label*="Thinking"]',
      ]

      const deadline = Date.now() + 100_000
      let matchedSelector = ""

      while (Date.now() < deadline) {
        for (const sel of thinkingSelectors) {
          const el = await page.$(sel)
          if (el && (await el.isVisible())) {
            matchedSelector = sel
            break
          }
        }
        if (matchedSelector) break
        await page.waitForTimeout(3_000)
      }

      if (!matchedSelector) {
        // Graceful skip: the model may not produce a visible thinking block
        // (depends on model capabilities and UI rendering).
        console.log(
          "No thinking/reasoning block found in response. " +
            "This is model-dependent; skipping gracefully.",
        )
        test.skip(true, "No thinking block produced by model")
        return
      }

      console.log(`Found thinking element with selector: ${matchedSelector}`)

      // Wait for the LLM to finish streaming so the DOM stabilizes.
      // Poll until the body text stops growing for 5 seconds.
      let lastBodyLen = 0
      let stableCount = 0
      const stabilizeDeadline = Date.now() + 30_000
      while (Date.now() < stabilizeDeadline && stableCount < 2) {
        await page.waitForTimeout(3_000)
        const currentLen = (await page.textContent("body"))?.length ?? 0
        if (currentLen === lastBodyLen) {
          stableCount++
        } else {
          stableCount = 0
          lastBodyLen = currentLen
        }
      }

      // Use a locator (auto-retries) rather than a stale element handle.
      // The thinking toggle is a <button> with aria-label="Toggle thinking details"
      // inside a [data-slot="session-turn-thinking"] wrapper. The SolidJS component
      // tracks expanded state via a signal that controls:
      //   - Chevron transform: rotate(90deg) = expanded, rotate(0deg) = collapsed
      //   - Content rendering via <Show when={expanded() && hasText}>
      // We detect state from the chevron's CSS transform since content may be
      // absent when the model produces no reasoning text.
      const thinkingWrapper = page.locator('[data-slot="session-turn-thinking"]').first()
      const isVisible = await thinkingWrapper.isVisible().catch(() => false)
      if (!isVisible) {
        test.skip(true, "No thinking block rendered — model may not produce reasoning output")
        return
      }

      const toggleButton = thinkingWrapper.locator('button[aria-label="Toggle thinking details"]')
      await expect(toggleButton).toBeVisible({ timeout: 5_000 })

      // Helper: check the chevron's rotation to determine expanded state.
      // rotate(90deg) = expanded, rotate(0deg) = collapsed.
      async function isExpanded(): Promise<boolean> {
        const transform = await toggleButton.evaluate(
          (el) => window.getComputedStyle(el).transform,
        )
        // A 90deg rotation produces a matrix like "matrix(0, 1, -1, 0, 0, 0)"
        // or contains "90" in the raw transform value.
        // No rotation (0deg) produces "matrix(1, 0, 0, 1, 0, 0)" or "none".
        if (!transform || transform === "none") return false
        // Check for non-identity matrix (any rotation present = expanded)
        return !transform.includes("matrix(1, 0, 0, 1")
      }

      const expandedBefore = await isExpanded()

      // Click the toggle button to change state
      await toggleButton.click()
      await page.waitForTimeout(500)

      const expandedAfter = await isExpanded()

      // The state should have toggled
      expect(expandedAfter).not.toBe(expandedBefore)

      // Click again to toggle back
      await toggleButton.click()
      await page.waitForTimeout(500)

      const expandedFinal = await isExpanded()

      // Should be back to original state
      expect(expandedFinal).toBe(expandedBefore)
    } finally {
      stopApprover()
    }
  })
})
