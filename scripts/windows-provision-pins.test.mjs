// Portable VM fixtures for the Windows provisioner's version policy: latest
// dist-tag resolution installed exactly as resolved, the reviewed-supplier
// table demoted to advisory warnings that never gate the channel, and a
// discovered native bundle directory. They prove the validation helper logic,
// not an installed Windows runtime. Adapted from the windows-provision-copy
// VM region pattern.
import assert from 'node:assert/strict';
import crypto from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import vm from 'node:vm';
import { spawnSync } from 'node:child_process';
import { fileURLToPath, pathToFileURL } from 'node:url';

const provision = process.argv[2] ?? fileURLToPath(new URL('./provision-gentle-shell-windows.mjs', import.meta.url));
const source = fs.readFileSync(provision, 'utf8');
const refusal = message => { throw Error(`Windows provision refused: ${message}`); };

// Static contract: no static root version survives in the provisioner; install
// declares every root at npm's `latest` dist-tag (rootDeclaration, tested below)
// and the channel resolution is final — the reviewed-supplier table is advisory
// data that warns and never gates.
assert.ok(!source.includes('4.0.0'), 'static gentle-pi 4.0.0 pin removed');
assert.ok(!source.includes("'gentle-pi': ["), 'no pinned [version, integrity] root entries remain');
assert.ok(!source.includes('.gentle-ai/v'), 'hardcoded native version path removed');
assert.ok(source.includes("exclusive(path.join(source, 'package.json'), rootDeclaration(pins));"), 'install declares the roots through rootDeclaration');
assert.ok(!source.includes('supplierFallbackVersion') && !source.includes('provision-supplier-fallback'), 'no fallback re-resolution remains');
assert.ok(!source.includes('reviewedSupplierLock') && !source.includes('reviewedSupplierEntry'), 'the reviewed table never gates the channel');
assert.ok(source.includes('reviewedSupplierAdvisory'), 'the reviewed table reports advisories only');

function extract(name) {
  const begin = `function ${name}(`;
  const found = source.indexOf(begin);
  const start = source.slice(found - 6, found) === 'async ' ? found - 6 : found;
  assert.ok(start >= 0, `${name} helper present`);
  assert.equal(source.indexOf(begin, found + 1), -1, `${name} helper unique`);
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
Object.assign(sandbox, { crypto, pathToFileURL, companionName: 'gentle-engram' });
vm.createContext(sandbox);
for (const name of ['absent', 'companionPin', 'companionDeclaration', 'publishEngramCore', 'exclusive', 'rootDeclaration', 'rewriteSupplierDeclaration', 'rootAuthorities', 'reviewedSuppliers', 'reviewedSupplierAdvisory', 'reviewedSupplierAdvisorySources', 'stockNativeDirectory', 'stockNativeIntegrity']) {
  vm.runInContext(extract(name), sandbox, { filename: `${name}.vm.js`, importModuleDynamically: vm.constants.USE_MAIN_CONTEXT_DEFAULT_LOADER });
}
const run = (code, ...args) => vm.runInContext(code, sandbox, { filename: 'call.vm.js' })(...args);
// Advisory capture: the sandbox shares the host console object, so overriding
// console.error here records the provisioner's advisory lines.
const capture = body => {
  const errors = [], original = console.error;
  console.error = message => errors.push(String(message));
  try { return { errors, result: body() }; } finally { console.error = original; }
};
const advisory = '(action, mainOverlay, version, integrity, table) => reviewedSupplierAdvisory(action, mainOverlay, version, integrity, table)';
const sources = '(gentle, version, table) => reviewedSupplierAdvisorySources(gentle, version, table)';

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
  assert.ok(Object.keys(table).length >= 1, 'the shipped list binds at least one reviewed release');
  for (const [version, entry] of Object.entries(table)) {
    assert.match(version, /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/, 'exact reviewed version');
    assert.match(entry.integrity, /^sha512-[A-Za-z0-9+/]{86}==$/, 'entry binds the npm tarball SRI');
    assert.match(entry.installer, /^[0-9a-f]{64}$/, 'entry binds the installer SHA-256');
    assert.match(entry.binary, /^[0-9a-f]{64}$/, 'entry binds the binary SHA-256');
  }
});

