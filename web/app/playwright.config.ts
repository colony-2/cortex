import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: [['html', { open: 'never' }]],
  timeout: 30000, // 30s per test
  expect: {
    timeout: 15000 // 15s default timeout for expect assertions
  },
  use: {
    baseURL: 'http://localhost:5173',
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
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: [
    {
      command: 'moon run ui-app:serve',
      port: 5173,
      reuseExistingServer: !process.env.CI,
      timeout: 120 * 1000,
      cwd: '../..',
    },
    {
      command: 'server/api/build/testserver -n .example -p 8080',
      port: 8080,
      reuseExistingServer: !process.env.CI,
      timeout: 120 * 1000,
      cwd: '../..',
    }
  ],
});