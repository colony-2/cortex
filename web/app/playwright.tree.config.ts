import { defineConfig } from '@playwright/test';
import { execFileSync } from 'node:child_process';
const chromium = process.env.CORTEX_CHROMIUM || execFileSync('which',['chromium'],{encoding:'utf8'}).trim();
export default defineConfig({
 testDir:'./tests', testMatch:'job-tree.spec.ts', workers:1, timeout:30000, reporter:'list',
 outputDir:'/tmp/cortex-tree-browser-results',
 use:{baseURL:'http://127.0.0.1:15176',headless:true,launchOptions:{executablePath:chromium}},
 webServer:{command:'pnpm exec vite --host 127.0.0.1 --port 15176 --strictPort',url:'http://127.0.0.1:15176',reuseExistingServer:false},
});
