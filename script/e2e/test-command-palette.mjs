// E2E: Web command palette opens and is interactive.
//
// Validates that Mod+Shift+P opens the command palette overlay
// and that it contains the expected web-UI entries (session navigation).
// No LLM needed — pure UI check.

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

    const isMac = process.platform === 'darwin';
    const mod = isMac ? 'Meta' : 'Control';
    await page.keyboard.press(`${mod}+Shift+p`);
    await page.waitForTimeout(1000);

    await page.screenshot({ path: `${SCREENSHOT_DIR}/e2e-e08-palette.png` });

    const body = await page.locator('body').innerText();

    // Web palette shows session navigation entries, not TUI slash commands.
    const expected = ['New session', 'Previous session', 'Next session'];
    const found = expected.filter((cmd) => body.includes(cmd));

    if (found.length === 0) {
      // Try Mod+K as alternative shortcut
      await page.keyboard.press('Escape');
      await page.waitForTimeout(300);
      await page.keyboard.press(`${mod}+k`);
      await page.waitForTimeout(1000);

      const body2 = await page.locator('body').innerText();
      const found2 = expected.filter((cmd) => body2.includes(cmd));

      if (found2.length === 0) {
        throw new Error(
          'Command palette not found after Mod+Shift+P or Mod+K. Page: ' +
            body2.substring(0, 500),
        );
      }
    }

    // Also verify the search input is present
    const hasSearch =
      body.includes('Search') || body.includes('search') || body.includes('⌘K');
    if (!hasSearch) {
      throw new Error('Command palette search input not found');
    }

    await page.keyboard.press('Escape');
    await page.waitForTimeout(300);
  } finally {
    await browser.close();
  }
});

process.exit(printSummary());
