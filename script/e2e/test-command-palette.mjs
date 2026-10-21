// E2E: Web command palette opens and is interactive.
//
// Validates that Mod+Shift+P (or Mod+K) opens the command palette overlay
// and that it contains actionable entries. No LLM needed — pure UI check.

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

await runTest('WEB-E08  Command palette opens with Mod+Shift+P', async () => {
  const session = await createSession(config, DIRECTORY);
  if (!session.id || !session.slug) {
    throw new Error(`Bad session response: ${JSON.stringify(session)}`);
  }

  const { browser, page } = await launchBrowser();
  try {
    await navigateToSession(page, config, session.slug);

    // Trigger command palette via keyboard shortcut
    const isMac = process.platform === 'darwin';
    const mod = isMac ? 'Meta' : 'Control';
    await page.keyboard.press(`${mod}+Shift+p`);
    await page.waitForTimeout(1000);

    await page.screenshot({ path: `${SCREENSHOT_DIR}/e2e-e08-palette.png` });

    // Look for the palette overlay — typically a search input + command list
    const body = await page.locator('body').innerText();

    // Command palette should show command names or a search input
    const hasPaletteContent =
      body.includes('compact') ||
      body.includes('connect') ||
      body.includes('theme') ||
      body.includes('help') ||
      body.includes('archive');

    if (!hasPaletteContent) {
      // Try Mod+K as alternative
      await page.keyboard.press('Escape');
      await page.waitForTimeout(300);
      await page.keyboard.press(`${mod}+k`);
      await page.waitForTimeout(1000);

      const body2 = await page.locator('body').innerText();
      const hasAlt =
        body2.includes('compact') ||
        body2.includes('connect') ||
        body2.includes('theme');

      if (!hasAlt) {
        throw new Error(
          'Command palette not found after Mod+Shift+P or Mod+K. Page: ' +
            body2.substring(0, 500),
        );
      }
    }

    // Dismiss
    await page.keyboard.press('Escape');
    await page.waitForTimeout(300);
  } finally {
    await browser.close();
  }
});

process.exit(printSummary());
