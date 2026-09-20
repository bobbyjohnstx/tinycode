import { type Page, type APIRequestContext } from "@playwright/test"

// ---------------------------------------------------------------------------
// Auth configuration
// ---------------------------------------------------------------------------

// The raw bearer token used for API calls (Authorization: Bearer <TOKEN>).
// Falls back to the value from the working prototype.
const TOKEN =
  process.env.TINYCODE_AUTH_TOKEN ?? "1f3a89333398837197e267aee1952196c5989cc94b6bbc69443a71ca863c0931"

// Base64-encoded "tinycode:<TOKEN>" used as the ?auth_token= query parameter
// to set the tinycode_auth cookie on first page load.
const AUTH_PARAM =
  process.env.TINYCODE_AUTH_PARAM ??
  Buffer.from(`tinycode:${TOKEN}`).toString("base64")

const BASE_URL = process.env.TINYCODE_BASE_URL ?? "http://127.0.0.1:4096"

// ---------------------------------------------------------------------------
// API helper
// ---------------------------------------------------------------------------

interface ApiOptions {
  method?: string
  body?: unknown
}

/**
 * Typed wrapper for server API calls. Uses the Playwright request context
 * when available; otherwise falls back to global fetch.
 */
export async function api<T = unknown>(
  request: APIRequestContext,
  path: string,
  opts: ApiOptions = {},
): Promise<T> {
  const method = opts.method ?? "GET"
  const url = `${BASE_URL}${path}`
  const headers: Record<string, string> = {
    Authorization: `Bearer ${TOKEN}`,
  }

  const response = await request.fetch(url, {
    method,
    headers,
    data: opts.body,
  })

  const text = await response.text()
  if (!text) return {} as T
  return JSON.parse(text) as T
}

// ---------------------------------------------------------------------------
// Session types
// ---------------------------------------------------------------------------

export interface Session {
  id: string
  directory: string
  title?: string
  slug?: string
  [key: string]: unknown
}

export interface Permission {
  id: string
  sessionID?: string
  [key: string]: unknown
}

// ---------------------------------------------------------------------------
// Auth helper
// ---------------------------------------------------------------------------

/**
 * Navigate to the app root with the auth_token query parameter, which sets
 * the tinycode_auth cookie for subsequent requests. Returns the page after
 * the cookie is confirmed.
 *
 * IMPORTANT: Uses waitUntil: "domcontentloaded" because SSE connections
 * stay open and "networkidle" would never resolve.
 */
export async function authenticatePage(page: Page): Promise<void> {
  await page.goto(`/?auth_token=${AUTH_PARAM}`, {
    waitUntil: "domcontentloaded",
    timeout: 15_000,
  })
  // Give the SPA time to initialize and set cookies
  await page.waitForTimeout(2_000)
}

// ---------------------------------------------------------------------------
// Session helpers
// ---------------------------------------------------------------------------

function base64UrlEncode(value: string): string {
  return Buffer.from(value, "utf8")
    .toString("base64")
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=/g, "")
}

/**
 * Create a session via the API, then navigate the browser to its URL.
 * Returns the session metadata.
 */
export async function createAndNavigateToSession(
  page: Page,
  request: APIRequestContext,
): Promise<Session> {
  const session = await api<Session>(request, "/session", {
    method: "POST",
    body: {},
  })

  const dirB64 = base64UrlEncode(session.directory)
  const sessionURL = `/${dirB64}/session/${session.id}`
  await page.goto(sessionURL, {
    waitUntil: "domcontentloaded",
    timeout: 15_000,
  })
  // Wait for the session view to render
  await page.waitForTimeout(2_000)

  return session
}

/**
 * Navigate to an existing session's URL. This triggers the SPA's
 * layout.projects.open() which populates the local project list —
 * required before session rows appear on the home page.
 */
export async function navigateToSession(
  page: Page,
  session: Session,
): Promise<void> {
  const dirB64 = base64UrlEncode(session.directory)
  await page.goto(`/${dirB64}/session/${session.id}`, {
    waitUntil: "domcontentloaded",
    timeout: 15_000,
  })
  await page.waitForTimeout(2_000)
}

/**
 * Seed the SPA's localStorage with a project entry so the home page
 * renders session rows. The SPA persists via localStorageWithPrefix
 * using key "tinycode.global.dat:server". Projects are keyed by
 * "local" for localhost/127.0.0.1 servers (see projectsKey() in
 * packages/app/src/context/server.tsx).
 */
