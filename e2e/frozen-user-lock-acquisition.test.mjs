import assert from 'node:assert/strict';
import crypto from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

const scripts = fileURLToPath(new URL('../scripts/', import.meta.url));
const reviewed = JSON.parse(fs.readFileSync(path.join(scripts, 'gentle-pi-suppliers.json'), 'utf8')).releases;
function newestReviewedListVersion(table) {
  const order = version => version.split('.').map(Number);
  return Object.keys(table).sort((left, right) => {
    const a = order(left), b = order(right);
    return a[0] - b[0] || a[1] - b[1] || a[2] - b[2];
  }).at(-1);
}
const targetVersion = newestReviewedListVersion(reviewed);
const rootNames = ['gentle-pi', '@earendil-works/pi-coding-agent', '@earendil-works/pi-tui', '@heyhuynhgiabuu/pi-pretty', 'typebox'];
// The stand-in listed gentle-pi pins its companion like gentle-pi 4.1.0+.
const companionPin = '0.4.2';
// A stand-in for the listed gentle-pi tarball stock npm packs before the lock
// is generated; the staged list binds its SRI like the real reviewed entry.
function packedGentlePi(t, pinned) {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'frozen-user-pack-'));
  t.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  fs.mkdirSync(path.join(directory, 'package/runtime'), { recursive: true });
  fs.writeFileSync(path.join(directory, 'package/runtime/gentle-shell-bundled.mjs'),
    pinned ? `export const OTHER = 1;\nexport const ENGRAM_COMPANION_SOURCE = "npm:gentle-engram@${companionPin}";\n` : 'export const OTHER = 1;\n');
  const tarball = path.join(directory, 'gentle-pi.tgz');
  const tar = spawnSync('/usr/bin/tar', ['-czf', tarball, '-C', directory, 'package']);
  assert.equal(tar.status, 0, String(tar.stderr));
  return { tarball, integrity: `sha512-${crypto.createHash('sha512').update(fs.readFileSync(tarball)).digest('base64')}` };
}

