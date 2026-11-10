import { defineConfig } from '@playwright/test';
import { execFileSync } from 'node:child_process';
export default defineConfig({
  testDir: './tests', testMatch: ['navigation.spec.ts', 'mobile.spec.ts'], workers: 1,
  outputDir: '/tmp/cortex-navigation-results', reporter: 'list',
  use: { baseURL: 'http://127.0.0.1:15173', viewport: { width: 1280, height: 800 },
    launchOptions: { executablePath: execFileSync('which', ['chromium'], { encoding: 'utf8' }).trim() } },
  webServer: { command: 'pnpm exec vite --host 127.0.0.1 --port 15173 --strictPort', port: 15173, reuseExistingServer: true },
});
