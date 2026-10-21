// E2E: Settings dialog tabs render correctly.
//
// Opens the settings dialog and verifies that the expected tab labels
// (General, Shortcuts) are present. No LLM needed — pure UI check.

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

await runTest('WEB-E07  Settings dialog tabs render', async () => {
  const session = await createSession(config, DIRECTORY);
  if (!session.id || !session.slug) {
    throw new Error(`Bad session response: ${JSON.stringify(session)}`);
  }

  const { browser, page } = await launchBrowser();
  try {
    await navigateToSession(page, config, session.slug);

    // Open settings via keyboard (Cmd/Ctrl + ,)
    await page.keyboard.press('Meta+,');
    await page.waitForTimeout(2000);

    await page.screenshot({ path: `${SCREENSHOT_DIR}/e2e-e07-settings.png` });

    const body = await page.locator('body').innerText();

    // Check for General tab
    const hasGeneral = body.includes('General');
    if (!hasGeneral) {
      // Fallback: try Ctrl+, (Linux/Windows)
      await page.keyboard.press('Control+,');
      await page.waitForTimeout(2000);
      await page.screenshot({ path: `${SCREENSHOT_DIR}/e2e-e07-settings-retry.png` });
    }

    const finalBody = await page.locator('body').innerText();

    if (!finalBody.includes('General')) {
      throw new Error(`Settings dialog missing "General" tab. Body: ${finalBody.substring(0, 300)}`);
    }

    if (!finalBody.includes('Shortcuts')) {
      throw new Error(`Settings dialog missing "Shortcuts" tab. Body: ${finalBody.substring(0, 300)}`);
    }

    // Verify Terminal tab is absent (PTY_SUPPORTED = false in web)
    const terminalTab = await page.locator('[data-action="settings-shell"]').isVisible().catch(() => false);
    if (terminalTab) {
      throw new Error('Terminal shell selector should be hidden in web mode');
    }
  } finally {
    await browser.close();
  }
});

process.exit(printSummary());
