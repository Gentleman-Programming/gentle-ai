// Portable VM fixtures for the Windows provisioner's version policy: latest
// dist-tag resolution with dynamic lock pinning, the reviewed-supplier table
// with its single fallback re-resolution, and a discovered native bundle
// directory. They prove the validation helper logic, not an installed Windows
// runtime. Adapted from the windows-provision-copy VM region pattern.
import assert from 'node:assert/strict';
import crypto from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import vm from 'node:vm';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const provision = process.argv[2] ?? fileURLToPath(new URL('./provision-gentle-shell-windows.mjs', import.meta.url));
const source = fs.readFileSync(provision, 'utf8');
const refusal = message => { throw Error(`Windows provision refused: ${message}`); };

// Static contract: no static root version survives in the provisioner; install
// declares every root at npm's `latest` dist-tag (rootDeclaration, tested below)
// and only gentle-pi is bound to the reviewed-supplier table.
assert.ok(!source.includes('4.0.0'), 'static gentle-pi 4.0.0 pin removed');
assert.ok(!source.includes("'gentle-pi': ["), 'no pinned [version, integrity] root entries remain');
assert.ok(!source.includes('.gentle-ai/v'), 'hardcoded native version path removed');
assert.ok(source.includes("exclusive(path.join(source, 'package.json'), rootDeclaration(pins));"), 'install declares the roots through rootDeclaration');

function extract(name) {
  const begin = `function ${name}(`;
  const start = source.indexOf(begin);
  assert.ok(start >= 0, `${name} helper present`);
  assert.equal(source.indexOf(begin, start + 1), -1, `${name} helper unique`);
  const end = source.indexOf('\n}', start);
  assert.ok(end > start, `${name} helper terminated`);
  return source.slice(start, end + 2);
}

const sandbox = { fs, path, Buffer, console, setTimeout, reject: refusal, read: file => fs.readFileSync(file) };
// The reviewed supplier list is loaded from the stage root: the fixture root
// carries the repository's real list, so every table scenario exercises the
// actual reviewed values the installer ships.
const listFixtureRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'provision-pins-list-'));
const listPath = path.join(listFixtureRoot, 'gentle-pi-suppliers.json');
fs.copyFileSync(fileURLToPath(new URL('./gentle-pi-suppliers.json', import.meta.url)), listPath);
sandbox.root = listFixtureRoot;
sandbox.digest = bytes => crypto.createHash('sha256').update(bytes).digest('hex');
vm.createContext(sandbox);
for (const name of ['exclusive', 'rootDeclaration', 'rewriteSupplierDeclaration', 'rootAuthorities', 'reviewedSuppliers', 'newestReviewedSupplier', 'supplierFallbackVersion', 'reviewedSupplierEntry', 'reviewedSupplierLock', 'stockNativeDirectory', 'stockNativeIntegrity', 'reviewedSupplierSources']) {
  vm.runInContext(extract(name), sandbox, { filename: `${name}.vm.js` });
}
const run = (code, ...args) => vm.runInContext(code, sandbox, { filename: 'call.vm.js' })(...args);

const names = ['gentle-pi', '@earendil-works/pi-coding-agent', '@earendil-works/pi-tui', '@heyhuynhgiabuu/pi-pretty', 'typebox'];
const sri = bytes => `sha512-${crypto.createHash('sha512').update(bytes).digest('base64')}`;
const record = (name, version, integrity) => ({
  version, integrity, resolved: `https://registry.npmjs.org/${name}/-/${name.split('/').at(-1)}-${version}.tgz`,
});
const lockFixture = (versions, mutations = {}) => ({
  lockfileVersion: 3,
  packages: {
    '': { name: 'gentle-shell-windows-private', dependencies: Object.fromEntries(names.map(name => [name, 'latest'])) },
    ...Object.fromEntries(names.map(name => [`node_modules/${name}`, record(name, versions[name] ?? '1.0.0', sri(name))])),
    ...mutations,
  },
});