function fixture(t, profile = 'modern', change, options = {}) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'frozen-user-lock-'));
  fs.chmodSync(root, 0o700);
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  for (const name of ['state', 'agent', 'home', 'tmp', 'config', 'runtime/node/bin', 'runtime/node/lib/node_modules/npm/bin', `user-locks/${profile}`]) {
    fs.mkdirSync(path.join(root, name), { recursive: true, mode: 0o700 });
  }
  fs.copyFileSync(process.execPath, path.join(root, 'runtime/node/bin/node'));
  fs.chmodSync(path.join(root, 'runtime/node/bin/node'), 0o700);
  const trace = path.join(root, 'npm-trace.jsonl');
  // Stop at the real child-process boundary: this is not npm-install qualification.
  const stagedLock = path.join(root, `user-locks/${profile}/package-lock.json`);
  const generatedLock = path.join(root, 'runtime/project/package-lock.json');
  const pinned = options.companion !== false && (options.mode ?? 'separate') === 'separate';
  const declaration = { ...Object.fromEntries(rootNames.map(name => [name, name === 'gentle-pi' ? targetVersion : 'latest'])), ...(pinned ? { 'gentle-engram': companionPin } : {}) };
  const packed = packedGentlePi(t, pinned);
  const listedIntegrity = options.listedIntegrity ?? packed.integrity;
  const generatedIntegrity = options.gentlePiIntegrity ?? listedIntegrity;
  const generatedVersion = options.gentlePiVersion ?? targetVersion;
  const npmBody = profile === 'modern'
    ? `if (args[0] === 'pack') {
  const destination = args.find(arg => arg.startsWith('--pack-destination=')).slice('--pack-destination='.length);
  require('node:fs').copyFileSync(${JSON.stringify(packed.tarball)}, require('node:path').join(destination, 'gentle-pi-' + args[1].split('@')[1] + '.tgz'));
  process.exit(0);
}
if (args[0] === 'install') {
  const lock = JSON.parse(require('node:fs').readFileSync(${JSON.stringify(stagedLock)}));
  lock.packages[''].dependencies = ${JSON.stringify(declaration)};
  if (lock.packages[''].dependencies['gentle-engram']) lock.packages['node_modules/gentle-engram'] = {
    version: ${JSON.stringify(companionPin)}, resolved: 'https://registry.npmjs.org/gentle-engram/-/gentle-engram-${companionPin}.tgz',
    integrity: 'sha512-' + Buffer.alloc(64).toString('base64'),
  };
  const gentle = lock.packages['node_modules/gentle-pi'];
  gentle.version = ${JSON.stringify(generatedVersion)};
  gentle.resolved = 'https://registry.npmjs.org/gentle-pi/-/gentle-pi-' + gentle.version + '.tgz';
  gentle.integrity = ${JSON.stringify(generatedIntegrity)};
  require('node:fs').writeFileSync(${JSON.stringify(generatedLock)}, JSON.stringify(lock));
  process.exit(0);
}
process.exit(73);`
    : 'process.exit(73);';
  fs.writeFileSync(path.join(root, 'runtime/node/lib/node_modules/npm/bin/npm-cli.js'),
    `require('node:fs').appendFileSync(${JSON.stringify(trace)}, JSON.stringify(process.argv.slice(2)) + '\\n');${options.npmTraceExtra ?? ''}
const args = process.argv.slice(2);
${npmBody}`, { mode: 0o600 });
  if (profile === 'modern' && !options.omitStagedList) {
    // The Go worker stages the selected reviewed list and the lock-SRI
    // completion helper into every install stage; the fixture mirrors that.
    const list = JSON.parse(fs.readFileSync(path.join(scripts, 'gentle-pi-suppliers.json')));
    list.releases[targetVersion].integrity = listedIntegrity;
    fs.writeFileSync(path.join(root, 'gentle-pi-suppliers.json'), JSON.stringify(list), { mode: 0o600 });
    fs.copyFileSync(path.join(scripts, 'complete-generated-lock-sri.mjs'), path.join(root, 'complete-generated-lock-sri.mjs'));
  }
  const original = fs.readFileSync(path.join(scripts, `user-locks/${profile}/package-lock.json`));
  const lock = JSON.parse(original);
  if (change) change(lock);
  fs.writeFileSync(stagedLock, change ? JSON.stringify(lock) : original, { mode: 0o400 });
  const prefix = path.join(root, 'prefix');
  if (profile === 'prior') {
    const coding = path.join(prefix, 'lib/node_modules/@earendil-works/pi-coding-agent');
    fs.mkdirSync(coding, { recursive: true, mode: 0o700 });
    fs.writeFileSync(path.join(coding, 'package.json'), '{"version":"0.99.2"}', { mode: 0o600 });
    fs.writeFileSync(path.join(root, 'state/prior-global-graph.json'), '[]', { mode: 0o600 });
  }
  const result = spawnSync(process.execPath, [path.join(scripts, 'provision-gentle-shell-private-global.mjs'),
    root, prefix, path.join(root, 'agent'), prefix, root, options.mode ?? 'separate', 'install'], {
    env: { HOME: path.join(root, 'home'), PATH: '/usr/bin:/bin' }, timeout: 10000, encoding: 'utf8', maxBuffer: 8192,
  });
  const calls = fs.existsSync(trace) ? fs.readFileSync(trace, 'utf8').trim().split('\n').filter(Boolean).map(JSON.parse) : [];
  return { root, trace, result, calls, lock, original, declaration, packed, source: path.join(root, `runtime/${profile === 'prior' ? 'prior' : 'project'}`) };
}

test('prior acquisition uses the complete frozen lock and only npm ci', t => {
  const f = fixture(t, 'prior');
  assert.notEqual(f.result.status, 0, 'the npm fixture deliberately stops acquisition');
  assert.equal(f.calls.length, 1);
  assert.equal(f.calls[0][0], 'ci');
  assert.ok(f.calls[0].includes('--install-strategy=shallow'));
  assert.ok(f.calls[0].includes('--ignore-scripts') && f.calls[0].includes('--engine-strict'));
  assert.ok(f.calls[0].includes('--min-release-age=3'));
  assert.deepEqual(fs.readFileSync(path.join(f.source, 'package-lock.json')), f.original);
  const manifest = JSON.parse(fs.readFileSync(path.join(f.source, 'package.json')));
  assert.deepEqual(manifest.dependencies, f.lock.packages[''].dependencies);
  assert.equal(manifest.name, f.lock.packages[''].name);
  assert.equal(manifest.version, f.lock.packages[''].version);
  assert.deepEqual(fs.readdirSync(path.join(f.root, 'agent')), []);
});

