// E2E: Send "Hello?" via the web UI and verify LLM responds.
//
// This test validates the full browser → Go server → LLM → SSE → browser path.
// It creates a session via API, opens it in headless Chromium, types a message,
// and waits for the model response to appear in the DOM.

import {
  getConfig,
  createSession,
  launchBrowser,
  navigateToSession,
  findInput,
  typeAndSend,
  waitForResponse,
  runTest,
  printSummary,
} from './helpers.mjs';

const DIRECTORY = process.env.E2E_PROJECT_DIR || process.cwd();
const SCREENSHOT_DIR = process.env.E2E_SCREENSHOT_DIR || '/tmp';

const config = getConfig();

await runTest('WEB-E01  Hello roundtrip', async () => {
  // 1. Create session via API (avoids localStorage/project-picker complexity)
  const session = await createSession(config, DIRECTORY);
  if (!session.id || !session.slug) {
    throw new Error(`Bad session response: ${JSON.stringify(session)}`);
  }

  // 2. Open browser and navigate to session
  const { browser, page, errors } = await launchBrowser();
  try {
    await navigateToSession(page, config, session.slug);

    // 3. Find input element
    const input = await findInput(page);
    if (!input) {
      await page.screenshot({ path: `${SCREENSHOT_DIR}/e2e-e01-no-input.png` });
      throw new Error('No input element found on session page');
    }

    // 4. Type and send
    await typeAndSend(page, input, 'Hello?');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/e2e-e01-sent.png` });

    // 5. Wait for LLM response
    const result = await waitForResponse(page, 'Hello?', 60);
    await page.screenshot({ path: `${SCREENSHOT_DIR}/e2e-e01-result.png` });

    if (!result.found) {
      const body = await page.locator('body').innerText();
      throw new Error(
        `No LLM response after ${result.elapsed}s. Page: ${body.substring(0, 300)}`,
      );
    }
  } finally {
    await browser.close();
  }
});

process.exit(printSummary());
