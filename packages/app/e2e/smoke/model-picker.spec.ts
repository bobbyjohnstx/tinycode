import { expect, test, type Page } from "@playwright/test"
import { trackPageErrors } from "../utils/errors"
import { mockOpenCodeServer } from "../utils/mock-server"

const directory = "C:/OpenCode/ModelPickerProject"
const projectID = "proj_model"
const sessionID = "ses_model_1"
const userMessageID = "msg_user_model"
const assistantMessageID = "msg_assistant_model"

function base64Encode(value: string) {
  return Buffer.from(value, "utf8").toString("base64").replace(/\+/g, "-").replace(/\//g, "_").replace(/=/g, "")
}

const model = { providerID: "tinycode", modelID: "claude-sonnet-4-5", variant: "max" }

function twoProviderFixture() {
  return {
    all: [
      {
        id: "tinycode",
        name: "OpenCode",
        models: {
          // cost.input > 0 ensures providers.paid() returns non-empty, enabling ModelSelectorPopover
          "claude-sonnet-4-5": {
            id: "claude-sonnet-4-5",
            name: "Claude Sonnet 4.5",
            limit: { context: 200_000 },
            cost: { input: 0.000003, output: 0.000015 },
          },
          "claude-opus-4-6": {
            id: "claude-opus-4-6",
            name: "Claude Opus 4.6",
            limit: { context: 200_000 },
            cost: { input: 0.000015, output: 0.000075 },
          },
        },
      },
      {
        id: "openai",
        name: "OpenAI",
        models: {
          "gpt-4o": { id: "gpt-4o", name: "GPT-4o", limit: { context: 128_000 } },
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
    name: "model-picker-project",
    time: { created: 1700000000000, updated: 1700000000000 },
    sandboxes: [],
  }
}

function session() {
  return {
    id: sessionID,
    slug: "model-test",
    projectID,
    directory,
    title: "Model picker test session",
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
          text: "Hello",
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
          text: "Hello there!",
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

async function setupModelPickerTest(page: Page, providerFixture: ReturnType<typeof twoProviderFixture>) {
  await mockOpenCodeServer(page, {
    directory,
    project: project(),
    provider: providerFixture,
    sessions: [session()],
    pageMessages: () => ({ items: messages() }),
  })
  await configurePage(page)
  await page.goto(`/${base64Encode(directory)}/session/${sessionID}`)
  await expect(page.getByRole("heading", { name: "Model picker test session" })).toBeVisible()
}

test.describe("smoke: model-picker", () => {
  test("model picker button shows current model name", async ({ page }) => {
    const errors = trackPageErrors(page)
    await setupModelPickerTest(page, twoProviderFixture())

    // In the v2 design (newLayoutDesigns=true) the model button uses data-action="prompt-model"
    // The data-component="prompt-model-control" wrapper only exists in the legacy design
    const modelButton = page.locator('[data-action="prompt-model"]')
    await expect(modelButton).toBeVisible()

    // Verify button contains current model name
    await expect(modelButton).toContainText(/claude.*sonnet.*4.*5/i)

    // Verify no console errors
    expect(errors).toEqual([])
  })

  test("model picker popover lists available models grouped by provider", async ({ page }) => {
    const errors = trackPageErrors(page)
    await setupModelPickerTest(page, twoProviderFixture())

    // Click model button to open picker
    const modelButton = page.locator('[data-action="prompt-model"]')
    await expect(modelButton).toBeVisible()
    await modelButton.click()

    // Verify models from fixture are visible in the popover.
    // Use .nth(1) to skip the composer button text and target the popover list entry.
    await expect(page.getByText("Claude Sonnet 4.5").nth(1)).toBeVisible({ timeout: 5_000 })
    await expect(page.getByText("Claude Opus 4.6").first()).toBeVisible()

    // Verify provider group label is visible — use exact text to avoid matching
    // "Free models provided by OpenCode" which also contains "OpenCode"
    await expect(page.getByText("OpenCode", { exact: true })).toBeVisible()

    // Verify no console errors
    expect(errors).toEqual([])
  })

  // Skip: the model picker only lists models from connected providers — disconnected
  // providers' models (e.g. GPT-4o from openai) are not shown in the picker at all.
  // The app's models.available() memo filters to providers.connected() only.
  test.skip("disconnected provider shown differently from connected provider", async ({ page }) => {
    const errors = trackPageErrors(page)
    await setupModelPickerTest(page, twoProviderFixture())

    // Open model picker
    const modelControl = page.locator('[data-action="prompt-model"]')
    await expect(modelControl).toBeVisible()
    await modelControl.click()

    // Verify connected provider's models are visible
    await expect(page.getByText("Claude Sonnet 4.5")).toBeVisible({ timeout: 5_000 })

    // Verify disconnected provider group label is visible
    // "OpenAI" should appear as a group header even when disconnected
    await expect(page.getByText("OpenAI")).toBeVisible()

    // Verify disconnected model is listed
    await expect(page.getByText("GPT-4o")).toBeVisible()

    // Verify no console errors
    expect(errors).toEqual([])
  })

  test("selecting a different model updates the model control", async ({ page }) => {
    const errors = trackPageErrors(page)
    await setupModelPickerTest(page, twoProviderFixture())

    // Open model picker
    const modelButton = page.locator('[data-action="prompt-model"]')
    await expect(modelButton).toBeVisible()
    await modelButton.click()

    // Wait for model list to be visible
    await expect(page.getByText("Claude Opus 4.6")).toBeVisible({ timeout: 5_000 })

    // Click "Claude Opus 4.6"
    await page.getByText("Claude Opus 4.6").click()

    // Verify the model button now shows the new model name
    // In the v2 design, data-action="prompt-model" is on the button itself (no wrapper div)
    await expect(modelButton).toContainText(/claude.*opus.*4.*6/i, { timeout: 5_000 })

    // Verify no console errors
    expect(errors).toEqual([])
  })
})
