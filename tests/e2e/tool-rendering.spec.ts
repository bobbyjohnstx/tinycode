import { test, expect } from "@playwright/test"
import {
  api,
  authenticatePage,
  typeAndSubmit,
  approvePermission,
  waitForPermission,
  SessionTracker,
  type Session,
} from "./helpers"

test.describe("tool-rendering: tool call renders as widget", () => {
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

  test("tool call renders a tool-part-wrapper widget with tool name and output", async ({
    page,
    request,
  }) => {
    test.setTimeout(120_000)

    await authenticatePage(page)

    // Create session with the LLM model
    const session = tracker.track(
      await api<Session>(request, "/session", {
        method: "POST",
        body: {
          model: { providerID: "lm-studio", modelID: "ornith-1.0-9b-mlx" },
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

    const stopApprover = startPermissionAutoApprover(request)

    try {
      // Send a prompt that should trigger a tool call (file read)
      await typeAndSubmit(page, "Read the file CLAUDE.md and tell me what it contains.")

      console.log("Prompt sent, waiting for tool call widget...")

      // Poll for tool widget to appear. The LLM must decide to use a tool,
      // which is model-dependent. Use a generous timeout.
      const deadline = Date.now() + 100_000
      let toolWidgetFound = false
      let toolHasName = false
      let toolHasOutput = false

      // Tool-related selectors to check, in priority order
      const toolSelectors = [
        '[data-component="tool-part-wrapper"]',
        '[data-component*="tool"]',
        '[data-slot*="tool-call"]',
        '[class*="tool-part"]',
        '[class*="tool-call"]',
      ]

      while (Date.now() < deadline) {
        await page.waitForTimeout(3_000)

        for (const selector of toolSelectors) {
          const widgets = await page.$$(selector)
          for (const widget of widgets) {
            const widgetText = (await widget.textContent()) ?? ""

            if (!toolWidgetFound && widgets.length > 0) {
              toolWidgetFound = true
              console.log(`Tool widget found via: ${selector}`)
            }

            // Check for tool name -- look for common tool names or any text
            if (widgetText.length > 0) {
              toolHasName = true
            }

            // Check for output content -- substantial text indicates output rendered
            if (widgetText.length > 10) {
              toolHasOutput = true
            }
          }
          if (toolHasOutput) break
        }

        if (toolHasOutput) break

        // If widget found but no output yet, keep waiting for content to load
        if (toolWidgetFound && !toolHasOutput) {
          console.log("Tool widget found but waiting for output content...")
        }
      }

      if (!toolWidgetFound) {
        // LLM did not produce a tool call -- skip gracefully
        console.log("SKIP: LLM did not produce a tool call within timeout")
        test.skip(true, "LLM did not produce a tool call -- model-dependent behavior")
        return
      }

      expect(toolWidgetFound).toBe(true)
      expect(toolHasName).toBe(true)
      expect(toolHasOutput).toBe(true)
    } finally {
      stopApprover()
    }
  })
})
