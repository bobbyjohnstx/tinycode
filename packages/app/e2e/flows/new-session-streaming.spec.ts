import { expect, test, type Page } from "@playwright/test"
import { trackPageErrors } from "../utils/errors"
import { mockOpenCodeServer } from "../utils/mock-server"

const directory = "C:/OpenCode/StreamingProject"
const projectID = "proj_stream"

type EventPayload = {
  directory: string
  payload: Record<string, unknown>
}

function base64Encode(value: string) {
  return Buffer.from(value, "utf8").toString("base64").replace(/\+/g, "-").replace(/\//g, "_").replace(/=/g, "")
}

function provider() {
  return {
    all: [
      {
        id: "tinycode",
        name: "OpenCode",
        models: {
          "claude-sonnet-4-5": { id: "claude-sonnet-4-5", name: "Claude Sonnet 4.5", limit: { context: 200_000 } },
        },
      },
    ],
    connected: ["tinycode"],
    default: { providerID: "tinycode", modelID: "claude-sonnet-4-5" },
  }
}

function project() {
  return {
    id: projectID,
    worktree: directory,
    vcs: "git",
    name: "stream-project",
    time: { created: 1700000000000, updated: 1700000000000 },
    sandboxes: [],
  }
}

const model = { providerID: "tinycode", modelID: "claude-sonnet-4-5", variant: "max" }

async function configurePage(page: Page) {
  await page.addInitScript(() => {
    localStorage.setItem(
      "settings.v3",
      JSON.stringify({
        general: {
          newLayoutDesigns: true,
          editToolPartsExpanded: true,
          shellToolPartsExpanded: true,
        },
      }),
    )
  })

  await page.addInitScript((dir) => {
    localStorage.setItem(
      "tinycode.global.dat:server",
      JSON.stringify({
        projects: {
          local: [{ worktree: dir, expanded: true }],
        },
        lastProject: {
          local: dir,
        },
      }),
    )
  }, directory)
}

test.describe("flows: new-session streaming", () => {
  test("submitting a prompt creates a session and shows streaming assistant response", async ({ page }) => {
    const errors = trackPageErrors(page)
    const events: EventPayload[] = []

    const existingSessionID = "ses_existing_1"
    const existingSession = {
      id: existingSessionID,
      slug: "existing",
      projectID,
      directory,
      title: "Existing session",
      version: "dev",
      time: { created: 1700000000000, updated: 1700000000000 },
    }

    const newSessionID = "ses_stream_new"
    const newSession = {
      id: newSessionID,
      slug: "stream-test",
      projectID,
      directory,
      title: "Stream test",
      version: "dev" as const,
      time: { created: Date.now(), updated: Date.now() },
    }

    let sessions = [existingSession]

    await mockOpenCodeServer(page, {
      directory,
      project: project(),
      provider: provider(),
      sessions,
      pageMessages: (sessionId) => {
        if (sessionId === existingSessionID) {
          return {
            items: [
              {
                info: {
                  id: "msg_u1",
                  sessionID: existingSessionID,
                  role: "user",
                  time: { created: 1700000000000 },
                  summary: { diffs: [] },
                  agent: "build",
                  model,
                },
                parts: [
                  {
                    id: "prt_u1",
                    sessionID: existingSessionID,
                    messageID: "msg_u1",
                    type: "text",
                    text: "Hello",
                  },
                ],
              },
            ],
          }
        }
        return { items: [] }
      },
      events: () => events.splice(0),
    })

    // Intercept POST /session to simulate session creation
    await page.route("**/session", async (route) => {
      const method = route.request().method()
      if (method !== "POST") return route.fallback()
      sessions = [existingSession, newSession]

      // Inject SSE events for session creation + busy state
      events.push(
        {
          directory,
          payload: {
            type: "session.created",
            properties: { info: newSession },
          },
        },
        {
          directory,
          payload: {
            type: "session.status",
            properties: {
              sessionID: newSessionID,
              status: { type: "running", alert: false },
            },
          },
        },
      )

      await route.fulfill({
        status: 200,
        contentType: "application/json",
        headers: { "access-control-allow-origin": "*" },
        body: JSON.stringify(newSession),
      })

      // After a brief delay, push streamed text part
      setTimeout(() => {
        events.push({
          directory,
          payload: {
            type: "message.part.updated",
            properties: {
              part: {
                id: "prt_stream_text",
                sessionID: newSessionID,
                messageID: "msg_stream_asst",
                type: "text",
                text: "Streaming response content.",
              },
            },
          },
        })
      }, 200)

      // Then set idle
      setTimeout(() => {
        events.push({
          directory,
          payload: {
            type: "session.status",
            properties: {
              sessionID: newSessionID,
              status: { type: "idle" },
            },
          },
        })
      }, 400)
    })

    await configurePage(page)
    await page.goto("/")

    // Click project row to expand sessions
    const projectRows = page.locator('[data-component="home-project-row"]')
    await expect(projectRows.first()).toBeVisible()
    await projectRows.first().click()

    // Find composer and type a prompt
    const textbox = page.locator('[data-component="prompt-input"]')
    await expect(textbox).toBeVisible()
    await textbox.click()
    await page.keyboard.type("Write a test for me")

    // Click submit button
    const submitButton = page.locator('[data-action="prompt-submit"]')
    await expect(submitButton).toBeVisible()
    await submitButton.click()

    // Wait for streamed text to appear
    await expect(page.locator('[data-timeline-part-id="prt_stream_text"]').first()).toBeVisible({ timeout: 15_000 })

    // Verify the streamed text content
    const streamedPart = page.locator('[data-timeline-part-id="prt_stream_text"]').first()
    await expect(streamedPart).toContainText("Streaming response content.")

    // Verify no console errors
    expect(errors).toEqual([])
  })

  test("navigating to a session URL directly shows its timeline", async ({ page }) => {
    const errors = trackPageErrors(page)

    const sessionID = "ses_direct_nav"
    const session = {
      id: sessionID,
      slug: "direct-nav",
      projectID,
      directory,
      title: "Direct navigation session",
      version: "dev",
      time: { created: 1700000000000, updated: 1700000000000 },
    }

    await mockOpenCodeServer(page, {
      directory,
      project: project(),
      provider: provider(),
      sessions: [session],
      pageMessages: (sid) => ({
        items: [
          {
            info: {
              id: "msg_u1",
              sessionID: sid,
              role: "user",
              time: { created: 1700000000000 },
              summary: { diffs: [] },
              agent: "build",
              model,
            },
            parts: [
              {
                id: "prt_u1_text",
                sessionID: sid,
                messageID: "msg_u1",
                type: "text",
                text: "What is this project?",
              },
            ],
          },
          {
            info: {
              id: "msg_a1",
              sessionID: sid,
              role: "assistant",
              time: { created: 1700000001000 },
              parentID: "msg_u1",
              modelID: model.modelID,
              providerID: model.providerID,
              mode: "build",
              agent: "build",
              path: { cwd: directory, root: directory },
              cost: 0.01,
              tokens: { input: 100, output: 200, reasoning: 0, cache: { read: 0, write: 0 } },
              variant: "max",
              finish: "stop",
            },
            parts: [
              {
                id: "prt_a1_text",
                sessionID: sid,
                messageID: "msg_a1",
                type: "text",
                text: "This is a test project.",
              },
            ],
          },
        ],
      }),
    })

    await configurePage(page)
    await page.goto(`/${base64Encode(directory)}/session/${sessionID}`)

    // Verify session title heading is visible
    await expect(page.getByRole("heading", { name: "Direct navigation session" })).toBeVisible()

    // Verify at least one timeline part is in the DOM
    await expect(page.locator("[data-timeline-part-id]").first()).toBeVisible()

    // Verify composer textarea is visible
    await expect(page.locator('[data-component="prompt-input"]')).toBeVisible()

    // Verify no console errors
    expect(errors).toEqual([])
  })
})
