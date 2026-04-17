import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  timeout: 30000,
  use: {
    baseURL: 'http://localhost:8080',
    headless: true,
  },
  webServer: {
    command: 'go run ./cmd/playwrightserver --port 8080',
    cwd: '.',
    port: 8080,
    reuseExistingServer: !process.env.CI,
    timeout: 180000,
  },
});