export async function openProjectInSPA(
  page: Page,
  directory: string,
): Promise<void> {
  await page.evaluate(
    ({ dir }) => {
      const STORAGE_KEY = "tinycode.global.dat:server"
      const PROJECTS_KEY = "local"
      const raw = localStorage.getItem(STORAGE_KEY)
      const data = raw ? JSON.parse(raw) : { list: [], projects: {}, lastProject: {} }
      const projects = data.projects[PROJECTS_KEY] ?? []
      if (!projects.find((p: any) => p.worktree === dir)) {
        projects.unshift({ worktree: dir, expanded: true })
        data.projects[PROJECTS_KEY] = projects
      }
      data.lastProject[PROJECTS_KEY] = dir
      localStorage.setItem(STORAGE_KEY, JSON.stringify(data))
    },
    { dir: directory },
  )
}

/**
 * Create a session via API only (no navigation).
 */
export async function createSession(
  request: APIRequestContext,
): Promise<Session> {
  return api<Session>(request, "/session", {
    method: "POST",
    body: {},
  })
}

/**
 * Delete a session via API.
 */
export async function deleteSession(
  request: APIRequestContext,
  sessionId: string,
): Promise<void> {
  await api(request, `/session/${sessionId}`, { method: "DELETE" })
}

// ---------------------------------------------------------------------------
// DOM wait helpers
// ---------------------------------------------------------------------------

/**
 * Wait for an element matching `selector` to contain the given text.
 * Polls the DOM instead of using networkidle (SSE never idles).
 */
export async function waitForContent(
  page: Page,
  selector: string,
  text: string,
  timeoutMs = 15_000,
): Promise<void> {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    const el = await page.$(selector)
    if (el) {
      const content = await el.textContent()
      if (content && content.includes(text)) return
    }
    await page.waitForTimeout(500)
  }
  throw new Error(
    `Timed out waiting for "${text}" in selector "${selector}" after ${timeoutMs}ms`,
  )
}

/**
 * Wait for any element matching `selector` to become visible.
 */
export async function waitForVisible(
  page: Page,
  selector: string,
  timeoutMs = 10_000,
): Promise<void> {
  await page.waitForSelector(selector, {
    state: "visible",
    timeout: timeoutMs,
  })
}

// ---------------------------------------------------------------------------
// Permission helpers
// ---------------------------------------------------------------------------

/**
 * Poll the /permission endpoint until a pending permission appears, then
 * return its ID. Returns null if no permission appears within the timeout.
 */
export async function waitForPermission(
  request: APIRequestContext,
  page: Page,
  timeoutMs = 60_000,
): Promise<Permission | null> {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    const data = await api<{ permissions?: Permission[] }>(request, "/permission")
    const perms = data.permissions ?? []
    if (perms.length > 0) return perms[0]
    await page.waitForTimeout(2_000)
  }
  return null
}

/**
 * Approve a permission via the API.
 */
export async function approvePermission(
  request: APIRequestContext,
  permissionId: string,
): Promise<void> {
  await api(request, `/permission/${permissionId}/reply`, {
    method: "POST",
    body: { action: "allow" },
  })
}

/**
 * Reject a permission via the API.
 */
export async function rejectPermission(
  request: APIRequestContext,
  permissionId: string,
): Promise<void> {
  await api(request, `/permission/${permissionId}/reply`, {
    method: "POST",
    body: { action: "deny" },
  })
}

// ---------------------------------------------------------------------------
// Prompt interaction helpers
// ---------------------------------------------------------------------------

/**
 * Find the prompt input element. Tries data-component attribute first,
 * then falls back to contenteditable.
 */
export async function findPromptInput(page: Page) {
  let el = await page.$('[data-component="prompt-input"]')
  if (!el) el = await page.$("[contenteditable=\"true\"]")
  return el
}

/**
 * Type text into the prompt and submit with Enter.
 */
export async function typeAndSubmit(
  page: Page,
  text: string,
): Promise<void> {
  const prompt = await findPromptInput(page)
  if (!prompt) throw new Error("Could not find prompt input element")

  await prompt.click()
  await page.waitForTimeout(300)
  await page.keyboard.type(text, { delay: 80 })
  await page.waitForTimeout(300)
  await page.keyboard.press("Enter")
}

// ---------------------------------------------------------------------------
// Cleanup helper
// ---------------------------------------------------------------------------

/** Session IDs created during a test, to clean up in afterEach. */
export class SessionTracker {
  private ids: string[] = []

  track(session: Session): Session {
    this.ids.push(session.id)
    return session
  }

  async cleanup(request: APIRequestContext): Promise<void> {
    for (const id of this.ids) {
      try {
        await deleteSession(request, id)
      } catch {
        // best-effort cleanup
      }
    }
    this.ids = []
  }
}