let total = 0;
const failures = [];
const scenario = (name, body) => {
  total++;
  try { body(); } catch (error) { failures.push(`${name}: ${error.message}`); }
};

scenario('root-lock-canonical', () => {
  const resolved = run('(lock, list) => rootAuthorities(lock, list)', lockFixture({
    'gentle-pi': '4.3.0', '@earendil-works/pi-coding-agent': '1.3.0', '@earendil-works/pi-tui': '1.3.0',
    '@heyhuynhgiabuu/pi-pretty': '0.7.1', typebox: '1.4.2',
  }), names);
  assert.deepEqual({ ...resolved }, {
    'gentle-pi': '4.3.0', '@earendil-works/pi-coding-agent': '1.3.0', '@earendil-works/pi-tui': '1.3.0',
    '@heyhuynhgiabuu/pi-pretty': '0.7.1', typebox: '1.4.2',
  }, 'exact resolved versions pinned per root package');
});

scenario('root-lock-missing-package-refused', () => {
  const lock = lockFixture({});
  delete lock.packages['node_modules/@earendil-works/pi-tui'];
  lock.packages[''].dependencies = Object.fromEntries(Object.entries(lock.packages[''].dependencies).filter(([name]) => name !== '@earendil-works/pi-tui'));
  assert.throws(() => run('(lock, list) => rootAuthorities(lock, list)', lock, names), /Windows provision refused: root package set/, 'missing root record refused');
});

scenario('root-lock-extra-package-refused', () => {
  const lock = lockFixture({});
  lock.packages[''].dependencies.foreign = 'latest';
  lock.packages['node_modules/foreign'] = record('foreign', '9.9.9', sri('foreign'));
  assert.throws(() => run('(lock, list) => rootAuthorities(lock, list)', lock, names), /Windows provision refused: root package set/, 'extra root package refused');
});

scenario('root-lock-renamed-package-refused', () => {
  const lock = lockFixture({});
  delete lock.packages[''].dependencies['gentle-pi'];
  lock.packages[''].dependencies['gentle-pi-next'] = 'latest';
  assert.throws(() => run('(lock, list) => rootAuthorities(lock, list)', lock, names), /Windows provision refused: root package set/, 'renamed root package refused');
});

scenario('root-lock-static-pin-refused', () => {
  // A stale static expectation (exact old version with its old SRI) must never be
  // the acceptance criterion; the lock's own resolved exact version is.
  const lock = lockFixture({ 'gentle-pi': '4.3.0' });
  assert.doesNotThrow(() => run('(lock, list) => rootAuthorities(lock, list)', lock, names), 'resolved latest accepted without a static pin');
});

for (const [name, version] of [['latest-literal', 'latest'], ['caret-range', '^4.3.0'], ['prerelease', '4.3.0-beta.1'], ['non-string', 4]]) {
  scenario(`root-lock-version-${name}-refused`, () => {
    const lock = lockFixture({});
    lock.packages['node_modules/gentle-pi'].version = version;
    assert.throws(() => run('(lock, list) => rootAuthorities(lock, list)', lock, names), /Windows provision refused: root SRI\/version/, `${name} version refused`);
  });
}

for (const [name, integrity] of [
  ['wrong-algorithm', 'md5-' + 'A'.repeat(86) + '=='],
  ['truncated', 'sha512-AAAA'],
  ['invalid-base64', `sha512-${'!'.repeat(86)}==`],
  ['missing', undefined],
]) {
  scenario(`root-lock-integrity-${name}-refused`, () => {
    const lock = lockFixture({});
    if (integrity === undefined) delete lock.packages['node_modules/gentle-pi'].integrity;
    else lock.packages['node_modules/gentle-pi'].integrity = integrity;
    assert.throws(() => run('(lock, list) => rootAuthorities(lock, list)', lock, names), /Windows provision refused: root SRI\/version/, `${name} integrity refused`);
  });
}

