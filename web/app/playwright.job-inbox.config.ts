import { defineConfig } from '@playwright/test';
import config from './playwright.config';

// Browser journeys use deterministic API fixtures; no runtime changes required.
export default defineConfig({
  ...config,
  timeout: 60_000,
  testMatch: ['job-inbox.spec.ts'],
  projects: config.projects?.filter(project => project.name === 'chromium'),
  webServer: Array.isArray(config.webServer) ? [config.webServer[3]] : undefined,
});
