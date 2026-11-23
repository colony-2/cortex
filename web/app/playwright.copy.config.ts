import { defineConfig } from '@playwright/test';
import fixtures from './playwright.fixtures.config';

// Playwright also fetches intercepted requests from Node, outside Chromium's resolver.
const noProxy = (process.env.no_proxy ?? process.env.NO_PROXY ?? '').split(',').filter(Boolean);
const localBypass = [...new Set([...noProxy, 'cortex-copy.test'])].join(',');
process.env.no_proxy = localBypass;
process.env.NO_PROXY = localBypass;

export default defineConfig({
  ...fixtures,
  testMatch: 'job-id-copy.spec.ts',
  reporter: [['list', { printFailuresInline: true }]],
  fullyParallel: false,
  workers: 1,
  projects: [{ name: 'copy-chromium', use: { browserName: 'chromium' } }],
  use: {
    ...fixtures.use,
    permissions: [],
    launchOptions: {
      ...fixtures.use?.launchOptions,
      args: ['--host-resolver-rules=MAP cortex-copy.test 127.0.0.1', '--no-proxy-server'],
    },
  },
  webServer: (Array.isArray(fixtures.webServer) ? fixtures.webServer : []).map(server => ({
    ...server,
    command: server.command.replace('pnpm exec vite ', 'pnpm exec vite --config vite.copy.config.ts '),
  })),
});