test('modern acquisition declares the newest listed gentle-pi and generates the lock', t => {
  const f = fixture(t, 'modern');
  assert.notEqual(f.result.status, 0, 'the npm fixture deliberately stops acquisition');
  assert.deepEqual(f.calls.map(args => args[0]), ['pack', 'install', 'ci'], f.result.stderr);
  assert.deepEqual(f.calls[0].slice(0, 2), ['pack', `gentle-pi@${targetVersion}`]);
  assert.ok(f.calls[0].includes('--ignore-scripts'));
  assert.ok(f.calls[1].includes('--package-lock-only'));
  assert.ok(f.calls[1].includes('--min-release-age=0'));
  const manifest = JSON.parse(fs.readFileSync(path.join(f.source, 'package.json')));
  assert.equal(manifest.name, 'gentle-shell-owned-seed');
  // The companion is declared at the listed gentle-pi's exact pin, never latest.
  assert.deepEqual(manifest.dependencies, { ...Object.fromEntries(rootNames.map(name => [name, name === 'gentle-pi' ? targetVersion : 'latest'])), 'gentle-engram': companionPin });
  const generated = JSON.parse(fs.readFileSync(path.join(f.source, 'package-lock.json')));
  assert.equal(generated.packages['node_modules/gentle-pi'].version, targetVersion);
  assert.equal(generated.packages['node_modules/gentle-pi'].integrity, f.packed.integrity);
  assert.deepEqual(fs.readdirSync(path.join(f.root, 'agent')), []);
  assert.deepEqual(fs.readdirSync(path.join(f.root, 'tmp')), [], 'the packed tarball is not retained');
});

test('a listed gentle-pi that pins no companion declares the five roots without failing', t => {
  const f = fixture(t, 'modern', undefined, { companion: false });
  assert.deepEqual(f.calls.map(args => args[0]), ['pack', 'install', 'ci'], f.result.stderr);
  const manifest = JSON.parse(fs.readFileSync(path.join(f.source, 'package.json')));
  assert.deepEqual(manifest.dependencies, Object.fromEntries(rootNames.map(name => [name, name === 'gentle-pi' ? targetVersion : 'latest'])));
});

// B1: the companion belongs to Separate only. An agent install keeps its
// private prefix exactly the five stock roots: no pack, no companion root.
test('an agent install declares the five roots without packing a companion pin', t => {
  const f = fixture(t, 'modern', undefined, { mode: 'agent' });
  assert.deepEqual(f.calls.map(args => args[0]), ['install', 'ci'], f.result.stderr);
  const manifest = JSON.parse(fs.readFileSync(path.join(f.source, 'package.json')));
  assert.deepEqual(manifest.dependencies, Object.fromEntries(rootNames.map(name => [name, name === 'gentle-pi' ? targetVersion : 'latest'])));
});

test('a packed gentle-pi whose bytes differ from the list SRI is refused before the lock', t => {
  const f = fixture(t, 'modern', undefined, { listedIntegrity: reviewed['4.3.0'].integrity });
  assert.notEqual(f.result.status, 0);
  assert.match(f.result.stderr, /stock supplier pin/);
  assert.deepEqual(f.calls.map(args => args[0]), ['pack'], 'no lock is generated from an unverified supplier');
  assert.equal(fs.existsSync(path.join(f.source, 'package.json')), false);
});

test('modern npm ci keeps the reviewed profile without re-resolution flags', t => {
  const f = fixture(t, 'modern');
  assert.notEqual(f.result.status, 0);
  assert.equal(f.calls[2][0], 'ci');
  assert.ok(f.calls[2].includes('--ignore-scripts') && f.calls[2].includes('--engine-strict'));
  assert.ok(f.calls[2].includes('--min-release-age=0'));
  assert.equal(f.calls[2].includes('--install-strategy=shallow'), false);
  assert.equal(f.calls[2].includes('--min-release-age=3'), false);
});

// S23/M4: a Main preview install request stages no reviewed list and keeps
// the frozen modern lock's stock base (gentle-pi 4.3.0), exactly the route
// every modern install took before the reviewed-list route existed.
test('a modern install without the staged reviewed list keeps the frozen lock route', t => {
  const f = fixture(t, 'modern', undefined, { omitStagedList: true });
  assert.notEqual(f.result.status, 0, 'the npm fixture deliberately stops acquisition');
  assert.deepEqual(f.calls.map(args => args[0]), ['ci'], f.result.stderr);
  assert.ok(f.calls[0].includes('--install-strategy=shallow'));
  assert.ok(f.calls[0].includes('--min-release-age=3'));
  const acquired = JSON.parse(fs.readFileSync(path.join(f.source, 'package-lock.json')));
  assert.equal(acquired.packages['node_modules/gentle-pi'].version, '4.3.0', 'Main preview stock base must stay the frozen 4.3.0 lock');
  assert.deepEqual(fs.readdirSync(path.join(f.root, 'agent')), []);
});

