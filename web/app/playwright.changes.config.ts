import { defineConfig } from '@playwright/test';
import base from './playwright.config';

export default defineConfig({
  ...base,
  testMatch: 'review-changes.spec.ts', fullyParallel: false, workers: 1, timeout: 60_000,
  reporter: 'list',
  projects: [{ name: 'recorded-changes', use: { ...base.use, baseURL: 'http://127.0.0.1:18086' }, testMatch: 'review-changes.spec.ts' }],
  webServer: [{
    command: 'make web-embed && bash scripts/go.sh run ./server/cmd/changestestserver --addr 127.0.0.1:18086',
    url: 'http://127.0.0.1:18086/api/health', cwd: '../..', reuseExistingServer: false, timeout: 180_000,
  }],
});
