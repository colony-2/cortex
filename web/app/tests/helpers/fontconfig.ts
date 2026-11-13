import { execFileSync } from 'node:child_process';
import { existsSync, mkdtempSync, readFileSync, realpathSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';

// Minimal Nix executors can supply Chromium's fonts without /etc/fonts.
// Use only fonts already in its closure; never install or download anything.
export function configureInstalledFonts(chromium?: string) {
  if (!chromium || process.env.FONTCONFIG_FILE || existsSync('/etc/fonts/fonts.conf')) return;
  try {
    let executable = realpathSync(chromium);
    // Worker package bindings may be a small shell wrapper, not a symlink.
    if (!executable.startsWith('/nix/store/')) {
      if (statSync(executable).size > 16_384) return;
      const target = readFileSync(executable, 'utf8').match(/^exec ['"]?(\/nix\/store\/[^'"\s]+\/bin\/chromium)['"]? /m)?.[1];
      if (!target) return;
      executable = target;
    }
    const closure = execFileSync('nix-store', ['--query', '--requisites', path.dirname(path.dirname(executable))], { encoding: 'utf8', timeout: 10_000, stdio: ['ignore', 'pipe', 'pipe'] });
    const fonts = closure.split('\n').find(p => p.includes('-dejavu-fonts') && existsSync(path.join(p, 'share/fonts')));
    if (!fonts) return;
    const scratch = mkdtempSync(path.join(tmpdir(), 'cortex-browser-fonts-'));
    const escape = (s: string) => s.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('"', '&quot;');
    const config = path.join(scratch, 'fonts.conf');
    writeFileSync(config, `<?xml version="1.0"?><!DOCTYPE fontconfig SYSTEM "fonts.dtd"><fontconfig><dir>${escape(path.join(fonts, 'share/fonts'))}</dir><cachedir>${escape(path.join(scratch, 'cache'))}</cachedir><config></config></fontconfig>`);
    process.env.FONTCONFIG_FILE = config;
    process.once('exit', () => rmSync(scratch, { recursive: true, force: true }));
  } catch { /* Standard browser diagnostics still report an unavailable font setup. */ }
}
