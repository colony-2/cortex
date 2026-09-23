const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const { EventEmitter } = require('node:events');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { execFileSync, spawnSync } = require('node:child_process');
const { Readable } = require('node:stream');
const test = require('node:test');
const vm = require('node:vm');

const packageRoot = path.resolve(__dirname, '..');
const installer = fs.readFileSync(path.join(packageRoot, 'scripts/postinstall.js'), 'utf8');

async function install(t, platform, arch, corrupt = false) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'cortex-installer-test-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const pkg = JSON.parse(fs.readFileSync(path.join(packageRoot, 'package.json'), 'utf8'));
  pkg.version = '1.2.3';
  fs.writeFileSync(path.join(root, 'package.json'), JSON.stringify(pkg));
  fs.mkdirSync(path.join(root, 'staged'));
  fs.writeFileSync(path.join(root, 'staged/cortex'), '#!/bin/sh\nprintf "cortex version 1.2.3\\n"\n');
  const archive = path.join(root, 'release.tar.gz');
  execFileSync('tar', ['-czf', archive, '-C', path.join(root, 'staged'), 'cortex']);
  const bytes = fs.readFileSync(archive);
  const digest = crypto.createHash('sha256').update(bytes).digest('hex');
  const asset = `cortex_1.2.3_${platform === 'linux' ? 'Linux' : 'Darwin'}_${arch === 'x64' ? 'x86_64' : 'arm64'}.tar.gz`;
  const urls = [];
  const errors = [];
  const fakeProcess = { platform, arch, exitCode: 0 };
  await vm.runInNewContext(installer, {
    __dirname: path.join(root, 'scripts'),
    process: fakeProcess,
    console: { error: message => errors.push(message) },
    setTimeout,
    require(name) {
      if (name !== 'https') return require(name);
      return {
        get(url, callback) {
          urls.push(url);
          const response = Readable.from([
            url.endsWith('checksums.txt') ? `${corrupt ? '0'.repeat(64) : digest}  ${asset}\n` : bytes,
          ]);
          response.statusCode = 200;
          process.nextTick(() => callback(response));
          return new EventEmitter();
        },
      };
    },
  });
  assert.deepEqual(urls, [
    `https://github.com/colony-2/cortex/releases/download/v1.2.3/${asset}`,
    'https://github.com/colony-2/cortex/releases/download/v1.2.3/checksums.txt',
  ]);
  return { root, fakeProcess, errors };
}

for (const platform of ['linux', 'darwin']) {
  for (const arch of ['x64', 'arm64']) {
    test(`installs verified ${platform}/${arch} release`, async t => {
      const { root, fakeProcess, errors } = await install(t, platform, arch);
      assert.equal(fakeProcess.exitCode, 0);
      assert.deepEqual(errors, []);
      const binary = path.join(root, 'vendor/cortex');
      assert.ok(fs.statSync(binary).mode & 0o111);
      assert.equal(execFileSync(binary, ['version'], { encoding: 'utf8' }), 'cortex version 1.2.3\n');
    });
  }
}

test('rejects a corrupt release before installing a binary', async t => {
  const { root, fakeProcess, errors } = await install(t, 'linux', 'arm64', true);
  assert.equal(fakeProcess.exitCode, 1);
  assert.match(errors[0], /Checksum mismatch/);
  assert.equal(fs.existsSync(path.join(root, 'vendor/cortex')), false);
});

test('launcher forwards arguments and exit status', async t => {
  const { root } = await install(t, 'linux', 'arm64');
  fs.mkdirSync(path.join(root, 'bin'));
  fs.copyFileSync(path.join(packageRoot, 'bin/cli.js'), path.join(root, 'bin/cli.js'));
  fs.writeFileSync(path.join(root, 'vendor/cortex'), '#!/bin/sh\nprintf "%s\\n" "$@"\nexit 7\n');
  const result = spawnSync(process.execPath, [path.join(root, 'bin/cli.js'), '--working-dir', '/a path/cell'], { encoding: 'utf8' });
  assert.equal(result.status, 7);
  assert.equal(result.stdout, '--working-dir\n/a path/cell\n');
});
