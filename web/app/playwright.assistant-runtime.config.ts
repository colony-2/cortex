import { defineConfig } from '@playwright/test';
import config from './playwright.config';

export default defineConfig({
  ...config, testMatch: 'assistant-runtime.spec.ts', workers: 1,
  timeout: 240_000, reporter: 'list',
  outputDir: process.env.PLAYWRIGHT_OUTPUT_DIR || './test-results/assistant-runtime',
  projects: config.projects?.filter(project => project.name === 'chromium').map(project => ({ ...project, testIgnore: [], use: { ...project.use, baseURL: 'http://127.0.0.1:15176' } })),
  webServer: [
    { command: 'bash scripts/assistant-runtime-fixture.sh', url: 'http://127.0.0.1:18086/api/health', cwd: '../..', timeout: 240_000, reuseExistingServer: process.env.CORTEX_ASSISTANT_REUSE === '1' },
    { command: 'pnpm exec vite --host 127.0.0.1 --port 15176 --strictPort', url: 'http://127.0.0.1:15176', cwd: '.', env: { CORTEX_API_URL: 'http://127.0.0.1:18086', VITE_CORTEX_API_BASE: '/api' }, timeout: 120_000, reuseExistingServer: process.env.CORTEX_ASSISTANT_REUSE === '1' },
  ],
});
