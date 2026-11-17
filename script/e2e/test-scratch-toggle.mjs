// E2E: Scratch thread toggle via /scratch command.
//
// Validates that /scratch opens a scratch session and /scratch close
// returns to the parent. No LLM needed — tests session lifecycle only.
//
// Requires a working session page in the web SPA. If the SPA crashes
// on session load, these tests fail with "Input not found".

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

await runTest('WEB-E15  /scratch opens scratch thread', async () => {
  const session = await createSession(config, DIRECTORY);
  const { browser, page } = await launchBrowser();
  try {
    await navigateToSession(page, config, session.slug);
    const input = await findInput(page);
    if (!input) throw new Error('Input not found');

    await typeAndSend(page, input, '/scratch');
    await page.waitForTimeout(2000);

    await page.screenshot({ path: `${SCREENSHOT_DIR}/e2e-e15-scratch-open.png` });

    const body = await page.locator('body').innerText();
    const hasScratch =
      body.includes('SCRATCH') ||
      body.includes('scratch') ||
      body.includes('Scratch');
    if (!hasScratch) {
      throw new Error(
        'Expected SCRATCH indicator or scratch state after /scratch, got: ' +
          body.substring(0, 300),
      );
    }
  } finally {
    await browser.close();
  }
});

await runTest('WEB-E16  /scratch close returns to parent', async () => {
  const session = await createSession(config, DIRECTORY);
  const { browser, page } = await launchBrowser();
  try {
    await navigateToSession(page, config, session.slug);
    const input = await findInput(page);
    if (!input) throw new Error('Input not found');

    // Open scratch
    await typeAndSend(page, input, '/scratch');
    await page.waitForTimeout(2000);

    // Close scratch
    const input2 = await findInput(page);
    if (!input2) throw new Error('Input not found after scratch open');
    await typeAndSend(page, input2, '/scratch close');
    await page.waitForTimeout(1500);

    await page.screenshot({ path: `${SCREENSHOT_DIR}/e2e-e16-scratch-close.png` });

    const body = await page.locator('body').innerText();
    // After closing, SCRATCH indicator should be gone
    const stillScratch = body.includes('SCRATCH');
    if (stillScratch) {
      throw new Error('SCRATCH indicator still visible after /scratch close');
    }
  } finally {
    await browser.close();
  }
});

process.exit(printSummary());
