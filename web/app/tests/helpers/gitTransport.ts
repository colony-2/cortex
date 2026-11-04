import { execFileSync } from 'node:child_process';
import { mkdirSync, writeFileSync } from 'node:fs';
import { delimiter, join } from 'node:path';
import { pathToFileURL } from 'node:url';

// Keep the registered cell's portable identity while serving fixture Git data
// locally. c2j deliberately isolates GIT_CONFIG_* environment variables inside
// workspaces; command-line configuration in this test-only Git shim survives.
export function fixtureGitTransport(repo: string): NodeJS.ProcessEnv {
  const git = execFileSync('sh', ['-c', 'command -v git'], { encoding: 'utf8' }).trim();
  const bin = join(repo, '.fixture-bin');
  mkdirSync(bin);
  const quote = (value: string) => "'" + value.replace(/'/g, "'\\''") + "'";
  const rewrite = `url.${pathToFileURL(repo).href}.insteadOf=https://github.com/cortex-test/platform`;
  const rewriteGit = `${rewrite}.git`;
  writeFileSync(join(bin, 'git'), `#!/bin/sh\nexec ${quote(git)} -c ${quote(rewrite)} -c ${quote(rewriteGit)} "$@"\n`, { mode: 0o755 });
  return { ...process.env, PATH: `${bin}${delimiter}${process.env.PATH}`, GIT_TERMINAL_PROMPT: '0' };
}
