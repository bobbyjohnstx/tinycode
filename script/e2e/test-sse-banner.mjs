// E2E: SSE connection banner appears on disconnect (#818).
//
// TODO: This test requires killing and restarting the tinycode server
// mid-test to trigger the SSE reconnection path. The current E2E
// harness does not support server lifecycle orchestration.
//
// To implement:
// 1. Navigate to a session page (server running)
// 2. Kill the tinycode web process
// 3. Wait for the SSE banner to appear ("reconnecting" or "offline")
// 4. Restart the server
// 5. Wait for the banner to clear
//
// Blocked on: server kill/restart orchestration in the test harness.

import { runTest, printSummary } from './helpers.mjs';

await runTest('WEB-E05  SSE banner on disconnect (STUB)', async () => {
  // Skip — requires server lifecycle control not yet available in harness.
  console.log('    SKIP: requires server kill/restart orchestration');
});

process.exit(printSummary());
