import { defineConfig } from '@playwright/test';
import { execFileSync } from 'node:child_process';
export default defineConfig({
 testDir:'./tests', testMatch:'inbox.spec.ts', workers:1, timeout:30000, reporter:'list',
 outputDir:'/tmp/cortex-inbox-scratch/browser-results',
 use:{baseURL:'http://127.0.0.1:15178',viewport:{width:1440,height:1000},headless:true,launchOptions:{executablePath:execFileSync('which',['chromium'],{encoding:'utf8'}).trim()}},
 webServer:{command:'pnpm exec vite --host 127.0.0.1 --port 15178 --strictPort',url:'http://127.0.0.1:15178',reuseExistingServer:false},
});