// Reviewed-supplier table: the Windows stock lane resolves gentle-pi only onto
// reviewed releases; an unreviewed `latest` falls back once to the newest
// reviewed version, and every listed-version deviation fails closed.
scenario('reviewed-table-shape', () => {
  const table = run('() => reviewedSuppliers()');
  assert.ok(table['4.3.0'] && table['4.4.0'] && table['4.5.0'], 'reviewed 4.3.0/4.4.0/4.5.0 listed');
  for (const [version, entry] of Object.entries(table)) {
    assert.match(version, /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/, 'exact reviewed version');
    assert.match(entry.integrity, /^sha512-[A-Za-z0-9+/]{86}==$/, 'entry binds the npm tarball SRI');
    assert.match(entry.installer, /^[0-9a-f]{64}$/, 'entry binds the installer SHA-256');
    assert.match(entry.binary, /^[0-9a-f]{64}$/, 'entry binds the binary SHA-256');
  }
});

scenario('reviewed-list-values', () => {
  // Independent literals: the staged list must carry exactly the reviewed
  // 4.3.0/4.4.0/4.5.0 supplier bindings.
  const table = run('() => reviewedSuppliers()');
  assert.deepEqual(JSON.parse(JSON.stringify(table)), {
    '4.3.0': { integrity: 'sha512-xXBEZaGqden4bblEPagQEnkF12L0JHhsPr1REBGvhSeDy6mbHDQmEvUU5ATr9lvWb7xsTOXV7gwnfq4U+z7lpA==', installer: '8679f49d3675323bf7ff5c24321f08016bb0f8e79ded307b5900c37648f52470', binary: '61bb8e7e23864a15243882fc029f25f64118d593088cd7f64c3bb519dcf80b80' },
    '4.4.0': { integrity: 'sha512-NlBlDil1sNeoEXqpmT4DkgTCLv/KCZiGYG4ukzx2Z0PQm1+X8FZEtoFnb0eeO5mmgTkWly7vItgAhc9ApGVF0A==', installer: '37be34091b5050e6837ec9b3727cc49a738c98331d73c3dd549453b05c6e77ed', binary: '61bb8e7e23864a15243882fc029f25f64118d593088cd7f64c3bb519dcf80b80' },
    '4.5.0': { integrity: 'sha512-hw4W2hBvyGWPbPEpvAIgx4F+5IgQjZ1Yzhj159DRcbrvFBde3H8Jh9qKQF2EYTH7WcI+ixJqTXFF92pykOuXIg==', installer: '37be34091b5050e6837ec9b3727cc49a738c98331d73c3dd549453b05c6e77ed', binary: '61bb8e7e23864a15243882fc029f25f64118d593088cd7f64c3bb519dcf80b80' },
  }, 'staged list binds the reviewed 4.3.0/4.4.0/4.5.0 supplier values');
});

