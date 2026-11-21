import {defineConfig} from '@playwright/test';
import config from './playwright.relationships.config';
import { configureInstalledFonts } from './tests/helpers/fontconfig';

configureInstalledFonts(config.use?.launchOptions?.executablePath);

// HTTP fixtures exercise implemented browser flows; Go checks cover real runtime reads.
export default defineConfig({
  ...config,
  // Compound restart journeys include a cold app load, dialogs, and a reload.
  // Match the main app's CI budget while bounding each interaction separately.
  timeout: process.env.CI ? 60_000 : 30_000,
  use: {
    ...config.use,
    actionTimeout: 15_000,
    navigationTimeout: 15_000,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  testMatch:['recovery-stage1.spec.ts','goals.spec.ts','job-relationships.spec.ts','new-goal-attachments.spec.ts'],
  webServer:[
    {command:'bash scripts/go.sh run ./server/cmd/uitestserver --addr 127.0.0.1:18081 --working-dir web/app/tests/fixtures/cell',url:'http://127.0.0.1:18081/api/health',cwd:'../..',reuseExistingServer:false,timeout:180000},
    {...config.webServer as object,env:{CORTEX_API_URL:'http://127.0.0.1:18081',VITE_CORTEX_API_BASE:'/api'}},
  ],
  outputDir:process.env.CORTEX_STAGE1_TEST_OUTPUT || '/tmp/cortex-stage1/browser-results',
});
