import { defineConfig } from '@playwright/test';
import config from './playwright.composer.config';

// Exercise native multipart persistence and input bindings against the SQLite test API.
export default defineConfig({
  ...config,
  use: { ...config.use, baseURL: 'http://127.0.0.1:15176' },
  testMatch: 'new-goal-attachments.spec.ts',
  outputDir: '/tmp/cortex-composer-runtime-results',
  webServer: [{
    command: 'bash scripts/go.sh run ./server/cmd/uitestserver --addr 127.0.0.1:18081 --working-dir web/app/tests/fixtures/cell',
    url: 'http://127.0.0.1:18081/api/health', cwd: '../..', timeout: 180_000, reuseExistingServer: false,
  }, {
    command: 'pnpm exec vite --host 127.0.0.1 --port 15176 --strictPort',
    env: { CORTEX_API_URL: 'http://127.0.0.1:18081', VITE_CORTEX_API_BASE: '/api' },
    url: 'http://127.0.0.1:15176', reuseExistingServer: false,
  }],
});
