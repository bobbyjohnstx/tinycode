// E2E: Scratch session lifecycle via API.
//
// Tests creating and deleting scratch sessions through the API.
// /scratch is a TUI-side command — the web SPA doesn't support it
// directly, so this exercises the API surface instead.

import { getConfig, api, createSession, runTest, printSummary } from './helpers.mjs';

const DIRECTORY = process.env.E2E_PROJECT_DIR || process.cwd();

const config = getConfig();

await runTest('WEB-E15  Create and get scratch session via API', async () => {
  // Create a parent session
  const parent = await createSession(config, DIRECTORY);

  // Fork it to simulate a scratch thread (scratch uses fork with parent_id)
  const forkResp = await api(config, 'POST', `/session/${parent.id}/fork`, {});
  if (forkResp.status !== 200 && forkResp.status !== 201) {
    throw new Error(`Fork returned ${forkResp.status}: ${JSON.stringify(forkResp.data)}`);
  }

  const childID = forkResp.data.id;
  if (!childID) {
    throw new Error('Fork response missing id');
  }

  // Verify the child session exists and references parent
  const getResp = await api(config, 'GET', `/session/${childID}`);
  if (getResp.status !== 200) {
    throw new Error(`GET child returned ${getResp.status}`);
  }

  if (getResp.data.parentID !== parent.id) {
    throw new Error(
      `Expected parentId=${parent.id}, got ${getResp.data.parentID}`,
    );
  }
});

await runTest('WEB-E16  Delete scratch session via API', async () => {
  const parent = await createSession(config, DIRECTORY);
  const forkResp = await api(config, 'POST', `/session/${parent.id}/fork`, {});
  if (forkResp.status !== 200 && forkResp.status !== 201) {
    throw new Error(`Fork returned ${forkResp.status}`);
  }

  const childID = forkResp.data.id;

  // Delete the child (simulates /scratch close discard)
  const delResp = await api(config, 'DELETE', `/session/${childID}`);
  if (delResp.status !== 204) {
    throw new Error(`DELETE returned ${delResp.status}`);
  }

  // Verify it's gone
  const getResp = await api(config, 'GET', `/session/${childID}`);
  if (getResp.status !== 404) {
    throw new Error(`Expected 404 after delete, got ${getResp.status}`);
  }
});

process.exit(printSummary());
