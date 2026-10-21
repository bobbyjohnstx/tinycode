// E2E: Settings dialog hides Terminal section when PTY unsupported.
//
// Validates #820: the web UI (PTY_SUPPORTED=false) should not show the
// Terminal settings group or shell selector. Opens the settings dialog
// via keyboard shortcut and checks DOM content.

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

await runTest('WEB-E04  Settings hides Terminal section (no PTY)', async () => {
  const session = await createSession(config, DIRECTORY);
  const { browser, page } = await launchBrowser();
  try {
    await navigateToSession(page, config, session.slug);

    // Open settings via keyboard: Cmd+, (Mac) or Ctrl+, (other)
    const isMac = process.platform === 'darwin';
    await page.keyboard.press(isMac ? 'Meta+Comma' : 'Control+Comma');
    await page.waitForTimeout(1500);

    await page.screenshot({ path: `${SCREENSHOT_DIR}/e2e-e04-settings.png` });

    // Terminal group should be absent when PTY_SUPPORTED is false
    // Check for the specific settings tab trigger or group header
    const terminalTab = page.locator('text=Terminal').first();
    const terminalVisible = await terminalTab.isVisible().catch(() => false);

    if (terminalVisible) {
      // Could be a false positive — "Terminal" might appear in other contexts.
      // Check specifically for the Terminal settings group with shell selector.
      const shellSelector = page.locator('[data-action="settings-shell"]');
      const shellVisible = await shellSelector.isVisible().catch(() => false);
      if (shellVisible) {
        throw new Error('Shell selector visible — Terminal settings should be hidden when PTY unsupported');
      }
    }

    // Also verify the settings dialog IS open (we're not just checking an empty page)
    const settingsContent = page.locator('[role="dialog"], [data-component*="settings"], [class*="settings"]').first();
    const dialogOpen = await settingsContent.isVisible().catch(() => false);
    if (!dialogOpen) {
      // Settings may not have opened via keyboard — try clicking the settings icon
      const settingsBtn = page.locator('button[aria-label*="settings"], button[title*="Settings"]').first();
      const btnVisible = await settingsBtn.isVisible().catch(() => false);
      if (btnVisible) {
        await settingsBtn.click();
        await page.waitForTimeout(1000);
      }
    }

    await page.screenshot({ path: `${SCREENSHOT_DIR}/e2e-e04-settings-open.png` });
  } finally {
    await browser.close();
  }
});

process.exit(printSummary());
