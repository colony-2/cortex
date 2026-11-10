import { defineConfig, devices } from '@playwright/test';
import base from './playwright.config';

// Local API fixtures exercise the real review/story hosts without JobDB or Go.
// This separate project deliberately includes markup coverage excluded elsewhere.
export default defineConfig({
  ...base,
  outputDir: process.env.CORTEX_MARKUP_OUTPUT || '/tmp/cortex-markup-results',
  reporter: [['list']],
  workers: 1,
  projects: [{ name: 'markup-chromium', testMatch: ['markup-preview.spec.ts', 'html-preview-security.spec.ts', 'markup-retained.spec.ts'], use: { ...devices['Desktop Chrome'] } }],
  webServer: [{command: 'node tests/fixtures/markup-server.mjs', url: 'http://127.0.0.1:15175/__probes'}, { command: 'pnpm exec vite --host 127.0.0.1 --port 15173 --strictPort', url: 'http://127.0.0.1:15173', reuseExistingServer: false, env: {CORTEX_API_URL:'http://127.0.0.1:15175'} }],
});
