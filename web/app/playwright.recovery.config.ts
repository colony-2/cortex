import {defineConfig} from '@playwright/test';
import config from './playwright.relationships.config';

// HTTP fixtures exercise implemented browser flows; Go checks cover real runtime reads.
export default defineConfig({
  ...config,
  testMatch:['recovery-stage1.spec.ts','goals.spec.ts','job-relationships.spec.ts','new-goal-attachments.spec.ts'],
  webServer:[
    {command:'bash scripts/go.sh run ./server/cmd/uitestserver --addr 127.0.0.1:18081 --working-dir web/app/tests/fixtures/cell',url:'http://127.0.0.1:18081/api/health',cwd:'../..',reuseExistingServer:false,timeout:180000},
    {...config.webServer as object,env:{CORTEX_API_URL:'http://127.0.0.1:18081',VITE_CORTEX_API_BASE:'/api'}},
  ],
  outputDir:process.env.CORTEX_STAGE1_TEST_OUTPUT || '/tmp/cortex-stage1/browser-results',
});
