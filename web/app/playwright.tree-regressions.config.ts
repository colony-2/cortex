import { defineConfig } from '@playwright/test';
import base from './playwright.tree.config';
export default defineConfig({
 ...base, testMatch:['goals.spec.ts','story-scroll.spec.ts'], timeout:60000,
 outputDir:'/tmp/cortex-tree-regression-results',
 use:{...base.use,baseURL:'http://127.0.0.1:15177'},
 webServer:[{
  command:'bash scripts/go.sh run ./server/cmd/uitestserver --addr 127.0.0.1:18085 --working-dir web/app/tests/fixtures/cell',
  cwd:'../..', url:'http://127.0.0.1:18085/api/health', timeout:180000,
 },{
  command:'pnpm exec vite --host 127.0.0.1 --port 15177 --strictPort',
  env:{CORTEX_API_URL:'http://127.0.0.1:18085'},url:'http://127.0.0.1:15177',
 }],
});