// The provisioner re-validates the staged list strictly: a foreign schema, a
// non-semver key, a malformed SRI or digest, an unknown entry field, an empty
// release set or a non-object shape all fail closed.
const listMutations = {
  'foreign-schema': list => { list.schema = 'other/v1'; },
  'missing-schema': list => { delete list.schema; },
  'missing-releases': list => { delete list.releases; },
  'empty-releases': list => { list.releases = {}; },
  'array-releases': list => { list.releases = []; },
  'range-key': list => { list.releases['^4.6.0'] = list.releases['4.5.0']; delete list.releases['4.5.0']; },
  'prerelease-key': list => { list.releases['4.6.0-beta.1'] = list.releases['4.5.0']; delete list.releases['4.5.0']; },
  'wrong-sri-algorithm': list => { list.releases['4.5.0'].integrity = 'sha1-' + list.releases['4.5.0'].integrity.slice(7); },
  'truncated-sri': list => { list.releases['4.5.0'].integrity = 'sha512-AAAA'; },
  'nonroundtrip-sri': list => { const value = list.releases['4.5.0'].integrity; list.releases['4.5.0'].integrity = value.slice(0, 95) + 'B' + value.slice(96); },
  'uppercase-installer-digest': list => { list.releases['4.5.0'].installer = list.releases['4.5.0'].installer.toUpperCase(); },
  'short-binary-digest': list => { list.releases['4.5.0'].binary = list.releases['4.5.0'].binary.slice(1); },
  'unknown-entry-field': list => { list.releases['4.5.0'].foreign = 'x'; },
  'unknown-top-level-field': list => { list.foreign = 'x'; },
  'array-entry': list => { list.releases['4.5.0'] = []; },
};
scenario('reviewed-list-refusals', () => {
  const original = fs.readFileSync(listPath);
  try {
    for (const [name, mutate] of Object.entries(listMutations)) {
      const list = JSON.parse(original);
      mutate(list);
      fs.writeFileSync(listPath, JSON.stringify(list));
      assert.throws(() => run('() => reviewedSuppliers()'), /Windows provision refused: reviewed supplier list/, `${name} staged list refused`);
    }
    fs.writeFileSync(listPath, 'not json');
    assert.throws(() => run('() => reviewedSuppliers()'), /Windows provision refused|Unexpected token/, 'malformed staged list refused');
    fs.rmSync(listPath);
    assert.throws(() => run('() => reviewedSuppliers()'), /ENOENT/, 'absent staged list refused');
  } finally {
    fs.writeFileSync(listPath, original);
  }
});

// The staged list read is bounded at 256 KiB like the Go loader: trailing
// whitespace keeps the JSON valid, so only the byte bound refuses it.
scenario('reviewed-list-oversize-refused', () => {
  const original = fs.readFileSync(listPath);
  try {
    fs.writeFileSync(listPath, Buffer.concat([original, Buffer.from(' '.repeat(262145))]));
    assert.throws(() => run('() => reviewedSuppliers()'), /Windows provision refused: reviewed supplier list/, 'oversized staged list refused');
  } finally {
    fs.writeFileSync(listPath, original);
  }
});

scenario('reviewed-latest-kept', () => {
  const table = run('() => reviewedSuppliers()');
  assert.deepEqual({ ...run('(version, integrity, table) => reviewedSupplierEntry(version, integrity, table)', '4.5.0', table['4.5.0'].integrity, table) }, { ...table['4.5.0'] }, 'reviewed version with matching integrity accepted');
  assert.equal(run('(version, integrity, table) => supplierFallbackVersion(version, integrity, table)', '4.5.0', table['4.5.0'].integrity, table), null, 'no fallback when latest resolves a reviewed version');
});

scenario('unreviewed-latest-falls-back', () => {
  const table = run('() => reviewedSuppliers()');
  assert.equal(run('(version, integrity, table) => supplierFallbackVersion(version, integrity, table)', '4.6.0', sri('unreviewed'), table), '4.5.0', 'unreviewed latest falls back to the newest reviewed version');
});

scenario('listed-wrong-integrity-refused-no-fallback', () => {
  const table = run('() => reviewedSuppliers()');
  assert.equal(run('(version, integrity, table) => reviewedSupplierEntry(version, integrity, table)', '4.3.0', sri('tampered'), table), null, 'listed version with wrong lock integrity refused');
  assert.equal(run('(version, integrity, table) => supplierFallbackVersion(version, integrity, table)', '4.3.0', sri('tampered'), table), null, 'listed version never falls back on an integrity deviation');
});

