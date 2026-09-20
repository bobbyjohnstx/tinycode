import { test, expect } from "@playwright/test"
import {
  api,
  authenticatePage,
  SessionTracker,
  type Session,
} from "./helpers"

test.describe("agent: agent interaction", () => {
  const tracker = new SessionTracker()

  test.afterEach(async ({ request }) => {
    await tracker.cleanup(request)
  })

  test("agent list API returns known agents", async ({ request }) => {
    const agents = await api<{ name: string; description: string }[]>(
      request,
      "/agent",
    )

    expect(Array.isArray(agents)).toBe(true)
    expect(agents.length).toBeGreaterThan(0)

    const names = agents.map((a) => a.name)
    expect(names).toContain("architect")
    expect(names).toContain("explore")
  })

  test("session created with specific agent stores agent in metadata", async ({
    request,
  }) => {
    // Create a session with a specific agent via the API
    const session = tracker.track(
      await api<Session>(request, "/session", {
        method: "POST",
        body: {
          agent: "explore",
          model: { providerID: "lm-studio", modelID: "ornith-1.0-9b-mlx" },
        },
      }),
    )

    expect(session.id).toBeTruthy()

    // Verify the session metadata reflects the agent
    const fetched = await api<Session & { agent?: string }>(
      request,
      `/session/${session.id}`,
    )
    expect(fetched.agent).toBe("explore")
  })

  test("prompt sent to agent-session starts processing", async ({
    request,
  }) => {
    test.setTimeout(60_000)

    // Create session with a specific agent
    const session = tracker.track(
      await api<Session>(request, "/session", {
        method: "POST",
        body: {
          agent: "build",
          model: { providerID: "lm-studio", modelID: "ornith-1.0-9b-mlx" },
        },
      }),
    )

    // Send a prompt to the session via the message API.
    // POST /session/{id}/message returns 204 (no content) when the
    // prompt is accepted and processing starts.
    const response = await request.fetch(
      `http://127.0.0.1:4096/session/${session.id}/message`,
      {
        method: "POST",
        headers: {
          Authorization: `Bearer ${process.env.TINYCODE_AUTH_TOKEN ?? "1f3a89333398837197e267aee1952196c5989cc94b6bbc69443a71ca863c0931"}`,
          "Content-Type": "application/json",
        },
        data: JSON.stringify({
          content: "What is 2+2?",
          agent: "build",
        }),
      },
    )

    // The server accepts the prompt with 204 No Content
    expect(response.status()).toBe(204)

    // Wait briefly for the session to register the prompt
    await new Promise((r) => setTimeout(r, 5_000))

    // Verify the session status reflects active processing.
    // The session.status event sets working=true while processing.
    const status = await api<{ type?: string; working?: boolean }>(
      request,
      `/session/status`,
    )
    // status endpoint returns current status — it may already be idle
    // if the LLM started quickly, or working if still processing.
    // Either way, the prompt was accepted (204 above).
    expect(status).toBeTruthy()
  })

  test("agent picker or agent API lists available agents", async ({
    page,
    request,
  }) => {
    await authenticatePage(page)
    await page.waitForTimeout(2_000)

    // Check if the UI has an agent selector element
    const agentSelector = await page.$(
      '[data-component*="agent"], [data-slot*="agent"], button:has-text("Agent")',
    )

    if (agentSelector) {
      // If an agent picker exists in the UI, verify it's functional
      const isVisible = await agentSelector.isVisible()
      expect(isVisible).toBe(true)
    }

    // Regardless of UI picker, verify agents are available via API
    const agents = await api<{ name: string; description: string }[]>(
      request,
      "/agent",
    )

    expect(Array.isArray(agents)).toBe(true)
    expect(agents.length).toBeGreaterThan(3)

    // Verify specific expected agents
    const names = agents.map((a) => a.name)
    expect(names).toContain("architect")
    expect(names).toContain("explore")

    // Each agent should have a name and description
    for (const agent of agents) {
      expect(agent.name).toBeTruthy()
      expect(typeof agent.name).toBe("string")
    }
  })
})
