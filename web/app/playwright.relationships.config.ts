import { defineConfig } from '@playwright/test';
import { execFileSync } from 'node:child_process';

// The relationship browser suite uses deterministic HTTP fixtures; actual backend
// filtering is covered by TestChildrenRuntimeIntegration independently.
const chromium = process.env.CORTEX_CHROMIUM || (() => {
  try { return execFileSync('which', ['chromium'], { encoding: 'utf8' }).trim(); } catch { return undefined; }
})();
export default defineConfig({
  testDir: './tests', testMatch: 'job-relationships.spec.ts',
  timeout: 30000, workers: 1, reporter: 'list',
  outputDir: process.env.CORTEX_RELATIONSHIP_TEST_OUTPUT || '/tmp/cortex-relationship-browser-results',
  use: { baseURL: 'http://127.0.0.1:15175', headless: true, launchOptions: chromium ? { executablePath: chromium } : {} },
  webServer: { command: 'pnpm exec vite --host 127.0.0.1 --port 15175 --strictPort', url: 'http://127.0.0.1:15175', reuseExistingServer: false },
});
