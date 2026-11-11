import { defineConfig } from '@playwright/test';
import config from './playwright.config';

// The dedicated fixture executable creates native input waits and retains them
// in a file-backed SQLite database. No HTTP history/detail/document doubles.
export default defineConfig({
  ...config,
  testMatch: 'job-conversation-runtime.spec.ts',
  reporter: 'list',
  outputDir: process.env.PLAYWRIGHT_OUTPUT_DIR || './test-results',
  projects: config.projects?.filter(project => project.name === 'chromium').map(project => ({ ...project, testIgnore: [] })),
  workers: 1,
  webServer: [
    {
      command: 'bash scripts/go.sh run ./server/cmd/historytestserver --addr 127.0.0.1:18085',
      url: 'http://127.0.0.1:18085/api/health',
      cwd: '../..', reuseExistingServer: false, timeout: 180_000,
    },
    {
      command: 'pnpm exec vite --host 127.0.0.1 --port 15173 --strictPort',
      url: 'http://127.0.0.1:15173',
      env: { CORTEX_API_URL: 'http://127.0.0.1:18085', VITE_CORTEX_API_BASE: '/api' },
      cwd: '.', reuseExistingServer: false, timeout: 120_000,
    },
  ],
});
