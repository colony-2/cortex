// Compatibility for scoped Git gateways that require repository .git suffixes.
// Keep this inside the native task: c2j installs its credential scope first.
import { spawnSync } from 'node:child_process';
import { readFileSync, existsSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const projectRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const fail = message => { throw new Error(message); };

function gitURL(original, env) {
  const result = spawnSync('git', ['ls-remote', '--get-url', original], { env, encoding: 'utf8' });
  if (result.status !== 0) fail('cannot resolve scoped Git route');
  return result.stdout.replace(/\r?\n$/, '');
}

function safeURL(raw) {
  if (typeof raw !== 'string' || /[\s\x00-\x1f\x7f\\]/.test(raw)) fail('invalid scoped Git route');
  let url;
  try { url = new URL(raw); } catch { fail('invalid scoped Git route'); }
  if (!['http:', 'https:'].includes(url.protocol) || raw.includes('@') || /[?#]/.test(raw) || url.search || url.hash || /%/.test(url.pathname)) {
    fail('unsafe scoped Git route');
  }
  return url;
}

export function composeGitEnvironment(env, roots, resolve = gitURL) {
  const text = env.GIT_CONFIG_COUNT ?? '0';
  if (!/^(0|[1-9][0-9]*)$/.test(text) || Number(text) > 10000) fail('invalid Git configuration count');
  let count = Number(text);
  for (let i = 0; i < count; i++) {
    if (!Object.hasOwn(env, `GIT_CONFIG_KEY_${i}`) || !Object.hasOwn(env, `GIT_CONFIG_VALUE_${i}`)) {
      fail('incomplete Git configuration indices');
    }
  }
  for (const key of Object.keys(env)) {
    const match = /^GIT_CONFIG_(?:KEY|VALUE)_(\d+)$/.exec(key);
    if (/^GIT_CONFIG_(?:KEY|VALUE)_/.test(key) && (!match || Number(match[1]) >= count || String(Number(match[1])) !== match[1])) {
      fail('conflicting Git configuration indices');
    }
  }
  const result = { ...env };
  if (roots.length === 0) return result;
  const baseRaw = resolve('https://github.com/', env);
  const base = safeURL(baseRaw);
  if (base.origin === 'https://github.com' || !base.pathname.endsWith('/')) fail('scoped Git rewrite required');
  const mappings = [...new Set(roots)].sort().map(root => {
    if (!/^https:\/\/github\.com\/colony-2\/[A-Za-z0-9_-][A-Za-z0-9_.-]*$/.test(root) || root.endsWith('.git')) {
      fail('invalid private repository root');
    }
    const relative = root.slice('https://github.com/'.length);
    const targetRaw = resolve(root + '.git', env);
    const target = safeURL(targetRaw);
    const original = safeURL(resolve(root, env));
    if (target.origin !== base.origin || target.pathname !== base.pathname + relative + '.git' ||
        original.origin !== base.origin || ![target.pathname, base.pathname + relative].includes(original.pathname)) {
      fail('conflicting scoped repository route');
    }
    return [root, targetRaw];
  });
  // Resolve every target in the baseline environment before adding any aliases.
  for (const [root, target] of mappings) {
    for (const original of [root, root + '.git']) {
      result[`GIT_CONFIG_KEY_${count}`] = `url.${target}.insteadOf`;
      result[`GIT_CONFIG_VALUE_${count++}`] = original;
    }
  }
  result.GIT_CONFIG_COUNT = String(count);
  for (const [root, target] of mappings) {
    if (resolve(root, result) !== target || resolve(root + '.git', result) !== target) {
      fail('Git compatibility alias composition failed');
    }
  }
  return result;
}

function privatePath(module, policy) {
  return policy.split(',').filter(Boolean).some(pattern => {
    // Go's GOPRIVATE patterns match prefixes, with '*' confined to a path segment.
    const parts = module.split('/');
    const segments = pattern.split('/').length;
    const prefix = parts.slice(0, segments).join('/');
    const expression = pattern.replace(/[.+^${}()|[\]\\]/g, '\\$&').replace(/\*/g, '[^/]*').replace(/\?/g, '[^/]');
    return new RegExp('^' + expression + '$').test(prefix);
  });
}

export function repositoryRoots(modules, policy) {
  const roots = new Set();
  for (const module of modules) {
    if (!privatePath(module, policy)) continue;
    const parts = module.split('/');
    if (parts[0] !== 'github.com' || parts[1] !== 'colony-2' || !parts[2]) fail('unsupported private module mapping');
    roots.add('https://' + parts.slice(0, 3).join('/'));
  }
  return [...roots].sort();
}

export function modulePaths(document, sums = '') {
  const replacements = document.Replace ?? [];
  const modules = [];
  for (const requirement of document.Require ?? []) {
    const replacement = replacements.find(item => item.Old.Path === requirement.Path &&
      (!item.Old.Version || item.Old.Version === requirement.Version));
    if (!replacement) modules.push(requirement.Path);
    else if (replacement.New.Version) modules.push(replacement.New.Path);
  }
  for (const item of replacements) if (item.New.Version) modules.push(item.New.Path);
  for (const line of sums.split('\n')) {
    const [module, version, checksum] = line.trim().split(/\s+/);
    if (module && version && checksum && !replacements.some(item => item.Old.Path === module)) modules.push(module);
  }
  return modules;
}

function goJSON(args, env) {
  const result = spawnSync('go', args, { env, encoding: 'utf8' });
  if (result.status !== 0) fail('cannot parse candidate Go metadata');
  try { return JSON.parse(result.stdout); } catch { fail('invalid candidate Go metadata'); }
}

function discover(env) {
  const workspace = path.join(projectRoot, 'go.work');
  let uses = [path.join(projectRoot, 'server')];
  let replacements = [];
  if (existsSync(workspace)) {
    const work = goJSON(['work', 'edit', '-json', workspace], env);
    uses = (work.Use ?? []).map(item => path.resolve(projectRoot, item.DiskPath));
    replacements = work.Replace ?? [];
  }
  const paths = [];
  for (const directory of uses) {
    const mod = goJSON(['mod', 'edit', '-json', path.join(directory, 'go.mod')], env);
    // Workspace replacements override the module-level entries for this build.
    mod.Replace = [...replacements, ...(mod.Replace ?? []).filter(item =>
      !replacements.some(work => work.Old.Path === item.Old.Path && (!work.Old.Version || work.Old.Version === item.Old.Version)))];
    const sums = path.join(directory, 'go.sum');
    paths.push(...modulePaths(mod, existsSync(sums) ? readFileSync(sums, 'utf8') : ''));
  }
  return repositoryRoots(paths, env.GOPRIVATE ?? '');
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const command = process.argv.slice(2);
    if (!command.length) fail('verification command required');
    const env = composeGitEnvironment(process.env, discover(process.env));
    const child = spawnSync(command[0], command.slice(1), { env, stdio: 'inherit' });
    if (child.error) fail('cannot launch verification command');
    process.exitCode = child.status ?? 1;
  } catch (error) {
    // All messages are fixed categories: never emit targets, config or credentials.
    console.error(`verify-command: ${error.message}`);
    process.exitCode = 1;
  }
}
