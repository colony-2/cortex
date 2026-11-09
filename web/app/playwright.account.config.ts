import { defineConfig } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { accountUIBuildEnvironment } from './tests/helpers/uiBuild.ts';
const chromium = execFileSync('which', ['chromium'], { encoding: 'utf8' }).trim();
export default defineConfig({
 testDir: './tests', testMatch: ['account-menu.spec.ts', 'ui-smoke.spec.ts'], workers: 1,
 outputDir: '/tmp/cortex-account/playwright-results', reporter: 'list',
 use: { viewport: { width: 1440, height: 900 }, launchOptions: { executablePath: chromium }, },
 projects: [ { name: 'vite', use: { baseURL: 'http://127.0.0.1:15173' } }, { name: 'embedded', use: { baseURL: 'http://127.0.0.1:18083' } } ],
 webServer: [
  { command: 'bash scripts/go.sh run ./server/cmd/uitestserver --jobdb-only --addr 127.0.0.1:18082', port: 18082, cwd: '../..', timeout: 240000 },
  { command: 'bash scripts/go.sh run ./server/cmd/uitestserver --addr 127.0.0.1:18081 --working-dir web/app/tests/fixtures/cell', port: 18081, cwd: '../..', timeout: 240000 },
  { command: `${process.env.CORTEX_ACCOUNT_BINARY || './build/cortex'} --addr 127.0.0.1:18083 --jobdb http://127.0.0.1:18082/1 --working-dir web/app/tests/fixtures/cell`, port: 18083, cwd: '../..', timeout: 120000 },
  { command: 'pnpm exec vite --host 127.0.0.1 --port 15173 --strictPort', port: 15173, env: { CORTEX_API_URL: 'http://127.0.0.1:18081', ...accountUIBuildEnvironment } },
 ],
});
