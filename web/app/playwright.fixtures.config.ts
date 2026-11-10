import { defineConfig } from '@playwright/test';
import config from './playwright.config';

// These tests supply HTTP fixtures and need only the browser UI server.
// Keep actual-runtime coverage in the normal configuration.
export default defineConfig({
  ...config,
  testMatch: ['review-queue.spec.ts', 'job-conversation.spec.ts', 'node-payload.spec.ts', 'story-scroll.spec.ts', 'mobile.spec.ts', 'job-relationships.spec.ts'],
  projects: config.projects?.filter(project => project.name === 'chromium'),
  webServer: Array.isArray(config.webServer) ? [config.webServer[3]] : undefined,
});