scenario('reviewed-list-roundtrips', () => {
  // The provisioner invents nothing and drops nothing: the parsed advisory
  // table is exactly the shipped list, whatever releases it currently binds.
  const file = JSON.parse(fs.readFileSync(fileURLToPath(new URL('./gentle-pi-suppliers.json', import.meta.url)), 'utf8'));
  const table = run('() => reviewedSuppliers()');
  assert.deepEqual(JSON.parse(JSON.stringify(table)), file.releases, 'the parsed table is exactly the shipped list');
});

// The provisioner still parses the staged list strictly, but the list is
// advisory data: a foreign schema, a non-semver key, a malformed SRI or
// digest, an unknown entry field, an empty release set or a non-object shape
// all degrade to no supplier advisories instead of refusing the install.
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
// The staged list is advisory data: an unreadable, malformed or empty list
// degrades to no supplier advisories instead of refusing the channel install.
scenario('reviewed-list-degrades-to-no-advisories', () => {
  const original = fs.readFileSync(listPath);
  try {
    for (const [name, mutate] of Object.entries(listMutations)) {
      const list = JSON.parse(original);
      mutate(list);
      fs.writeFileSync(listPath, JSON.stringify(list));
      const { errors, result } = capture(() => run('() => reviewedSuppliers()'));
      assert.deepEqual({ ...result }, {}, `${name} staged list degrades to no advisories`);
      assert.equal(errors.length, 1, `${name} staged list warns once`);
      assert.match(errors[0], /reviewed supplier list is unusable/, `${name} staged list names the advisory`);
    }
    fs.writeFileSync(listPath, 'not json');
    assert.deepEqual({ ...capture(() => run('() => reviewedSuppliers()')).result }, {}, 'malformed staged list degrades');
    fs.rmSync(listPath);
    assert.deepEqual({ ...capture(() => run('() => reviewedSuppliers()')).result }, {}, 'absent staged list degrades');
  } finally {
    fs.writeFileSync(listPath, original);
  }
});

// The staged list read is bounded at 256 KiB like the Go loader: trailing
// whitespace keeps the JSON valid, so only the byte bound refuses it.
scenario('reviewed-list-oversize-degrades', () => {
  const original = fs.readFileSync(listPath);
  try {
    fs.writeFileSync(listPath, Buffer.concat([original, Buffer.from(' '.repeat(262145))]));
    const { errors, result } = capture(() => run('() => reviewedSuppliers()'));
    assert.deepEqual({ ...result }, {}, 'oversized staged list degrades to no advisories');
    assert.match(errors[0], /reviewed supplier list is unusable/, 'oversized staged list warns once');
  } finally {
    fs.writeFileSync(listPath, original);
  }
});

scenario('unreviewed-latest-advisory', () => {
  const table = run('() => reviewedSuppliers()');
  const { errors } = capture(() => run(advisory, 'install', false, '4.6.0', sri('unreviewed'), table));
  assert.deepEqual(errors, ['Windows provision advisory: gentle-pi@4.6.0 is outside the reviewed supplier table; installing the channel resolution'],
    'unreviewed latest installs the channel resolution and says so');
});

scenario('listed-wrong-integrity-advisory', () => {
  const table = run('() => reviewedSuppliers()');
  const { errors } = capture(() => run(advisory, 'install', false, '4.3.0', sri('tampered'), table));
  assert.deepEqual(errors, ['Windows provision advisory: gentle-pi@4.3.0 lock SRI differs from its reviewed entry; installing the channel resolution'],
    'a listed version whose lock SRI differs installs anyway and says so');
});

scenario('supplier-advisory-keeps-main-overlay-exemption', () => {
  const table = run('() => reviewedSuppliers()');
  // Main overlay/verify gentle-pi source is bound by the frozen Main ZIP, so
  // the stock lock advisory stays silent there.
  for (const action of ['overlay', 'verify']) {
    assert.deepEqual(capture(() => run(advisory, action, true, '4.2.0', sri('older'), table)).errors, [], `Main ${action} stays silent about the stock lock version`);
  }
  assert.deepEqual(capture(() => run(advisory, 'verify', false, '4.2.0', sri('older'), table)).errors,
    ['Windows provision advisory: gentle-pi@4.2.0 is outside the reviewed supplier table; installing the channel resolution'],
    'stable verify of an unreviewed lock advises');
  assert.deepEqual(capture(() => run(advisory, 'install', false, '4.3.0', sri('tampered'), table)).errors,
    ['Windows provision advisory: gentle-pi@4.3.0 lock SRI differs from its reviewed entry; installing the channel resolution'],
    'install of a listed version with a different SRI advises');
  assert.deepEqual(capture(() => run(advisory, 'verify', false, '4.3.0', table['4.3.0'].integrity, table)).errors, [], 'stable verify of a reviewed 4.3.0 lock stays silent');
});

