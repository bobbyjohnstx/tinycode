import { expect, test, type Page } from "@playwright/test"
import { trackPageErrors } from "../utils/errors"
import { mockOpenCodeServer } from "../utils/mock-server"

const directory = "C:/OpenCode/HomeListProject"
const projectID = "proj_home_list"

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
    name: "home-list-project",
    time: { created: 1700000000000, updated: 1700000000000 },
    sandboxes: [],
  }
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

test.describe("smoke: home session list", () => {
  test("renders sessions grouped by date", async ({ page }) => {
    const errors = trackPageErrors(page)

    const now = Date.now()
    const sessions = [
      {
        id: "ses_today_1",
        slug: "today-1",
        projectID,
        directory,
        title: "First session today",
        version: "dev",
        time: { created: now - 1_000, updated: now - 1_000 },
      },
      {
        id: "ses_today_2",
        slug: "today-2",
        projectID,
        directory,
        title: "Second session today",
        version: "dev",
        time: { created: now - 3_600_000, updated: now - 3_600_000 },
      },
      {
        id: "ses_yesterday_1",
        slug: "yesterday-1",
        projectID,
        directory,
        title: "Yesterday session",
        version: "dev",
        time: { created: now - 90_000_000, updated: now - 90_000_000 },
      },
      {
        id: "ses_older_1",
        slug: "older-1",
        projectID,
        directory,
        title: "Older session",
        version: "dev",
        time: { created: now - 7 * 86_400_000, updated: now - 7 * 86_400_000 },
      },
    ]

    await mockOpenCodeServer(page, {
      directory,
      project: project(),
      provider: provider(),
      sessions,
      pageMessages: () => ({ items: [] }),
    })

    await configurePage(page)
    await page.goto("/")

    // Click the project row to expand sessions
    const projectRows = page.locator('[data-component="home-project-row"]')
    await expect(projectRows.first()).toBeVisible()
    await projectRows.first().click()

    // Wait for session rows to appear
    const sessionRows = page.locator('[data-component="home-session-row"]')
    await expect(sessionRows).toHaveCount(4)

    // Verify "Today" and "Yesterday" group headers are visible
    await expect(page.getByText("Today", { exact: true })).toBeVisible()
    await expect(page.getByText("Yesterday", { exact: true })).toBeVisible()

    // Verify sessions appear in expected order (most recent first within each group)
    const sessionTexts = await sessionRows.allTextContents()
    const todaySessions = sessionTexts.filter(
      (text) => text.includes("First session today") || text.includes("Second session today"),
    )
    expect(todaySessions.length).toBe(2)
    // "First session today" should appear before "Second session today" (more recent first)
    const firstIdx = sessionTexts.findIndex((text) => text.includes("First session today"))
    const secondIdx = sessionTexts.findIndex((text) => text.includes("Second session today"))
    expect(firstIdx).toBeLessThan(secondIdx)

    // Verify no console errors
    expect(errors).toEqual([])
  })

  test("clicking a session row navigates to session URL", async ({ page }) => {
    const errors = trackPageErrors(page)

    const now = Date.now()
    const sessionID = "ses_nav_1"
    const sessions = [
      {
        id: sessionID,
        slug: "nav-1",
        projectID,
        directory,
        title: "Navigable session",
        version: "dev",
        time: { created: now - 1_000, updated: now - 1_000 },
      },
      {
        id: "ses_nav_2",
        slug: "nav-2",
        projectID,
        directory,
        title: "Another session",
        version: "dev",
        time: { created: now - 3_600_000, updated: now - 3_600_000 },
      },
    ]

    const model = { providerID: "tinycode", modelID: "claude-sonnet-4-5", variant: "max" }

    await mockOpenCodeServer(page, {
      directory,
      project: project(),
      provider: provider(),
      sessions,
      pageMessages: (sid) => ({
        items: [
          {
            info: {
              id: "msg_u1",
              sessionID: sid,
              role: "user",
              time: { created: Date.now() },
              summary: { diffs: [] },
              agent: "build",
              model,
            },
            parts: [
              {
                id: "prt_u1",
                sessionID: sid,
                messageID: "msg_u1",
                type: "text",
                text: "Hello",
              },
            ],
          },
        ],
      }),
    })

    await configurePage(page)
    await page.goto("/")

    // Click the project row
    const projectRows = page.locator('[data-component="home-project-row"]')
    await expect(projectRows.first()).toBeVisible()
    await projectRows.first().click()

    // Click the first session row
    const sessionRows = page.locator('[data-component="home-session-row"]')
    await expect(sessionRows.first()).toBeVisible()
    await sessionRows.first().click()

    // Verify URL matches expected session URL
    const expectedPath = `/${base64Encode(directory)}/session/${sessionID}`
    await expect(page).toHaveURL(new RegExp(expectedPath.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")))

    // Verify page heading is visible
    await expect(page.getByRole("heading", { name: "Navigable session" })).toBeVisible()

    // Verify no console errors
    expect(errors).toEqual([])
  })
})
