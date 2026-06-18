import { expect, test, type Page } from "@playwright/test"
import { trackPageErrors } from "../utils/errors"
import { mockOpenCodeServer } from "../utils/mock-server"

const directory = "C:/OpenCode/VCSProject"
const projectID = "proj_vcs"
const sessionID = "ses_vcs_1"
const userMessageID = "msg_user_vcs"
const assistantMessageID = "msg_assistant_vcs"

function base64Encode(value: string) {
  return Buffer.from(value, "utf8").toString("base64").replace(/\+/g, "-").replace(/\//g, "_").replace(/=/g, "")
}

const model = { providerID: "tinycode", modelID: "claude-sonnet-4-5", variant: "max" }

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
    name: "vcs-project",
    time: { created: 1700000000000, updated: 1700000000000 },
    sandboxes: [],
  }
}

function session() {
  return {
    id: sessionID,
    slug: "vcs-test",
    projectID,
    directory,
    title: "VCS test session",
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
        summary: {
          diffs: [
            {
              file: "src/existing.ts",
              additions: 3,
              deletions: 1,
              status: "modified",
            },
          ],
        },
        agent: "build",
        model,
      },
      parts: [
        {
          id: "prt_user_text",
          sessionID,
          messageID: userMessageID,
          type: "text",
          text: "Update the existing file.",
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
          text: "I updated the file.",
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

test.describe("smoke: vcs review panel", () => {
  // The VCS review panel is accessed via the mobile "Changes" tab (on narrow viewports)
  // or a desktop side panel. This test uses a narrow viewport to directly access the tab.
  test.use({ viewport: { width: 600, height: 900 } })

  test("VCS diff panel renders file changes via mobile Changes tab", async ({ page }) => {
    const errors = trackPageErrors(page)

    await mockOpenCodeServer(page, {
      directory,
      project: project(),
      provider: provider(),
      sessions: [session()],
      pageMessages: () => ({ items: messages() }),
    })

    // Override /vcs/status and /vcs/diff AFTER mockOpenCodeServer so these routes
    // take priority (Playwright evaluates routes in LIFO order — last registered wins)
    await page.route("**/vcs/status", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        headers: { "access-control-allow-origin": "*" },
        body: JSON.stringify({
          added: ["src/new-file.ts"],
          modified: ["src/existing.ts"],
          deleted: [],
          staged: [],
        }),
      }),
    )

    await page.route("**/vcs/diff**", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        headers: { "access-control-allow-origin": "*" },
        body: JSON.stringify([
          {
            file: "src/existing.ts",
            additions: 3,
            deletions: 1,
            patch:
              "diff --git a/src/existing.ts b/src/existing.ts\n-const old = 1\n+const new1 = 1\n+const new2 = 2\n+const new3 = 3",
            status: "modified",
          },
          {
            file: "src/new-file.ts",
            additions: 5,
            deletions: 0,
            patch: "diff --git a/src/new-file.ts b/src/new-file.ts\n+export const value = 'hello'",
            status: "added",
          },
        ]),
      }),
    )

    await configurePage(page)
    await page.goto(`/${base64Encode(directory)}/session/${sessionID}`)
    await expect(page.getByRole("heading", { name: "VCS test session" })).toBeVisible()

    // Click the "Changes" tab (mobile viewport has Session/Changes tabs)
    const changesTab = page.getByRole("tab", { name: /changes/i })
    await expect(changesTab).toBeVisible({ timeout: 10_000 })
    await changesTab.click()

    // Wait for file names to appear in the review panel.
    // The path is rendered split across two spans (directory + filename), so match
    // the filename portion which appears in its own element.
    await expect(page.getByText("existing.ts").first()).toBeVisible({ timeout: 10_000 })
    await expect(page.getByText("new-file.ts").first()).toBeVisible({ timeout: 10_000 })

    // Verify no console errors
    expect(errors).toEqual([])
  })
})
