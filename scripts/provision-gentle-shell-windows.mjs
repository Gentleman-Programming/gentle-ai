// Windows Separate only. Stock npm, explicit stock native installer, no updater.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { spawnSync } from 'node:child_process';
import { pathToFileURL } from 'node:url';

const [root, action] = process.argv.slice(2);
const reject = message => { throw Error(`Windows provision refused: ${message}`); };
if (process.platform !== 'win32' || process.arch !== 'x64' || process.argv.length !== 4 || !['install', 'overlay', 'verify'].includes(action) || !path.isAbsolute(root)) reject('platform/arguments');
const digest = bytes => crypto.createHash('sha256').update(bytes).digest('hex');
const node = path.join(root, 'runtime/node/node.exe');
const npm = path.join(root, 'runtime/node/node_modules/npm/bin/npm-cli.js');
const prefix = path.join(root, 'prefix');
const modules = path.join(prefix, 'node_modules');
const source = path.join(root, 'source');
const agent = path.join(root, 'agent');
const selection = JSON.parse(read(path.join(root, 'selection.json')));
const selectionKeys = ['Channel', 'Confirmation', 'Destination', 'Mode', 'SharedAgent', 'SharedPrefix'];
if (selection === null || typeof selection !== 'object' || Array.isArray(selection) ||
    Object.keys(selection).sort().join(',') !== selectionKeys.join(',') || selectionKeys.some(key => typeof selection[key] !== 'string') ||
    selection.Mode !== 'separate' || selection.SharedPrefix !== '' || selection.SharedAgent !== '' || !['stable', 'main'].includes(selection.Channel) ||
    !path.isAbsolute(selection.Destination) || (action === 'verify' && selection.Destination !== root)) reject('publication channel/selection');
