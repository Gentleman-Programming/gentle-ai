import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

const scripts = fileURLToPath(new URL('../scripts/', import.meta.url));
function fixture(t, profile = 'modern', change) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'frozen-user-lock-'));
  fs.chmodSync(root, 0o700);
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  for (const name of ['state', 'agent', 'home', 'tmp', 'config', 'runtime/node/bin', 'runtime/node/lib/node_modules/npm/bin', `user-locks/${profile}`]) {
    fs.mkdirSync(path.join(root, name), { recursive: true, mode: 0o700 });
  }
  fs.copyFileSync(process.execPath, path.join(root, 'runtime/node/bin/node'));
  fs.chmodSync(path.join(root, 'runtime/node/bin/node'), 0o700);
  const trace = path.join(root, 'npm-trace.json');
  // Stop at the real child-process boundary: this is not npm-install qualification.
  fs.writeFileSync(path.join(root, 'runtime/node/lib/node_modules/npm/bin/npm-cli.js'),
    `require('node:fs').writeFileSync(${JSON.stringify(trace)}, JSON.stringify(process.argv.slice(2))); process.exit(73);`, { mode: 0o600 });
  const original = fs.readFileSync(path.join(scripts, `user-locks/${profile}/package-lock.json`));
  const lock = JSON.parse(original);
  if (change) change(lock);
  fs.writeFileSync(path.join(root, `user-locks/${profile}/package-lock.json`), change ? JSON.stringify(lock) : original, { mode: 0o400 });
  const prefix = path.join(root, 'prefix');
  if (profile === 'prior') {
    const coding = path.join(prefix, 'lib/node_modules/@earendil-works/pi-coding-agent');
    fs.mkdirSync(coding, { recursive: true, mode: 0o700 });
    fs.writeFileSync(path.join(coding, 'package.json'), '{"version":"0.99.2"}', { mode: 0o600 });
    fs.writeFileSync(path.join(root, 'state/prior-global-graph.json'), '[]', { mode: 0o600 });
  }
  const result = spawnSync(process.execPath, [path.join(scripts, 'provision-gentle-shell-private-global.mjs'),
    root, prefix, path.join(root, 'agent'), prefix, root, 'separate', 'install'], {
    env: { HOME: path.join(root, 'home'), PATH: '/usr/bin:/bin' }, timeout: 10000, encoding: 'utf8', maxBuffer: 8192,
  });
  return { root, trace, result, lock, original, source: path.join(root, `runtime/${profile === 'prior' ? 'prior' : 'project'}`) };
}

for (const profile of ['modern', 'prior']) {
  test(`${profile} acquisition uses the complete frozen lock and only npm ci`, t => {
    const f = fixture(t, profile);
    assert.notEqual(f.result.status, 0, 'the npm fixture deliberately stops acquisition');
    const args = JSON.parse(fs.readFileSync(f.trace));
    assert.equal(args[0], 'ci');
    assert.ok(args.includes('--install-strategy=shallow'));
    assert.ok(args.includes('--ignore-scripts') && args.includes('--engine-strict'));
    assert.ok(args.includes('--min-release-age=3'));
    assert.deepEqual(fs.readFileSync(path.join(f.source, 'package-lock.json')), f.original);
    const manifest = JSON.parse(fs.readFileSync(path.join(f.source, 'package.json')));
    assert.deepEqual(manifest.dependencies, f.lock.packages[''].dependencies);
    assert.equal(manifest.name, f.lock.packages[''].name);
    assert.equal(manifest.version, f.lock.packages[''].version);
    assert.deepEqual(fs.readdirSync(path.join(f.root, 'agent')), []);
  });
}

function changeVersion(entry, version) {
  entry.resolved = entry.resolved.replace(`-${entry.version}.tgz`, `-${version}.tgz`);
  entry.version = version;
}
const mutations = {
  'missing transitive integrity': entry => { delete entry.integrity; },
  'foreign registry': entry => { entry.resolved = 'https://foreign.invalid/package.tgz'; },
  'mismatched name': entry => { entry.name = 'different-package'; },
  'unsafe version': entry => changeVersion(entry, '../outside'),
  'noncanonical version': entry => changeVersion(entry, '01.0.0'),
  'noncanonical prerelease': entry => changeVersion(entry, '1.0.0-01'),
  'linked package': entry => { entry.link = true; },
  'noncanonical SRI': entry => { entry.integrity = entry.integrity.slice(0, -3) + 'B=='; },
};
for (const [name, mutate] of Object.entries(mutations)) {
  test(`${name} refuses before any npm invocation`, t => {
    const f = fixture(t, 'modern', lock => mutate(lock.packages['node_modules/@earendil-works/pi-ai']));
    assert.notEqual(f.result.status, 0);
    assert.equal(fs.existsSync(f.trace), false, f.result.stderr);
    assert.equal(fs.existsSync(path.join(f.root, 'prefix')), false);
    assert.deepEqual(fs.readdirSync(path.join(f.root, 'agent')), []);
  });
}

test('unsafe package placement refuses before npm ci', t => {
  const f = fixture(t, 'modern', lock => { lock.packages['node_modules/../outside'] = lock.packages['node_modules/typebox']; });
  assert.notEqual(f.result.status, 0);
  assert.equal(fs.existsSync(f.trace), false, f.result.stderr);
});

test('duplicate package identities cannot supply conflicting integrity', t => {
  const f = fixture(t, 'modern', lock => {
    lock.packages['node_modules/typebox/node_modules/@earendil-works/pi-ai'] = {
      ...lock.packages['node_modules/@earendil-works/pi-ai'], integrity: `sha512-${Buffer.alloc(64).toString('base64')}`,
    };
  });
  assert.notEqual(f.result.status, 0);
  assert.equal(fs.existsSync(f.trace), false, f.result.stderr);
});

test('root declarations and package identities must agree before npm ci', t => {
  const f = fixture(t, 'modern', lock => { lock.packages[''].dependencies['typebox'] = '9.9.9'; });
  assert.notEqual(f.result.status, 0);
  assert.equal(fs.existsSync(f.trace), false, f.result.stderr);
});
