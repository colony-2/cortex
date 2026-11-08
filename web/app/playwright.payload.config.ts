import { defineConfig } from '@playwright/test';
export default defineConfig({
  testDir: './tests', testMatch: 'node-payload.spec.ts',
  use: { baseURL: 'http://127.0.0.1:3000', launchOptions: { executablePath: process.env.CHROMIUM_PATH } },
  webServer: { command: 'pnpm dev --host 127.0.0.1', url: 'http://127.0.0.1:3000', reuseExistingServer: true },
});