scenario('lock-gate-keeps-main-overlay-exemption', () => {
  const table = run('() => reviewedSuppliers()');
  const gate = '(action, mainOverlay, version, integrity, table) => reviewedSupplierLock(action, mainOverlay, version, integrity, table)';
  // Main overlay/verify gentle-pi source is bound by the frozen Main ZIP, so an
  // older stock lock (installed before its version was reviewed) stays verifiable.
  for (const action of ['overlay', 'verify']) {
    assert.doesNotThrow(() => run(gate, action, true, '4.2.0', sri('older'), table), `Main ${action} ignores the stock lock version`);
  }
  assert.throws(() => run(gate, 'verify', false, '4.2.0', sri('older'), table),
    /Windows provision refused: stock native supplier source pin/, 'stable verify of an unreviewed lock refused');
  assert.throws(() => run(gate, 'install', false, '4.3.0', sri('tampered'), table),
    /Windows provision refused: stock native supplier source pin/, 'install of a listed version with a different SRI refused');
  assert.doesNotThrow(() => run(gate, 'verify', false, '4.3.0', table['4.3.0'].integrity, table), 'stable verify of a reviewed 4.3.0 lock accepted');
});

scenario('supplier-digest-mismatch-refused', () => {
  const gentle = path.join(fs.mkdtempSync(path.join(os.tmpdir(), 'provision-pins-')), 'gentle-pi');
  fs.mkdirSync(path.join(gentle, 'scripts'), { recursive: true });
  fs.mkdirSync(path.join(gentle, 'runtime'), { recursive: true });
  fs.writeFileSync(path.join(gentle, 'scripts/gentle-ai-installer.mjs'), 'installer');
  fs.writeFileSync(path.join(gentle, 'runtime/gentle-ai-binary.mjs'), 'binary');
  const table = { '4.5.0': { integrity: sri('tarball'), installer: '0'.repeat(64), binary: '0'.repeat(64) } };
  assert.throws(() => run('(gentle, version, table) => reviewedSupplierSources(gentle, version, table)', gentle, '4.5.0', table),
    /Windows provision refused: stock native supplier source pin/, 'supplier digest mismatch refused');
  assert.throws(() => run('(gentle, version, table) => reviewedSupplierSources(gentle, version, table)', gentle, '4.6.0', table),
    /Windows provision refused: stock native supplier source pin/, 'unlisted version at the supplier check refused');
});

scenario('root-declaration-latest', () => {
  const manifest = JSON.parse(run('(names) => rootDeclaration(names)', names));
  assert.deepEqual(Object.keys(manifest), ['name', 'private', 'version', 'dependencies'], 'declaration shape unchanged');
  assert.deepEqual(manifest.dependencies, Object.fromEntries(names.map(name => [name, 'latest'])), 'every root resolves the latest dist-tag');
});

scenario('fallback-rewrite-pins-gentle-pi-and-discards-lock', () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'provision-pins-'));
  fs.writeFileSync(path.join(directory, 'package.json'), run('(names) => rootDeclaration(names)', names));
  fs.writeFileSync(path.join(directory, 'package-lock.json'), '{"lockfileVersion":3}');
  run('(directory, names, fallback) => rewriteSupplierDeclaration(directory, names, fallback)', directory, names, '4.5.0');
  const manifest = JSON.parse(fs.readFileSync(path.join(directory, 'package.json'), 'utf8'));
  assert.deepEqual(manifest.dependencies, Object.fromEntries(names.map(name => [name, name === 'gentle-pi' ? '4.5.0' : 'latest'])),
    'gentle-pi pinned to the reviewed fallback, the other roots stay latest');
  assert.equal(fs.existsSync(path.join(directory, 'package-lock.json')), false, 'generated lock discarded before re-resolution');
});