const finalRoot = selection.Destination; // Install/overlay stages differ from the final root.
const mainOverlay = selection.Channel === 'main' && action !== 'install';
if (selection.Channel === 'main') {
  const snapshot = JSON.parse(read(path.join(root, 'main.json'))), commit = snapshot?.Prefix?.slice('gentle-shell-'.length);
  if (Object.keys(snapshot).sort().join(',') !== 'Archive,Bound,Prefix,SHA,Size,URL' || !/^[0-9a-f]{40}$/.test(commit) ||
      snapshot.Prefix !== `gentle-shell-${commit}` || snapshot.URL !== `https://codeload.github.com/Gentleman-Programming/gentle-shell/zip/${commit}` ||
      !/^[0-9a-f]{64}$/.test(snapshot.SHA) || snapshot.Archive !== 'main.zip' || snapshot.Bound !== 33554432 || snapshot.Size !== 0) reject('Main descriptor authority');
}
// Stable resolves npm's `latest` dist-tag for the five stock root packages at
// install time; the generated package-lock.json pins the exact resolved versions
// and SRI. gentle-pi additionally resolves only onto reviewed supplier versions
// (see reviewedSuppliers): an unreviewed `latest` falls back once to the newest
// reviewed release, and every listed-version deviation fails closed.
const pins = ['gentle-pi', '@earendil-works/pi-coding-agent', '@earendil-works/pi-tui', '@heyhuynhgiabuu/pi-pretty', 'typebox'];
// Reviewed gentle-pi releases for the Windows stock lane, loaded from the
// stage list the Go worker writes (gentle-pi-suppliers.json, schema
// gentle-shell-reviewed-suppliers/v1): the live repository copy when its
// fetch and strict validation succeeded, otherwise the embedded copy. The
// provisioner re-validates the staged bytes strictly and never widens them.
// Each entry binds the exact version to its npm tarball SRI (the lock's
// `integrity`) and to the SHA-256 of the two native supplier files inside the
// package. Reviewing a new release is a one-line change to that JSON; the
// root specs stay `latest`.
function reviewedSuppliers() {
  const raw = read(path.join(root, 'gentle-pi-suppliers.json'));
  // The staged list read is bounded at 256 KiB, matching the Go loader's bound.
  if (raw.length > 262144) reject('reviewed supplier list');
  const list = JSON.parse(raw);
  const semver = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/;
  const sri = value => typeof value === 'string' && /^sha512-[A-Za-z0-9+/]{86}==$/.test(value) &&
    Buffer.from(value.slice(7), 'base64').toString('base64') === value.slice(7);
  if (list === null || typeof list !== 'object' || Array.isArray(list) || list.schema !== 'gentle-shell-reviewed-suppliers/v1' ||
      list.releases === null || typeof list.releases !== 'object' || Array.isArray(list.releases)) reject('reviewed supplier list');
  // Strict top-level shape: exactly the schema and releases fields, mirroring
  // the Go decoder's DisallowUnknownFields.
  if (Object.keys(list).sort().join(',') !== 'releases,schema') reject('reviewed supplier list');
  const table = {};
  for (const [version, entry] of Object.entries(list.releases)) {
    if (!semver.test(version) || entry === null || typeof entry !== 'object' || Array.isArray(entry) ||
        Object.keys(entry).sort().join(',') !== 'binary,installer,integrity' || !sri(entry.integrity) ||
        !/^[0-9a-f]{64}$/.test(entry.installer) || !/^[0-9a-f]{64}$/.test(entry.binary)) reject('reviewed supplier list');
    table[version] = entry;
  }
  if (Object.keys(table).length === 0) reject('reviewed supplier list');
  return table;
}
// Newest reviewed version in semantic order (4.10.0 > 4.9.0): the fallback
// target when npm's `latest` resolution is unreviewed.
function newestReviewedSupplier(table) {
  const order = version => version.split('.').map(Number);
  return Object.keys(table).sort((left, right) => {
    const a = order(left), b = order(right);
    return a[0] - b[0] || a[1] - b[1] || a[2] - b[2];
  }).at(-1);
}
// NTFS owner/DACL/reparse checks are enforced by the Go worker before entry and
// on the complete produced tree afterward. This is not a substitute for them.
function read(file) {
  const before = fs.lstatSync(file);
  if (!before.isFile() || before.isSymbolicLink() || before.size > 33554432) reject('physical file/bound');
  const value = fs.readFileSync(file);
  const after = fs.lstatSync(file);
  if (before.ino !== after.ino || before.size !== after.size || before.mtimeMs !== after.mtimeMs || before.ctimeMs !== after.ctimeMs) reject('file preimage differs');
  return value;
}
function exclusive(file, value) {
  fs.writeFileSync(file, value, { flag: 'wx' });
  if (!read(file).equals(Buffer.from(value))) reject('exclusive readback');
}
function absent(absolute) {
  try { fs.lstatSync(absolute); reject('occupied path'); }
  catch (error) { if (error.code !== 'ENOENT') throw error; }
}
// Root authority: the generated lock must pin the complete, closed root set with
// an exact resolved version and a well-formed sha512 SRI for every stock package.
// A missing or extra root package, a range/prerelease version, or a malformed
// integrity value fails closed; dist-tag resolution happens upstream in npm.
function rootAuthorities(lock, names) {
  if (lock === null || typeof lock !== 'object' || Array.isArray(lock) || lock.lockfileVersion !== 3 ||
      !lock.packages || typeof lock.packages !== 'object' || Array.isArray(lock.packages) ||
      !lock.packages[''] || Object.keys(lock.packages).length > 4096 ||
      JSON.stringify(Object.keys(lock.packages[''].dependencies ?? {}).sort()) !== JSON.stringify([...names].sort())) reject('root package set');
  const exact = value => typeof value === 'string' && /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(value);
  const sri = value => typeof value === 'string' && /^sha512-[A-Za-z0-9+/]{86}==$/.test(value) &&
    Buffer.from(value.slice(7), 'base64').toString('base64') === value.slice(7);
  const resolved = {};
  for (const name of names) {
    const record = lock.packages[`node_modules/${name}`];
    if (!record || !exact(record.version) || !sri(record.integrity) || typeof record.resolved !== 'string') reject('root SRI/version');
    resolved[name] = record.version;
  }
  return resolved;
}
// The locked gentle-pi version must be reviewed with the entry's exact npm
// tarball SRI. A listed version with a differing integrity fails closed and
// never falls back; only an unlisted one is fallback-eligible.
function reviewedSupplierEntry(version, integrity, table) {
  const entry = table[version];
  return entry && entry.integrity === integrity ? entry : null;
}
// Lock gate: stable install/verify and every install must lock a reviewed
// gentle-pi with its entry's exact SRI. Main overlay/verify keep the supplier
// exemption below: their gentle-pi is bound by the frozen Main ZIP, not this lock.
function reviewedSupplierLock(action, mainOverlay, version, integrity, table) {
  if ((action === 'install' || !mainOverlay) && !reviewedSupplierEntry(version, integrity, table)) reject('stock native supplier source pin');
}
// Single fallback re-resolution target: an unreviewed `latest` resolution is
// replaced by the newest reviewed version instead of failing. Listed versions
// are decided by reviewedSupplierEntry alone; verify/overlay never fall back.
function supplierFallbackVersion(version, integrity, table) {
  return table[version] ? null : newestReviewedSupplier(table);
}
// Locate the single supported native bundle directory under .gentle-ai: exactly
// one entry, named v<resolved gentle-pi version>, a physical directory. A
// missing, duplicate, foreign or non-directory version layout fails closed.
function stockNativeDirectory(nativeRoot) {
  let entries;
  try { entries = fs.readdirSync(nativeRoot); }
  catch { return reject('stock native source layout'); }
  const match = entries.length === 1 ? /^v((0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*))$/.exec(entries[0]) : null;
  if (!match) reject('stock native source layout');
  const directory = path.join(nativeRoot, entries[0]);
  if (!fs.lstatSync(directory).isDirectory() || !fs.existsSync(path.join(directory, 'integrity.json'))) reject('stock native source layout');
  return { directory, version: match[1] };
}
// The stock installer's Windows source-build manifest: the dynamic native bundle
// version (which can differ from the gentle-pi version), the supported go-sumdb
// source-build method, a well-formed Go module checksum, and the digest of the
// produced executable.
function stockNativeIntegrity(manifest, native, version) {
  if (manifest === null || typeof manifest !== 'object' || Array.isArray(manifest) ||
      manifest.version !== version || manifest.method !== 'go-sumdb-source-build' ||
      !/^h1:[A-Za-z0-9+/]{43}=$/.test(manifest.moduleChecksum) ||
      manifest.binarySha256 !== digest(read(path.join(native, 'gentle-ai.exe')))) reject('stock native source manifest');
}
// Reviewed supplier sources: the materialized/installed gentle-pi tree must
// carry the reviewed entry's exact installer and binary digests. Unlisted
// versions fail closed before any supplier file is read.
function reviewedSupplierSources(gentle, version, table) {
  const entry = table[version];
  if (!entry) reject('stock native supplier source pin');
  for (const [name, expected] of [
    ['scripts/gentle-ai-installer.mjs', entry.installer],
    ['runtime/gentle-ai-binary.mjs', entry.binary],
  ]) if (digest(read(path.join(gentle, name))) !== expected) reject('stock native supplier source pin');
}
// Whole-tree materialization digest adapted from the private-global provisioner's
// inventory: bounded entry/byte counts, physical regular files only, every entry
// content-addressed. Symlinks and special files fail closed before any copy.
function treeDigest(directory) {
  const records = [];
  let bytes = 0;
  function visit(absolute) {
    if (records.length >= 250000) reject('inventory entry bound');
    const relative = path.relative(directory, absolute);
    const info = fs.lstatSync(absolute);
    if (info.isSymbolicLink()) reject('inventory alias');
    if (info.isDirectory()) {
      records.push([relative, 'dir']);
      for (const name of fs.readdirSync(absolute).sort()) visit(path.join(absolute, name));
    } else {
      if (!info.isFile()) reject('inventory special file');
      const raw = read(absolute);
      bytes += raw.length;
      if (bytes > 1073741824) reject('inventory aggregate bound');
      records.push([relative, 'file', info.size, digest(raw)]);
    }
  }
  visit(directory);
  return digest(JSON.stringify(records));
}
// Single materialization: npm ci already writes the authenticated tree at
// source/node_modules; promote it by rename into the final prefix instead of
// copying and re-hashing every file. The lock/authority and metadata checks
// still run before the promotion.
let materialized = false;
function materializeGlobal() {
  fs.mkdirSync(prefix, { recursive: true });
  const previous = path.join(prefix, `.previous-${crypto.randomUUID()}`);
  absent(previous);
  const acquired = path.join(source, 'node_modules');
  if (!fs.existsSync(acquired)) reject('acquired source absent');
  const hidden = path.join(acquired, '.package-lock.json');
  if (fs.existsSync(hidden)) fs.unlinkSync(hidden);
  if (fs.existsSync(modules)) {
    fs.renameSync(modules, previous);
  }
  fs.renameSync(acquired, modules);
  materialized = true;
}
const NPM_NETWORK_TIMEOUT = 1200000; // Network-bound npm runs must not depend on connection speed.
function npmCommand(args, timeout = NPM_NETWORK_TIMEOUT) {
  const started = Date.now();
  let child = spawnSync(node, [npm, ...args], { cwd: source, env: process.env, shell: false, timeout, maxBuffer: 8388608 });
  if (child.error) {
    // Transport-class failure (spawn error or the spawn timeout): one bounded
    // retry with a fresh window separates a slow connection from a dead one.
    child = spawnSync(node, [npm, ...args], { cwd: source, env: process.env, shell: false, timeout, maxBuffer: 8388608 });
  }
  if (child.error || child.status !== 0 || child.signal) {
    const bytes = Buffer.concat([child.stdout ?? Buffer.alloc(0), child.stderr ?? Buffer.alloc(0)]);
    const text = bytes.toString('utf8');
    const detail = bytes.length <= 8192 && Buffer.from(text, 'utf8').equals(bytes) && !text.includes('\0')
      ? JSON.stringify(text).replace(/[\u007f-\u009f]/g, char => `\\u${char.charCodeAt(0).toString(16).padStart(4, '0')}`)
      : 'whole npm diagnostic withheld (size/encoding guard)';
    reject(`npm ${args[0]} failed; status=${child.status}; error=${child.error?.code ?? 'none'}; elapsedMs=${Date.now() - started}; bytes=${bytes.length}; sha256=${digest(bytes)}; diagnostic=${detail}`);
  }
}
const lockPath = path.join(source, 'package-lock.json');
const lockInstall = ['install', '--package-lock-only', '--ignore-scripts', '--engine-strict', '--no-audit', '--no-fund', '--min-release-age=0', '--registry=https://registry.npmjs.org/'];
// Root declaration: every stock root at `latest`, except a reviewed gentle-pi
// fallback, which is pinned to its exact version.
function rootDeclaration(names, gentlePi = 'latest') {
  const dependencies = Object.fromEntries(names.map(name => [name, name === 'gentle-pi' ? gentlePi : 'latest']));
  return JSON.stringify({ name: 'gentle-shell-windows-private', private: true, version: '1.0.0', dependencies });
}
// Fallback rewrite before the single re-resolution: exclusive() writes with wx,
// so the declaration is removed first, and the generated lock is discarded.
function rewriteSupplierDeclaration(directory, names, fallback) {
  fs.unlinkSync(path.join(directory, 'package.json'));
  fs.unlinkSync(path.join(directory, 'package-lock.json'));
  exclusive(path.join(directory, 'package.json'), rootDeclaration(names, fallback));
}
if (action === 'install') {
  console.error('Windows install phase: provision-lock');
  fs.mkdirSync(source);
  exclusive(path.join(source, 'package.json'), rootDeclaration(pins));
  npmCommand(lockInstall);
  const { completeFile } = await import(pathToFileURL(path.join(root, 'complete-generated-lock-sri.mjs')).href);
  await completeFile(lockPath);
  // Reviewed-latest fallback: an unreviewed `latest` resolution installs the
  // newest reviewed gentle-pi version instead of failing; the other four roots
  // stay `latest`. One re-resolution only.
  const first = JSON.parse(read(lockPath)), firstResolved = rootAuthorities(first, pins);
  const fallback = supplierFallbackVersion(firstResolved['gentle-pi'], first.packages['node_modules/gentle-pi'].integrity, reviewedSuppliers());
  if (fallback) {
    console.error(`Windows install phase: provision-supplier-fallback ${firstResolved['gentle-pi']} -> ${fallback}`);
    rewriteSupplierDeclaration(source, pins, fallback);
    npmCommand(lockInstall);
    await completeFile(lockPath);
  }
}
const lockBytes = read(lockPath), lock = JSON.parse(lockBytes);
const resolved = rootAuthorities(lock, pins);
// A listed version whose integrity differs from the reviewed entry never falls back.
reviewedSupplierLock(action, mainOverlay, resolved['gentle-pi'], lock.packages['node_modules/gentle-pi'].integrity, reviewedSuppliers());
for (const [relative, record] of Object.entries(lock.packages)) {
  if (!relative) continue;
  if (!/^node_modules\/(?:@?[a-z0-9][a-z0-9._-]*\/)*[a-z0-9][a-z0-9._-]*$/.test(relative)) reject('lock placement');
  const name = record.name ?? relative.split('node_modules/').at(-1);
  if (record.resolved !== `https://registry.npmjs.org/${name}/-/${name.split('/').at(-1)}-${record.version}.tgz` || !/^sha512-[A-Za-z0-9+/]{86}==$/.test(record.integrity)) reject('locked supplier');
}
if (action === 'install') {
  console.error('Windows install phase: provision-ci');
  npmCommand(['ci', '--ignore-scripts', '--engine-strict', '--no-audit', '--no-fund', '--min-release-age=0', '--registry=https://registry.npmjs.org/']);
  console.error('Windows install phase: provision-copy');
  materializeGlobal();
  console.error('Windows install phase: provision-rebuild');
  npmCommand(['rebuild', '--global', '--prefix', prefix, '--offline', '--ignore-scripts', '--bin-links=true', '--engine-strict', '--no-audit', '--no-fund'], 180000);
}
const authority = new Map();
// An install promotes source/node_modules into prefix/node_modules. A later
// verify/overlay run therefore has no source tree left; use the promoted tree
// as authority whenever it exists. A source tree that was not promoted (for
// example a staged overlay) remains authoritative.
const promotedAuthority = materialized || !fs.existsSync(path.join(source, 'node_modules'));
for (const [relative, record] of Object.entries(lock.packages)) {
  if (!relative) continue;
  const acquiredRoot = promotedAuthority ? modules : source;
  // The promoted root is already prefix/node_modules, so lock-relative
  // node_modules/... paths must lose that one prefix.
  const acquiredRelative = promotedAuthority ? relative.slice('node_modules/'.length) : relative;
  const metadata = path.join(acquiredRoot, acquiredRelative, 'package.json');
  if (!fs.existsSync(metadata)) {
    if (record.optional !== true) reject('required acquired source absent');
    continue;
  }
  const bytes = read(metadata), identity = JSON.parse(bytes);
  const name = record.name ?? relative.split('node_modules/').at(-1);
  if (identity.name !== name || identity.version !== record.version) reject('acquired identity');
  const key = `${name}@${record.version}`, previous = authority.get(key);
  if (previous && (!previous.bytes.equals(bytes) || previous.integrity !== record.integrity)) reject('ambiguous acquired source');
  authority.set(key, { directory: path.dirname(metadata), bytes, integrity: record.integrity });
}
let count = 0, compared = 0, bytes = 0;
function compare(actual, original, gentle = false) {
  const names = directory => fs.readdirSync(directory).filter(name => name !== 'node_modules' && !(gentle && name === '.gentle-ai')).sort();
  const a = names(actual), b = names(original);
  if (JSON.stringify(a) !== JSON.stringify(b)) reject('global source set differs');
  for (const name of a) {
    if (++compared > 250000) reject('source file count');
    const left = path.join(actual, name), right = path.join(original, name);
    const info = fs.lstatSync(left);
    if (info.isSymbolicLink()) reject('source alias');
    if (info.isDirectory()) compare(left, right);
    else {
      const value = read(left);
      bytes += value.length;
      if (bytes > 1073741824 || !value.equals(read(right))) reject('source bytes/bound');
    }
  }
}
function packages(directory) {
  for (const name of fs.readdirSync(directory).sort()) {
    if (name === '.bin' || name === '.package-lock.json') continue;
    const absolute = path.join(directory, name);
    const info = fs.lstatSync(absolute);
    if (!info.isDirectory() || info.isSymbolicLink()) reject('global package alias/type');
    if (name.startsWith('@')) { packages(absolute); continue; }
    if (++count > 4096) reject('global graph bound');
    const metadata = read(path.join(absolute, 'package.json')), record = JSON.parse(metadata);
    const acquired = authority.get(`${record.name}@${record.version}`);
    if (mainOverlay && absolute === path.join(modules, 'gentle-pi')) {
      // Go has compared every Main source member/set against the retained ZIP.
      // Its authority is the frozen publisher snapshot, not this npm SRI record.
      const peers = { '@earendil-works/pi-coding-agent': '>=0.99.1', '@earendil-works/pi-ai': '*', '@earendil-works/pi-tui': '*', typebox: '*' };
      if (!acquired || record.name !== 'gentle-pi' || record.version !== resolved['gentle-pi'] || record.engines?.node !== '>=22.19.0' ||
          JSON.stringify(record.dependencies) !== JSON.stringify(JSON.parse(acquired.bytes).dependencies) ||
          Object.keys(record.peerDependencies ?? {}).length !== 4 || Object.entries(peers).some(([name, range]) => record.peerDependencies?.[name] !== range)) reject('unsupported Main dependency/native composition');
    } else {
      if (!acquired || !metadata.equals(acquired.bytes)) reject('global unauthenticated metadata');
      // A successful materializeGlobal already proved the copied tree equals
      // the authenticated source by whole-tree digest. Re-reading every file
      // here is redundant and dominates the Windows install.
      if (!promotedAuthority) compare(absolute, acquired.directory, record.name === 'gentle-pi');
    }
    if (fs.existsSync(path.join(absolute, 'node_modules'))) packages(path.join(absolute, 'node_modules'));
  }
}
console.error('Windows install phase: provision-compare');
packages(modules);
const gentle = path.join(modules, 'gentle-pi');
const piMetadata = JSON.parse(read(path.join(modules, '@earendil-works/pi-coding-agent/package.json')));
const gentleMetadata = JSON.parse(read(path.join(gentle, 'package.json')));
if (piMetadata.bin?.pi !== 'dist/bundle/cli.js' || gentleMetadata.bin?.['gentle-shell'] !== 'bin/gentle-shell.mjs') reject('stock role bin metadata');
read(path.join(modules, '@earendil-works/pi-coding-agent/dist/bundle/cli.js'));
read(path.join(gentle, 'bin/gentle-shell.mjs'));
// Stock supplier pins authorize installation; Main source is bound by the frozen ZIP.
if (action === 'install' || !mainOverlay) reviewedSupplierSources(gentle, resolved['gentle-pi'], reviewedSuppliers());
if (action === 'install') {
  const { installGentleAi } = await import(pathToFileURL(path.join(gentle, 'scripts/gentle-ai-installer.mjs')).href);
  console.error('Windows install phase: provision-native'); // Explicit, authenticated stock API, no postinstall/fullscreen hook.
  await installGentleAi({ packageRoot: gentle });
}
const nativeBundle = stockNativeDirectory(path.join(gentle, '.gentle-ai'));
const nativeVersion = nativeBundle.version;
const native = nativeBundle.directory;
stockNativeIntegrity(JSON.parse(read(path.join(native, 'integrity.json'))), native, nativeVersion);
const configPath = path.join(root, 'config/gentle-shell.json');
const homeKey = path.join(finalRoot, 'agent');
if (action === 'install') exclusive(configPath, `${JSON.stringify({ home: 'isolated', provisioned: { [homeKey]: { gentleAi: nativeVersion, gentlePi: gentleMetadata.version, at: new Date().toISOString() } } })}\n`);
const config = JSON.parse(read(configPath));
const provisioned = config?.provisioned, entry = provisioned?.[homeKey];
if (config?.home !== 'isolated' || Object.keys(config).sort().join(',') !== 'home,provisioned' ||
    provisioned === null || typeof provisioned !== 'object' || Array.isArray(provisioned) || Object.keys(provisioned).length !== 1 ||
    entry === null || typeof entry !== 'object' || Array.isArray(entry) || Object.keys(entry).sort().join(',') !== 'at,gentleAi,gentlePi' ||
    entry.gentleAi !== nativeVersion || entry.gentlePi !== gentleMetadata.version || typeof entry.at !== 'string' ||
    !Number.isFinite(Date.parse(entry.at))) reject('owned Shell provisioning binding changed');
