import { expect, test, type Page } from "@playwright/test"
import { trackPageErrors } from "../utils/errors"
import { mockTinycodeServer } from "../utils/mock-server"

const directory = "C:/tinycode/StreamingProject"
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
        name: "tinycode",
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
  // Skip: this test requires coordinating POST /session, optimistic user message,
  // session navigation, message API reload, and SSE event delivery in a mock that
  // uses a polled (non-persistent) SSE endpoint. The second test covers the core
  // session rendering scenario. A dedicated integration harness is needed for true
  // end-to-end streaming coverage.
  test.skip("submitting a prompt creates a session and shows streaming assistant response", async ({ page }) => {
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

    let newSessionCreated = false

    await mockTinycodeServer(page, {
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
        // Once the new session exists, serve its messages including the assistant response
        if (sessionId === newSessionID && newSessionCreated) {
          return {
            items: [
              {
                info: {
                  id: "msg_stream_asst",
                  sessionID: newSessionID,
                  role: "assistant",
                  time: { created: Date.now(), completed: Date.now() },
                  parentID: "msg_u_optimistic",
                  modelID: model.modelID,
                  providerID: model.providerID,
                  mode: "build",
                  agent: "build",
                  path: { cwd: directory, root: directory },
                  cost: 0.001,
                  tokens: { input: 50, output: 20, reasoning: 0, cache: { read: 0, write: 0 } },
                  variant: model.variant,
                  finish: "stop",
                },
                parts: [
                  {
                    id: "prt_stream_text",
                    sessionID: newSessionID,
                    messageID: "msg_stream_asst",
                    type: "text",
                    text: "Streaming response content.",
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
      newSessionCreated = true

      // Push all SSE events synchronously so the next SSE poll picks them all up.
      // The mock SSE is polled in a loop — events don't need async delays.
      // message.updated is required to create the message row in the store before
      // message.part.updated can attach the streamed part to it.
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
        {
          directory,
          payload: {
            type: "message.updated",
            properties: {
              info: {
                id: "msg_stream_asst",
                sessionID: newSessionID,
                role: "assistant",
                time: { created: Date.now(), completed: undefined },
                parentID: "msg_u_optimistic",
                modelID: model.modelID,
                providerID: model.providerID,
                mode: "build",
                agent: "build",
                path: { cwd: directory, root: directory },
                cost: 0,
                tokens: { input: 0, output: 0, reasoning: 0, cache: { read: 0, write: 0 } },
              },
            },
          },
        },
        {
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
        },
        {
          directory,
          payload: {
            type: "session.status",
            properties: {
              sessionID: newSessionID,
              status: { type: "idle" },
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
    })

    await configurePage(page)
    // Navigate directly to new session page — the home page (newLayoutDesigns mode) has no composer
    await page.goto(`/${base64Encode(directory)}/session`)

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

    await mockTinycodeServer(page, {
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