scenario('installer-mismatch-with-matching-binary-refused', () => {
  const gentle = path.join(fs.mkdtempSync(path.join(os.tmpdir(), 'provision-pins-')), 'gentle-pi');
  fs.mkdirSync(path.join(gentle, 'scripts'), { recursive: true });
  fs.mkdirSync(path.join(gentle, 'runtime'), { recursive: true });
  fs.writeFileSync(path.join(gentle, 'scripts/gentle-ai-installer.mjs'), 'tampered installer');
  fs.writeFileSync(path.join(gentle, 'runtime/gentle-ai-binary.mjs'), 'binary');
  const sha256 = bytes => crypto.createHash('sha256').update(bytes).digest('hex');
  const table = { '4.5.0': { integrity: sri('tarball'), installer: sha256('installer'), binary: sha256('binary') } };
  assert.throws(() => run('(gentle, version, table) => reviewedSupplierSources(gentle, version, table)', gentle, '4.5.0', table),
    /Windows provision refused: stock native supplier source pin/, 'installer digest mismatch refused even when the binary matches');
});

scenario('supplier-digest-match-accepted', () => {
  const gentle = path.join(fs.mkdtempSync(path.join(os.tmpdir(), 'provision-pins-')), 'gentle-pi');
  fs.mkdirSync(path.join(gentle, 'scripts'), { recursive: true });
  fs.mkdirSync(path.join(gentle, 'runtime'), { recursive: true });
  fs.writeFileSync(path.join(gentle, 'scripts/gentle-ai-installer.mjs'), 'installer');
  fs.writeFileSync(path.join(gentle, 'runtime/gentle-ai-binary.mjs'), 'binary');
  const sha256 = bytes => crypto.createHash('sha256').update(bytes).digest('hex');
  const table = { '4.5.0': { integrity: sri('tarball'), installer: sha256('installer'), binary: sha256('binary') } };
  assert.doesNotThrow(() => run('(gentle, version, table) => reviewedSupplierSources(gentle, version, table)', gentle, '4.5.0', table), 'matching reviewed supplier digests accepted');
});

scenario('semver-ordering', () => {
  assert.equal(run('(table) => newestReviewedSupplier(table)', { '4.9.0': {}, '4.10.0': {}, '4.2.0': {} }), '4.10.0', '4.10.0 sorts above 4.9.0');
  assert.equal(run('(table) => newestReviewedSupplier(table)', run('() => reviewedSuppliers()')), '4.5.0', 'newest reviewed table entry selected');
});

function nativeFixture(version) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'provision-pins-'));
  const bundle = path.join(root, '.gentle-ai', `v${version}`);
  fs.mkdirSync(bundle, { recursive: true });
  fs.writeFileSync(path.join(bundle, 'gentle-ai.exe'), 'binary');
  fs.writeFileSync(path.join(bundle, 'integrity.json'), JSON.stringify({
    version, method: 'go-sumdb-source-build', moduleChecksum: `h1:${'A'.repeat(43)}=`, binarySha256: sri('binary'),
  }));
  return root;
}

scenario('native-directory-dynamic', () => {
  const root = nativeFixture('4.3.0');
  const located = run('(nativeRoot) => stockNativeDirectory(nativeRoot)', path.join(root, '.gentle-ai'));
  assert.equal(located.directory, path.join(root, '.gentle-ai', 'v4.3.0'), 'dynamic version directory located');
  assert.equal(located.version, '4.3.0', 'dynamic version read from the directory');
});

for (const [name, versions] of [
  ['missing', []],
  ['foreign-only', ['v4.0.0']],
  ['duplicate-extra', ['v4.3.0', 'v4.0.0']],
]) {
  scenario(`native-directory-${name}-refused`, () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), 'provision-pins-'));
    fs.mkdirSync(path.join(root, '.gentle-ai'), { recursive: true });
    for (const label of versions) fs.mkdirSync(path.join(root, '.gentle-ai', label));
    assert.throws(() => run('(nativeRoot, version) => stockNativeDirectory(nativeRoot)', path.join(root, '.gentle-ai')),
      /Windows provision refused: stock native source layout/, `${name} native directory refused`);
  });
}

scenario('native-directory-absent-refused', () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'provision-pins-'));
  assert.throws(() => run('(nativeRoot, version) => stockNativeDirectory(nativeRoot)', path.join(root, '.gentle-ai')),
    /Windows provision refused: stock native source layout/, 'absent .gentle-ai refused');
});

