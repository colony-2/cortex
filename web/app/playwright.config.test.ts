import { execFileSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { expect, test } from 'vitest';

test.each([
  { name: 'uses the CI browser installation despite Chromium alone on PATH', managed: true, binaries: ['chromium'], system: false },
  { name: 'uses the explicit browser installation despite a complete system pair', managed: true, binaries: ['chromium', 'ffmpeg'], system: false },
  { name: 'uses the system pair when no browser installation is specified', managed: false, binaries: ['chromium', 'ffmpeg'], system: true },
  { name: 'falls back to Playwright when only system Chromium is available', managed: false, binaries: ['chromium'], system: false },
  { name: 'uses Playwright when no system browser is available', managed: false, binaries: [], system: false },
])('$name', ({ managed, binaries, system }) => {
  const directory = mkdtempSync(path.join(tmpdir(), 'cortex-playwright-config-'));
  const bin = path.join(directory, 'bin');
  const browsers = path.join(directory, 'browsers');
  mkdirSync(bin);
  for (const binary of binaries) {
    writeFileSync(path.join(bin, binary), '#!/bin/sh\nexit 1\n', { mode: 0o755 });
  }

  try {
    // Load the real config in isolation: Playwright caches its executable registry
    // at import time, and the system FFmpeg adapter mutates that registry.
    const output = execFileSync(process.execPath, ['--input-type=module', '--eval', `
      import config from './playwright.config.ts';
      import { createRequire } from 'node:module';
      const require = createRequire(import.meta.url);
      const playwrightRequire = createRequire(require.resolve('playwright/package.json'));
      const { registry: { registry } } = playwrightRequire('playwright-core/lib/coreBundle');
      console.log(JSON.stringify({
        launchOptions: config.use.launchOptions,
        ffmpeg: registry.findExecutable('ffmpeg').executablePath(),
        video: config.use.video,
      }));
    `], {
      cwd: __dirname,
      encoding: 'utf8',
      env: {
        ...process.env,
        PATH: bin,
        CORTEX_PLAYWRIGHT_BROWSERS_PATH: managed ? browsers : '',
        PLAYWRIGHT_BROWSERS_PATH: browsers,
      },
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    const config = JSON.parse(output);
    expect(config.launchOptions).toEqual(system ? { executablePath: path.join(bin, 'chromium') } : {});
    if (system) {
      expect(config.ffmpeg).toBe(path.join(bin, 'ffmpeg'));
    } else {
      expect(config.ffmpeg.startsWith(`${browsers}${path.sep}`)).toBe(true);
    }
    expect(config.video).toBe('retain-on-failure');
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});
