import { execFileSync } from 'node:child_process';
export function resolveUIBuild(env: Record<string, string | undefined>, git = (args: string[]) => execFileSync('git', args, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim()) {
  let commit = env.CORTEX_UI_COMMIT ?? null;
  if (commit === null) {
    try {
      commit = git(['rev-parse', 'HEAD']);
      // Only source changes count; ignored generated bundles never taint provenance.
      if (git(['status', '--porcelain', '--untracked-files=normal'])) commit += '-dirty';
    } catch { commit = null; }
  }
  return { version: env.CORTEX_UI_VERSION || null, commit: commit || null };
}
