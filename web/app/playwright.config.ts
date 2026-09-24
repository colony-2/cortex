import { defineConfig, devices } from '@playwright/test';

const webURL = 'http://127.0.0.1:15173';
const apiURL = 'http://127.0.0.1:18081';

export default defineConfig({
  testDir: './tests',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: [['list'], ['html', { open: 'never' }]],
  timeout: 30000, // 30s per test
  expect: {
    timeout: 15000 // 15s default timeout for expect assertions
  },
  use: {
    baseURL: webURL,
    trace: 'on-first-retry',
    headless: true,
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
    actionTimeout: 15000, // 15s default timeout for actions like click, fill, etc.
    navigationTimeout: 15000 // 15s default timeout for page navigation
  },
  projects: [
    {
      name: 'chromium',
      testIgnore: 'recipe-input.spec.ts',
      use: { ...devices['Desktop Chrome'] },
    },
    {
      name: 'production',
      testMatch: ['api-integration.spec.ts', 'recipe-input.spec.ts'],
      use: { ...devices['Desktop Chrome'], baseURL: 'http://127.0.0.1:18083' },
    },
  ],
  webServer: [
    {
      command: 'bash scripts/go.sh run ./server/cmd/uitestserver --jobdb-only --sqlite --addr 127.0.0.1:18082',
      port: 18082,
      reuseExistingServer: false,
      timeout: 180_000,
      cwd: '../..',
    },
    {
      command: 'make build test-worker && exec ./build/cortex serve --addr 127.0.0.1:18083 --working-dir web/app/tests/fixtures/cell',
      url: 'http://127.0.0.1:18083/api/health',
      env: { C2J_JOBDB: '', JOBDB_URL: '', CORTEX_TENANT_ID: '' },
      reuseExistingServer: false,
      timeout: 180_000,
      cwd: '../..',
    },
    {
      command: 'bash scripts/go.sh run ./server/cmd/uitestserver --addr 127.0.0.1:18081 --working-dir web/app/tests/fixtures/cell',
      url: `${apiURL}/api/health`,
      reuseExistingServer: false,
      timeout: 180_000,
      cwd: '../..',
    },
    {
      command: 'pnpm exec vite --host 127.0.0.1 --port 15173 --strictPort',
      url: webURL,
      env: { CORTEX_API_URL: apiURL, VITE_CORTEX_API_BASE: '/api' },
      reuseExistingServer: false,
      timeout: 120_000,
      cwd: '.',
    },
  ],
});
