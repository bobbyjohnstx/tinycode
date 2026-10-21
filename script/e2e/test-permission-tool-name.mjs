// E2E: Permission dock shows tool name instead of generic "Permission" (#823).
//
// TODO: This test requires an LLM-connected session that triggers a tool
// call (e.g., shell/bash) to generate a permission prompt. The permission
// dock should display the tool name (e.g., "shell") in the header instead
// of the generic "Permission required" title.
//
// To implement:
// 1. Create a session with a connected LLM
// 2. Send a prompt that triggers a shell tool call
// 3. Wait for the permission dock to appear
// 4. Assert the dock header contains the tool name (not just "Permission required")
// 5. Dismiss the permission prompt
//
// Blocked on: LLM-connected test infrastructure for web E2E.

import { runTest, printSummary } from './helpers.mjs';

await runTest('WEB-E06  Permission dock shows tool name (STUB)', async () => {
  // Skip — requires LLM to trigger tool call and permission prompt.
  console.log('    SKIP: requires LLM-connected test infrastructure');
});

process.exit(printSummary());