scenario('native-directory-file-refused', () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'provision-pins-'));
  fs.mkdirSync(path.join(root, '.gentle-ai'), { recursive: true });
  fs.writeFileSync(path.join(root, '.gentle-ai', 'v4.3.0'), 'not a directory');
  assert.throws(() => run('(nativeRoot, version) => stockNativeDirectory(nativeRoot)', path.join(root, '.gentle-ai')),
    /Windows provision refused: stock native source layout/, 'non-directory version entry refused');
});

const nativeSandbox = { path, digest: bytes => sri(bytes), read: file => fs.readFileSync(file), reject: refusal };
const integrityFn = vm.runInNewContext(`(${extract('stockNativeIntegrity')})`, nativeSandbox, { filename: 'integrity.vm.js' });
scenario('native-manifest-canonical', () => {
  const root = nativeFixture('4.3.0');
  const manifest = JSON.parse(fs.readFileSync(path.join(root, '.gentle-ai/v4.3.0/integrity.json')));
  integrityFn(manifest, path.join(root, '.gentle-ai/v4.3.0'), '4.3.0');
});

for (const [name, mutate] of [
  ['version-mismatch', manifest => { manifest.version = '4.0.0'; }],
  ['method-swapped', manifest => { manifest.method = 'signed-release-asset'; }],
  ['checksum-malformed', manifest => { manifest.moduleChecksum = 'changed'; }],
  ['digest-mismatch', manifest => { manifest.binarySha256 = 'owned-native'; }],
]) {
  scenario(`native-manifest-${name}-refused`, () => {
    const root = nativeFixture('4.3.0');
    const manifest = JSON.parse(fs.readFileSync(path.join(root, '.gentle-ai/v4.3.0/integrity.json')));
    mutate(manifest);
    assert.throws(() => integrityFn(manifest, path.join(root, '.gentle-ai/v4.3.0'), '4.3.0'),
    /Windows provision refused: stock native source manifest/, `${name} manifest refused`);
  });
}

// Live evidence that npm's `latest` dist-tag resolves to exact versions with
// SRI, and that the root validator accepts the real generated lock. Skipped
// only when npm or the registry is unavailable; the Guest install itself has
// registry access.
scenario('latest-resolution-live', () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'provision-pins-'));
  fs.writeFileSync(path.join(root, 'package.json'), JSON.stringify({
    name: 'gentle-shell-windows-private', private: true, version: '1.0.0',
    dependencies: Object.fromEntries(names.map(name => [name, 'latest'])),
  }));
  const npm = spawnSync('npm', ['install', '--package-lock-only', '--ignore-scripts', '--engine-strict', '--no-audit', '--no-fund', '--min-release-age=0', '--registry=https://registry.npmjs.org/'], { cwd: root, encoding: 'utf8', timeout: 240000 });
  if (npm.error || npm.status !== 0) {
    console.log(`SKIP latest-resolution-live: npm unavailable or registry unreachable (${npm.error?.code ?? `status=${npm.status}`})`);
    return;
  }
  const lock = JSON.parse(fs.readFileSync(path.join(root, 'package-lock.json'), 'utf8'));
  assert.equal(lock.packages[''].dependencies['gentle-pi'], 'latest', 'root spec stays the dist-tag');
  const resolved = run('(lock, list) => rootAuthorities(lock, list)', lock, names);
  for (const name of names) {
    assert.match(resolved[name], /^\d+\.\d+\.\d+$/, `${name} resolved to an exact version`);
    assert.notEqual(resolved[name], name === 'gentle-pi' ? '4.0.0' : '1.0.0', `${name} is not the retired static pin`);
  }
});

console.log(`windows-provision-pins: ${total - failures.length}/${total} scenarios passed`);
for (const failure of failures) console.log(`FAIL ${failure}`);
process.exitCode = failures.length === 0 ? 0 : 1;