scenario('supplier-digest-mismatch-advisory', () => {
  const gentle = path.join(fs.mkdtempSync(path.join(os.tmpdir(), 'provision-pins-')), 'gentle-pi');
  fs.mkdirSync(path.join(gentle, 'scripts'), { recursive: true });
  fs.mkdirSync(path.join(gentle, 'runtime'), { recursive: true });
  fs.writeFileSync(path.join(gentle, 'scripts/gentle-ai-installer.mjs'), 'installer');
  fs.writeFileSync(path.join(gentle, 'runtime/gentle-ai-binary.mjs'), 'binary');
  const table = { '4.5.0': { integrity: sri('tarball'), installer: '0'.repeat(64), binary: '0'.repeat(64) } };
  assert.deepEqual(capture(() => run(sources, gentle, '4.5.0', table)).errors,
    ['Windows provision advisory: gentle-pi@4.5.0 supplier file scripts/gentle-ai-installer.mjs differs from its reviewed entry'],
    'supplier digest mismatch advises');
  assert.deepEqual(capture(() => run(sources, gentle, '4.6.0', table)).errors,
    ['Windows provision advisory: gentle-pi@4.6.0 is outside the reviewed supplier table; supplier file digests unchecked'],
    'unlisted version at the supplier check advises');
});

scenario('root-declaration-latest', () => {
  const manifest = JSON.parse(run('(names) => rootDeclaration(names)', names));
  assert.deepEqual(Object.keys(manifest), ['name', 'private', 'version', 'dependencies'], 'declaration shape unchanged');
  assert.deepEqual(manifest.dependencies, Object.fromEntries(names.map(name => [name, 'latest'])), 'every root resolves the latest dist-tag');
});

scenario('companion-rewrite-pins-gentle-pi-and-discards-lock', () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'provision-pins-'));
  fs.writeFileSync(path.join(directory, 'package.json'), run('(names) => rootDeclaration(names)', names));
  fs.writeFileSync(path.join(directory, 'package-lock.json'), '{"lockfileVersion":3}');
  run('(directory, names, fallback) => rewriteSupplierDeclaration(directory, names, fallback)', directory, names, '4.5.0');
  const manifest = JSON.parse(fs.readFileSync(path.join(directory, 'package.json'), 'utf8'));
  assert.deepEqual(manifest.dependencies, Object.fromEntries(names.map(name => [name, name === 'gentle-pi' ? '4.5.0' : 'latest'])),
    'gentle-pi pinned to the companion re-resolution, the other roots stay latest');
  assert.equal(fs.existsSync(path.join(directory, 'package-lock.json')), false, 'generated lock discarded before re-resolution');
});

scenario('companion-declared-at-exact-pin', () => {
  const manifest = JSON.parse(run('(names, gentlePi, companion) => rootDeclaration(names, gentlePi, companion)', names, '4.5.1', '0.4.2'));
  assert.deepEqual(manifest.dependencies, { ...Object.fromEntries(names.map(name => [name, name === 'gentle-pi' ? '4.5.1' : 'latest'])), 'gentle-engram': '0.4.2' },
    'the companion is pinned to the gentle-pi pin, never latest');
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'provision-pins-'));
  fs.writeFileSync(path.join(directory, 'package.json'), run('(names) => rootDeclaration(names)', names));
  fs.writeFileSync(path.join(directory, 'package-lock.json'), '{"lockfileVersion":3}');
  run('(directory, names, version, companion) => rewriteSupplierDeclaration(directory, names, version, companion)', directory, names, '4.5.1', '0.4.2');
  assert.deepEqual(JSON.parse(fs.readFileSync(path.join(directory, 'package.json'), 'utf8')).dependencies, manifest.dependencies);
  assert.equal(fs.existsSync(path.join(directory, 'package-lock.json')), false, 'companion re-resolution discards the first lock');
  // The companion root joins the closed root set only as the sixth root.
  const lock = lockFixture({}, { 'node_modules/gentle-engram': record('gentle-engram', '0.4.2', sri('gentle-engram')) });
  lock.packages[''].dependencies['gentle-engram'] = '0.4.2';
  assert.equal(run('(lock, list) => rootAuthorities(lock, list)', lock, [...names, 'gentle-engram'])['gentle-engram'], '0.4.2');
  assert.throws(() => run('(lock, list) => rootAuthorities(lock, list)', lock, names), /Windows provision refused: root package set/, 'an undeclared extra root stays refused');
  assert.ok(source.includes("const roots = Object.hasOwn(lock?.packages?.['']?.dependencies ?? {}, companionName) ? [...pins, companionName] : pins;"), 'root set admits only the companion');
});

