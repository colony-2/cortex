import { defineConfig } from '@playwright/test';
import { execFileSync } from 'node:child_process';
const chromium = process.env.CORTEX_CHROMIUM || execFileSync('which', ['chromium'], { encoding: 'utf8' }).trim();
export default defineConfig({
  testDir: './tests', testMatch: 'job-lease-timeline.spec.ts', workers: 1,
  timeout: 30000, reporter: 'list', outputDir: '/tmp/cortex-timeline-checks/browser',
  use: { baseURL: 'http://127.0.0.1:15177', headless: true, launchOptions: { executablePath: chromium }, trace: 'retain-on-failure' },
  webServer: { command: 'pnpm exec vite --host 127.0.0.1 --port 15177 --strictPort', url: 'http://127.0.0.1:15177', reuseExistingServer: false },
});
