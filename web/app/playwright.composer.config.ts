import { defineConfig } from '@playwright/test';
import config from './playwright.relationships.config';
import { configureInstalledFonts } from './tests/helpers/fontconfig';

configureInstalledFonts(config.use?.launchOptions?.executablePath);

export default defineConfig({
  ...config,
  // Paste, mode switching and retries form one journey on shared CI workers.
  // Keep the existing action/assertion budgets while allowing the whole flow.
  timeout: 60_000,
  testMatch: 'new-goal-composer.spec.ts',
  outputDir: process.env.CORTEX_COMPOSER_TEST_OUTPUT || '/tmp/cortex-composer-browser-results',
});