scenario('companion-pin-exact-only', () => {
  assert.equal(run('text => companionPin(text)', 'x\nexport const ENGRAM_COMPANION_SOURCE = "npm:gentle-engram@0.4.2";\ny'), '0.4.2');
  for (const text of ['export const ENGRAM_COMPANION_SOURCE = "npm:gentle-engram@latest";', 'export const ENGRAM_COMPANION_SOURCE = "npm:gentle-engram@~0.4.2";', '']) {
    assert.equal(run('text => companionPin(text)', text), null, `${text} refused as a pin`);
  }
});

scenario('companion-gate-agreement', () => {
  const errors = [], original = console.error;
  console.error = message => errors.push(message);
  try {
    const gate = (...args) => run('(pin, locked, installed) => companionDeclaration(pin, locked, installed)', ...args);
    assert.equal(gate('0.4.2', '0.4.2', '0.4.2'), 'npm:gentle-engram@0.4.2');
    // A lock/pin mismatch declares nothing and never refuses the install.
    assert.equal(gate('0.5.0', '0.4.2', '0.4.2'), null);
    assert.equal(gate('0.4.2', '0.4.2', '0.5.0'), null);
    // A gentle-pi that pins none (4.0.0) and a lock without the root: no companion, no note.
    assert.equal(gate(null, undefined, undefined), null);
  } finally { console.error = original; }
  assert.equal(errors.length, 2);
  assert.match(errors[0], /^Engram companion not declared: gentle-pi pins 0\.5\.0, lock 0\.4\.2, installed 0\.4\.2; Shell stays usable without memory$/);
  // Memory is the companion and the core together: the core follows the gate.
  assert.ok(source.includes("if (action === 'install' && declaredCompanion) await publishEngramCore(gentle, path.join(agent, 'bin'), 'engram.exe', 'win32', 'x64');"), 'core published only for a declared companion');
  assert.ok(!source.includes("if (action === 'install') await publishEngramCore("), 'no ungated core publication');
  // Verify admits the owned declaration with or without exactly the locked companion.
  assert.ok(source.includes("if (lockedCompanion && JSON.stringify(observedSettings?.packages) === JSON.stringify([...settings.packages, lockedCompanion])) settings.packages.push(lockedCompanion);"));
});

scenario('installer-mismatch-with-matching-binary-advisory', () => {
  const gentle = path.join(fs.mkdtempSync(path.join(os.tmpdir(), 'provision-pins-')), 'gentle-pi');
  fs.mkdirSync(path.join(gentle, 'scripts'), { recursive: true });
  fs.mkdirSync(path.join(gentle, 'runtime'), { recursive: true });
  fs.writeFileSync(path.join(gentle, 'scripts/gentle-ai-installer.mjs'), 'tampered installer');
  fs.writeFileSync(path.join(gentle, 'runtime/gentle-ai-binary.mjs'), 'binary');
  const sha256 = bytes => crypto.createHash('sha256').update(bytes).digest('hex');
  const table = { '4.5.0': { integrity: sri('tarball'), installer: sha256('installer'), binary: sha256('binary') } };
  assert.deepEqual(capture(() => run(sources, gentle, '4.5.0', table)).errors,
    ['Windows provision advisory: gentle-pi@4.5.0 supplier file scripts/gentle-ai-installer.mjs differs from its reviewed entry'],
    'installer digest mismatch advises even when the binary matches');
});

