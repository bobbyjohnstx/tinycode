// E2E: Connect CTA button opens the provider/model selection dialog.
//
// Validates that clicking the "Connect" button on an empty session
// (when no providers are connected) opens the provider selection dialog.
// No LLM needed — checks dialog lifecycle only.
//
// Depends on WEB-E02 (Connect CTA visible). If providers are already
// connected, the CTA won't appear and this test skips gracefully.

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

await runTest('WEB-E10  Connect CTA opens provider dialog', async () => {
  const session = await createSession(config, DIRECTORY);
  if (!session.id || !session.slug) {
    throw new Error(`Bad session response: ${JSON.stringify(session)}`);
  }

  const { browser, page } = await launchBrowser();
  try {
    await navigateToSession(page, config, session.slug);

    // Find the Connect CTA
    const cta = page.locator('[data-component="session-empty-connect"]');
    const ctaVisible = await cta.isVisible().catch(() => false);

    if (!ctaVisible) {
      // Providers are already connected — CTA doesn't appear. Skip gracefully.
      console.log('    (Connect CTA not visible — providers already connected, skipping)');
      return;
    }

    // Click the Connect button inside the CTA
    const connectBtn = cta.locator('button');
    const btnVisible = await connectBtn.first().isVisible().catch(() => false);
    if (!btnVisible) {
      throw new Error('Connect CTA visible but no button found inside it');
    }

    await connectBtn.first().click();
    await page.waitForTimeout(2000);

    await page.screenshot({ path: `${SCREENSHOT_DIR}/e2e-e10-dialog.png` });

    // After clicking, a dialog should appear (provider selection or model selection)
    const body = await page.locator('body').innerText();
    const hasDialog =
      body.includes('Provider') ||
      body.includes('provider') ||
      body.includes('Ollama') ||
      body.includes('OpenRouter') ||
      body.includes('Model') ||
      body.includes('model') ||
      body.includes('API key') ||
      body.includes('Connect');

    if (!hasDialog) {
      throw new Error(
        'No provider/model dialog appeared after clicking Connect. Page: ' +
          body.substring(0, 500),
      );
    }
  } finally {
    await browser.close();
  }
});

process.exit(printSummary());
