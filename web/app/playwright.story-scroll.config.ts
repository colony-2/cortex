import { defineConfig } from '@playwright/test';
import base from './playwright.artifact.config';

// Story geometry and navigation need only deterministic HTTP fixtures.
export default defineConfig({
  ...base,
  outputDir: process.env.CORTEX_SCROLL_TEST_OUTPUT || '/tmp/cortex-scroll-results',
  projects: [{ name: 'scroll', testMatch: 'story-scroll.spec.ts', use: { baseURL: 'http://127.0.0.1:15176' } }],
  webServer: [{
    command: 'pnpm exec vite --host 127.0.0.1 --port 15176 --strictPort',
    url: 'http://127.0.0.1:15176', reuseExistingServer: false,
  }],
});
