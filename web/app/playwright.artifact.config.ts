import { defineConfig } from '@playwright/test';
import base from './playwright.config';

// Deterministic browser coverage of the real UI with HTTP fixtures; no JobDB build required.
export default defineConfig({
  ...base,
  projects: [{ name: 'artifact-viewer', use: { baseURL: 'http://127.0.0.1:15175' }, testMatch: 'artifact-viewer.spec.ts' }],
  webServer: [{
    command: 'node tests/helpers/artifactServer.mjs', port: 18084, reuseExistingServer: false,
  }, {
    command: 'node node_modules/vite/bin/vite.js --host 127.0.0.1 --port 15175 --strictPort',
    env: { CORTEX_API_URL: 'http://127.0.0.1:18084' },
    url: 'http://127.0.0.1:15175', reuseExistingServer: false, timeout: 120_000,
  }],
});
