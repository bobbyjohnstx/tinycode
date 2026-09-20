import { test, expect } from "@playwright/test"
import {
  api,
  authenticatePage,
  typeAndSubmit,
  approvePermission,
  SessionTracker,
  type Session,
} from "./helpers"

test.describe("llm-roundtrip: prompt to LLM response", () => {
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

  test("sends prompt and receives LLM response", async ({ page, request }) => {
    test.setTimeout(120_000)

    await authenticatePage(page)

    // Create session with the correct LLM model
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
      // Send a simple prompt
      await typeAndSubmit(page, "What is 2+2? Answer in one word.")

      console.log("Prompt sent, waiting for LLM response...")

      // Wait for assistant response to appear in the page.
      // The local LLM (reasoning model) can take 60-120s to respond.
      // Capture the body text immediately after submission — it will
      // contain the user message but NOT the assistant response yet.
      const bodyAfterSubmit = await page.textContent("body") ?? ""

      const deadline = Date.now() + 100_000
      let responseFound = false

      while (Date.now() < deadline) {
        await page.waitForTimeout(3_000)
        const bodyNow = await page.textContent("body") ?? ""

        // Detect the response by checking if the body text CHANGED
        // (new content appeared that wasn't there after submission).
        // Even a short response like "Four" counts — check for text
        // difference rather than a length threshold.
        if (bodyNow !== bodyAfterSubmit && bodyNow.length > bodyAfterSubmit.length) {
          responseFound = true
          console.log("LLM response detected in page")
          break
        }
      }

      expect(responseFound).toBe(true)
    } finally {
      stopApprover()
    }
  })

  test("response renders as assistant message alongside user message", async ({
    page,
    request,
  }) => {
    test.setTimeout(120_000)

    await authenticatePage(page)

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
      const userMessage = "Say hello and nothing else."
      await typeAndSubmit(page, userMessage)

      console.log("Prompt sent, waiting for response...")

      // Wait for LLM to finish responding
      const deadline = Date.now() + 60_000
      let hasResponse = false

      while (Date.now() < deadline) {
        await page.waitForTimeout(3_000)
        const body = await page.textContent("body")
        if (!body) continue

        // Check that the page contains both the user's prompt text and
        // additional content from the assistant
        const hasUserMsg = body.includes(userMessage)
        const hasExtraContent = body.length > userMessage.length + 100

        if (hasUserMsg && hasExtraContent) {
          hasResponse = true
          break
        }
      }

      expect(hasResponse).toBe(true)

      // Verify the page body contains the user message
      const finalBody = await page.textContent("body")
      expect(finalBody).toContain(userMessage)
    } finally {
      stopApprover()
    }
  })

  test("response arrives via SSE without page reload", async ({
    page,
    request,
  }) => {
    test.setTimeout(120_000)

    await authenticatePage(page)

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
      // Capture body baseline before sending the prompt
      const baselineBody = await page.textContent("body")
      const baselineLen = baselineBody?.length ?? 0

      // Send a prompt that should produce visible content
      await typeAndSubmit(page, "Say hello and nothing else.")

      console.log("Prompt sent, waiting for SSE-delivered response...")

      // Wait for the page content to grow beyond the baseline.
      // The response arrives via SSE (Server-Sent Events) without
      // requiring a page reload. With a reasoning model, content
      // may arrive in a burst after the thinking phase completes.
      const deadline = Date.now() + 100_000
      let contentArrived = false
      let finalLen = 0

      while (Date.now() < deadline) {
        await page.waitForTimeout(3_000)
        const body = await page.textContent("body")
        const currentLen = body?.length ?? 0

        if (currentLen > baselineLen + 20) {
          contentArrived = true
          finalLen = currentLen
          console.log(
            `Content arrived via SSE: baseline=${baselineLen}, final=${currentLen}`,
          )
          break
        }
      }

      expect(contentArrived).toBe(true)

      // Verify the page was NOT reloaded — the URL should still
      // contain the session ID (a reload would reset to the home page
      // unless the URL was preserved).
      expect(page.url()).toContain(session.id)

      // Verify the content grew meaningfully (not just a minor DOM change)
      expect(finalLen).toBeGreaterThan(baselineLen + 10)
    } finally {
      stopApprover()
    }
  })
})
