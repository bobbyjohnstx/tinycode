import { expect, test, type Page } from "@playwright/test"
import { trackPageErrors } from "../utils/errors"
import { mockTinycodeServer } from "../utils/mock-server"

const directory = "C:/tinycode/PermissionProject"
const projectID = "proj_perm"
const sessionID = "ses_perm_1"
const userMessageID = "msg_user_perm"
const assistantMessageID = "msg_assistant_perm"

type EventPayload = {
  directory: string
  payload: Record<string, unknown>
}

function base64Encode(value: string) {
  return Buffer.from(value, "utf8").toString("base64").replace(/\+/g, "-").replace(/\//g, "_").replace(/=/g, "")
}

const model = { providerID: "tinycode", modelID: "claude-sonnet-4-5", variant: "max" }

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
    name: "permission-project",
    time: { created: 1700000000000, updated: 1700000000000 },
    sandboxes: [],
  }
}

function session() {
  return {
    id: sessionID,
    slug: "permission-test",
    projectID,
    directory,
    title: "Permission test session",
    version: "dev",
    time: { created: 1700000000000, updated: 1700000000000 },
  }
}

function messages() {
  return [
    {
      info: {
        id: userMessageID,
        sessionID,
        role: "user",
        time: { created: 1700000000000 },
        summary: { diffs: [] },
        agent: "build",
        model,
      },
      parts: [
        {
          id: "prt_user_text",
          sessionID,
          messageID: userMessageID,
          type: "text",
          text: "Run the build script.",
        },
      ],
    },
    {
      info: {
        id: assistantMessageID,
        sessionID,
        role: "assistant",
        time: { created: 1700000001000 },
        parentID: userMessageID,
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
          id: "prt_assistant_text",
          sessionID,
          messageID: assistantMessageID,
          type: "text",
          text: "I will run the build.",
        },
      ],
    },
  ]
}

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

test.describe("flows: permission dock", () => {
  test("permission dock appears when session emits permission.asked event", async ({ page }) => {
    const errors = trackPageErrors(page)
    const events: EventPayload[] = []

    await mockTinycodeServer(page, {
      directory,
      project: project(),
      provider: provider(),
      sessions: [session()],
      pageMessages: () => ({ items: messages() }),
      events: () => events.splice(0),
    })

    await configurePage(page)
    await page.goto(`/${base64Encode(directory)}/session/${sessionID}`)
    await expect(page.getByRole("heading", { name: "Permission test session" })).toBeVisible()

    // Inject permission.asked SSE event
    events.push({
      directory,
      payload: {
        type: "permission.asked",
        properties: {
          id: "perm_001",
          sessionID,
          permission: "bash",
          patterns: ["*"],
          metadata: {},
        },
      },
    })

    // Wait for permission footer actions to appear
    await expect(page.locator('[data-slot="permission-footer-actions"]')).toBeVisible({ timeout: 10_000 })

    // Assert buttons are visible
    await expect(page.getByRole("button", { name: /allow once/i })).toBeVisible()
    await expect(page.getByRole("button", { name: /allow always/i })).toBeVisible()
    await expect(page.getByRole("button", { name: /deny/i })).toBeVisible()

    // Verify no console errors
    expect(errors).toEqual([])
  })

  test("clicking Allow Once calls the permission reply endpoint", async ({ page }) => {
    const errors = trackPageErrors(page)
    const events: EventPayload[] = []

    await mockTinycodeServer(page, {
      directory,
      project: project(),
      provider: provider(),
      sessions: [session()],
      pageMessages: () => ({ items: messages() }),
      events: () => events.splice(0),
    })

    // Intercept the permission respond endpoint
    let permissionReply: unknown = null
    await page.route(`**/session/${sessionID}/permissions/perm_001`, async (route) => {
      if (route.request().method() === "POST") {
        permissionReply = await route.request().postDataJSON()
        // Inject permission.replied event to clear the dock
        events.push({
          directory,
          payload: {
            type: "permission.replied",
            properties: {
              sessionID,
              permissionID: "perm_001",
            },
          },
        })
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          headers: { "access-control-allow-origin": "*" },
          body: JSON.stringify({}),
        })
      } else {
        await route.fallback()
      }
    })

    await configurePage(page)
    await page.goto(`/${base64Encode(directory)}/session/${sessionID}`)
    await expect(page.getByRole("heading", { name: "Permission test session" })).toBeVisible()

    // Inject permission.asked SSE event
    events.push({
      directory,
      payload: {
        type: "permission.asked",
        properties: {
          id: "perm_001",
          sessionID,
          permission: "bash",
          patterns: ["*"],
          metadata: {},
        },
      },
    })

    // Wait for permission dock to appear
    await expect(page.locator('[data-slot="permission-footer-actions"]')).toBeVisible({ timeout: 10_000 })

    // Click "Allow Once"
    await page.getByRole("button", { name: /allow once/i }).click()

    // Verify the request body
    await expect.poll(() => permissionReply, { timeout: 5_000 }).toBeTruthy()
    expect(permissionReply).toMatchObject({ response: "once" })

    // Verify no console errors
    expect(errors).toEqual([])
  })
})