const settingsPath = path.join(agent, 'settings.json');
const settings = { packages: [path.join(finalRoot, 'prefix/node_modules/gentle-pi')], npmCommand: [path.join(finalRoot, 'runtime/node/node.exe'), path.join(finalRoot, 'runtime/node/node_modules/npm/bin/npm-cli.js'), '--prefix', path.join(finalRoot, 'prefix')] };
if (action === 'install') exclusive(settingsPath, `${JSON.stringify(settings, null, 2)}\n`);
const observedSettings = JSON.parse(read(settingsPath));
// The stable Pi SettingsManager writes these typed scalars from changelog, model,
// thinking-level and theme selection. A theme is a name lookup, never a path.
// Packages, commands, resources, trust, redirection and telemetry stay refused.
const uiScalar = value => typeof value === 'string' && value.length > 0 && value.length <= 256 && !/[\u0000-\u001f\u007f]/.test(value);
const mutableSettings = {
  lastChangelogVersion: value => typeof value === 'string',
  defaultProvider: uiScalar,
  defaultModel: uiScalar,
  defaultThinkingLevel: value => ['off', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max'].includes(value),
  theme: value => uiScalar(value) && /^[A-Za-z0-9][A-Za-z0-9 ._-]*$/.test(value) && !value.includes('..'),
};
// The authenticated launcher excludes exactly this builtin; no resource paths.
const settingsKeys = ['packages', 'npmCommand', 'extensions', ...Object.keys(mutableSettings)];
if (observedSettings === null || typeof observedSettings !== 'object' || Array.isArray(observedSettings) ||
    Object.keys(observedSettings).some(key => !settingsKeys.includes(key)) ||
    Object.entries(mutableSettings).some(([key, valid]) => Object.hasOwn(observedSettings, key) && !valid(observedSettings[key])) ||
    (Object.hasOwn(observedSettings, 'extensions') && JSON.stringify(observedSettings.extensions) !== '["-builtin:codemode"]') ||
    JSON.stringify({ packages: observedSettings.packages, npmCommand: observedSettings.npmCommand }) !== JSON.stringify(settings)) {
  reject('owned package/settings bindings changed');
}
if (!read(lockPath).equals(lockBytes)) reject('source lock changed');
console.log(`Windows stock composition verified; packages=${count}; registration and full Ready remain unqualified`);