scenario('supplier-digest-match-stays-silent', () => {
  const gentle = path.join(fs.mkdtempSync(path.join(os.tmpdir(), 'provision-pins-')), 'gentle-pi');
  fs.mkdirSync(path.join(gentle, 'scripts'), { recursive: true });
  fs.mkdirSync(path.join(gentle, 'runtime'), { recursive: true });
  fs.writeFileSync(path.join(gentle, 'scripts/gentle-ai-installer.mjs'), 'installer');
  fs.writeFileSync(path.join(gentle, 'runtime/gentle-ai-binary.mjs'), 'binary');
  const sha256 = bytes => crypto.createHash('sha256').update(bytes).digest('hex');
  const table = { '4.5.0': { integrity: sri('tarball'), installer: sha256('installer'), binary: sha256('binary') } };
  assert.deepEqual(capture(() => run(sources, gentle, '4.5.0', table)).errors, [], 'matching reviewed supplier digests stay silent');
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

// The Engram core at the contract path <agent>\\bin\\engram.exe, published
// from a stand-in gentle-pi with the stock export names through injected
// adapters: no network, no Windows host required.
const scenarioAsync = async (name, body) => {
  total++;
  try { await body(); } catch (error) { failures.push(`${name}: ${error.message}`); }
};
function standInGentlePi(exports = true) {
  const gentle = path.join(fs.mkdtempSync(path.join(os.tmpdir(), 'provision-pins-')), 'gentle-pi');
  fs.mkdirSync(path.join(gentle, 'scripts'), { recursive: true });
  if (exports) {
    fs.writeFileSync(path.join(gentle, 'scripts/installer-downloads.mjs'), `
export function artifactFor(name, platform, arch) { return { name, platform, arch, url: 'https://example.invalid/engram_' + platform + '_' + arch + '.zip' }; }
export async function verifiedDownload(name, adapters = {}, platform, arch) {
  const bytes = await adapters.download((adapters.artifact ?? artifactFor)(name, platform, arch));
  if (!bytes.toString().startsWith('pinned:')) throw new Error('Download integrity mismatch');
  return bytes;
}`);
    fs.writeFileSync(path.join(gentle, 'scripts/bundled-install.mjs'), `
export function engramExecutable(bytes, descriptor) { return Buffer.from('exe<' + bytes + '>' + descriptor.url); }`);
  }
  return gentle;
}
const publish = (...args) => run('(...args) => publishEngramCore(...args)', ...args);
await scenarioAsync('engram-core-published', async () => {
  const gentle = standInGentlePi(), bin = path.join(path.dirname(gentle), 'agent/bin');
  fs.mkdirSync(bin, { recursive: true });
  assert.equal(await publish(gentle, bin, 'engram.exe', 'win32', 'x64', { download: async () => Buffer.from('pinned:zip') }), true);
  assert.equal(fs.readFileSync(path.join(bin, 'engram.exe'), 'utf8'), 'exe<pinned:zip>https://example.invalid/engram_win32_x64.zip');
  assert.deepEqual(fs.readdirSync(bin), ['engram.exe'], 'atomic publication leaves no stage file');
});
await scenarioAsync('engram-core-skipped-without-exports-or-verification', async () => {
  const errors = [], original = console.error;
  console.error = message => errors.push(message);
  try {
    const bare = standInGentlePi(false), bin = path.join(path.dirname(bare), 'agent/bin');
    assert.equal(await publish(bare, bin, 'engram.exe', 'win32', 'x64', {}), false);
    assert.equal(fs.existsSync(bin), false);
    const gentle = standInGentlePi(), verifiedBin = path.join(path.dirname(gentle), 'agent/bin');
    fs.mkdirSync(verifiedBin, { recursive: true });
    assert.equal(await publish(gentle, verifiedBin, 'engram.exe', 'win32', 'x64', { download: async () => Buffer.from('tampered') }), false);
    assert.deepEqual(fs.readdirSync(verifiedBin), []);
  } finally { console.error = original; }
  assert.deepEqual(errors, ['Engram core skipped: the installed gentle-pi pins no Engram core; Shell stays usable without memory',
    'Engram core skipped: Download integrity mismatch; Shell stays usable without memory']);
});

console.log(`windows-provision-pins: ${total - failures.length}/${total} scenarios passed`);
for (const failure of failures) console.log(`FAIL ${failure}`);
process.exitCode = failures.length === 0 ? 0 : 1;
