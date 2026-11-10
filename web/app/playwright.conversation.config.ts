import { defineConfig } from '@playwright/test';
import config from './playwright.config';
// Conversation UI fixtures intercept all API calls; only the web app is needed.
export default defineConfig({
  ...config,
  reporter: 'list',
  outputDir: process.env.PLAYWRIGHT_OUTPUT_DIR || './test-results',
  testMatch: 'job-conversation.spec.ts',
  projects: config.projects?.filter(p => p.name === 'chromium'),
  webServer: [{ command: 'pnpm exec vite --host 127.0.0.1 --port 15173 --strictPort', url: 'http://127.0.0.1:15173', cwd: '.', reuseExistingServer: false }],
});
