// E2E: Slash command API endpoints work without a browser.
//
// /warnings, /instructions, and /chdir are TUI-side commands processed
// by the Go server. The web SPA doesn't support typing them into the
// chat input. Test via the /command API endpoint instead.

import { getConfig, api, createSession, runTest, printSummary } from './helpers.mjs';

const DIRECTORY = process.env.E2E_PROJECT_DIR || process.cwd();

const config = getConfig();

await runTest('WEB-E12  /warnings command via API', async () => {
  const session = await createSession(config, DIRECTORY);
  const resp = await api(config, 'POST', `/session/${session.id}/command`, {
    command: 'warnings',
  });
  // Command endpoint returns 200 on success (even if warnings list is empty)
  if (resp.status !== 200 && resp.status !== 202) {
    throw new Error(`/command warnings returned ${resp.status}: ${JSON.stringify(resp.data)}`);
  }
});

await runTest('WEB-E13  /instructions command via API', async () => {
  const session = await createSession(config, DIRECTORY);
  const resp = await api(config, 'POST', `/session/${session.id}/command`, {
    command: 'instructions',
  });
  if (resp.status !== 200 && resp.status !== 202) {
    throw new Error(
      `/command instructions returned ${resp.status}: ${JSON.stringify(resp.data)}`,
    );
  }
});

await runTest('WEB-E14  Session directory update via PATCH', async () => {
  const session = await createSession(config, DIRECTORY);

  // Update directory via PATCH
  const resp = await api(config, 'PATCH', `/session/${session.id}`, {
    directory: '/tmp',
  });
  if (resp.status !== 200 && resp.status !== 202) {
    throw new Error(`PATCH directory returned ${resp.status}: ${JSON.stringify(resp.data)}`);
  }

  // Verify it persisted
  const getResp = await api(config, 'GET', `/session/${session.id}`);
  if (getResp.status !== 200) {
    throw new Error(`GET session returned ${getResp.status}`);
  }

  const dir = getResp.data.directory;
  // macOS resolves /tmp → /private/tmp
  if (!dir.includes('tmp')) {
    throw new Error(`Expected directory to contain 'tmp', got: ${dir}`);
  }
});

process.exit(printSummary());
