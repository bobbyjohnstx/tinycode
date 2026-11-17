// E2E: Slash commands that produce UI feedback without an LLM.
//
// Tests /warnings, /instructions, and /chdir — all client-side commands
// that show toasts or dialogs. No model connection needed.

import {
  getConfig,
  createSession,
  launchBrowser,
  navigateToSession,
  findInput,
  typeAndSend,
  runTest,
  printSummary,
} from './helpers.mjs';

const DIRECTORY = process.env.E2E_PROJECT_DIR || process.cwd();
const SCREENSHOT_DIR = process.env.E2E_SCREENSHOT_DIR || '/tmp';

const config = getConfig();

await runTest('WEB-E12  /warnings shows dialog', async () => {
  const session = await createSession(config, DIRECTORY);
  const { browser, page } = await launchBrowser();
  try {
    await navigateToSession(page, config, session.slug);
    const input = await findInput(page);
    if (!input) throw new Error('Input not found');

    await typeAndSend(page, input, '/warnings');
    await page.waitForTimeout(1500);

    await page.screenshot({ path: `${SCREENSHOT_DIR}/e2e-e12-warnings.png` });

    const body = await page.locator('body').innerText();
    const hasWarnings = body.includes('Warnings') || body.includes('No warnings');
    if (!hasWarnings) {
      throw new Error('Expected warnings dialog or empty state, got: ' + body.substring(0, 300));
    }
  } finally {
    await browser.close();
  }
});

await runTest('WEB-E13  /instructions shows dialog', async () => {
  const session = await createSession(config, DIRECTORY);
  const { browser, page } = await launchBrowser();
  try {
    await navigateToSession(page, config, session.slug);
    const input = await findInput(page);
    if (!input) throw new Error('Input not found');

    await typeAndSend(page, input, '/instructions');
    await page.waitForTimeout(1500);

    await page.screenshot({ path: `${SCREENSHOT_DIR}/e2e-e13-instructions.png` });

    const body = await page.locator('body').innerText();
    const hasInstructions =
      body.includes('Instructions') ||
      body.includes('CLAUDE.md') ||
      body.includes('AGENTS.md') ||
      body.includes('No instruction');
    if (!hasInstructions) {
      throw new Error(
        'Expected instructions dialog content, got: ' + body.substring(0, 300),
      );
    }
  } finally {
    await browser.close();
  }
});

await runTest('WEB-E14  /chdir updates status bar', async () => {
  const session = await createSession(config, DIRECTORY);
  const { browser, page } = await launchBrowser();
  try {
    await navigateToSession(page, config, session.slug);
    const input = await findInput(page);
    if (!input) throw new Error('Input not found');

    await typeAndSend(page, input, '/chdir /tmp');
    await page.waitForTimeout(1500);

    await page.screenshot({ path: `${SCREENSHOT_DIR}/e2e-e14-chdir.png` });

    const body = await page.locator('body').innerText();
    // After /chdir /tmp, the status bar or a toast should reference /tmp
    // or private/tmp (macOS symlink resolution)
    const hasNewDir = body.includes('/tmp') || body.includes('private/tmp');
    if (!hasNewDir) {
      throw new Error('Expected /tmp in page after /chdir, got: ' + body.substring(0, 300));
    }
  } finally {
    await browser.close();
  }
});

process.exit(printSummary());
