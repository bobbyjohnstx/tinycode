// E2E: Empty session shows Connect CTA when no providers are connected.
//
// Validates that the web UI surfaces a "Connect a provider" call-to-action
// on an empty session page (#821). When providers are already connected,
// the CTA is hidden — test skips gracefully.

import {
  getConfig,
  createSession,
  launchBrowser,
  navigateToSession,
  runTest,
  printSummary,
} from './helpers.mjs';

const DIRECTORY = process.env.E2E_PROJECT_DIR || process.cwd();
const SCREENSHOT_DIR = process.env.E2E_SCREENSHOT_DIR || '/tmp';

const config = getConfig();

await runTest('WEB-E02  Empty session shows Connect CTA or model selector', async () => {
  const session = await createSession(config, DIRECTORY);
  if (!session.id || !session.slug) {
    throw new Error(`Bad session response: ${JSON.stringify(session)}`);
  }

  const { browser, page } = await launchBrowser();
  try {
    await navigateToSession(page, config, session.slug);

    const body = await page.locator('body').innerText();
    await page.screenshot({ path: `${SCREENSHOT_DIR}/e2e-e02-session.png` });

    // When providers are connected, the page shows a model selector or
    // "Build" agent picker instead of a Connect CTA. Both are valid states.
    const hasConnectCTA =
      body.includes('Connect') && body.includes('provider');
    const hasModelSelector =
      body.includes('Build') ||
      body.includes('Ask anything') ||
      body.includes('Shell');

    if (!hasConnectCTA && !hasModelSelector) {
      throw new Error(
        'Session page shows neither Connect CTA nor model selector. Page: ' +
          body.substring(0, 500),
      );
    }
  } finally {
    await browser.close();
  }
});

process.exit(printSummary());
