import { defineConfig } from '@playwright/test';
import base from './playwright.artifact.config';

export default defineConfig({
  ...base,
  // Fresh CI worktrees serve and transform the full module graph on navigation.
  // Keep action/assertion limits while allowing that cold load to finish.
  use: { ...base.use, navigationTimeout: process.env.CI ? 30_000 : 15_000 },
  outputDir: '/tmp/cortex-review-history-results',
  workers: 1,
  timeout: 90_000,
  projects: [{ name: 'review-history', testMatch: ['review-document-history.spec.ts','review-scroll.spec.ts','review-v2-usability.spec.ts','input-discovery.spec.ts','job-relationships.spec.ts','mobile.spec.ts','navigation.spec.ts','review-queue.spec.ts','story-scroll.spec.ts'], use: { baseURL: 'http://127.0.0.1:15177' } }],
  webServer: [{
    command: 'pnpm exec vite --host 127.0.0.1 --port 15177 --strictPort',
    url: 'http://127.0.0.1:15177', reuseExistingServer: false,
    env: { VITE_CORTEX_API_BASE: '/api' },
  }],
});
