import { test } from 'node:test';
import assert from 'node:assert/strict';
import { composeGitEnvironment, repositoryRoots, modulePaths } from './verify-command.mjs';

const root = 'https://github.com/colony-2/c2j';
const target = 'https://gateway.invalid/github/colony-2/c2j.git';
function resolve(source, env) {
  const mappings = [['https://github.com/', 'https://gateway.invalid/github/']];
  for (let i = 0; i < Number(env.GIT_CONFIG_COUNT ?? 0); i++) {
    const key = env[`GIT_CONFIG_KEY_${i}`];
    if (key.startsWith('url.') && key.endsWith('.insteadOf')) {
      mappings.push([env[`GIT_CONFIG_VALUE_${i}`], key.slice(4, -10)]);
    }
  }
  const match = mappings.filter(([prefix]) => source.startsWith(prefix)).sort((a, b) => b[0].length - a[0].length)[0];
  return match ? match[1] + source.slice(match[0].length) : source;
}

test('both spellings compose at zero and nonzero counts without changing the task environment', () => {
  for (const base of [{}, {
    GIT_CONFIG_COUNT: '2', GIT_CONFIG_KEY_0: 'include.path', GIT_CONFIG_VALUE_0: '/scope/config',
    GIT_CONFIG_KEY_1: 'credential.helper', GIT_CONFIG_VALUE_1: 'scoped-helper',
    GIT_CONFIG_PARAMETERS: "'unrelated.value=literal value'", RESOLVER_SOCKET: '/scope/socket', OTHER: 'keep',
  }]) {
    const before = { ...base };
    const result = composeGitEnvironment(base, [root], resolve);
    assert.deepEqual(base, before);
    for (const [key, value] of Object.entries(base)) if (key !== 'GIT_CONFIG_COUNT') assert.equal(result[key], value);
    assert.equal(Number(result.GIT_CONFIG_COUNT), Number(base.GIT_CONFIG_COUNT ?? 0) + 2);
    assert.equal(resolve(root, result), target);
    assert.equal(resolve(root + '.git', result), target);
  }
});

test('malformed count and incomplete or colliding indices fail closed', () => {
  for (const env of [
    { GIT_CONFIG_COUNT: '-1' }, { GIT_CONFIG_COUNT: '1x' }, { GIT_CONFIG_COUNT: '1' },
    { GIT_CONFIG_COUNT: '0', GIT_CONFIG_KEY_0: 'unexpected' },
    { GIT_CONFIG_COUNT: '0', GIT_CONFIG_VALUE_1: 'unexpected' },
    { GIT_CONFIG_COUNT: '1', GIT_CONFIG_KEY_0: 'include.path' },
  ]) assert.throws(() => composeGitEnvironment(env, [root], resolve), /configuration/);
});

test('rejects credential-bearing, ambiguous or unexpected routes without printing target', () => {
  for (const bad of [
    'https://secret@gateway.invalid/github/colony-2/c2j.git',
    'https://@gateway.invalid/github/colony-2/c2j.git',
    target + '?token=secret', target + '#secret', target + '\nsecret',
    'https://other.invalid/github/colony-2/c2j.git',
    'https://gateway.invalid/github/colony-2/other.git',
    'https://gateway.invalid/github/colony-2/c2j.git.git',
    'https://gateway.invalid/github/colony-2/%63%32j.git',
  ]) {
    assert.throws(() => composeGitEnvironment({}, [root], (source, env) =>
      source === root + '.git' ? bad : resolve(source, env)), error => !error.message.includes('secret'));
  }
  assert.throws(() => composeGitEnvironment({}, [root], source => source), /scoped Git rewrite required/);
});

test('conflicting original mapping fails instead of changing authority', () => {
  assert.throws(() => composeGitEnvironment({}, [root], (source, env) =>
    source === root ? 'https://other.invalid/c2j' : resolve(source, env)), /conflicting/);
});

test('private submodules and major versions deduplicate, public roots stay untouched', () => {
  assert.deepEqual(repositoryRoots(['github.com/colony-2/c2j/v2/sub', 'github.com/colony-2/c2j',
    'github.com/public/lib'], 'github.com/colony-2/*'), [root]);
  assert.throws(() => repositoryRoots(['private.invalid/lib'], 'private.invalid/*'), /unsupported/);
});

test('versioned replacements replace roots, local replacements require no remote alias', () => {
  const mod = { Require: [{ Path: 'github.com/colony-2/old', Version: 'v1' },
    { Path: 'github.com/colony-2/local', Version: 'v1' }], Replace: [
    { Old: { Path: 'github.com/colony-2/old' }, New: { Path: 'github.com/colony-2/new/v2', Version: 'v2' } },
    { Old: { Path: 'github.com/colony-2/local' }, New: { Path: '../local' } },
  ] };
  const paths = modulePaths(mod, 'github.com/colony-2/transitive v1 h1:hash\n');
  assert.deepEqual(repositoryRoots(paths, 'github.com/colony-2/*'), [
    'https://github.com/colony-2/new', 'https://github.com/colony-2/transitive',
  ]);
});
