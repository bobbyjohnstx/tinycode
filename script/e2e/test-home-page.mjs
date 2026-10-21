// E2E: Home page loads and renders the app shell.
//
// Validates that the root URL (/) renders the tinycode web UI —
// project picker, session list, or at minimum the app shell.
// No LLM needed — pure navigation check.

import {
  getConfig,
  launchBrowser,
  runTest,
  printSummary,
} from './helpers.mjs';

const SCREENSHOT_DIR = process.env.E2E_SCREENSHOT_DIR || '/tmp';

const config = getConfig();

await runTest('WEB-E09  Home page loads and renders', async () => {
  const { browser, page } = await launchBrowser();
  try {
    const url = `${config.base}/?auth_token=${config.authParam}`;
    await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 15000 });
    await page.waitForTimeout(3000);

    await page.screenshot({ path: `${SCREENSHOT_DIR}/e2e-e09-home.png` });

    const body = await page.locator('body').innerText();

    // The home page should show at least one of: project list, session list,
    // new session view, or the tinycode wordmark/logo.
    const hasContent =
      body.includes('tinycode') ||
      body.includes('New Session') ||
      body.includes('session') ||
      body.includes('project') ||
      body.includes('Ask anything') ||
      body.length > 100; // at minimum the app shell rendered something

    if (!hasContent) {
      throw new Error(
        'Home page appears empty or failed to render. Body: ' +
          body.substring(0, 500),
      );
    }

    // Verify no crash — check for React/Solid error boundaries
    if (body.includes('Something went wrong') || body.includes('Uncaught')) {
      throw new Error('Home page shows error state: ' + body.substring(0, 300));
    }
  } finally {
    await browser.close();
  }
});

process.exit(printSummary());
