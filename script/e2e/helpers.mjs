// Reusable helpers for Playwright-based web UI E2E tests.
//
// Prerequisites:
//   npx playwright install chromium
//
// Environment:
//   TINYCODE_WEB_TOKEN   Auth token (reads from data dir if not set)
//   TINYCODE_WEB_PORT    Server port (default: 4096)
//   TINYCODE_WEB_HOST    Server host (default: 127.0.0.1)

import { chromium } from 'playwright';
import { readFileSync } from 'fs';
import { join } from 'path';
import { homedir } from 'os';

// --- Config ---

export function getConfig() {
  const host = process.env.TINYCODE_WEB_HOST || '127.0.0.1';
  const port = process.env.TINYCODE_WEB_PORT || '4096';
  const base = `http://${host}:${port}`;
  const token = resolveToken();
  const authParam = Buffer.from(`tinycode:${token}`).toString('base64');
  const headers = {
    Authorization: `Basic ${authParam}`,
    'Content-Type': 'application/json',
  };
  return { base, token, authParam, headers };
}

function resolveToken() {
  if (process.env.TINYCODE_WEB_TOKEN) return process.env.TINYCODE_WEB_TOKEN;
  const paths = [
    join(homedir(), 'Library', 'Application Support', 'tinycode', 'web_token'),
    join(homedir(), '.config', 'tinycode', 'web_token'),
  ];
  for (const p of paths) {
    try {
      const t = readFileSync(p, 'utf8').trim();
      if (t.length === 64) return t;
    } catch {}
  }
  throw new Error(
    'No web token found. Set TINYCODE_WEB_TOKEN or run `tinycode web` once to generate one.',
  );
}

// --- API helpers ---

export async function api(config, method, path, body) {
  const url = `${config.base}${path}`;
  const resp = await fetch(url, {
    method,
    headers: config.headers,
    body: body ? JSON.stringify(body) : undefined,
  });
  return { status: resp.status, data: await resp.json().catch(() => null) };
}

export async function createSession(config, directory, model) {
  const qs = `?directory=${encodeURIComponent(directory)}`;
  const body = model ? { model } : {};
  const resp = await api(config, 'POST', `/session${qs}`, body);
  if (resp.status !== 200 && resp.status !== 201) {
    throw new Error(`Failed to create session: ${resp.status} ${JSON.stringify(resp.data)}`);
  }
  return resp.data;
}

// --- Browser helpers ---

export async function launchBrowser(opts = {}) {
  const browser = await chromium.launch({
    headless: opts.headless !== false,
  });
  const page = await browser.newPage();

  const errors = [];
  page.on('console', (msg) => {
    if (msg.type() === 'error') errors.push(msg.text());
  });

  return { browser, page, errors };
}

export async function navigateToSession(page, config, sessionSlug) {
  const url = `${config.base}/${sessionSlug}/session?auth_token=${config.authParam}`;
  await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 15000 });
  await page.waitForTimeout(3000);
}

// --- Input interaction ---

export async function findInput(page) {
  const editable = page.locator('[contenteditable="true"]').first();
  const textarea = page.locator('textarea').first();

  if (await editable.isVisible().catch(() => false)) return editable;
  if (await textarea.isVisible().catch(() => false)) return textarea;
  return null;
}

export async function typeAndSend(page, input, text) {
  await input.click();
  await page.keyboard.type(text, { delay: 30 });
  await page.waitForTimeout(300);

  // Try send button first (aria-label or title containing "send")
  const buttons = await page.locator('button').all();
  for (const btn of buttons) {
    const label = (await btn.getAttribute('aria-label').catch(() => '')) || '';
    const title = (await btn.getAttribute('title').catch(() => '')) || '';
    if (label.toLowerCase().includes('send') || title.toLowerCase().includes('send')) {
      await btn.click();
      return;
    }
  }

  // Fallback: Enter key
  await page.keyboard.press('Enter');
}

// --- Response detection ---

export async function waitForResponse(page, sentText, timeoutSec = 60) {
  for (let i = 0; i < timeoutSec; i++) {
    await page.waitForTimeout(1000);
    const body = await page.locator('body').innerText();

    if (body.includes('Select an agent and model')) {
      throw new Error('No model selected — "Select an agent and model" toast appeared');
    }

    const lines = body.split('\n').filter((l) => l.trim().length > 0);
    const idx = lines.findIndex((l) => l.includes(sentText));
    if (idx >= 0 && lines.length > idx + 2) {
      const after = lines
        .slice(idx + 1)
        .join(' ')
        .trim();
      if (
        after.length > 20 &&
        !after.startsWith('Ask anything') &&
        !after.includes('Select model')
      ) {
        return { found: true, elapsed: i + 1, content: after.substring(0, 500) };
      }
    }
  }
  return { found: false, elapsed: timeoutSec, content: '' };
}

// --- Test runner ---

const results = [];
let pass = 0;
let fail = 0;

export async function runTest(name, fn) {
  const filter = process.env.E2E_FILTER;
  if (filter && !name.toLowerCase().includes(filter.toLowerCase())) return;

  process.stdout.write(`  ${name.padEnd(50)}`);
  const start = Date.now();
  try {
    await fn();
    const ms = Date.now() - start;
    console.log(`\x1b[32mPASS\x1b[0m (${(ms / 1000).toFixed(1)}s)`);
    pass++;
    results.push({ name, status: 'PASS', ms });
  } catch (err) {
    const ms = Date.now() - start;
    console.log(`\x1b[31mFAIL\x1b[0m (${(ms / 1000).toFixed(1)}s)`);
    console.log(`    ${err.message}`);
    fail++;
    results.push({ name, status: 'FAIL', ms, error: err.message });
  }
}

export function printSummary() {
  console.log('');
  console.log('====================================');
  console.log(`  \x1b[32m${pass} passed\x1b[0m  \x1b[31m${fail} failed\x1b[0m`);
  console.log('');
  for (const r of results) {
    const tag = r.status === 'PASS' ? '\x1b[32mPASS\x1b[0m' : '\x1b[31mFAIL\x1b[0m';
    console.log(`  ${tag}  ${r.name} (${(r.ms / 1000).toFixed(1)}s)`);
  }
  console.log('');
  return fail === 0 ? 0 : 1;
}
