import { defineConfig } from '@playwright/test';
import base from './playwright.config';

// Run the inbox migration's mutations against the production HTTP/SQLite stack.
// The general shards and worker-driven recipe suite remain independent gates.
export default defineConfig({
  ...base,
  workers: 1,
  retries: 0,
  outputDir: '/tmp/cortex-inbox-http-results',
  projects: [...(base.projects?.filter(project => ['production', 'goals-runtime'].includes(project.name!)).map(project => project.name === 'production' ? {
    ...project,
    testMatch: ['api-integration.spec.ts', 'admin.spec.ts', 'account-menu.spec.ts', 'new-goal-attachments.spec.ts'],
  } : project) ?? []), {
    name: 'inbox-input-runtime',
    testMatch: 'recipe-input.spec.ts',
    grep: /answers recipe input and resumes the worker to completion|reviews render immutable Markdown|holds the retained reader|wraps long literal titles and instructions/,
    use: base.projects?.find(project => project.name === 'production')?.use,
  }],
  webServer: Array.isArray(base.webServer) ? base.webServer.slice(0, 3) : undefined,
});
