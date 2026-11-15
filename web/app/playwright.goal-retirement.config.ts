import { defineConfig } from '@playwright/test';
import base from './playwright.config';

// Exercise the locally built embedded production UI with stale HTTP fixtures.
export default defineConfig({
  ...base, testMatch: 'goal-retirement.spec.ts', workers: 1, reporter: 'list',
  outputDir: '/tmp/cortex-retirement-browser',
  projects: [{ name: 'retirement-production', use: { baseURL: 'http://127.0.0.1:18087' } }],
  webServer: [{ command: 'make build && ./build/cortex serve --addr 127.0.0.1:18087 --jobdb http://127.0.0.1:1/retirement --working-dir web/app/tests/fixtures/cell', cwd: '../..', url: 'http://127.0.0.1:18087/api/health', timeout: 180_000, reuseExistingServer: false }],
});
