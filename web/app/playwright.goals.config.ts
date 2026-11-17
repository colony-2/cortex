import { defineConfig } from '@playwright/test';
import base from './playwright.config';
import { accountUIBuildEnvironment } from './tests/helpers/uiBuild';

// Goals coverage uses the local Cortex API and disposable SQLite JobDB jobs.
export default defineConfig({
  ...base,
  workers: 1,
  reporter: 'list',
  outputDir: '/tmp/cortex-goals-results',
  projects: [{ name: 'goals-runtime', testMatch: 'goals.spec.ts' }],
  webServer: [{
    command: 'bash scripts/go.sh run ./server/cmd/uitestserver --jobdb-only --sqlite --addr 127.0.0.1:18082',
    port: 18082, cwd: '../..', reuseExistingServer: false, timeout: 180_000,
  }, {
    command: 'bash scripts/go.sh run ./server/cmd/api serve --addr 127.0.0.1:18081 --working-dir web/app/tests/fixtures/cell',
    url: 'http://127.0.0.1:18081/api/health',
    env: { C2J_JOBDB: '', JOBDB_URL: '', CORTEX_TENANT_ID: '' },
    cwd: '../..', reuseExistingServer: false, timeout: 180_000,
  }, {
    command: 'pnpm exec vite --host 127.0.0.1 --port 15173 --strictPort',
    url: 'http://127.0.0.1:15173',
    env: { ...accountUIBuildEnvironment, CORTEX_API_URL: 'http://127.0.0.1:18081', VITE_CORTEX_API_BASE: '/api' },
    reuseExistingServer: false, timeout: 120_000,
  }],
});