test('modern acquisition pins umask 022 over a group-writable login umask', t => {
  const observed = path.join(fs.mkdtempSync(path.join(os.tmpdir(), 'provision-umask-')), 'umask.json');
  t.after(() => fs.rmSync(path.dirname(observed), { recursive: true, force: true }));
  const extra = `require('node:fs').writeFileSync(${JSON.stringify(observed)}, JSON.stringify(process.umask()));`;
  // Simulate the Ubuntu/WSL default login umask that npm would otherwise inherit.
  const previous = process.umask(0o002);
  let f;
  try { f = fixture(t, 'modern', undefined, { npmTraceExtra: extra }); } finally { process.umask(previous); }
  assert.notEqual(f.result.status, 0, 'the npm fixture deliberately stops acquisition');
  assert.equal(JSON.parse(fs.readFileSync(observed)), 0o22, 'npm must observe the pinned 022 umask');
});

function changeVersion(entry, version) {
  entry.resolved = entry.resolved.replace(`-${entry.version}.tgz`, `-${version}.tgz`);
  entry.version = version;
}
// A missing integrity is not a refusal case on the generated route: the SRI
// completion helper fills it from the registry metadata like on Windows. Every
// other deviation from the canonical per-package shape refuses before npm ci.
const mutations = {
  'foreign registry': entry => { entry.resolved = 'https://foreign.invalid/package.tgz'; },
  'mismatched name': entry => { entry.name = 'different-package'; },
  'unsafe version': entry => changeVersion(entry, '../outside'),
  'noncanonical version': entry => changeVersion(entry, '01.0.0'),
  'noncanonical prerelease': entry => changeVersion(entry, '1.0.0-01'),
  'linked package': entry => { entry.link = true; },
  'noncanonical SRI': entry => { entry.integrity = entry.integrity.slice(0, -3) + 'B=='; },
};
for (const [name, mutate] of Object.entries(mutations)) {
  test(`${name} refuses the generated lock before npm ci`, t => {
    const f = fixture(t, 'modern', lock => mutate(lock.packages['node_modules/@earendil-works/pi-ai']));
    assert.notEqual(f.result.status, 0);
    assert.deepEqual(f.calls.map(args => args[0]), ['pack', 'install'], f.result.stderr);
    assert.equal(fs.existsSync(path.join(f.root, 'prefix')), false);
    assert.deepEqual(fs.readdirSync(path.join(f.root, 'agent')), []);
  });
}

test('a generated lock whose gentle-pi SRI differs from the list entry is refused', t => {
  const wrong = reviewed['4.3.0'].integrity;
  const f = fixture(t, 'modern', undefined, { gentlePiIntegrity: wrong });
  assert.notEqual(wrong, f.packed.integrity);
  assert.notEqual(f.result.status, 0);
  assert.match(f.result.stderr, /stock supplier pin/);
  assert.deepEqual(f.calls.map(args => args[0]), ['pack', 'install'], 'npm ci must not run after a reviewed-list SRI mismatch');
});

test('a generated lock resolving gentle-pi off the listed release is refused', t => {
  const f = fixture(t, 'modern', undefined, { gentlePiVersion: '5.0.0' });
  assert.notEqual(f.result.status, 0);
  assert.match(f.result.stderr, /stock supplier pin/);
  assert.deepEqual(f.calls.map(args => args[0]), ['pack', 'install']);
});

test('unsafe package placement refuses before npm ci', t => {
  const f = fixture(t, 'modern', lock => { lock.packages['node_modules/../outside'] = lock.packages['node_modules/typebox']; });
  assert.notEqual(f.result.status, 0);
  assert.deepEqual(f.calls.map(args => args[0]), ['pack', 'install']);
});

test('duplicate package identities cannot supply conflicting integrity', t => {
  const f = fixture(t, 'modern', lock => {
    lock.packages['node_modules/typebox/node_modules/@earendil-works/pi-ai'] = {
      ...lock.packages['node_modules/@earendil-works/pi-ai'], integrity: `sha512-${Buffer.alloc(64).toString('base64')}`,
    };
  });
  assert.notEqual(f.result.status, 0);
  assert.deepEqual(f.calls.map(args => args[0]), ['pack', 'install']);
});

test('prior root declarations and package identities must agree before npm ci', t => {
  const f = fixture(t, 'prior', lock => { lock.packages[''].dependencies['typebox'] = '9.9.9'; });
  assert.notEqual(f.result.status, 0);
  assert.deepEqual(f.calls, []);
});
