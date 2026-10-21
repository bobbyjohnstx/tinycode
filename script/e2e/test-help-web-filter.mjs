// E2E: Help endpoint with ?client=web omits TUI-only content.
//
// Pure API test — no browser needed. Validates #826: the /help?client=web
// endpoint filters out TUI leader keybindings and desktop-only features.

import { getConfig, api, runTest, printSummary } from './helpers.mjs';

const config = getConfig();

await runTest('WEB-E03  Help web filter omits TUI leader keys', async () => {
  const resp = await api(config, 'GET', '/help?client=web');

  if (resp.status !== 200) {
    throw new Error(`/help?client=web returned ${resp.status}`);
  }

  const { keybindings, features } = resp.data;

  if (!Array.isArray(keybindings) || keybindings.length === 0) {
    throw new Error('keybindings array is empty or missing');
  }

  if (!Array.isArray(features) || features.length === 0) {
    throw new Error('features array is empty or missing');
  }

  // Assert no TUI leader keybindings
  const tuiLeader = keybindings.filter((kb) => kb.category === 'TUI leader');
  if (tuiLeader.length > 0) {
    throw new Error(
      `Found ${tuiLeader.length} TUI leader keybinding(s): ${tuiLeader.map((k) => k.key).join(', ')}`,
    );
  }

  // Assert excluded features are absent
  const excluded = ['MCP Servers', 'Which-Key Panel', 'Terminal Bell'];
  for (const name of excluded) {
    const found = features.find((f) => f.name === name);
    if (found) {
      throw new Error(`Feature "${name}" should be filtered from web help`);
    }
  }
});

// Also verify the default (non-web) endpoint still has TUI leader keys
await runTest('WEB-E03b Default help includes TUI leader keys', async () => {
  const resp = await api(config, 'GET', '/help');

  if (resp.status !== 200) {
    throw new Error(`/help returned ${resp.status}`);
  }

  const tuiLeader = resp.data.keybindings.filter((kb) => kb.category === 'TUI leader');
  if (tuiLeader.length === 0) {
    throw new Error('Default /help should include TUI leader keybindings');
  }
});

process.exit(printSummary());
