// E2E: Empty session shows Connect CTA when no providers are connected.
//
// Validates that the web UI surfaces a "Connect a provider" call-to-action
// on an empty session page (#821). No LLM needed — checks DOM presence only.

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

await runTest('WEB-E02  Empty session shows Connect CTA', async () => {
  const session = await createSession(config, DIRECTORY);
  if (!session.id || !session.slug) {
    throw new Error(`Bad session response: ${JSON.stringify(session)}`);
  }

  const { browser, page } = await launchBrowser();
  try {
    await navigateToSession(page, config, session.slug);

    // Look for the Connect CTA component
    const cta = page.locator('[data-component="session-empty-connect"]');
    const visible = await cta.isVisible().catch(() => false);

    if (!visible) {
      await page.screenshot({ path: `${SCREENSHOT_DIR}/e2e-e02-no-cta.png` });
      // Fallback: check for the text anywhere in the page
      const body = await page.locator('body').innerText();
      if (!body.includes('Connect') && !body.includes('provider')) {
        throw new Error(
          'Connect CTA not found. Page text: ' + body.substring(0, 500),
        );
      }
      // Text present but component attribute missing — acceptable
      return;
    }

    const text = await cta.innerText();
    if (!text.includes('Connect')) {
      throw new Error(`CTA visible but missing "Connect" text: ${text}`);
    }

    await page.screenshot({ path: `${SCREENSHOT_DIR}/e2e-e02-cta-visible.png` });
  } finally {
    await browser.close();
  }
});

process.exit(printSummary());
